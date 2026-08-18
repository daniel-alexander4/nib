package server

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

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
	s.adoptVault(v)
}

// adoptVault installs a freshly opened vault and mints the CSRF token for the session.
//
// **One place, because something now happens ON unlock.** These four lines were written
// twice — here and in handleUnlock — identically, which was harmless while they only
// assigned two fields. P07.S02 hangs the pending-open drain off this moment, and a
// second copy would mean a hand-off queued against a locked instance opens when the user
// unlocks through one route and silently never opens when they unlock through the other.
// That is the duplicate-derivation defect this repo has paid for repeatedly — most
// recently `closeDocument` carrying its own copy of `tearDownView`.
//
// The nil check is the guard that makes it idempotent: a second unlock must not mint a
// new CSRF token under a client already holding the first.
func (s *Server) adoptVault(v *vault.Vault) {
	s.mu.Lock()
	fresh := s.vault == nil
	if fresh {
		s.vault = v
		s.csrf = newToken()
	}
	s.mu.Unlock()
	if fresh {
		s.drainPendingOpens()
	}
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

// normalizeKeyPath turns a user-supplied key location into one that will still
// mean the same thing on the next launch: it expands a leading "~" (reusing the
// same expander the folder browser uses, which accepts "~\" as well as "~/") and
// then requires the result be absolute.
//
// The absolute requirement is the point, not a formality. A relative path is
// resolved against the process's working directory, which is a property of how
// the app was started rather than of the machine — on Windows an Explorer
// double-click, an "Open with", and a shortcut each give a different one. A key
// written relative to one of those is not found by the next launch, and the vault
// it unlocks is then unopenable. Observed before this guard existed: enrolling
// with "~/.ssh/id_ed25519" created a directory literally named "~" beside the
// working directory, and the vault recorded that as the key's home.
func normalizeKeyPath(p string) (string, error) {
	p = expandHome(strings.TrimSpace(p))
	if p == "" {
		return "", errors.New("key path required")
	}
	if !filepath.IsAbs(p) {
		return "", fmt.Errorf("key path must be absolute, not %q — a relative path is "+
			"resolved against wherever Nib happened to be started, so the key would not "+
			"be found next time", p)
	}
	return filepath.Clean(p), nil
}

// resolveKey prepares the public-key line and key path for enrollment.
func resolveKey(mode, keyPath string) (pubLine, path string, err error) {
	switch mode {
	case "create":
		if strings.TrimSpace(keyPath) == "" {
			keyPath = sshkey.DefaultNewKeyPath()
		}
		target, err := normalizeKeyPath(keyPath)
		if err != nil {
			return "", "", err
		}
		pub, err := sshkey.Generate(target)
		return pub, target, err
	case "use":
		target, err := normalizeKeyPath(keyPath)
		if err != nil {
			return "", "", err
		}
		pub, err := sshkey.PublicKeyLine(target)
		return pub, target, err
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
	s.adoptVault(v)
	writeJSON(w, s.currentStatus())
}

// handleRepoint is the way out of `key-missing` when the key still exists but not where
// the vault recorded it — the case that bites on Windows, where the CWD differs between an
// Explorer double-click, an "Open with", and a shortcut, so a relative path recorded at
// enrollment resolves differently every launch.
//
// Public (pre-unlock) and loopback-guarded, exactly like unlock and enroll: there is no
// CSRF token before the vault opens, so a loopback Origin is the only write guard
// available. It grants nothing a local caller does not already have — unlocking still
// requires possessing a private key that unwraps a slot.
func (s *Server) handleRepoint(w http.ResponseWriter, r *http.Request) {
	var req struct {
		KeyPath    string `json:"keyPath"`
		Passphrase string `json:"passphrase"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	// Normalized through the SAME function enrollment uses, which is the point: a
	// recovery that accepted a relative or `~`-prefixed path would write back exactly the
	// kind of path that caused this state.
	keyPath, err := normalizeKeyPath(req.KeyPath)
	if err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
		return
	}
	var pass []byte
	if req.Passphrase != "" {
		pass = []byte(req.Passphrase)
	}
	v, err := vault.OpenSSHAt(s.configDir, keyPath, pass)
	if err != nil {
		switch {
		case errors.Is(err, vault.ErrWrongPassphrase):
			httpError(w, http.StatusUnauthorized, "wrong passphrase for that key")
		case errors.Is(err, vault.ErrKeyLocked):
			httpError(w, http.StatusUnauthorized, "that key is passphrase-protected — enter its passphrase")
		case errors.Is(err, vault.ErrKeyMissing):
			// Deliberately specific: "no such file" and "that key does not open this
			// vault" are different problems with different next steps, and one message
			// covering both sends the user to the wrong one.
			if _, sterr := os.Stat(keyPath); sterr != nil {
				httpError(w, http.StatusBadRequest, "no key file at "+keyPath)
				return
			}
			httpError(w, http.StatusUnauthorized, "that key does not open this vault — it is sealed to a different key")
		default:
			httpError(w, http.StatusBadRequest, "could not unlock with that key")
		}
		return
	}
	s.adoptVault(v)
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
// unlock.
//
// The write below is destructive and has no undo — it overwrites the live vault in
// place — so vault.Validate is not a formality here but the only thing standing
// between a mis-picked file and the permanent loss of the signing identity. It
// proves the backup is openable ON THIS MACHINE before the overwrite, and its
// message is passed through rather than flattened: "sealed to a key this machine
// does not have" and "predates SSH-key sealing" send the user somewhere different,
// and the generic refusal this used to return sent them nowhere.
func (s *Server) handleVaultImport(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 16<<20)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		httpError(w, http.StatusBadRequest, "could not read upload")
		return
	}
	if err := vault.Validate(raw); err != nil {
		httpError(w, http.StatusBadRequest, err.Error())
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
