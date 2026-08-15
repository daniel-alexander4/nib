package server

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"nib/internal/sshkey"
	"nib/internal/vault"
)

// Authorized-key management. The vault's content key is sealed to one or more
// SSH public keys (one slot each); these handlers let the user authorize an
// additional key — to unlock Nib from a second machine — or remove an old one,
// while the vault is unlocked.

// keysResponse is the management view: the enrolled keys plus any local ~/.ssh
// keys not yet enrolled, offered as quick-add candidates.
type keysResponse struct {
	Keys       []vault.KeyInfo `json:"keys"`
	Candidates []string        `json:"candidates"`
}

// keysPayload builds the management view for the given vault.
func (s *Server) keysPayload(v *vault.Vault) keysResponse {
	keys := v.Keys()
	enrolled := make(map[string]bool, len(keys))
	for _, k := range keys {
		enrolled[k.KeyPath] = true
	}
	var cands []string
	for _, p := range sshkey.Candidates() {
		if !enrolled[p] {
			cands = append(cands, p)
		}
	}
	return keysResponse{Keys: keys, Candidates: cands}
}

func (s *Server) handleKeysList(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, s.keysPayload(vaultFrom(r)))
}

type addKeyRequest struct {
	Mode    string `json:"mode"`    // "use" (read a local key path) | "paste" (an authorized_keys line) | "create" (generate a new key)
	KeyPath string `json:"keyPath"` // "use": key to read; "paste": where the private key lives (optional); "create": where to write it (optional)
	PubKey  string `json:"pubKey"`  // "paste": the authorized_keys line
}

func (s *Server) handleKeysAdd(w http.ResponseWriter, r *http.Request) {
	v := vaultFrom(r)
	var req addKeyRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var pubLine, keyPath string
	switch req.Mode {
	// Every mode below stores a key path in the vault, and that path is re-read at
	// each unlock — so it goes through normalizeKeyPath for the same reason
	// resolveKey does: a relative one is anchored to whatever directory Nib
	// happened to be started in and will not be found next launch.
	case "use":
		target, err := normalizeKeyPath(req.KeyPath)
		if err != nil {
			httpError(w, http.StatusBadRequest, err.Error())
			return
		}
		pl, err := sshkey.PublicKeyLine(target)
		if err != nil {
			httpError(w, http.StatusBadRequest, "could not read key: "+err.Error())
			return
		}
		pubLine, keyPath = pl, target
	case "paste":
		pubLine = strings.TrimSpace(req.PubKey)
		if pubLine == "" {
			httpError(w, http.StatusBadRequest, "public key required")
			return
		}
		// A pasted key's private half usually lives on ANOTHER machine — that is
		// the point of pasting one. So the path stays optional and an empty one is
		// recorded as empty, which reads as "we cannot unlock with this locally".
		// (It used to default to this machine's own key path, which claimed the
		// pasted key lived somewhere it does not.) A path that IS given still has
		// to be one we could find again.
		if strings.TrimSpace(req.KeyPath) != "" {
			target, err := normalizeKeyPath(req.KeyPath)
			if err != nil {
				httpError(w, http.StatusBadRequest, err.Error())
				return
			}
			keyPath = target
		}
	case "create":
		raw := strings.TrimSpace(req.KeyPath)
		if raw == "" {
			raw = sshkey.DefaultNewKeyPath()
		}
		target, err := normalizeKeyPath(raw)
		if err != nil {
			httpError(w, http.StatusBadRequest, err.Error())
			return
		}
		keyPath = target
		pl, err := sshkey.Generate(keyPath)
		if err != nil {
			writeKeyPrepError(w, keyPath, err)
			return
		}
		pubLine = pl
	default:
		httpError(w, http.StatusBadRequest, "unknown mode")
		return
	}
	switch err := v.AddKey(pubLine, keyPath); {
	case err == nil:
	case errors.Is(err, vault.ErrKeyExists):
		httpError(w, http.StatusConflict, "that key is already authorized")
		return
	case errors.Is(err, vault.ErrBadKey):
		httpError(w, http.StatusBadRequest, "not a valid SSH public key")
		return
	default:
		httpError(w, http.StatusInternalServerError, "could not authorize key")
		return
	}
	writeJSON(w, s.keysPayload(vaultFrom(r)))
}

type removeKeyRequest struct {
	PubKey string `json:"pubKey"`
}

func (s *Server) handleKeysRemove(w http.ResponseWriter, r *http.Request) {
	v := vaultFrom(r)
	var req removeKeyRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	switch err := v.RemoveKey(req.PubKey); {
	case err == nil:
	case errors.Is(err, vault.ErrNoSuchKey):
		httpError(w, http.StatusNotFound, "no such authorized key")
		return
	case errors.Is(err, vault.ErrLastKey):
		httpError(w, http.StatusConflict, "can't remove the only authorized key")
		return
	case errors.Is(err, vault.ErrCurrentKey):
		httpError(w, http.StatusConflict, "can't remove the key you're currently using")
		return
	default:
		httpError(w, http.StatusInternalServerError, "could not remove key")
		return
	}
	writeJSON(w, s.keysPayload(vaultFrom(r)))
}
