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

// keysPayload builds the management view for the current vault.
func (s *Server) keysPayload() keysResponse {
	keys := s.unlockedVault().Keys()
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
	writeJSON(w, s.keysPayload())
}

type addKeyRequest struct {
	Mode    string `json:"mode"`    // "use" (read a local key path) | "paste" (an authorized_keys line) | "create" (generate a new key)
	KeyPath string `json:"keyPath"` // "use": key to read; "paste": where the private key lives (optional); "create": where to write it (optional)
	PubKey  string `json:"pubKey"`  // "paste": the authorized_keys line
}

func (s *Server) handleKeysAdd(w http.ResponseWriter, r *http.Request) {
	v := s.unlockedVault()
	var req addKeyRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var pubLine, keyPath string
	switch req.Mode {
	case "use":
		if req.KeyPath == "" {
			httpError(w, http.StatusBadRequest, "key path required")
			return
		}
		pl, err := sshkey.PublicKeyLine(req.KeyPath)
		if err != nil {
			httpError(w, http.StatusBadRequest, "could not read key: "+err.Error())
			return
		}
		pubLine, keyPath = pl, req.KeyPath
	case "paste":
		pubLine = strings.TrimSpace(req.PubKey)
		if pubLine == "" {
			httpError(w, http.StatusBadRequest, "public key required")
			return
		}
		keyPath = strings.TrimSpace(req.KeyPath)
		if keyPath == "" {
			keyPath = sshkey.DefaultNewKeyPath()
		}
	case "create":
		keyPath = strings.TrimSpace(req.KeyPath)
		if keyPath == "" {
			keyPath = sshkey.DefaultNewKeyPath()
		}
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
	writeJSON(w, s.keysPayload())
}

type removeKeyRequest struct {
	PubKey string `json:"pubKey"`
}

func (s *Server) handleKeysRemove(w http.ResponseWriter, r *http.Request) {
	v := s.unlockedVault()
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
	writeJSON(w, s.keysPayload())
}
