package ceremony

import (
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"nib/internal/p2p"
	"nib/internal/sign"
)

// Convene is the pre-signing pass: the first place in the product that ever constructs a
// ceremony.Record (P07.S02a).
//
// # Why it lives in this package and imports p2p
//
// Measured at the slice grill on a clean tree: `internal/ceremony` MAY import `internal/p2p`
// in production code — build, vet AND test-compile all green. Only the reverse edge cycles,
// through record_test.go, and that cycle is invisible to `go build`. So the door sits beside
// the Record it builds and calls p2p for the geometry.
//
// Two alternatives were refused. A door in `internal/server` would put the convene-side
// predicate on one side of a package boundary and Record.Verify's on the other — one rule,
// two implementations, which is what already produced three disagreeing fingerprint
// comparisons. And injecting PrepareCeremonyDocument as a func parameter, to dodge the
// import, would let a test pass a no-op: the ordering guard would then go green with the
// readme never appended, which is this repo's signature failure.
//
// # The order, and which arrows anything enforces
//
// readme+pages -> docHash -> Sign -> Embed -> first signature. Only the LAST arrow is
// enforced by the callee (Embed refuses a signed document). The rest are enforced by being
// in one function that nobody can enter halfway — which is the whole reason this is a door
// rather than a sequence a route performs.
type ConveneRequest struct {
	// Roster is the parties in SIGNING ORDER. The convener need not be in it; see Convene.
	Roster []Party
	// Intent is the recital every party agrees to, and D20 makes it the only home for it.
	Intent string
	// Expires is the ceremony deadline.
	Expires time.Time
	// HopBudget is the worst-case wall-clock ONE hop can consume.
	//
	// **A parameter rather than a constant this package reaches for, and that is forced.**
	// It sums four terms — two unexported in `internal/server`'s clocks.go, two derived
	// inside `internal/p2p` — so neither this package nor p2p can compute it. Both panels
	// that produced a per-hop figure during P07's planning did so without checking which
	// package could arrive at it, and both got a different number. `server.ceremonyHopBudget()`
	// is the one place that can, and it fills this in.
	//
	// A zero is REFUSED rather than defaulted: an injectable duration whose zero value means
	// "everything fits" would make C20's guard pass with the rule switched off.
	HopBudget time.Duration
	// ConvenerSigns is used only when the convener is not already in Roster.
	ConvenerSigns bool
}

// Invite is one party's invitation, ready to hand over.
//
// A slice in ROSTER ORDER, not the map NewInvitations returns, and that is a correctness
// point rather than taste: the map is keyed by the raw fingerprint string, so a case-variant
// entry keys under a spelling every lowercase lookup misses — and roster order IS hop order,
// which a map destroys.
type Invite struct {
	Party      Party
	Invitation Invitation
	Text       string
}

// Warning is a named soft refusal the API carries and a surface later renders.
//
// Machine-tagged rather than prose-only, so a panel can bind a warning to the control that
// caused it, and so a test can assert a warning's IDENTITY rather than pinning its wording.
type Warning struct {
	Code string `json:"code"`
	Text string `json:"text"`
}

// WarnSittingCeiling is D22's practical single-sitting ceiling.
//
// **A warning, not a refusal, and the distinction is D22's own.** Its pin keeps two numbers
// with separated roles: 32 is the hard cap — "what the code refuses past" — and ~8 is the
// practical sitting ceiling, "what the UI should be designed and copy-written for". Refusing
// at 8 would also make three of this phase's own criteria unreachable through this door:
// C03 drives nine parties, C18 drives five of nine, C21 measures a stalled nine-party
// ceremony.
const WarnSittingCeiling = "sitting-ceiling"

// SittingCeiling is D22's ~8.
const SittingCeiling = 8

// Convened is what the door produces.
type Convened struct {
	Record   Record
	Document []byte
	Invites  []Invite
	Warnings []Warning
}

var (
	// ErrAlreadyConvened: this document already carries a ceremony record.
	//
	// Refused BEFORE Embed deliberately. pdfops.AddAttachment refuses a duplicate name with
	// `an attachment named "nib-ceremony.json" already exists` — a true sentence naming a PDF
	// internal, shown to a solicitor. This is the same fact in words they can act on.
	ErrAlreadyConvened = errors.New("this document is already part of a ceremony")
	// ErrRosterTooSmall: a ceremony of one has no hop and nothing to invite.
	ErrRosterTooSmall = errors.New("a ceremony needs at least two parties")
	// ErrRosterTooLarge: past the hard cap (D33).
	ErrRosterTooLarge = errors.New("that ceremony names more parties than Nib will carry")
	// ErrDuplicateParty: the same fingerprint twice.
	//
	// Its own sentinel because the harm is specific and silent: NewInvitations keys its
	// output map by fingerprint, so a duplicate collapses two parties into one invitation
	// while Hops() still counts both — a ceremony that can never complete, with no error at
	// any door. Measured at the grill.
	ErrDuplicateParty = errors.New("the same party appears on this roster twice")
	// ErrNoIntent: the recital is the one thing everybody is agreeing to.
	ErrNoIntent = errors.New("a ceremony needs a recital saying what the parties are agreeing to")
	// ErrIntentTooLong: longer than a signature block renders verbatim.
	ErrIntentTooLong = errors.New("that recital is longer than a signature block can show in full")
	// ErrDeadlineTooTight: the deadline does not admit every hop (C20).
	ErrDeadlineTooTight = errors.New("that deadline does not leave time for every party to sign")
	// ErrNoHopBudget: the caller did not supply one. See ConveneRequest.HopBudget.
	ErrNoHopBudget = errors.New("convene was called without a hop budget")
)

// Convene runs the whole pre-signing pass and returns the convened document, its record and
// one invitation per party.
//
// `now` is threaded rather than read, for the reason Record.Verify and CheckDocument both
// already take one: a wall-clock read inside a validation verdict is nondeterminism reaching
// a decision.
func Convene(pdf []byte, req ConveneRequest, certPEM, keyPEM []byte, now time.Time) (Convened, error) {
	if req.HopBudget <= 0 {
		return Convened{}, ErrNoHopBudget
	}
	// The already-convened check FIRST, before anything structural, so the refusal names the
	// ceremony rather than the attachment layer.
	if _, err := Extract(pdf); !errors.Is(err, ErrNoRecord) {
		if err == nil {
			return Convened{}, ErrAlreadyConvened
		}
		return Convened{}, fmt.Errorf("%w: it carries a record that will not parse: %v",
			ErrAlreadyConvened, err)
	}
	if sign.Verify(pdf).State != sign.Unsigned {
		return Convened{}, errors.New("this document is already signed; a ceremony is convened " +
			"before anybody signs, because appending pages would break every signature on it")
	}

	convFP, err := sign.Fingerprint(certPEM)
	if err != nil {
		return Convened{}, fmt.Errorf("read this machine's identity: %w", err)
	}
	roster, err := canonicalRoster(req, hex.EncodeToString(convFP))
	if err != nil {
		return Convened{}, err
	}
	if err := checkIntent(req.Intent); err != nil {
		return Convened{}, err
	}

	signing := 0
	for _, p := range roster {
		if p.Signs {
			signing++
		}
	}
	if signing == 0 {
		return Convened{}, fmt.Errorf("%w: nobody on it is marked as a signer", ErrRosterTooSmall)
	}
	// C20, at CONVENE rather than at hop 3. The reservation is every hop, not one: a deadline
	// that admits the first hop and not the last is a ceremony that strands a document
	// carrying real signatures, and the convener is the only person who can still fix it.
	hops := len(roster) - 1
	if need := time.Duration(hops) * req.HopBudget; !req.Expires.After(now.Add(need)) {
		return Convened{}, fmt.Errorf("%w: %d hops need about %s and this ceremony ends at %s, "+
			"which is %s away", ErrDeadlineTooTight, hops, need,
			req.Expires.UTC().Format(time.RFC3339), req.Expires.Sub(now).Truncate(time.Minute))
	}

	idHex, err := NewID()
	if err != nil {
		return Convened{}, err
	}
	var id p2p.CeremonyID
	raw, err := hex.DecodeString(idHex)
	if err != nil || len(raw) != len(id) {
		return Convened{}, fmt.Errorf("new ceremony id is not %d bytes: %q", len(id), idHex)
	}
	copy(id[:], raw)

	prepared, err := p2p.PrepareCeremonyDocument(pdf, id, convFP, signing)
	if err != nil {
		return Convened{}, fmt.Errorf("prepare the document: %w", err)
	}
	// The digest AFTER every page is in place. Nothing may append after this line: page count
	// and page content are both inside ContentDigest, so a later append moves DocHash under
	// every party.
	docHash, err := DocumentHash(prepared)
	if err != nil {
		return Convened{}, fmt.Errorf("hash the prepared document: %w", err)
	}

	rec := Record{
		ID:      idHex,
		DocHash: docHash,
		Roster:  roster,
		Intent:  req.Intent,
		Expires: req.Expires,
	}
	if err := rec.Sign(certPEM, keyPEM); err != nil {
		return Convened{}, err
	}
	// Belt and braces, and not ceremony: Verify is the door every NON-convener passes, so a
	// record this door emits that Verify would refuse is a ceremony nobody can join. It also
	// makes the roster bounds real at both ends rather than only at the emitter.
	if err := rec.Verify(now); err != nil {
		return Convened{}, fmt.Errorf("the record this convene produced does not verify: %w", err)
	}

	doc, err := Embed(prepared, rec)
	if err != nil {
		return Convened{}, fmt.Errorf("attach the ceremony record: %w", err)
	}

	invMap, err := NewInvitations(rec)
	if err != nil {
		return Convened{}, err
	}
	invites := make([]Invite, 0, len(invMap))
	for _, p := range rec.Roster {
		inv, ok := invMap[p.Fingerprint]
		if !ok {
			continue // the convener holds every party's and receives none
		}
		text, err := inv.Encode()
		if err != nil {
			return Convened{}, fmt.Errorf("encode the invitation for %s: %w", short(p.Fingerprint), err)
		}
		invites = append(invites, Invite{Party: p, Invitation: inv, Text: text})
	}
	// One per non-convener party, asserted rather than assumed — NewInvitations keys a map by
	// fingerprint, and a silent collapse there is the duplicate-party defect arriving through
	// a door that already refused it.
	if want := len(rec.Roster) - 1; len(invites) != want {
		return Convened{}, fmt.Errorf("convene produced %d invitations for a roster of %d",
			len(invites), len(rec.Roster))
	}

	var warnings []Warning
	if len(roster) > SittingCeiling {
		warnings = append(warnings, Warning{
			Code: WarnSittingCeiling,
			Text: fmt.Sprintf("This ceremony has %d parties. Each hop needs both people present "+
				"and can take up to %s, so %d hops is roughly %s of the convener's attention — "+
				"more than one sitting for most people.",
				len(roster), req.HopBudget, hops, time.Duration(hops)*req.HopBudget),
		})
	}
	return Convened{Record: rec, Document: doc, Invites: invites, Warnings: warnings}, nil
}

// canonicalRoster validates the requested roster and returns it in the one representation
// the commitment binds, with the convener present.
func canonicalRoster(req ConveneRequest, convFP string) ([]Party, error) {
	roster := make([]Party, 0, len(req.Roster)+1)
	seen := map[string]int{}
	haveConvener := false
	for i, p := range req.Roster {
		fp := strings.ToLower(strings.TrimSpace(p.Fingerprint))
		b, err := hex.DecodeString(fp)
		if err != nil || len(b) != 32 {
			return nil, fmt.Errorf("party %d does not have a full fingerprint: %q", i+1, p.Fingerprint)
		}
		if first, dup := seen[fp]; dup {
			// Names WHICH party and WHICH two positions: a convener staring at nine 64-character
			// hex strings cannot otherwise find it.
			return nil, fmt.Errorf("%w: %s is party %d and party %d", ErrDuplicateParty,
				short(fp), first+1, i+1)
		}
		seen[fp] = i
		p.Fingerprint = fp
		if fp == convFP {
			haveConvener = true
		}
		roster = append(roster, p)
	}
	if !haveConvener {
		// **The door inserts it, and A8 is why.** identity() mints this machine's key on first
		// use, so on a fresh vault the fingerprint does not exist until the moment of convening
		// and a client-supplied roster cannot contain it. Without this, Record.Verify answers
		// ErrConvenerNotInRoster — "signed by someone not in its roster" — which is an
		// accusation, about themselves, on a fresh install doing nothing wrong.
		//
		// At position 0: the convener is the hub who dials every hop, and today's code has a
		// signing convener sign first regardless of roster position. A caller who wants another
		// position includes themselves in the roster and this branch does not run.
		roster = append([]Party{{Fingerprint: convFP, Signs: req.ConvenerSigns}}, roster...)
	}
	if len(roster) < 2 {
		return nil, ErrRosterTooSmall
	}
	if len(roster) > MaxRoster {
		return nil, fmt.Errorf("%w: it names %d and the limit is %d", ErrRosterTooLarge,
			len(roster), MaxRoster)
	}
	return roster, nil
}

// maxIntentInput bounds what checkIntent will even MEASURE.
//
// The block's real ceiling is far below this — see p2p.IntentFitsBlock — so this is not the
// product rule, it is the bound that keeps unbounded input away from the width code at all.
// The route's body limit is 1 MiB; without a cheap bound here, every rejected megabyte is
// measured before it is refused. Belt and braces alongside MaxIntentRunes's bisection: a
// cheap constant refusal beats a fast measurement of something absurd.
const maxIntentInput = 4096

func checkIntent(intent string) error {
	if strings.TrimSpace(intent) == "" {
		return ErrNoIntent
	}
	if n := len([]rune(intent)); n > maxIntentInput {
		return fmt.Errorf("%w: it is %d characters, and a recital is a sentence rather than a "+
			"document", ErrIntentTooLong, n)
	}
	if !p2p.IntentFitsBlock(intent) {
		return fmt.Errorf("%w: it is %d characters and about %d fit. Every signature block "+
			"carries the recital in full, so Nib refuses a recital it would have to cut rather "+
			"than showing a shortened one above somebody's signature",
			ErrIntentTooLong, len([]rune(intent)), p2p.MaxIntentRunes(intent))
	}
	return nil
}
