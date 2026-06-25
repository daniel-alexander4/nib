package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"

	"nib/internal/pdfops"
	"nib/internal/sshkey"
	"nib/internal/vault"
)

// Nib unlocks its vault from the user's SSH key at startup — there is no
// password and no login session. The server holds the unlocked vault for the
// process lifetime plus a per-process CSRF token; writes require that token and
// a loopback Origin (the loopback Host guard already blocks remote callers).

func newToken() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// unlockedVault returns the unlocked vault, or nil when locked. It answers "is
// the vault open right now?" for lifecycle and public callers; protected
// handlers use vaultFrom instead, which is pinned to the request.
func (s *Server) unlockedVault() *vault.Vault {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.vault
}

// vaultCtxKey carries the request-scoped vault snapshot taken by requireUnlocked.
type vaultCtxKey struct{}

// vaultFrom returns the vault a protected request was authorized against. It's
// the snapshot requireUnlocked took (and already nil-checked), so it stays
// non-nil even if a concurrent vault import nils s.vault mid-request. Only valid
// inside a requireUnlocked-wrapped handler.
func vaultFrom(r *http.Request) *vault.Vault {
	v, _ := r.Context().Value(vaultCtxKey{}).(*vault.Vault)
	return v
}

// ensureUnlocked tries to unlock the vault from the enrolled SSH key if it isn't
// already open. Safe to call repeatedly.
func (s *Server) ensureUnlocked() {
	if s.unlockedVault() != nil {
		return
	}
	// Serialize setup+open so concurrent callers (e.g. two tabs hitting /api/status
	// at first run) can't both run AutoSetup. setupMu is separate from s.mu so the
	// request lock is never held across file I/O.
	s.setupMu.Lock()
	defer s.setupMu.Unlock()
	if s.unlockedVault() != nil { // another caller unlocked while we waited
		return
	}
	// On a trusted machine (one holding a builtin key's private half), create the
	// vault without the first-run wizard. No-op when a vault exists or no builtin
	// key is local — the wizard then handles setup.
	if !vault.Exists(s.configDir) {
		_, _ = vault.AutoSetup(s.configDir)
	}
	v, err := vault.OpenSSH(s.configDir)
	if err != nil {
		return
	}
	s.mu.Lock()
	if s.vault == nil {
		s.vault = v
		s.csrf = newToken()
	}
	s.mu.Unlock()
}

// requireUnlocked guards protected routes: the vault must be open, and writes
// must carry the CSRF token and a loopback Origin.
func (s *Server) requireUnlocked(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s.mu.Lock()
		v, csrf := s.vault, s.csrf
		s.mu.Unlock()
		if v == nil {
			httpError(w, http.StatusUnauthorized, "locked")
			return
		}
		if r.Method != http.MethodGet {
			if subtle.ConstantTimeCompare([]byte(r.Header.Get("X-CSRF-Token")), []byte(csrf)) != 1 {
				httpError(w, http.StatusForbidden, "bad csrf token")
				return
			}
			if !originIsLoopback(r) {
				httpError(w, http.StatusForbidden, "bad origin")
				return
			}
		}
		next(w, r.WithContext(context.WithValue(r.Context(), vaultCtxKey{}, v)))
	}
}

// originIsLoopback allows requests with no Origin (same-origin) and rejects any
// cross-site Origin.
func originIsLoopback(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		return true
	}
	u, err := url.Parse(origin)
	if err != nil {
		return false
	}
	switch u.Hostname() {
	case "127.0.0.1", "localhost", "::1":
		return true
	}
	return false
}

// requirePublicLoopback guards a public (pre-unlock) mutating route. No CSRF
// token exists before the vault unlocks, so a loopback Origin is the only write
// guard these routes can apply.
func requirePublicLoopback(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !originIsLoopback(r) {
			httpError(w, http.StatusForbidden, "bad origin")
			return
		}
		next(w, r)
	}
}

// --- status ------------------------------------------------------------------

type statusResponse struct {
	State                 string   `json:"state"` // ready | setup | migrate | key-missing
	CSRF                  string   `json:"csrf,omitempty"`
	Candidates            []string `json:"candidates,omitempty"`            // detected ~/.ssh keys
	DefaultKeyPath        string   `json:"defaultKeyPath,omitempty"`        // where a new key would be created
	KeyPath               string   `json:"keyPath,omitempty"`               // enrolled key path (key-missing)
	AutoUpdate            bool     `json:"autoUpdate"`                      // run the startup update check (effective: env AND user preference)
	UpdateCheckLocked     bool     `json:"updateCheckLocked"`               // NIB_NO_UPDATE_CHECK forces the check off; the UI toggle can't override it
	ToolbarStyle          string   `json:"toolbarStyle,omitempty"`          // menus | toolbar | both (saved layout preference)
	Appearance            string   `json:"appearance,omitempty"`            // dark | light (saved theme preference)
	RecentHighlightColors []string `json:"recentHighlightColors,omitempty"` // last-used highlight colors, newest first
	Version               string   `json:"version"`                         // running build, shown in the About dialog
	Ghostscript           bool     `json:"ghostscript"`                     // gs installed → offer the general (vector-preserving) PDF/A converter
	LibreOffice           bool     `json:"libreoffice"`                     // LibreOffice installed → offer office-document → PDF conversion
}

// currentStatus describes how (and whether) the vault can be unlocked, stamped
// with the UI preferences (update check, toolbar layout). NIB_NO_UPDATE_CHECK is
// a hard override that wins over the saved auto-update preference.
func (s *Server) currentStatus() statusResponse {
	st := s.vaultStatus()
	st.Version = s.version
	st.Ghostscript = pdfops.GhostscriptAvailable()
	st.LibreOffice = pdfops.LibreOfficeAvailable()
	envAllows := os.Getenv("NIB_NO_UPDATE_CHECK") == ""
	st.UpdateCheckLocked = !envAllows
	st.AutoUpdate = envAllows
	if v := s.unlockedVault(); v != nil {
		set := v.Settings()
		st.ToolbarStyle = set.ToolbarStyle
		st.Appearance = set.Appearance
		st.RecentHighlightColors = set.RecentHighlightColors
		if set.DisableAutoUpdate {
			st.AutoUpdate = false
		}
	}
	return st
}

func (s *Server) vaultStatus() statusResponse {
	if s.unlockedVault() != nil {
		s.mu.Lock()
		csrf := s.csrf
		s.mu.Unlock()
		return statusResponse{State: "ready", CSRF: csrf}
	}
	if !vault.Exists(s.configDir) {
		return statusResponse{State: "setup", Candidates: sshkey.Candidates(), DefaultKeyPath: sshkey.DefaultNewKeyPath()}
	}
	if vault.NeedsMigration(s.configDir) {
		return statusResponse{State: "migrate", Candidates: sshkey.Candidates(), DefaultKeyPath: sshkey.DefaultNewKeyPath()}
	}
	keyPath := ""
	if slots, err := vault.Slots(s.configDir); err == nil && len(slots) > 0 {
		keyPath = slots[0].KeyPath
	}
	// An enrolled key that's present but passphrase-protected is "key-locked" (the
	// UI offers a passphrase prompt), distinct from a genuinely missing key. The
	// re-attempt is cheap: parsing an encrypted key fails fast, before any decrypt.
	state := "key-missing"
	if _, err := vault.OpenSSH(s.configDir); errors.Is(err, vault.ErrKeyLocked) {
		state = "key-locked"
	}
	return statusResponse{State: state, KeyPath: keyPath}
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	s.ensureUnlocked()
	writeJSON(w, s.currentStatus())
}

// --- enroll / migrate --------------------------------------------------------

type enrollRequest struct {
	Mode     string `json:"mode"`    // "use" | "create"
	KeyPath  string `json:"keyPath"` // existing key to use, or where to create one
	Password string `json:"password"`
}

// writeKeyPrepError reports a failure to prepare the enrollment key. A path that
// already holds a key gets a clear "choose another" message rather than a raw
// os error, since the fix is for the user to pick a different name or location.
func writeKeyPrepError(w http.ResponseWriter, keyPath string, err error) {
	if errors.Is(err, os.ErrExist) {
		httpError(w, http.StatusConflict, "a key already exists at "+keyPath+" — pick a different location or name")
		return
	}
	httpError(w, http.StatusBadRequest, "could not prepare key: "+err.Error())
}

// resolveKey prepares the public-key line and key path for enrollment.
func resolveKey(mode, keyPath string) (pubLine, path string, err error) {
	switch mode {
	case "create":
		if keyPath == "" {
			keyPath = sshkey.DefaultNewKeyPath()
		}
		pub, err := sshkey.Generate(keyPath)
		return pub, keyPath, err
	case "use":
		if keyPath == "" {
			return "", "", errors.New("key path required")
		}
		pub, err := sshkey.PublicKeyLine(keyPath)
		return pub, keyPath, err
	default:
		return "", "", errors.New("unknown mode")
	}
}

func (s *Server) handleEnroll(w http.ResponseWriter, r *http.Request) {
	if vault.Exists(s.configDir) {
		httpError(w, http.StatusConflict, "already set up")
		return
	}
	req, ok := decodeEnroll(w, r)
	if !ok {
		return
	}
	pubLine, keyPath, err := resolveKey(req.Mode, req.KeyPath)
	if err != nil {
		writeKeyPrepError(w, keyPath, err)
		return
	}
	if _, err := vault.Create(s.configDir, pubLine, keyPath); err != nil {
		httpError(w, http.StatusInternalServerError, "could not create vault")
		return
	}
	s.ensureUnlocked()
	writeJSON(w, s.currentStatus())
}

func (s *Server) handleMigrate(w http.ResponseWriter, r *http.Request) {
	if !vault.NeedsMigration(s.configDir) {
		httpError(w, http.StatusConflict, "nothing to migrate")
		return
	}
	req, ok := decodeEnroll(w, r)
	if !ok {
		return
	}
	pubLine, keyPath, err := resolveKey(req.Mode, req.KeyPath)
	if err != nil {
		writeKeyPrepError(w, keyPath, err)
		return
	}
	if _, err := vault.Migrate(s.configDir, req.Password, pubLine, keyPath); err != nil {
		if errors.Is(err, vault.ErrWrongPassword) {
			httpError(w, http.StatusUnauthorized, "wrong password")
			return
		}
		httpError(w, http.StatusInternalServerError, "migration failed")
		return
	}
	s.ensureUnlocked()
	writeJSON(w, s.currentStatus())
}

// handleUnlock unlocks a vault whose enrolled SSH key is passphrase-protected,
// using the passphrase the user supplied. Like enroll/migrate it runs pre-unlock
// (no CSRF token exists yet), guarded by a loopback Origin. The passphrase is
// used only to decrypt the key in memory for this unlock and is never persisted.
func (s *Server) handleUnlock(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Passphrase string `json:"passphrase"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	v, err := vault.OpenSSHWithPassphrase(s.configDir, []byte(req.Passphrase))
	if err != nil {
		if errors.Is(err, vault.ErrWrongPassphrase) {
			httpError(w, http.StatusUnauthorized, "wrong passphrase")
			return
		}
		httpError(w, http.StatusBadRequest, "could not unlock")
		return
	}
	s.mu.Lock()
	if s.vault == nil {
		s.vault = v
		s.csrf = newToken()
	}
	s.mu.Unlock()
	writeJSON(w, s.currentStatus())
}

func decodeEnroll(w http.ResponseWriter, r *http.Request) (enrollRequest, bool) {
	var req enrollRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request body")
		return req, false
	}
	return req, true
}

// --- vault backup / restore --------------------------------------------------

// handleVaultExport streams the (encrypted) vault file for backup.
func (s *Server) handleVaultExport(w http.ResponseWriter, r *http.Request) {
	raw, err := os.ReadFile(vault.Path(s.configDir))
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not read vault")
		return
	}
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", `attachment; filename="vault.nib"`)
	_, _ = w.Write(raw)
}

// handleVaultImport replaces the vault with an uploaded backup, then re-attempts
// unlock (it only opens if this machine's SSH key matches a slot in the backup).
func (s *Server) handleVaultImport(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		httpError(w, http.StatusBadRequest, "could not read upload")
		return
	}
	if err := vault.Validate(raw); err != nil {
		httpError(w, http.StatusBadRequest, "not a valid vault backup")
		return
	}
	if err := writeFileAtomic(vault.Path(s.configDir), raw); err != nil {
		httpError(w, http.StatusInternalServerError, "could not write vault")
		return
	}
	s.mu.Lock()
	s.vault, s.csrf = nil, ""
	s.mu.Unlock()
	s.ensureUnlocked()
	writeJSON(w, s.currentStatus())
}
