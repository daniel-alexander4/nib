package p2p

import (
	"bytes"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"nib/internal/sign"
	"nib/internal/testpdf"
)

type l3Party struct {
	cert, key []byte
	fp        string
}

func l3Identity(t *testing.T, name string) l3Party {
	t.Helper()
	c, k, err := sign.GenerateIdentity(name)
	if err != nil {
		t.Fatal(err)
	}
	f, err := sign.Fingerprint(c)
	if err != nil {
		t.Fatal(err)
	}
	return l3Party{c, k, hex.EncodeToString(f)}
}

// l3Chain signs `doc` with each party in turn, each accepting the party named in `accepts` at
// the same index — the shape a ceremony produces, and the shape whose cross-binding the gate
// reads.
//
// **The accepted peer is a PARAMETER, and that is a fixture defect made unrepresentable.** The
// first draft derived it from the slice's own next element and fell back to `order[0]` at the
// end, so calling it one party at a time made every signer accept THEMSELVES — `Matched` then
// stayed false for every signature and the control test failed against a correct gate.
func l3Chain(t *testing.T, doc []byte, order []l3Party, accepts []l3Party, rosterHash string) []byte {
	t.Helper()
	if len(accepts) != len(order) {
		t.Fatalf("setup: %d signers and %d accepted peers", len(order), len(accepts))
	}
	for i, p := range order {
		accepted := accepts[i].fp
		place, err := NextPlacement(doc)
		if err != nil {
			t.Fatal(err)
		}
		doc, err = Contribute(doc, p.cert, p.key, Attestation{
			Signer: "P", AcceptedPeer: accepted, Intent: "ok", When: time.Now(),
			RosterHash: rosterHash,
		}, nil, place)
		if err != nil {
			t.Fatalf("signature %d: %v", i+1, err)
		}
	}
	return doc
}

func l3Prepared(t *testing.T) []byte {
	t.Helper()
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	doc, err := PrepareDocument(base)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func l3Roster(parties ...l3Party) Roster {
	r := Roster{}
	for _, p := range parties {
		r.Entries = append(r.Entries, RosterEntry{Fingerprint: p.fp, Signs: true})
	}
	return r
}

// TestTheGateAdmitsTheRightPartyAtEveryPosition — the control, and it comes FIRST.
//
// Every refusal below is worthless without it: a gate that refuses everything satisfies all four
// negative arms and none of them would notice.
func TestTheGateAdmitsTheRightPartyAtEveryPosition(t *testing.T) {
	a, b, c := l3Identity(t, "A"), l3Identity(t, "B"), l3Identity(t, "C")
	r := l3Roster(a, b, c)
	doc := l3Prepared(t)
	order := []l3Party{a, b, c}
	for i, p := range order {
		next, err := NextContributor(doc, r)
		if err != nil {
			t.Fatalf("position %d: NextContributor: %v", i+1, err)
		}
		if !strings.EqualFold(next.Fingerprint, p.fp) {
			t.Fatalf("position %d: the document waits for %s, want %s",
				i+1, shortFP(next.Fingerprint), shortFP(p.fp))
		}
		if err := AdmitContribution(doc, r, p.fp); err != nil {
			t.Fatalf("position %d: the right party was refused: %v", i+1, err)
		}
		doc = l3Chain(t, doc, []l3Party{p}, []l3Party{order[(i+1)%len(order)]}, "")
	}
	// And the end state is its own answer rather than a fourth turn.
	if _, err := NextContributor(doc, r); !errors.Is(err, ErrCeremonyComplete) {
		t.Errorf("after every signing party signed, NextContributor said %v, want "+
			"ErrCeremonyComplete — a caller cannot tell 'finished' from 'broken'", err)
	}
}

// TestTheGateRefusesEachThingByName — four refusals, each asserted DISTINCT from the others.
//
// One helper printing one sentence satisfies four rows otherwise, which is what C05's own note
// forbids: the wrong-party and wrong-prefix cases are DRIVEN SEPARATELY, on separate fixtures.
func TestTheGateRefusesEachThingByName(t *testing.T) {
	a, b, c := l3Identity(t, "A"), l3Identity(t, "B"), l3Identity(t, "C")
	stranger := l3Identity(t, "Stranger")
	r := l3Roster(a, b, c)

	seen := map[string]string{}
	record := func(t *testing.T, name string, err error, want error) {
		t.Helper()
		if err == nil {
			t.Fatalf("%s: admitted", name)
		}
		if !errors.Is(err, want) {
			t.Errorf("%s: got %v, want %v", name, err, want)
		}
		if prior, dup := seen[err.Error()]; dup {
			t.Errorf("%s and %s produce the SAME sentence (%q) — a party cannot tell which "+
				"happened, and D23 says a refusal names its reason", name, prior, err)
		}
		seen[err.Error()] = name
	}

	// 1. The WRONG PARTY: the prefix is perfect and it is somebody else's turn.
	fresh := l3Prepared(t)
	record(t, "wrong party", AdmitContribution(fresh, r, b.fp), ErrNotYourTurn)

	// 2. NOT IN THE ROSTER at all — its own fact, and its own sentence.
	record(t, "not in the roster", AdmitContribution(fresh, r, stranger.fp), ErrNotInRoster)

	// 3. The WRONG PREFIX, on its OWN fixture: B signed first, so the document's first
	//    signature is not the roster's first signer.
	wrongOrder := l3Chain(t, l3Prepared(t), []l3Party{b}, []l3Party{c}, "")
	record(t, "wrong prefix", AdmitContribution(wrongOrder, r, c.fp), ErrPrefixMismatch)

	// 4. A prefix that is NOT CROSS-BOUND: A signed accepting a stranger who never signs, so
	//    A's attestation attests to nobody on this document. The identities are exactly right,
	//    which is the point — an identity-only check would admit this.
	place, err := NextPlacement(l3Prepared(t))
	if err != nil {
		t.Fatal(err)
	}
	dangling, err := Contribute(l3Prepared(t), a.cert, a.key, Attestation{
		Signer: "A", AcceptedPeer: stranger.fp, Intent: "ok", When: time.Now(),
	}, nil, place)
	if err != nil {
		t.Fatal(err)
	}
	// Then B signs on top, so A is no longer the LAST signature and its cross-binding is due.
	dangling = l3Chain(t, dangling, []l3Party{b}, []l3Party{c}, "")
	// Stimulus: the identities ARE the roster prefix, so this fixture isolates cross-binding.
	ats := ReadAttestations(dangling)
	if len(ats) != 2 || !strings.EqualFold(ats[0].Fingerprint, a.fp) ||
		!strings.EqualFold(ats[1].Fingerprint, b.fp) {
		t.Fatalf("setup: the fixture's signers are not A then B, so a refusal below would be "+
			"about the prefix rather than about cross-binding: %+v", ats)
	}
	record(t, "prefix not cross-bound", AdmitContribution(dangling, r, c.fp), ErrPrefixUnproven)

	// 5. A SUBSTITUTED but well-formed proceeding: valid signatures in the right order, each
	//    committing to a ceremony that is not this one.
	other := strings.Repeat("ab", 32)
	substituted := l3Chain(t, l3Prepared(t), []l3Party{a, b}, []l3Party{b, c}, other)
	mine := r
	mine.Commitment = strings.Repeat("cd", 32)
	record(t, "substituted proceeding", AdmitContribution(substituted, mine, c.fp), ErrProceedingMismatch)
}

// TestAnInvalidPrefixSignatureIsRefused — the "each one valid" half of L3, which an identity
// comparison satisfies without checking.
func TestAnInvalidPrefixSignatureIsRefused(t *testing.T) {
	a, b := l3Identity(t, "A"), l3Identity(t, "B")
	r := l3Roster(a, b)
	doc := l3Chain(t, l3Prepared(t), []l3Party{a}, []l3Party{b}, "")
	// Stimulus: it is admitted while the signature is intact, so the refusal below is about the
	// damage and not about the fixture.
	if err := AdmitContribution(doc, r, b.fp); err != nil {
		t.Fatalf("setup: B was refused on an intact document (%v)", err)
	}
	broken := corruptSignatureBlob(t, doc)
	// **The stimulus is the STATE, not merely "not valid", and the difference is the finding.**
	// Measured: a body tamper leaves the document `unsigned` with zero signers — the signature
	// vanishes rather than reporting itself broken — and only a corrupted /Contents blob produces
	// `invalid`. A test that accepted any non-valid state would pass against the body tamper,
	// where the gate legitimately cannot tell position 1 of an honest ceremony from a destroyed
	// prefix.
	if st := sign.Verify(broken); st.State != sign.Invalid {
		t.Fatalf("setup: the tampered document reports %q, want invalid — this test is about a "+
			"signature that is PRESENT and unreadable", st.State)
	}
	if ats := ReadAttestations(broken); len(ats) != 0 {
		t.Fatalf("setup: %d attestation(s) survived the tamper, so the gate could see the "+
			"prefix and this fixture is not the blind case", len(ats))
	}
	if err := AdmitContribution(broken, r, b.fp); !errors.Is(err, ErrPrefixUnproven) {
		t.Errorf("a document carrying an unreadable signature was answered %v, want "+
			"ErrPrefixUnproven. ReadAttestations returns NOTHING for it, so an attestation-only "+
			"gate reads it as 'nobody has signed yet' and tells the second party to wait — while "+
			"the document can never become valid however many honest blocks are added.", err)
	}
}

// corruptSignatureBlob mangles the /Contents hex in place: same length, still valid hex, but the
// PKCS#7 SEQUENCE tag is destroyed. The technique is `internal/sign`'s own
// (`TestVerifyUnparseableSignatureIsInvalid`), reused rather than re-invented — a hand-rolled
// tamper that changes the length moves the ByteRange offsets and breaks the file structurally,
// which is a different fixture answering a different question.
func corruptSignatureBlob(t *testing.T, pdf []byte) []byte {
	t.Helper()
	out := append([]byte(nil), pdf...)
	ci := bytes.LastIndex(out, []byte("/Contents"))
	if ci < 0 {
		t.Fatal("the signed document has no /Contents")
	}
	lt := bytes.IndexByte(out[ci:], '<')
	if lt < 0 {
		t.Fatal("no < opening the /Contents hex string")
	}
	flipped := 0
	for j := ci + lt + 1; j < len(out) && flipped < 8; j++ {
		c := out[j]
		if (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F') {
			if c == '0' {
				out[j] = '1'
			} else {
				out[j] = '0'
			}
			flipped++
		}
	}
	if flipped < 8 {
		t.Fatal("could not find hex digits to corrupt in /Contents")
	}
	return out
}

// TestTheCommitmentCheckIsLimitedUntilS04 — the boundary, asserted rather than left as a
// comfortable green.
//
// No production attestation carries a `RosterHash` today: neither `coSignExchange`'s `att` nor
// `internal/server`'s `cosignAttestation` sets one. So the substituted-proceeding refusal above
// fires only on signatures that DO carry a commitment, and on the real path there are none —
// which means L3's proceeding check is, for now, defence against a document some future build
// wrote, not against today's. Making signatures carry it is P07.S04's.
//
// If this test ever fails because a commitment IS present, that is good news and the limit
// recorded here should be revisited — but it must not pass silently.
func TestTheCommitmentCheckIsLimitedUntilS04(t *testing.T) {
	a, b := l3Identity(t, "A"), l3Identity(t, "B")
	r := l3Roster(a, b)
	r.Commitment = strings.Repeat("cd", 32)
	// A document signed the way PRODUCTION signs it — no RosterHash on the attestation.
	doc := l3Chain(t, l3Prepared(t), []l3Party{a}, []l3Party{b}, "")
	ats := ReadAttestations(doc)
	if len(ats) != 1 {
		t.Fatalf("setup: %d signatures", len(ats))
	}
	if ats[0].RosterHash != "" {
		t.Skip("a signature now carries a commitment — P07.S04 has landed and this limit is " +
			"lifted; revisit Roster.Commitment's doc and this test")
	}
	if err := AdmitContribution(doc, r, b.fp); err != nil {
		t.Errorf("the gate refused a document whose signatures carry NO commitment (%v). "+
			"Nothing in production sets one yet, so requiring it would refuse every honest "+
			"ceremony — the same shape P07.S02b caught when CheckDocument would have refused "+
			"every honest hop-1 arrival", err)
	}
}

// TestEveryContributionEntryPointReachesTheGate — ADR-009, asserted on the ROUTING.
//
// The rule is "no contribution out of roster order", and it holds at every site that adds a
// signature block. Asserting that `AdmitContribution` *can* refuse says nothing about whether
// anything calls it — which is the shape ADR-009 was written from, and exactly how this slice's
// gate could ship dead.
//
// **Two packages, and that is the difference from the precedent.**
// `TestL2CoversEveryDocumentCarryingEntryPoint` walks one file. The contribution entry points are
// in two packages — `coSignExchange` here adds the RECEIVING party's block and
// `internal/server`'s `buildCoSigned` adds the INITIATING party's — so a walk that silently read
// only one of them would look exactly like a clean pass. The stimulus assertion below therefore
// checks the population per directory, not in total.
func TestEveryContributionEntryPointReachesTheGate(t *testing.T) {
	dirs := map[string]string{"internal/p2p": ".", "internal/server": filepath.Join("..", "server")}
	// A site may be exempt, and an exemption is a sentence rather than an absence.
	exempt := map[string]string{
		"Contribute": "the primitive itself — it applies a signature and takes no roster; " +
			"gating here would put the rule inside the thing the rule is about",
	}
	perDir := map[string]int{}
	for label, dir := range dirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		for _, e := range entries {
			if !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
				continue
			}
			raw, rerr := os.ReadFile(filepath.Join(dir, e.Name()))
			if rerr != nil {
				t.Fatal(rerr)
			}
			// Comments stripped: a scan satisfied by prose that merely names the call is how
			// `handleSave`'s freeze guard read its own explanation as proof of coverage
			// (v1.117.155). Same lesson, one slice later.
			for _, fn := range l3Funcs(l3StripComments(string(raw))) {
				if !strings.Contains(fn.body, "Contribute(") {
					continue
				}
				if why := exempt[fn.name]; why != "" {
					continue
				}
				perDir[label]++
				if !strings.Contains(fn.body, "AdmitContribution(") {
					t.Errorf("%s/%s: %s adds a signature block and never reaches the L3 gate, so "+
						"a party can contribute out of roster order through it. D23: no "+
						"contribution out of roster order, at every site that makes one.",
						label, e.Name(), fn.name)
				}
			}
		}
	}
	// **The stimulus is PER DIRECTORY.** A total of two is also what "read one package and found
	// both there" looks like, and the second package is the one a p2p-local guard would miss.
	for label := range dirs {
		if perDir[label] == 0 {
			t.Errorf("no contribution entry point was found in %s — the scan matched nothing "+
				"there, so its clean result for that package means nothing", label)
		}
	}
	t.Logf("contribution entry points reaching the gate: %v", perDir)
}

type l3Func struct{ name, body string }

// l3Funcs returns each top-level function's name and brace-matched body.
//
// A third copy of this shape in the tree (`internal/server` and `internal/vault` have the other
// two) and deliberately so: Go cannot share unexported test helpers across packages, and the
// alternative — a test-only exported package — would be a production-visible type existing only
// for tests. Kept behaviourally identical to the others so a reader who knows one knows all three.
func l3Funcs(src string) []l3Func {
	var out []l3Func
	re := regexp.MustCompile(`(?m)^func (?:\([^)]*\) )?(\w+)\(`)
	for _, m := range re.FindAllStringSubmatchIndex(src, -1) {
		name := src[m[2]:m[3]]
		rest := src[m[0]:]
		open := strings.Index(rest, "{")
		if open < 0 {
			continue
		}
		depth := 0
		for j := open; j < len(rest); j++ {
			switch rest[j] {
			case '{':
				depth++
			case '}':
				depth--
				if depth == 0 {
					out = append(out, l3Func{name: name, body: rest[open : j+1]})
					j = len(rest)
				}
			}
		}
	}
	return out
}

func l3StripComments(src string) string {
	var b strings.Builder
	for _, line := range strings.Split(src, "\n") {
		if i := strings.Index(line, "//"); i >= 0 {
			line = line[:i]
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return b.String()
}

// TestTheChannelBindingReadsTheLastSigner — the change conditioning `len(ats) != 1` forced, and
// the one nothing else would have caught.
//
// `coSignExchange`'s two channel bindings ask whether the document was signed by the connected
// peer and whether that signer accepted this user. At N=2 there is one attestation and reading
// index 0 or index last is the same thing — which is why every existing test stays green either
// way. At hop k there are k of them, and `ats[0]` is the party who signed FIRST. With the
// single-prior-signer rule off for ceremonies, reading index 0 would bind the channel to whoever
// signed first and let every later hop past: a peer could hand over a document they had nothing
// to do with, provided the FIRST signer happened to be the pinned identity.
//
// Driven at three signatures, where the two readings genuinely disagree.
func TestTheChannelBindingReadsTheLastSigner(t *testing.T) {
	a, b, c := l3Identity(t, "A"), l3Identity(t, "B"), l3Identity(t, "C")
	me := l3Identity(t, "Me")
	roster := Roster{Entries: []RosterEntry{
		{Fingerprint: a.fp, Signs: true},
		{Fingerprint: b.fp, Signs: true},
		{Fingerprint: c.fp, Signs: true},
		{Fingerprint: me.fp, Signs: true},
	}}
	// A, then B, then C — each accepting the next, and C accepting ME because C is the party
	// handing the document over.
	doc := l3Chain(t, l3Prepared(t), []l3Party{a, b, c}, []l3Party{b, c, me}, "")
	ats := ReadAttestations(doc)
	if len(ats) != 3 {
		t.Fatalf("setup: %d signatures, want 3", len(ats))
	}
	// **The stimulus, and it is the whole test:** the first and last signers must DIFFER, or
	// index 0 and index last are the same attestation and nothing below discriminates.
	if strings.EqualFold(ats[0].Fingerprint, ats[len(ats)-1].Fingerprint) {
		t.Fatal("setup: the first and last signers are the same party, so this fixture cannot " +
			"tell ats[0] from ats[len-1]")
	}

	cFPb, err := hex.DecodeString(c.fp)
	if err != nil {
		t.Fatal(err)
	}
	// C is the connected peer. Reading ats[0] would compare A against C and refuse.
	if _, err := coSignExchange(me.cert, me.key, cFPb, "C", doc,
		l3Confirmer{accept: true, intent: "I agree"}, nil, roster); err != nil {
		t.Fatalf("the document handed over by C, the LAST signer, was refused: %v. The channel "+
			"binding is reading the first attestation, so at every hop past the first it binds "+
			"the connection to the wrong party.", err)
	}

	// And the other direction, which is what stops "read the last one" becoming "read any of
	// them": a document whose last signer is NOT the connected peer is still refused.
	aFPb, _ := hex.DecodeString(a.fp)
	_, err = coSignExchange(me.cert, me.key, aFPb, "A", doc,
		l3Confirmer{accept: true, intent: "I agree"}, nil, roster)
	if err == nil {
		t.Error("a document was accepted from a peer who is not its last signer — the binding " +
			"has become 'any signer on the document', which is no binding at all")
	}
}

// l3Confirmer is a Confirmer that answers immediately, so these tests are about the gate rather
// than about the consent gate one line below it.
type l3Confirmer struct {
	accept bool
	intent string
}

func (c l3Confirmer) Confirm(SignerAttestation, []byte) (bool, string, []byte, error) {
	return c.accept, c.intent, nil, nil
}

// TestTheRelayCeilingAtFourParties — where the N-party relay actually stops, measured, and it is
// not where the plan said.
//
// P07.S03b's driver clause assumes the relay completes at N=4 once `len(ats) != 1` is conditioned,
// and notes that "through `Initiate` every intermediate party signs twice, so the count is
// 2(N−1)". **Those two cannot both be true under L3.** A signature twice from the same party is a
// prefix that is not the roster's signing order, and refusing that is the whole of D23.
//
// What this drives is the real hop sequence and prints where it stops:
//
//   - hop 1: the convener signs, A co-signs. Two signatures, exactly the roster prefix.
//   - hop 2: `/api/session/initiate` applies the LOCAL signature before it sends
//     (`buildCoSigned`), so the carrier would sign again — and L3 refuses it, by name, at the
//     near end. **This is the gate.**
//   - and B, handed the document unchanged, IS admitted.
//
// So the model already supports the relay; what does not exist is a route that hands the baton on
// **without contributing**. That is P07.S05's carry verb, and this test is the measurement that
// says so — it will start failing at its last assertion the day S05 lands, which is the right
// moment to delete it.
func TestTheRelayCeilingAtFourParties(t *testing.T) {
	conv, a, b := l3Identity(t, "Convener"), l3Identity(t, "A"), l3Identity(t, "B")
	c := l3Identity(t, "C")
	roster := Roster{Entries: []RosterEntry{
		{Fingerprint: conv.fp, Signs: true},
		{Fingerprint: a.fp, Signs: true},
		{Fingerprint: b.fp, Signs: true},
		{Fingerprint: c.fp, Signs: true},
	}}

	// Hop 1, in full: the convener contributes, then A does.
	doc := l3Chain(t, l3Prepared(t), []l3Party{conv}, []l3Party{a}, "")
	if err := AdmitContribution(doc, roster, a.fp); err != nil {
		t.Fatalf("hop 1: A was refused on an honest first hop: %v", err)
	}
	doc = l3Chain(t, doc, []l3Party{a}, []l3Party{conv}, "")
	if n := len(ReadAttestations(doc)); n != 2 {
		t.Fatalf("setup: hop 1 left %d signatures, want 2", n)
	}

	// Hop 2, as `/api/session/initiate` would do it: the carrier signs before sending.
	err := AdmitContribution(doc, roster, conv.fp)
	if err == nil {
		t.Fatal("the carrier was admitted to sign a SECOND time. Two signatures from one party " +
			"are not the roster's signing order, and if this is now allowed then L3's prefix " +
			"rule has stopped being the rule D23 describes.")
	}
	if !errors.Is(err, ErrNotYourTurn) {
		t.Errorf("the carrier's second signature was refused with %v, want ErrNotYourTurn — the "+
			"reason matters, because it is what points at S05's carry route rather than at a "+
			"broken document", err)
	}

	// And the other half, which is what makes the ceiling a ROUTE problem and not a model one:
	// B, handed the document unchanged, is admitted.
	if err := AdmitContribution(doc, roster, b.fp); err != nil {
		t.Errorf("B was refused a document carrying exactly the roster prefix before them (%v). "+
			"If this fails, the relay is blocked by the MODEL and not merely by the absence of a "+
			"carry route — which would move the problem from P07.S05 to here.", err)
	}
}
