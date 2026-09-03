package server

import (
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"strings"
	"time"

	"nib/internal/ceremony"
	"nib/internal/vault"
)

// The convene route (P07.S02a): the first surface in the product that creates a ceremony.
//
// # What it inherits, and why each one is not optional
//
// **`requireUnlocked`** — without it `vaultFrom(r)` is nil and the handler signs against a
// nil vault. **POST, not GET** — `requireUnlocked` applies CSRF and the loopback-origin check
// only when the method is not GET (browsers omit Origin on sub-resource GETs), so a GET
// convene route would be triggerable blind by any page the user has open: secrets minted, the
// vault written, a directory created. **`resolveDoc`** — convene is multi-second (a readme
// render, N page appends, a content digest, a signature, an attachment), and `docFor` falls
// back to the active document when no `X-Nib-Doc` is present, so an unpinned convene would
// commit the record into whichever tab is active when it finishes: ADR-001 arriving through
// the fallback. **`commitMutation`** — it is what applies ADR-008's byte cap and maps a
// refusal to 409, and convene GROWS the document, so a handler assigning doc.data directly
// would be a sixth writer past the 512 MiB ceiling.

type convenePartyRequest struct {
	Fingerprint string `json:"fingerprint"`
	Label       string `json:"label"`
	Capacity    string `json:"capacity"`
	Signs       bool   `json:"signs"`
}

type conveneRequest struct {
	Roster []convenePartyRequest `json:"roster"`
	Intent string                `json:"intent"`
	// Expires is an absolute RFC3339 instant, not a count of days.
	//
	// A real transaction has a date ("completion 30 September"), and a convener asked "how
	// many days?" guesses — wrong by however long it takes to get the invitations out. It is
	// also what makes C20 constructible: the record's Expires is a full time.Time, so a
	// 40-minute deadline is expressible and a day-granular picker would be the only thing
	// making it otherwise.
	Expires       string `json:"expires"`
	ConvenerSigns bool   `json:"convenerSigns"`
}

// conveneInvite is one party's invitation as the API hands it over.
type conveneInvite struct {
	Fingerprint string `json:"fingerprint"`
	Label       string `json:"label"`
	Signs       bool   `json:"signs"`
	// Name is the six-word pairing name, DERIVED from the fingerprint.
	//
	// The identity vocabulary the rest of the product already uses (`nameOrEmpty`), and the
	// only pre-commit check available for a value the convener typed once and can never
	// change: they read it down the phone before anybody signs. Eight characters of hex is
	// neither readable nor speakable, which is why every peer control in the UI is labelled
	// this way already.
	Name string `json:"name"`
	// Invitation is the pasteable text form.
	Invitation string `json:"invitation"`
}

type conveneResponse struct {
	Ceremony string          `json:"ceremony"`
	Intent   string          `json:"intent"`
	Expires  string          `json:"expires"`
	Invites  []conveneInvite `json:"invites"`
	// Warnings are named soft refusals — machine-tagged so a panel can bind one to the
	// control that caused it rather than re-parsing English.
	Warnings []ceremony.Warning `json:"warnings"`
	Doc      docResponse        `json:"doc"`
}

func (s *Server) handleCeremonyConvene(w http.ResponseWriter, r *http.Request) {
	v := vaultFrom(r)
	doc, ok := s.resolveDoc(w, r)
	if !ok {
		return
	}
	var req conveneRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	expires, err := time.Parse(time.RFC3339, req.Expires)
	if err != nil {
		httpError(w, http.StatusBadRequest,
			"that deadline is not a date Nib can read — it needs a full date and time")
		return
	}
	cert, key, err := identity(v)
	if err != nil {
		// Wrapped, not replaced: identity() fails for entropy exhaustion and for every way a
		// vault write can fail (disk full, read-only config dir, an SELinux denial), and one
		// constant sentence for all of them sends the user to the wrong problem.
		httpError(w, http.StatusInternalServerError,
			"could not prepare your signing identity: "+err.Error())
		return
	}

	roster := make([]ceremony.Party, 0, len(req.Roster))
	for _, p := range req.Roster {
		roster = append(roster, ceremony.Party{
			Fingerprint: p.Fingerprint,
			Label:       p.Label,
			Capacity:    p.Capacity,
			Signs:       p.Signs,
		})
	}

	before := s.docBytes(doc)
	out, err := ceremony.Convene(before, ceremony.ConveneRequest{
		Roster:         roster,
		Intent:         req.Intent,
		Expires:        expires,
		HopBudget:      ceremonyHopBudget(),
		DeliveryBudget: ceremonyDeliveryLegBudget(),
		ConvenerSigns:  req.ConvenerSigns,
	}, cert, key, time.Now())
	if err != nil {
		httpError(w, conveneStatus(err), err.Error())
		return
	}

	// **Persisted BEFORE the response returns (C22), and before the commit.**
	//
	// If the commit refuses — ADR-008's byte cap is reachable by convene itself, since it
	// appends a readme, N signature pages and an attachment — the user must not be left with
	// a ceremony whose secrets are on disk and whose document is not. Writing first and
	// pruning on failure keeps those two facts together.
	root := defaultOutputDir()
	// **The rollback is armed BEFORE the first write, not after it.**
	//
	// The first draft armed it after WriteMirror, and the slice's diff review found the gap:
	// WriteMirror creates the directory and writes document.pdf before record.json, so a
	// failure on the second write returned 500 with the rollback never armed — leaving a full
	// copy of the user's document at 0600 under a directory whose id was minted inside
	// Convene and discarded with the error. Nothing could ever name it again, so no prune
	// would ever reach it. `~/nib/ceremonies/` would accumulate documents the user believes
	// were never convened.
	committed := false
	defer func() {
		if !committed {
			s.unconvene(v, root, out.Record.ID)
		}
	}()
	if _, err := ceremony.WriteMirror(root, out.Record, out.Document); err != nil {
		httpError(w, http.StatusInternalServerError,
			"the ceremony could not be saved to disk, so nothing was convened: "+err.Error())
		return
	}
	// **Everything after this line is rolled back unless the commit succeeds.**
	//
	// A deferred rollback rather than one at each failure branch, and it is not merely
	// tidier: it covers the branches nobody thought of — a panic, and any early return added
	// later — where a per-branch call covers exactly the ones written today. The state it
	// prevents is a ceremony whose secrets are in the vault and whose document never
	// committed: the user can neither see it nor cancel it, which is worse than either half
	// alone.
	for _, inv := range out.Invites {
		fp, derr := hex.DecodeString(inv.Party.Fingerprint)
		if derr != nil {
			httpError(w, http.StatusInternalServerError, "could not store an invitation secret")
			return
		}
		if err := v.AddCeremonySecret(out.Record.ID, fp, inv.Invitation.Secret); err != nil {
			httpError(w, http.StatusInternalServerError,
				"the ceremony's invitations could not be saved, so nothing was convened: "+err.Error())
			return
		}
	}
	// **The convener's own pins, through the same door the invitee uses (P07.S02b, ADR-009).**
	//
	// D21 removes the manual pin for a party who was invited; it is the same step for the
	// convener, who otherwise pins N-1 fingerprints by hand before arming against any of them.
	// This route never pinned anything — its only vault write was AddCeremonySecret above —
	// so the rule reached one of its two sites.
	//
	// Inside the rollback window, and `unconvene` prunes them: a convene whose commit fails
	// must not leave the convener's peer list carrying a ceremony that does not exist.
	//
	// The whole roster minus the convener, which is the OPPOSITE of what the invitee passes
	// and is the same fact about D22 read from the hub rather than a spoke — see
	// pinCeremonyRoster.
	//
	// **`selfFP` is read from the record rather than from `cert`**, because the record is what
	// the parties will hold: if those two ever disagreed, deriving from `cert` would skip a
	// party the roster does contain and pin one it does not. `Convener()` looks the convener up
	// IN the roster, so a false here is the door having produced a record that does not name
	// its own convener — an internal inconsistency, not a user error.
	convener, ok := out.Record.Convener()
	if !ok {
		httpError(w, http.StatusInternalServerError,
			"the convened record does not name its own convener, so nothing was convened")
		return
	}
	// The convener's own position, from the value this path already holds (P06.S02). Same
	// best-effort footing as the accept side's, and for the same reason.
	if merr := ceremony.WriteMe(root, out.Record.ID, convener.Fingerprint); merr != nil {
		log.Printf("convened ceremony %s: could not record which party this machine is: %v — the "+
			"ceremony works, but the panel cannot show your position in it", out.Record.ID, merr)
	}
	if _, perr := pinCeremonyRoster(v, out.Record.ID, out.Record.Roster, convener.Fingerprint); perr != nil {
		httpError(w, http.StatusInternalServerError,
			"the ceremony's parties could not be pinned, so nothing was convened: "+perr.Error())
		return
	}
	// **commitBarrier, not commitMutation — undo must not be able to un-convene.**
	//
	// Convene creates state OUTSIDE the document: N-1 secrets in the vault and a directory
	// under ~/nib. `handleUndo` pops the undo stack directly and touches neither, so an undo
	// across a convene would restore the pre-convene bytes and leave both orphaned — a
	// ceremony the user can neither see nor cancel, with parties holding invitations naming a
	// document hash that no longer exists anywhere.
	//
	// A check inside handleUndo would refuse it; a barrier makes it unrepresentable, because
	// there is nothing left on the stack to undo back past. That is the same trade this slice
	// took for canonicalisation and for the ceremony id, and the same door redaction already
	// uses for the same structural reason: an operation whose effects escape the undo stack
	// must not leave the stack claiming it can reverse them.
	//
	// The cost, stated: edits made before convening lose their undo history. That is
	// defensible on its own — those edits are now part of what every party is invited to
	// sign, so undoing them after the invitations go out would be wrong regardless.
	//
	// The result goes STRAIGHT to wroteCommitFailure — ADR-004's 409-never-404 rule lives in
	// that one function, and a call site that maps the error itself is a second copy of it.
	if err := s.commitBarrier(doc, out.Document); wroteCommitFailure(w, err) {
		return
	}
	committed = true

	// **No `Seeds` on a FIRST-issued invitation either, and the silence is what produced
	// `/pending 357` (recorded 2026-09-03).** The re-issue door below states the absence for its
	// own case; this one said nothing, so a reader had to re-derive the whole situation.
	//
	// D6's second half — a bootstrap seed hint carried in the invitation, so a machine whose
	// shipped list has rotted can still reach the DHT — was BUILT at P04.S06 (v1.115.0) and is
	// wired to nothing on the real path. Named searches: `grep -rn 'SeedSample' --include=*.go .`
	// and `grep -rn '\.Seed(' --include=*.go .` outside tests each return one production caller,
	// and it is the same one — `internal/cli/rendezvous.go`, the `nib rendezvous` diagnostic. So
	// the feature is a closed loop inside one CLI tool.
	//
	// **Wiring the producing half alone is INERT**, which is why setting `Seeds` here would be
	// worse than leaving it: no server ever calls `rendezvous.Server.Seed()`, so the addresses
	// would travel, be parsed and scope-checked by `validateSeeds`, and then be dropped. A field
	// that crosses the wire and is discarded is the shape this repo's reader scans exist to refuse.
	//
	// **And wiring the consuming half ARMS something.** The P04.S06 slice wrote the chain out:
	// hostile seeds → a routing table containing nothing else → every `probeTargets` entry
	// attacker-chosen → `classify`'s majority satisfied → D33's punch budget aimed at a victim of
	// the sender's choosing, from this user's IP. It is latent today precisely because the consume
	// half is unwired, and it is bounded by `invSeedsOnly`, which admits invitation seeds only
	// after a DEMONSTRATED bootstrap failure.
	//
	// So the open question is not this line: it is whether a recipient can be told, before they
	// open an invitation, that doing so may cause outbound contact to addresses the sender chose,
	// with their real IP and an exact timestamp. That is a different consent question from "Nib
	// uses the DHT", and it is P06's. Until it has an answer, seeds stay off both halves — stated
	// here so the next reader inherits the reasoning rather than the silence.
	invites := make([]conveneInvite, 0, len(out.Invites))
	for _, inv := range out.Invites {
		fp, _ := hex.DecodeString(inv.Party.Fingerprint)
		invites = append(invites, conveneInvite{
			Fingerprint: inv.Party.Fingerprint,
			Label:       inv.Party.Label,
			Signs:       inv.Party.Signs,
			Name:        nameOrEmpty(fp),
			Invitation:  inv.Text,
		})
	}
	writeJSON(w, conveneResponse{
		Ceremony: out.Record.ID,
		Intent:   out.Record.Intent,
		Expires:  out.Record.Expires.UTC().Format(time.RFC3339),
		Invites:  invites,
		Warnings: out.Warnings,
		Doc:      s.docResponse(doc),
	})
}

// unconvene removes what a failed convene had already written, and REPORTS what it could not.
//
// Best-effort by construction — if the disk is refusing writes there is nothing better to do
// than try. **The first draft's comment claimed it was "NOT silent about being best-effort"
// and it discarded both errors and logged nothing**, which is the repo's signature defect on
// its own residue path; the slice's diff review found it.
//
// Reporting matters here more than anywhere else in the slice: this is the only teardown for
// key material, the response has already been written by the branch that triggered it, and a
// user whose rollback failed is holding a ceremony whose secrets are in the vault and whose
// document never committed — one they can neither see nor cancel. A log line is the only
// channel left, and it names both halves separately because they fail independently.
func (s *Server) unconvene(v *vault.Vault, root, id string) {
	// **The three vault stores go through the same door the close-out uses** (ADR-009, P08.S06).
	// They were three separate `Prune*` calls with three hand-written log lines here, and a
	// fourth store added later would have had to be remembered in two places — which is how
	// P08.S01 found this function reaching one of two invitation stores in the first place.
	_ = closeOutStores(v, id, "convene rollback")
	// **The mirror is NOT in that door, and it is the reason it is not.** A rollback DELETES: a
	// convene whose commit failed never produced a contribution, so there is nothing to preserve
	// and leaving the directory behind would put a ceremony that was never convened in front of
	// the listing. A close-out MOVES, because on every machine but the convener's the mirror is
	// the only place that party's own signature exists. One door with a verb parameter would be
	// two behaviours wearing one name.
	if err := ceremony.RemoveMirror(root, id); err != nil {
		log.Printf("convene rollback: could not remove %s: %v — a ceremony directory is left "+
			"under the output folder for a ceremony that was never convened", id, err)
	}
}

// conveneStatus maps a convene refusal to a status code.
//
// **400 for everything the convener can fix, 409 for the one that is about the document's
// state.** A blanket 400 would tell a user their roster is wrong when the truth is that this
// document is already part of a ceremony — and C04's whole point is that the answer to that
// is a different action, not a corrected field.
func conveneStatus(err error) int {
	if errors.Is(err, ceremony.ErrAlreadyConvened) {
		return http.StatusConflict
	}
	if errors.Is(err, ceremony.ErrNoHopBudget) || errors.Is(err, ceremony.ErrNoDeliveryBudget) {
		// The caller did not supply one, and the caller is this package. A user cannot cause
		// it and cannot fix it.
		//
		// **Both budgets, not just the hop one (P08.S05b).** The delivery budget arrived as a
		// second required parameter and its sentinel fell through to 400 — telling a user their
		// request was bad about a field no request carries and no user can set.
		return http.StatusInternalServerError
	}
	return http.StatusBadRequest
}

// --- the read door ------------------------------------------------------------

type ceremonyInvitesRequest struct {
	Ceremony string `json:"ceremony"`
}

// handleCeremonyInvites re-issues the invitations for a ceremony this machine convened.
//
// # Why this exists at all, and why it is a POST
//
// Without it the secrets are written, tested, and reachable by nothing — the ninth member of
// the built-but-unwired set this phase's own review named as its through-line. The convener
// is the only party who can re-issue an invitation, and before this the invitations existed
// in exactly one HTTP response: close the tab and the ceremony was unrecoverable rather than
// merely stalled.
//
// POST because `requireUnlocked` applies CSRF and the loopback-origin check only to non-GET
// methods. A GET here would be vault-gated and nothing else, and this route returns channel
// secrets.
func (s *Server) handleCeremonyInvites(w http.ResponseWriter, r *http.Request) {
	v := vaultFrom(r)
	var req ceremonyInvitesRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<16)).Decode(&req); err != nil {
		httpError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	// Through MirrorDir, so the id is validated as an id before it names a path — the same
	// door the write side uses rather than a second opinion about what an id looks like.
	root := defaultOutputDir()
	rec, _, err := ceremony.ReadMirror(root, req.Ceremony, time.Now())
	if err != nil {
		httpError(w, http.StatusNotFound,
			"Nib has no record of that ceremony on this machine: "+err.Error())
		return
	}
	// **The convener's own entry is the ONLY legitimate skip, and it is identified rather
	// than inferred from a missing secret.** The first draft `continue`d on any miss, so a
	// ceremony whose secrets had been pruned returned 200 with an empty list and no sentence
	// — the convener asked to re-issue and was told nothing. Found at the slice's diff review.
	conv := convenerFingerprintOf(rec)
	if conv == "" {
		httpError(w, http.StatusInternalServerError,
			"that ceremony's record does not name a convener, so Nib cannot re-issue its "+
				"invitations")
		return
	}
	rh := rosterHashHex(rec)
	if rh == "" {
		// An invitation with an empty commitment is refused by every recipient's
		// MatchesRecord — with a message that reads as the convener's fault. Refuse here
		// instead, where the truth is legible.
		httpError(w, http.StatusInternalServerError,
			"that ceremony's record will not re-hash, so Nib cannot re-issue its invitations")
		return
	}
	invites := make([]conveneInvite, 0, len(rec.Roster))
	for _, p := range rec.Roster {
		if strings.EqualFold(p.Fingerprint, conv) {
			continue // the convener holds every party's invitation and receives none
		}
		fp, derr := hex.DecodeString(p.Fingerprint)
		if derr != nil {
			httpError(w, http.StatusInternalServerError,
				"that ceremony's record names a party Nib cannot read")
			return
		}
		// **One door for minting a party's invitation from the convener's own secret (ADR-009).**
		// This route held the only copy until P08.S05g needed the same thing for the delivery round —
		// per-party, because each party's secret differs and that is what makes their rendezvous
		// targets distinct. Two copies of that would drift exactly the way `Intent` already did here:
		// omitted once, and every re-issued invitation was refused at the recipient's arrival gate.
		//
		// `Seeds` is still absent and cannot be recovered — the record does not carry them — so a
		// re-issued invitation has no DHT seed hints. Stated rather than papered over; every other
		// field a recipient CHECKS is present.
		inv, ierr := convenerInvitationFor(v, rec, p)
		if ierr != nil {
			httpError(w, http.StatusGone, ierr.Error())
			return
		}
		text, eerr := inv.Encode()
		if eerr != nil {
			httpError(w, http.StatusInternalServerError, "could not re-issue an invitation")
			return
		}
		invites = append(invites, conveneInvite{
			Fingerprint: p.Fingerprint,
			Label:       p.Label,
			Signs:       p.Signs,
			Name:        nameOrEmpty(fp),
			Invitation:  text,
		})
	}
	writeJSON(w, conveneResponse{
		Ceremony: rec.ID,
		Intent:   rec.Intent,
		Expires:  rec.Expires.UTC().Format(time.RFC3339),
		Invites:  invites,
	})
}

func convenerFingerprintOf(rec ceremony.Record) string {
	if c, ok := rec.Convener(); ok {
		return c.Fingerprint
	}
	return ""
}

func rosterHashHex(rec ceremony.Record) string {
	h, err := rec.RosterHash()
	if err != nil {
		return ""
	}
	return hex.EncodeToString(h)
}

// ceremoniesResponse is the listing (P08.S03).
type ceremoniesResponse struct {
	Ceremonies []ceremony.Stored `json:"ceremonies"`
	// Primary is false when another Nib holds this machine's instance record. The listing is
	// still answered — reading is safe — but this process must not resume or prune, because two
	// processes racing one mirror is two writers with no lock between them.
	Primary bool `json:"primary"`
	// Note is the sentence a non-primary Nib shows instead of an action.
	Note string `json:"note,omitempty"`
	// Ended is what this machine has closed out (P08.S06), newest first.
	//
	// **Carried in the same response as the live ones because the pair is the answer.** A user
	// asking "what ceremonies do I have" after one finishes gets an empty `Ceremonies` and no
	// explanation, and the signed contribution the close-out deliberately preserved is at a path
	// nothing named. Two routes would make the second one optional, which is how the preserved
	// copy becomes a file nobody knows exists.
	Ended []ceremony.Receipt `json:"ended,omitempty"`
}

// handleCeremonies lists the ceremonies stored on this machine (P08.S03, C12).
//
// # Why it does not open a single document
//
// `ReadMirror` runs `sign.Verify` and, on an unsigned document, a full `ContentDigest`. Measured at
// the P08.S01 deepdive: 10 ms at 100 pages, 69 ms at 500, 195 ms at 1000 — superlinear, on text-only
// fixtures, and these are contracts with images. Fifty stored ceremonies would be seconds on a
// request path. `ceremony.ListStored` reads `record.json` and nothing else.
//
// The cost of that is stated rather than hidden: this answer carries no signature count and no next
// action, because both live in the document. The panel opens ONE ceremony to say "2 of 4 signed".
//
// # Why there is no lock, and this took a wrong turn first
//
// The plan asked for an exclusive lock on `~/nib/ceremonies/`. There is no locking anywhere in this
// tree, and adding some would be a SECOND cross-process policy contradicting the one that exists:
// `cmd/nib/main.go` decides, deliberately and with the reasoning at the line, that a launch which
// loses the instance race *"carries on and serves"* — *"a launch that loses twice is better off
// running than refusing to start"*.
//
// The signal that distinguishes the two is already maintained. `instanceToken` is empty exactly when
// this process is not the recorded instance, so a non-primary Nib can read and must not act. One
// mechanism, already tested, and no new file in a directory whose file set other checks assume.
func (s *Server) handleCeremonies(w http.ResponseWriter, r *http.Request) {
	// **The sweep runs BEFORE the listing is read, so what the user sees is post-close-out**
	// (P08.S06's third trigger). Synchronous and not on a goroutine: a sweep racing the
	// `ListStored` below would show the user a ceremony it is in the middle of moving, and the
	// route's whole job is to say what is on disk. It is the same function the unlock hook calls
	// — ADR-009, one rule and every site calls it — and it returns immediately when this Nib is
	// not the primary one or the vault is shut, which is what makes an unconditional call here
	// correct rather than merely convenient.
	//
	// **Why the listing is a trigger at all**: the other one is unlock, and a machine that stays
	// unlocked for a fortnight would otherwise never sweep. This route is what a user opens to
	// look at their ceremonies, which is the moment the answer needs to be current.
	// **Unconditional, and the nil check lives in the door (ADR-009, P06.S01).** This read the
	// vault and skipped on nil, which is a second implementation of a rule `closeOutEnded` already
	// holds — and a mutation removing THIS guard left the whole suite green, which is what proved
	// the door was the load-bearing one. Removing both goes red. Two guards for one rule is the
	// shape ADR-009 was written after finding six of.
	s.closeOutEnded(s.unlockedVault(), time.Now())
	list, err := ceremony.ListStored(defaultOutputDir(), time.Now())
	if err != nil {
		httpError(w, http.StatusInternalServerError,
			"the ceremonies folder could not be read: "+err.Error())
		return
	}
	s.mu.Lock()
	primary := s.instanceToken != ""
	s.mu.Unlock()
	// Best-effort: a ceremonies folder that reads and an `ended/` that does not is still a
	// listing worth answering, and the live half is the one a user is acting on.
	ended, _ := ceremony.ListEnded(defaultOutputDir())
	out := ceremoniesResponse{Ceremonies: list, Primary: primary, Ended: ended}
	if !primary {
		out.Note = "another copy of Nib is already running on this machine. This one can show " +
			"your ceremonies but must not continue or remove them, because both would be " +
			"writing to the same folder."
	}
	writeJSON(w, out)
}
