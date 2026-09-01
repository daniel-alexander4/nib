package server

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"

	"nib/internal/p2p"
	"nib/internal/pairing"
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
	Name        string `json:"name,omitempty"`
	Label       string `json:"label,omitempty"`
}

// peersResponse is the management view: the user's own fingerprint (to share
// out-of-band) plus the pinned peers.
//
// Name is the six-word pairing name (D3): an encoding of the leading bits of the
// fingerprint, so it is a *display* identity rather than a second identifier. It is
// derived on every read and stored nowhere — the vault holds fingerprints and labels
// only, and `TestPeerNameIsNeverStored` reads the vault file to prove it.
//
// **Nothing addresses a peer by name.** Pinning, co-signing and the serve routes all
// take the hex fingerprint, and L1 forbids a name resolving a pin; a name that reached
// those paths would be the design D31 removed.
type peersResponse struct {
	Fingerprint string `json:"fingerprint"` // the user's own hex SPKI fingerprint
	Name        string `json:"name,omitempty"`
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
		peers = append(peers, peer{
			Fingerprint: hex.EncodeToString(p.Fingerprint),
			Name:        nameOrEmpty(p.Fingerprint),
			Label:       p.Label,
		})
	}
	return peersResponse{
		Fingerprint: hex.EncodeToString(fp),
		Name:        nameOrEmpty(fp),
		Peers:       peers,
	}, nil
}

// nameOrEmpty derives the six-word pairing name, or "" if it cannot.
//
// A name that will not derive must not fail the whole peers view: the fingerprint is
// the load-bearing value on every one of these paths and the name is a display
// convenience beside it. The only way it errors is a fingerprint that is not 32 bytes,
// which the vault cannot hold — so this is a floor under a case that should not happen,
// not a tolerated one. The client renders hex when the name is absent.
func nameOrEmpty(fp []byte) string {
	n, err := pairing.Name(fp)
	if err != nil {
		return ""
	}
	return n
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
	label := strings.TrimSpace(req.Label)
	// **The rule's fourth site** (/pending 286). Intent, Capacity and Signer are all
	// bounded by p2p's one block-width door; this label was not, so the only limit on it
	// was the 64 KiB body read above — and the signature block then clipped it silently,
	// because the canvas draws each line with no maxWidth and nothing wraps. Refused here,
	// at the door where the user typed it, rather than discovered at signing time on a
	// block that is already in the document.
	//
	// The fingerprint is part of the measurement: this line renders as
	// `Accepts: <label>  [<short fingerprint>]`, so a label bounded on its own would pass
	// one that pushes the fingerprint off the block.
	if !p2p.AcceptsFitsBlock(label, hex.EncodeToString(fp)) {
		httpError(w, http.StatusBadRequest, fmt.Sprintf(
			"that label is too long for the signature block: %d characters, and it renders alongside the fingerprint, so at most %d fit",
			len([]rune(label)), p2p.MaxAcceptsRunes(label, hex.EncodeToString(fp))))
		return
	}
	if err := v.AddPinnedPeer(fp, label); err != nil {
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
