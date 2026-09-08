package server

import (
	"encoding/json"
	"io"
	"net/http"

	"nib/internal/vault"
)

// The convener's setup draft (P03.S02, D4).
//
// # Why it exists
//
// Today the first durable state in a ceremony is the SIGNED RECORD, so a convener who closes Nib
// halfway through setup retypes the roster and the recital. D4's words: *"without it the sheet is a
// form that cannot be left, which is the abandonment case every long form has."*
//
// # Why the vault, and not `localStorage`
//
// `localStorage` is keyed by ORIGIN and `cmd/nib` binds `127.0.0.1:0` — a random port by design, so
// the app is never network-exposed. A new port is a new origin is an empty store: a draft kept there
// is gone on exactly the restart the exit criterion is about, and every test that did not restart
// the process would have passed. It is also a mechanism this app uses nowhere (`grep -c localStorage
// web/app.js` → 0).
//
// The vault is where per-machine state already lives — `handleSettings` puts appearance, the card
// hue and the recent highlight colours there — and it is encrypted at rest, which matters because an
// abandoned draft records who the user was about to transact with and what they were about to agree.
//
// # Why its own route rather than a field on /api/status
//
// `/api/status` is polled. A form's contents on that payload would ship the roster and the recital
// on every poll, and mix a transient draft into durable preferences.

// draftRequest carries the whole draft as an opaque blob.
//
// **Opaque on purpose.** The server neither parses nor validates it: what is in a half-finished
// form is the client's business, and `convene` is the door that decides whether the finished thing
// is a ceremony. A server-side schema here would be a second, drifting definition of a form — and
// the one that matters already exists at `handleCeremonyConvene`, which refuses everything this
// could have checked and does it on the values actually submitted.
type draftRequest struct {
	Draft string `json:"draft"`
}

type draftResponse struct {
	// Draft is the stored draft, or empty when there is none. The client branches on emptiness,
	// which is the same answer the vault gives — see `Vault.SetCeremonyDraft` for why an empty
	// draft and an absent one are deliberately one state.
	Draft string `json:"draft"`
}

// handleCeremonyDraft reads the stored draft (GET) or replaces it (POST).
func (s *Server) handleCeremonyDraft(w http.ResponseWriter, r *http.Request) {
	v := vaultFrom(r)
	if r.Method == http.MethodGet {
		draft, _ := v.CeremonyDraft()
		writeJSON(w, draftResponse{Draft: draft})
		return
	}
	var req draftRequest
	// **A cap, because this is a blob the server does not read.** A form's contents are small; the
	// roster is a list of fingerprints the vault already holds and the recital is capped at 200
	// characters by the input itself. 64 KiB is far past any honest draft and far short of a way to
	// grow the vault without bound.
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if err := v.SetCeremonyDraft(req.Draft); err != nil {
		httpError(w, http.StatusInternalServerError,
			"your setup could not be saved, so closing Nib would lose it: "+err.Error())
		return
	}
	writeJSON(w, draftResponse{Draft: req.Draft})
}

// clearCeremonyDraft is the consume door (P03.S03 will call it at convene).
//
// Best-effort and logged by its caller: a ceremony that convened is convened whether or not the
// draft that led to it was tidied away, and failing a convene over a bookkeeping row would trade the
// proceeding for the record of it.
func clearCeremonyDraft(v *vault.Vault) error { return v.ClearCeremonyDraft() }
