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
	// DeliveryBudget is the worst-case wall-clock ONE leg of the delivery round can consume
	// (P08.S05b, D22's delivery pin).
	//
	// **A parameter for HopBudget's reason, and the reason is precise rather than approximate**:
	// two of its three terms — `bootstrapBudget` and `connectDeadline` — are UNEXPORTED constants
	// in `internal/server`'s clocks.go, so no other package can read them at any price. The third,
	// `p2p.DeliveryLegBudget()`, this package could reach (it already imports `internal/p2p`), and
	// saying otherwise would be the loose claim a commit gate is for: what makes the figure
	// uncomputable here is the server half alone. `server.ceremonyDeliveryLegBudget()` is the one
	// place that can see all three, and it fills this in.
	//
	// **A SEPARATE figure from HopBudget even though they are equal today.** They are equal only
	// because the transfer path and the co-sign path currently arm the same deadlines; P08.S05d is
	// scheduled to shrink this one by making both of the delivery leg's gates non-interactive. Two
	// names is what lets the reservation follow that change, and what lets the refusal below say
	// which half of the time a convener is short of.
	//
	// A zero is REFUSED rather than defaulted, for HopBudget's reason and one more: a second
	// duration field that silently defaults to zero is the shape this repo has already been bitten
	// by once, where a newly added config field was missed at one of its call sites and nothing
	// failed.
	DeliveryBudget time.Duration
	// ConvenerSigns is used only when the convener is not already in Roster.
	ConvenerSigns bool
}

// plural renders a count with its noun, so a two-party ceremony does not read "1 hops".
//
// The refusal and the sitting warning are two of the few sentences in this product a solicitor
// reads once and acts on, and "1 hops need about 29m20s" is the COMMON case — the ordinary
// ceremony has two parties and therefore one hop.
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
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
	//
	// # It names the answer and the cost, and the cost is the half that was missing
	//
	// C04 asks for two things beyond the refusal: that the message "names a new ceremony as the
	// answer", AND "states that the signatures already collected cannot be carried into it" —
	// and it says why the second is written separately: *"it is the half a builder omits, being
	// the bad news."* It was omitted. Until P07's phase close this read "this document is
	// already part of a ceremony" and stopped, which tells a convener what is wrong and nothing
	// about what it costs them.
	//
	// The cost is real and is the reason C04 exists. Adding a party changes `rosterHash`, so
	// every invitation already issued fails `MatchesRecord` on roster length; a new ceremony is
	// the only way forward, and it starts from a document with no signatures on it. A convener
	// who remembers the second landlord after three people have signed needs to know that before
	// they start, not after.
	ErrAlreadyConvened = errors.New("this document is already part of a ceremony, so it cannot " +
		"take another party. Convene a NEW ceremony from the original unsigned document — and " +
		"note that any signatures already collected on this one cannot be carried into it: " +
		"everyone who has signed will have to sign again")
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
	// ErrDeadlineTooTight: the deadline does not admit every hop AND the delivery round that
	// follows it (C20, widened at P08.S05b).
	//
	// **The wording gained its second clause because the refusal is no longer only about
	// signing.** A deadline that fits every hop with room to spare is now refused when the round
	// does not fit, and a sentinel saying signing does not fit would name the wrong half.
	ErrDeadlineTooTight = errors.New("that deadline does not leave time for every party to sign and receive the finished document")
	// ErrNoHopBudget: the caller did not supply one. See ConveneRequest.HopBudget.
	ErrNoHopBudget = errors.New("convene was called without a hop budget")
	// ErrNoDeliveryBudget: likewise for the round. See ConveneRequest.DeliveryBudget.
	ErrNoDeliveryBudget = errors.New("convene was called without a delivery budget")
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
	if req.DeliveryBudget <= 0 {
		return Convened{}, ErrNoDeliveryBudget
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
	// **The other two strings that reach a signature block (P07.S07a).**
	//
	// Until this slice a party's `Label` and `Capacity` were carried, committed and never
	// rendered, so their width could not matter. Now a block says `Signer: <label>` and
	// `Capacity: <capacity>`, drawn by the same `ctx.fillText` with no `maxWidth` that
	// the block bound exists to protect the recital from — two more silent clippings, of which
	// capacity is the worse: it is a claim about a party's AUTHORITY, it is inside the signed
	// commitment, and half of it on the page is a document that says something other than what
	// the parties agreed.
	//
	// Refused at CONVENE, on the same reasoning as the recital: this is the last moment the
	// convener can retype it, and the alternative is discovering it on a finished document that
	// several people have already signed.
	if err := checkRosterText(roster); err != nil {
		return Convened{}, err
	}
	// The JOINT height rule, once both halves are known — see checkBlocksFit.
	if err := checkBlocksFit(roster, req.Intent); err != nil {
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
	//
	// **The round is reserved too, and it is `hops` LEGS rather than one (P08.S05b).** D22's
	// delivery pin puts the finished document in every party's hands, so the round costs about a
	// leg per party — reserving one term for it under-reserves by `hops-1` legs, which at nine
	// parties is most of four hours against a round the arm is bounded by `Expires` to complete
	// inside.
	//
	// **`hops` over-reserves by one leg, deliberately.** The LAST signer already holds the finished
	// document at the end of its own hop, so only `hops-1` legs carry new bytes. Reserving the
	// extra one is the safe direction — the cost is minutes on a deadline measured in hours, and
	// the alternative is a figure that has to be re-derived every time D22's topology moves.
	//
	// The two terms are kept SEPARATE in the sentence rather than summed into one figure. A
	// convener who is refused has to choose a new deadline, and "N hops need X" where X silently
	// includes non-hop time is an arithmetic lie at the one string they reason from.
	hops := len(roster) - 1
	signTime := time.Duration(hops) * req.HopBudget
	deliverTime := time.Duration(hops) * req.DeliveryBudget
	if need := signTime + deliverTime; !req.Expires.After(now.Add(need)) {
		return Convened{}, fmt.Errorf("%w: %s need about %s to sign and about %s to deliver "+
			"afterwards — %s in all — and this ceremony ends at %s, which is %s away",
			ErrDeadlineTooTight, plural(hops, "hop"), signTime, deliverTime, need,
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
			// **The attention figure stays `hops × HopBudget` and the round is a SEPARATE clause
			// (P08.S05b).** Folding the delivery reservation into this total would break the
			// sentence's own arithmetic — a reader multiplying the two numbers it gives them would
			// get a different answer than the total it prints — and it would miscategorise the
			// round, whose subject is machine time on legs the convener does not sit through,
			// as "the convener's attention".
			Text: fmt.Sprintf("This ceremony has %d parties. Each hop needs both people present "+
				"and can take up to %s, so %s is roughly %s of the convener's attention — "+
				"more than one sitting for most people. Delivering the finished document "+
				"afterwards reserves a further %s.",
				len(roster), req.HopBudget, plural(hops, "hop"), time.Duration(hops)*req.HopBudget,
				time.Duration(hops)*req.DeliveryBudget),
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
// The block's real ceiling is far below this — see p2p.BlockFits — so this is not the
// product rule, it is the bound that keeps unbounded input away from the width code at all.
// The route's body limit is 1 MiB; without a cheap bound here, every rejected megabyte is
// measured before it is refused. Belt and braces alongside BlockOverflow's bisection: a
// cheap constant refusal beats a fast measurement of something absurd.
const maxIntentInput = 4096

// checkRosterText refuses a label or a capacity that would not render in full on a block.
//
// Named per party rather than generically, because the convener's action is to shorten ONE
// person's entry and a message that does not say whose sends them to re-read the whole roster.
func checkRosterText(roster []Party) error {
	for _, p := range roster {
		who := p.Label
		if who == "" {
			who = short(p.Fingerprint)
		}
		if n := len([]rune(p.Label)); n > maxIntentInput {
			return fmt.Errorf("%w: %s's label is %d characters, and a label is a name rather "+
				"than a document", ErrIntentTooLong, who, n)
		}

		if n := len([]rune(p.Capacity)); n > maxIntentInput {
			return fmt.Errorf("%w: %s's capacity is %d characters, and a capacity is a phrase "+
				"rather than a document", ErrIntentTooLong, who, n)
		}

	}
	return nil
}

func checkIntent(intent string) error {
	if strings.TrimSpace(intent) == "" {
		return ErrNoIntent
	}
	if n := len([]rune(intent)); n > maxIntentInput {
		return fmt.Errorf("%w: it is %d characters, and a recital is a sentence rather than a "+
			"document", ErrIntentTooLong, n)
	}
	return nil
}

// checkBlocksFit is the block-height rule, and it is JOINT (/pending 286).
//
// # Why it replaced three separate width checks
//
// Until block lines wrapped, the recital, each party's label and each party's capacity had one
// ceiling each — "does this render on ONE line" — and the three were independent. A line may now
// wrap over as many lines as it needs, up to `maxBlockLines`, so they are not: a recital that fits
// beside a short capacity does not fit beside a long one. Asking the question per field is how two
// separately-legal values combine into a block nobody can read.
//
// # Per SIGNING party, and only those
//
// A non-signing party has no block, so its label and capacity render nowhere and cannot overflow
// one. Their absurd-input bounds still apply in `checkRosterText` above — this is about geometry.
//
// # The position is not the point
//
// `Position` and `RosterSize` here select the ceremony branch of `AppearanceLines` (the
// `Party k of n` line) rather than describing any real party's place. That line's WIDTH does not
// vary with either number at any roster this repo admits — "Party 1 of 2" and "Party 99 of 99" are
// both a fraction of the block — so the count is unaffected, and computing the true signing order
// here would be a second implementation of `SigningOrder`'s rule (ADR-009).
func checkBlocksFit(roster []Party, intent string) error {
	signing := 0
	for _, p := range roster {
		if p.Signs {
			signing++
		}
	}
	for _, p := range roster {
		if !p.Signs {
			continue
		}
		who := p.Label
		if who == "" {
			who = short(p.Fingerprint)
		}
		// The label falls back to the short fingerprint exactly as `StampCommitment` does, so this
		// measures the block that will actually be drawn rather than an idealised one.
		signer := p.Label
		if signer == "" {
			signer = short(p.Fingerprint)
		}
		att := p2p.Attestation{
			Signer: signer, Capacity: p.Capacity, Intent: intent,
			Position: 1, RosterSize: signing,
		}
		if p2p.BlockFits(att) {
			continue
		}
		lines, limit, worst, fits := p2p.BlockOverflow(att)
		return fmt.Errorf("%w: %s's signature block needs %d lines and %d is the limit, so it "+
			"would render too small to read. The longest part is %s, and about %d characters of "+
			"it fit alongside the rest. Every block carries the recital, the party's name and "+
			"their capacity in full, so Nib refuses a block it would have to shrink past "+
			"legibility rather than producing one nobody can read",
			ErrIntentTooLong, who, lines, limit, worst, fits)
	}
	return nil
}
