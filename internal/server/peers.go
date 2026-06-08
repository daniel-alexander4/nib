package server

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"nib/internal/sign"
	"nib/internal/vault"
)

// Pinned-peer management. Co-signing binds each signature to the SHA-256 SPKI
// fingerprint of the counterparty the signer accepts (see internal/p2p). Peers
// are pinned out-of-band: the user compares the other party's fingerprint over a
// channel they trust, then stores it here. These handlers expose the user's own
// fingerprint (to share) and let them pin / unpin peers while the vault is
// unlocked. Nothing here trusts the network — the fingerprint is public by
// design, and pinning is a deliberate, user-driven act.

// peer is one pinned peer in the management view.
type peer struct {
	Fingerprint string `json:"fingerprint"` // hex SHA-256 of the peer's SPKI
	Label       string `json:"label,omitempty"`
}

// peersResponse is the management view: the user's own fingerprint (to share
// out-of-band) plus the pinned peers.
type peersResponse struct {
	Fingerprint string `json:"fingerprint"` // the user's own hex SPKI fingerprint
	Peers       []peer `json:"peers"`
}

// peersPayload builds the management view, creating the signing identity on
// first use (as Finalize does) so the user always has a fingerprint to share.
func (s *Server) peersPayload(v *vault.Vault) (peersResponse, error) {
	cert, _, err := identity(v)
	if err != nil {
		return peersResponse{}, err
	}
	fp, err := sign.Fingerprint(cert)
	if err != nil {
		return peersResponse{}, err
	}
	pinned := v.PinnedPeers()
	peers := make([]peer, 0, len(pinned))
	for _, p := range pinned {
		peers = append(peers, peer{Fingerprint: hex.EncodeToString(p.Fingerprint), Label: p.Label})
	}
	return peersResponse{Fingerprint: hex.EncodeToString(fp), Peers: peers}, nil
}

func (s *Server) handlePeersList(w http.ResponseWriter, r *http.Request) {
	s.respondPeers(w, vaultFrom(r))
}

type pinPeerRequest struct {
	Fingerprint string `json:"fingerprint"` // hex SHA-256 SPKI (64 hex chars; spaces tolerated)
	Label       string `json:"label"`
}

func (s *Server) handlePeersPin(w http.ResponseWriter, r *http.Request) {
	v := vaultFrom(r)
	var req pinPeerRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	fp, err := parseFingerprint(req.Fingerprint)
	if err != nil {
		httpError(w, http.StatusBadRequest, "not a valid fingerprint (expected 64 hex characters)")
		return
	}
	if err := v.AddPinnedPeer(fp, strings.TrimSpace(req.Label)); err != nil {
		httpError(w, http.StatusInternalServerError, "could not pin peer")
		return
	}
	s.respondPeers(w, v)
}

type removePeerRequest struct {
	Fingerprint string `json:"fingerprint"`
}

func (s *Server) handlePeersRemove(w http.ResponseWriter, r *http.Request) {
	v := vaultFrom(r)
	var req removePeerRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	fp, err := parseFingerprint(req.Fingerprint)
	if err != nil {
		httpError(w, http.StatusBadRequest, "not a valid fingerprint")
		return
	}
	if err := v.RemovePinnedPeer(fp); err != nil {
		httpError(w, http.StatusInternalServerError, "could not unpin peer")
		return
	}
	s.respondPeers(w, v)
}

func (s *Server) respondPeers(w http.ResponseWriter, v *vault.Vault) {
	payload, err := s.peersPayload(v)
	if err != nil {
		httpError(w, http.StatusInternalServerError, "could not load identity")
		return
	}
	writeJSON(w, payload)
}

// parseFingerprint accepts the hex fingerprint the user pastes — tolerating the
// spaced/grouped form the UI displays and any case — and returns the 32 raw
// bytes, or an error if it isn't a SHA-256-length hex value.
func parseFingerprint(s string) ([]byte, error) {
	s = strings.ToLower(strings.Join(strings.Fields(s), ""))
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 32 {
		return nil, errors.New("not a valid fingerprint")
	}
	return b, nil
}
