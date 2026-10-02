package sign

import (
	"bytes"
	"cmp"
	"crypto"
	_ "crypto/sha1"
	_ "crypto/sha256"
	_ "crypto/sha512"
	"crypto/x509"
	"encoding/asn1"
	"encoding/hex"
	"regexp"
	"slices"
	"sort"

	"github.com/digitorus/pkcs7"
)

// RevisionCause names why the version a signer signed could not be handed back (D7, as amended by the P02
// plan-review). Each is a different sentence to the user, so they are never lumped.
type RevisionCause string

const (
	// RevisionNoSignature: nothing in the file AS IT NOW STANDS is a signature — a document timestamp is not one
	// (ADR-060), so a timestamp-only document lands here too, and the sentence must not say "nothing signed this".
	// An earlier revision may still hold someone else's signature whose dictionary a later revision replaced (only the
	// asking signer's own earlier revisions are walked), so P03 words it as the file as it stands, never as "unsigned".
	RevisionNoSignature RevisionCause = "no-signature"
	// RevisionResaved: a signature NAMING the requested certificate is in the file and does not verify against it. The
	// name is the SignerInfo's own claim, which anyone can write (the certificate rides in every document its owner
	// signed), so this is never "your signature" unless `Attributed` — and even then it says only "made with that key
	// over other bytes", which a re-save and a transplant both produce (plan-review C1, W10).
	RevisionResaved RevisionCause = "resaved"
	// RevisionNotYours: the file holds signatures, and none of them names the requested certificate.
	RevisionNotYours RevisionCause = "not-your-signature"
	// RevisionPrefixFailed: a candidate version was cut out of the file and did not stand up on its own (D4) — it did
	// not parse, or the signature did not re-verify across exactly its bytes.
	RevisionPrefixFailed RevisionCause = "prefix-failed-reverify"
	// RevisionCouldNotCheck: the whole file could not be checked and no candidate re-verified. **Parked for Dan as an
	// amendment to D7** (plan-review C2): none of the four causes is true of a file nib could not read, and borrowing
	// one would state something nib never observed.
	RevisionCouldNotCheck RevisionCause = "could-not-check"
)

// maxRevisionCandidates bounds how many prefixes `SignedRevisionFor` re-verifies, each a full `Revisions`.
const maxRevisionCandidates = 16

// maxCandidateRecords bounds how many naming records are considered at all; each costs at most one signature check
// over its signed attributes (~0.1 ms) before anything is hashed.
const maxCandidateRecords = 1024

// screenBudgetFactor bounds the bytes hashed screening candidates when the whole file did not verify, as a multiple
// of the file's length. **Measured 2026-10-02**: without a screen, 40 appended copies of the signer's own blob
// claiming ends past the real one filled every verify slot ahead of it — `could-not-check` for a version that was in
// the file, after 13 s at 40 MB; with it, 1.4 s and the version. Hashing is ~3% of a verify per file length, so the
// budget is under one verify. **Declared residual**: copies an attacker makes WELL-FORMED (each its own dictionary
// with its own gap holding the signer's blob) sort with the genuine record, and enough of them covering the whole file
// exhaust the budget first — the answer is then `could-not-check`, which is honest, never `not-your-signature`.
const screenBudgetFactor = 16

// maxBoundaryScreens bounds how many earlier revisions the boundary walk reads with pdfcpu (P02.S02) — only those a
// `prescreen` of their own bytes passed, so in practice the signer's own signed revisions.
const maxBoundaryScreens = 16

// maxBoundaryPrescreens bounds how many boundaries the walk prescreens. A prescreen parses one signature blob from the
// bytes and hashes at most the file once per candidate (charged to the budget) — measured 53 ms for 20 fake xref
// sections in a 100 MB file, where pdfcpu-reading each had cost 2 min 6 s — so this bound can be generous: crowding a
// version out takes hundreds of fake sections, not seventeen. **That held only once a SignerInfo with no signed
// attributes was checked against its key** (P02 phase close): a forged one admitted on "it encapsulates content" alone
// passed every prescreen, and seventeen fakes cut the walk at 16 pdfcpu reads — `could-not-check` for a version in the
// file, 1.3 s at 23 MB. Checked, the same seventeen cost no pdfcpu read and the version comes back.
const maxBoundaryPrescreens = 256

// maxEncapsulatedContent is the most encapsulated content a `signerProof` keeps a copy of. The library verifies a
// SignerInfo over the encapsulated content FOLLOWED BY the bytes the ranges select (pdfsign `verify/signature.go`
// appends them), so a SignerInfo with no signed attributes cannot be screened without it. The `adbe.pkcs7.sha1` shape
// carries a 20-byte digest; a SignerInfo encapsulating more than this is not screened and so never admitted — declared:
// one the library would verify is then lost to the screened paths, and none exists on disk.
const maxEncapsulatedContent = 1024

// maxXrefLeadingSpace is how much white space a `startxref` offset may point before its section: pdfsign's points one
// byte early, at the end-of-line before the header.
const maxXrefLeadingSpace = 4

// SignedRevision is what the file says about one signer's signed version: the prefix, or why there is none.
type SignedRevision struct {
	// Prefix is the signed version — a slice of the input, capped at its own length so an append cannot write into
	// the caller's file. Nil on a refusal.
	Prefix []byte `json:"-"`
	// Cause is empty when Prefix is set.
	Cause RevisionCause `json:"cause,omitempty"`
	// Obj is the object number of the signature that re-verified inside the prefix, and End the prefix's length.
	Obj uint32 `json:"obj,omitempty"`
	End int64  `json:"end,omitempty"`
	// RedefinedObj is set when the version was found only through a record the file's latest revision refused — a
	// later revision redefined the signature's dictionary (plan-review W8). That is evidence, and P03 says so.
	RedefinedObj uint32 `json:"redefinedObj,omitempty"`
	// Later names records naming this certificate that reach past End or out of the file: a later signature of the
	// same name that did not hold (W3). Their names are unverified claims.
	Later []uint32 `json:"later,omitempty"`
	// Earlier is the coverage end of each earlier verified signature by the same certificate (W3).
	Earlier []int64 `json:"earlier,omitempty"`
	// Refused is every refused record in the file, on a refusal (I6).
	Refused []RefusedSignature `json:"refused,omitempty"`
	// Attributed, on `resaved` only: the SignerInfo's signature over its signed attributes checks against the named
	// certificate's key (C1). False means the name is only a claim.
	Attributed bool `json:"attributed,omitempty"`
	// EarlierRevision: the version was found by walking the file's earlier revision boundaries (P02.S02): no record of
	// the file as nib could read it reached that version — a later revision rewrote or replaced the signature's
	// dictionary (then `RedefinedObj` names it), or the file as a whole could not be read.
	EarlierRevision bool `json:"earlierRevision,omitempty"`
	// LaterUnchecked: a version was found, and the search for a LATER one by the same signer was stopped by a bound
	// before every earlier revision was looked at (P02.S02's re-review: the signer's certificate is public, so anyone
	// can copy a dictionary naming it and spend the walk's screens ahead of the signer's last version) — or before every
	// record naming the signer was considered (`maxCandidateRecords`). The version returned is genuine; whether it is
	// the signer's last is not established, and the surface must say so.
	LaterUnchecked bool `json:"laterUnchecked,omitempty"`
}

// SignedRevisionFor returns the version of pdf that the signer with this certificate fingerprint signed (D1), or the
// reason there is none. A version is returned only if it stands up on its own: cut out of the file, it parses and that
// signer's signature re-verifies across exactly its bytes (D4). An identity the file merely NAMES chooses which
// versions to try and never, by itself, what nib says (plan-review C1).
//
// **One verify of the whole file, then one per candidate** (`maxRevisionCandidates`). Candidates come from the
// records `verifyIndexed` returns — beside its error, where it has one — and never from a second sweep, which would put
// the digitorus reader on a file the pdfcpu gate refused (ADR-041).
func SignedRevisionFor(pdf []byte, fingerprint string) SignedRevision {
	_, revs, err := verifyIndexed(pdf)
	cands, capped := revisionCandidates(pdf, revs, err, fingerprint)
	// A capped candidate list left records naming the signer unconsidered, any of which could end later — at the file's
	// own end, which the walk never looks at — so nothing found here or by the walk is known to be the last.
	failed, cut := false, capped
	verifies := 0
	budget := int64(screenBudgetFactor) * int64(len(pdf))
	tried := map[int64]bool{} // ends actually re-verified — a candidate screened out was never tried (P02.S02 review C3)
	var best *SignedRevision
	for _, c := range cands {
		// A candidate the library verified over a file that verified is exact. Any other is only a NAME, and one is
		// re-verified only if it could hold: its SignerInfo checks against the named key, and the bytes its ranges
		// select hash to the digest that signature covers. A copy of the signer's own blob with other ranges cannot
		// pass. What remains is the budget (`screenBudgetFactor`'s declared residual): enough well-formed copies spend
		// it, and the search then ends as `could-not-check` — as it does at the verify cap.
		if !c.checked && !c.screen(pdf, &budget) {
			if budget < 0 {
				cut = true
				break
			}
			continue
		}
		if verifies == maxRevisionCandidates {
			cut = true
			break
		}
		verifies++
		tried[c.end] = true
		prefix := pdf[:c.end:c.end]
		prs, perr := revs, err
		if c.end != int64(len(pdf)) || err != nil {
			prs, perr = Revisions(prefix)
		}
		if perr != nil {
			failed = true
			continue
		}
		held, ok := holder(prs, fingerprint, c.end)
		if !ok {
			failed = true
			continue
		}
		best = &SignedRevision{Prefix: prefix, Obj: held.Obj, End: c.end}
		// The record that re-verified names the object, never the first proposer of its end in xref order — which can be
		// an appended copy — and only where the file as it stands holds no well-formed record of it with these ranges.
		if c.onlyRefused && redefinedIn(revs, held) {
			best.RedefinedObj = held.Obj
		}
		for _, r := range revs {
			if r.Fingerprint == fingerprint && r.countsAsSigner() && r.CoverageEnd < c.end {
				best.Earlier = append(best.Earlier, r.CoverageEnd)
			}
		}
		break
	}
	// The walk (P02.S02) looks at the revisions before the file as it stands: for any version when no record reached
	// one, and — when one did — for a LATER version (W3: "since I signed" means the signer's last signature), which a
	// later revision can hide by replacing that signature's dictionary while the earlier one still verifies. It
	// shares the verify cap and the hashing budget, so nothing a hostile file can write buys more than one bound.
	// Measured cost on an HONEST document: asking about signer 1 of N screens the N-2 later boundaries a ByteRange ends
	// at, ~3.5 ms each at 50 KB, at most `maxBoundaryScreens` — results unchanged.
	var after int64
	if best != nil {
		after = best.End
	}
	w := walkBoundaries(pdf, fingerprint, tried, after, &verifies, &budget)
	// A walk prefix that passed every screen and then did not re-verify is the main loop's `failed` (I3), not a
	// statement about the signer: without it the refusal read `not-your-signature` or `no-signature` about a signer
	// whose own key and digest nib had just checked.
	failed = failed || w.failed
	if found := w.found; found != nil {
		found.Later = laterNaming(pdf, revs, fingerprint, found.End)
		found.LaterUnchecked = capped
		// W8 is a COMPARISON with the file as it stands, never an inference from the walk having been needed: the
		// candidate cap can drop the very record that still holds the object unchanged. Where the whole file was not
		// read, nothing can be said about what the latest revision holds.
		if err == nil && redefinedIn(revs, w.held) {
			found.RedefinedObj = found.Obj
		}
		return *found
	}
	if best != nil {
		best.Later = laterNaming(pdf, revs, fingerprint, best.End)
		best.LaterUnchecked = w.cut || capped
		return *best
	}
	out := SignedRevision{Refused: refusedOf(revs)}
	switch {
	case err != nil, cut, w.cut:
		out.Cause = RevisionCouldNotCheck
	case failed:
		out.Cause = RevisionPrefixFailed
	default:
		out.Cause = RevisionNoSignature
		for _, r := range revs {
			if fingerprint != "" && r.named == fingerprint && !r.Timestamp {
				out.Cause = RevisionResaved
				if !out.Attributed && r.proof.attributed() { // a key verify each; one that checks settles it
					out.Attributed = true
				}
				continue
			}
			if out.Cause != RevisionResaved && (r.countsAsSigner() || (r.Cause != "" && r.hasContents)) {
				out.Cause = RevisionNotYours
			}
		}
	}
	return out
}

// walkResult is what the boundary walk saw. found is the signer's version and held the record that re-verified inside
// it; cut is true when a bound stopped the walk with boundaries untried (the answer is then `could-not-check`, never a
// cause that claims nib looked everywhere); failed is true when a prefix passed every screen and then did not
// re-verify; screens is how many prefixes it read with pdfcpu, for the bound tests.
type walkResult struct {
	found   *SignedRevision
	held    Revision
	cut     bool
	failed  bool
	screens int
}

// walkBoundaries tries each earlier revision of pdf after `after`, newest first, for the signer's version. A revision
// boundary is a `%%EOF` the file's own cross-reference structure ends at (`revisionBoundaries`), and only one a literal
// ByteRange ends at is worth a look (`rawByteRangesEndingAt`) — every other costs nothing. Each is SCREENED behind
// pdfcpu's read (`boundaryCandidate`, ADR-041), then re-verified like any candidate.
//
// **Both whole-file scans are charged to the budget, and the boundaries are found FIRST** (P02's phase-close review):
// they materialised every literal ByteRange and every marker uncharged, measured at 1.7 GB allocated, ~907 MB peak, for
// 64 MB of `/ByteRange[0 1 1 1]` — ~14× the file, on every request. The boundary scan keeps one end per offset, and the
// literal scan keeps only literals ending at a boundary still to try, as an end and an offset, re-reading the numbers
// when one is prescreened; a file with no such boundary skips it.
func walkBoundaries(pdf []byte, fingerprint string, tried map[int64]bool, after int64, verifies *int, budget *int64) (w walkResult) {
	if fingerprint == "" {
		return w
	}
	bounds, ok := revisionBoundaries(pdf, budget)
	if !ok {
		w.cut = true
		return w
	}
	keep := bounds[:0]
	for _, b := range bounds {
		if b > after && b != int64(len(pdf)) && !tried[b] {
			keep = append(keep, b)
		}
	}
	if len(keep) == 0 {
		return w
	}
	lits, ok := rawByteRangesEndingAt(pdf, keep, budget)
	if !ok {
		w.cut = true
		return w
	}
	prescreens := 0
	for k := 0; k < len(lits); {
		b, n := lits[k].end, k
		for n < len(lits) && lits[n].end == b {
			n++
		}
		run := lits[k:n]
		k = n
		if prescreens == maxBoundaryPrescreens || w.screens == maxBoundaryScreens || *verifies == maxRevisionCandidates || *budget < 0 {
			w.cut = true
			return w
		}
		prescreens++
		if !prescreen(pdf, run, fingerprint, budget) {
			if *budget < 0 {
				w.cut = true // the budget is spent with boundaries untried
				return w
			}
			continue
		}
		w.screens++ // only a boundary whose own bytes say it could be the signer's costs a pdfcpu read
		prefix := pdf[:b:b]
		c := boundaryCandidate(prefix, fingerprint)
		if c == nil {
			continue
		}
		if !c.screen(prefix, budget) {
			if *budget < 0 {
				w.cut = true // the budget is spent with boundaries untried
				return w
			}
			continue
		}
		*verifies++
		prs, err := Revisions(prefix)
		if err != nil {
			w.failed = true
			continue
		}
		held, ok := holder(prs, fingerprint, b)
		if !ok {
			w.failed = true
			continue
		}
		out := &SignedRevision{Prefix: prefix, Obj: held.Obj, End: b, EarlierRevision: true}
		for _, r := range prs {
			if r.Fingerprint == fingerprint && r.countsAsSigner() && r.CoverageEnd < b {
				out.Earlier = append(out.Earlier, r.CoverageEnd)
			}
		}
		w.found, w.held = out, held
		return w
	}
	return w
}

// boundaryCandidate reads a prefix the way the verify door does — pdfcpu first, then the sweep (ADR-041; the guard
// `TestEverySweepRunsBehindThePdfcpuGate` reads this function) — and proposes it if a record naming the signer ends
// exactly there. Nil when pdfcpu or the sweep refuses the prefix, or nothing names the signer at its end.
func boundaryCandidate(prefix []byte, fingerprint string) *candidate {
	if _, err := pdfcpuRead(prefix); err != nil {
		return nil
	}
	revs, err := sweepRevisions(prefix)
	if err != nil {
		return nil
	}
	c := &candidate{end: int64(len(prefix))}
	for i := range revs {
		r := &revs[i]
		if r.named != fingerprint || r.Timestamp {
			continue
		}
		if e, ok := lastPairEnd(r.ByteRange, len(prefix)); ok && e == c.end {
			c.proposers = append(c.proposers, r)
			c.obj = r.Obj
		}
	}
	if len(c.proposers) == 0 {
		return nil
	}
	return c
}

var (
	byteRangeName = []byte("/ByteRange")
	reObjHeader   = regexp.MustCompile(`^\d+\s+\d+\s+obj`)
)

// rawByteRange is one literal ByteRange: where it begins in the file, and where its last pair ends. Its numbers are
// re-read from the file when it is prescreened (`parseByteRange`), so a hostile file's literals cost 16 bytes each to
// keep — and only those ending at a boundary are kept at all.
type rawByteRange struct {
	end int64
	at  int
}

// rawByteRangesEndingAt is every literal `/ByteRange [ints]` in pdf whose last pair ends at one of bounds (newest first,
// strictly descending, as `revisionBoundaries` gives them), ordered by that end newest first and by position within it.
// A signature that can verify has its ByteRange in literal bytes (an indirect one is refused by the sweep), so a
// boundary no literal ends at holds no version `holder` could accept, and is never screened. Every byte scanned is
// charged to budget; false when it runs out.
func rawByteRangesEndingAt(pdf []byte, bounds []int64, budget *int64) ([]rawByteRange, bool) {
	*budget -= int64(len(pdf)) // the search for the name, once across the file
	if *budget < 0 {
		return nil, false
	}
	var out []rawByteRange
	var buf []int64
	for i := 0; ; {
		j := bytes.Index(pdf[i:], byteRangeName)
		if j < 0 {
			break
		}
		at := i + j
		br, next, ok := parseByteRange(pdf, at, buf[:0])
		*budget -= int64(next - at)
		if *budget < 0 {
			return nil, false
		}
		i = next
		if !ok {
			continue
		}
		buf = br
		e, ok := lastPairEnd(br, len(pdf))
		if !ok {
			continue
		}
		if _, hit := slices.BinarySearchFunc(bounds, e, func(b, t int64) int { return cmp.Compare(t, b) }); hit {
			out = append(out, rawByteRange{end: e, at: at})
		}
	}
	slices.SortStableFunc(out, func(a, b rawByteRange) int { return cmp.Compare(b.end, a.end) })
	return out, true
}

// parseByteRange reads the literal ByteRange beginning at pdf[at] (which holds `/ByteRange`) into dst: the name, PDF
// white space, `[`, then only digits, signs and white space up to `]` — the shape `/ByteRange\s*\[([0-9\s+\-]*)\]`
// names, read by hand so a scan over every literal allocates nothing — and every field a whole base-10 integer, as
// `strconv.ParseInt` takes it. next is where a scan resumes: past the `]` of a literal, else past the name.
func parseByteRange(pdf []byte, at int, dst []int64) (br []int64, next int, ok bool) {
	i := at + len(byteRangeName)
	next = i
	for i < len(pdf) && isRegexpSpace(pdf[i]) {
		i++
	}
	if i >= len(pdf) || pdf[i] != '[' {
		return nil, next, false
	}
	i++
	start := i
	for i < len(pdf) && (isRegexpSpace(pdf[i]) || pdf[i] >= '0' && pdf[i] <= '9' || pdf[i] == '+' || pdf[i] == '-') {
		i++
	}
	if i >= len(pdf) || pdf[i] != ']' {
		return nil, next, false
	}
	next = i + 1
	br = dst
	for f := start; f < i; {
		for f < i && isRegexpSpace(pdf[f]) {
			f++
		}
		g := f
		for g < i && !isRegexpSpace(pdf[g]) {
			g++
		}
		if g == f {
			break
		}
		v, ok := parseInt64(pdf[f:g])
		if !ok {
			return nil, next, false
		}
		br = append(br, v)
		f = g
	}
	return br, next, true
}

// isRegexpSpace is RE2's `\s`, the white space `parseByteRange` admits.
func isRegexpSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\f' || c == '\r'
}

// parseInt64 is `strconv.ParseInt(string(tok), 10, 64)` without the string: an optional sign, then decimal digits, in
// range.
func parseInt64(tok []byte) (int64, bool) {
	neg := false
	if len(tok) > 0 && (tok[0] == '+' || tok[0] == '-') {
		neg, tok = tok[0] == '-', tok[1:]
	}
	if len(tok) == 0 {
		return 0, false
	}
	limit := uint64(1<<63 - 1)
	if neg {
		limit++
	}
	var u uint64
	for _, c := range tok {
		if c < '0' || c > '9' {
			return 0, false
		}
		d := uint64(c - '0')
		if u > (limit-d)/10 {
			return 0, false
		}
		u = u*10 + d
	}
	if neg {
		return -int64(u-1) - 1, true
	}
	return int64(u), true
}

var reRawContents = regexp.MustCompile(`/Contents\s*<([0-9A-Fa-f\s]*)>`)

// prescreen reports whether a literal ByteRange ending at a boundary could be the signer's, from the bytes around it
// alone and BEFORE pdfcpu reads the prefix: its object's literal `/Contents` must parse as a SignerInfo naming the
// signer, and that SignerInfo must pass `couldHold` over the literal's ranges — the screen the main loop's candidates
// pass. A genuine holder passes by construction — the sweep refuses an indirect `/Contents` (conjunct 9) and a
// signature inside an object stream (conjunct 10), so its blob is literal bytes in its own object. Measured (P02.S03):
// without it, 20 fake xref sections in a 100 MB file cost 2 min 6 s, every one a full pdfcpu read of a prefix it then
// refused.
func prescreen(pdf []byte, lits []rawByteRange, fingerprint string, budget *int64) bool {
	// EVERY byte this looks at is charged to the budget, the scans as well as the hashing (P02.S03's review: 64,000
	// literals in one 1.4 MB object cost 3 min uncharged — each one scanned back to its header and forward to `endobj`).
	// With the charge the work is linear in the file, and a document that exhausts it reads `could-not-check`.
	seen := map[int]bool{}
	for _, l := range lits {
		if *budget < 0 {
			return false
		}
		br, next, ok := parseByteRange(pdf, l.at, nil)
		*budget -= int64(next - l.at) // the numbers, re-read
		if !ok {
			continue
		}
		start := bytes.LastIndex(pdf[:l.at], []byte(" obj"))
		if start < 0 {
			*budget -= int64(l.at)
			continue
		}
		*budget -= int64(l.at - start)
		if seen[start] {
			continue // one object, examined once
		}
		seen[start] = true
		end := bytes.Index(pdf[l.at:], []byte("endobj"))
		if end < 0 {
			*budget -= int64(len(pdf) - l.at)
			continue
		}
		span := pdf[start : l.at+end]
		*budget -= int64(end) + int64(len(span)) // the forward scan, then the /Contents search over the object
		if *budget < 0 {
			return false
		}
		m := reRawContents.FindSubmatch(span)
		if m == nil {
			continue
		}
		raw := make([]byte, 0, len(m[1]))
		for _, f := range bytes.Fields(m[1]) {
			raw = append(raw, f...)
		}
		blob := make([]byte, len(raw)/2)
		if _, err := hex.Decode(blob, raw[:len(blob)*2]); err != nil {
			continue
		}
		// Untrimmed, as the sweep parses it: a signature value can end in 0x00, and trimming the padding cut one in
		// 256 genuine signatures (P02.S03's review).
		p7, err := pkcs7.Parse(blob)
		if err != nil {
			continue
		}
		pr := proofOf(p7)
		if pr == nil || hex.EncodeToString(fingerprintOf(pr.cert)) != fingerprint {
			continue
		}
		if pr.couldHold(pdf, br, budget) {
			return true
		}
		if *budget < 0 {
			return false
		}
	}
	return false
}

// revisionBoundaries is every place pdfcpu-shaped bytes say a revision ends, newest first: a `%%EOF` whose `startxref`
// (within 64 bytes before it) names an offset that — after PDF white space, since pdfsign's points one byte early —
// begins `xref` or an object whose dictionary says `/XRef`. Each marker offers its end bare and through `\r`, `\n` or
// `\r\n`. Markers naming one offset collapse to the EARLIEST (P02.S02's grill: 5,000 appended markers pointing at the
// genuine xref otherwise queue ahead of it), and a linearized file's first-page `startxref 0` names nothing. The ends
// are strictly descending. Every byte looked at is charged to budget; false when it runs out.
//
// It keeps one int64 per end and one set entry per offset (P02's phase-close review: a struct and a slice per marker
// measured ~573 MB peak for 64 MB of distinct xref markers).
func revisionBoundaries(pdf []byte, budget *int64) ([]int64, bool) {
	*budget -= int64(len(pdf)) // the search for the marker, once across the file
	if *budget < 0 {
		return nil, false
	}
	var out []int64 // oldest first, reversed at the end
	seen := map[int64]struct{}{}
	for i := 0; ; {
		j := bytes.Index(pdf[i:], []byte("%%EOF"))
		if j < 0 {
			break
		}
		p := i + j
		i = p + 5
		lo := max(p-64, 0)
		*budget -= int64(p - lo)
		if *budget < 0 {
			return nil, false
		}
		sx := bytes.LastIndex(pdf[lo:p], []byte("startxref"))
		if sx < 0 {
			continue
		}
		off, ok := parseInt64(bytes.TrimSpace(pdf[lo+sx+9 : p]))
		if !ok || off <= 0 || off >= int64(p) {
			continue
		}
		if _, dup := seen[off]; dup {
			continue
		}
		// A BOUNDED look at the offset, so each marker costs a constant (P02.S02's review: an unbounded white-space skip,
		// re-walked for every marker naming one long run, measured 33.8 s for 2,000 markers over 4 MiB, before any cap).
		at := pdf[off:p]
		for n := 0; n < maxXrefLeadingSpace && len(at) > 0 && isPDFSpace(at[0]); n++ {
			at = at[1:]
		}
		look := int64(maxXrefLeadingSpace + 32)
		isXref := bytes.HasPrefix(at, []byte("xref"))
		if !isXref && reObjHeader.Match(at[:min(len(at), 32)]) {
			w := at[:min(len(at), 1024)]
			look += 2 * int64(len(w)) // the search for `stream`, then for `/XRef`
			if k := bytes.Index(w, []byte("stream")); k >= 0 {
				w = w[:k]
			}
			isXref = bytes.Contains(w, []byte("/XRef"))
		}
		*budget -= look
		if *budget < 0 {
			return nil, false
		}
		if !isXref {
			continue
		}
		seen[off] = struct{}{}
		e := int64(p + 5)
		out = append(out, e)
		if e < int64(len(pdf)) && (pdf[e] == '\r' || pdf[e] == '\n') {
			out = append(out, e+1)
			if pdf[e] == '\r' && e+1 < int64(len(pdf)) && pdf[e+1] == '\n' {
				out = append(out, e+2)
			}
		}
	}
	slices.Reverse(out)
	return out, true
}

// candidate is one version worth re-verifying: where a record naming the signer says its signature ends.
type candidate struct {
	end int64
	// obj is a record that proposed this end; onlyRefused is true when every record proposing it was refused.
	obj         uint32
	onlyRefused bool
	// proposers are the records proposing this end, for the screen.
	proposers []*Revision
	// checked is true when a proposer was verified by the library over a file that verified: the screen is then the
	// library's own, and exact.
	checked bool
}

// screen reports whether any record proposing this end could hold (`couldHold`). Once a record would overrun the
// budget, it is spent (-1) and nothing is hashed again — every later candidate is refused here, which ends the search
// as `could-not-check`.
func (c candidate) screen(pdf []byte, budget *int64) bool {
	for _, r := range c.proposers {
		if r.proof.couldHold(pdf, r.ByteRange, budget) {
			return true
		}
		if *budget < 0 {
			return false
		}
	}
	return false
}

// couldHold is THE screen (ADR-009): whether this SignerInfo could be one the library verifies over the bytes br
// selects. Its signature must check against the certificate it names — over its signed attributes, whose
// messageDigest the ranges' bytes must then hash to; or, with none, over the encapsulated content followed by those
// bytes, which is what the library verifies (pdfsign `verify/signature.go` appends the ranges to `p7.Content`). The
// main loop's candidates and the walk's prescreen both call it; they had each carried their own copy, and both copies
// admitted a SignerInfo with no signed attributes on "it encapsulates content" alone — so anyone holding the signer's
// PUBLIC certificate could sign with their own key and pass (P02's phase-close review). The bytes are charged to budget
// BEFORE they are hashed; a record that would overrun it spends it (-1) and is refused.
func (pr *signerProof) couldHold(pdf []byte, br []int64, budget *int64) bool {
	if pr == nil {
		return false
	}
	bare := pr.signedAttrs == nil // signs the content itself, so the check IS the signature over the bytes
	if !bare && !pr.attributed() {
		return false
	}
	cost, ok := rangeCost(br, len(pdf))
	if !ok {
		return false
	}
	if bare {
		if pr.encapsulated && pr.content == nil {
			return false // more than `maxEncapsulatedContent` was encapsulated, so nothing was kept to check it over
		}
		cost = satAdd(cost, int64(len(pr.content)))
	}
	if cost > *budget {
		*budget = -1
		return false
	}
	*budget -= cost
	if bare {
		return pr.checks(selected(pr.content, pdf, br))
	}
	return pr.digestMatches(pdf, br)
}

// revisionCandidates is the seam P02.S02 extends with the file's earlier revision boundaries.
//
// A candidate is the last-pair end of a record whose SignerInfo names the fingerprint and which is not a document
// timestamp (a timestamp authority's certificate is never "the version you signed"). While the whole file verified,
// only a record the library VERIFIED is a candidate — the prefix hashes the same bytes, so that screen is exact and
// free. When it did not, every naming record is one: the error says nothing about which record was at fault (one
// appended negative-length copy fails the whole file while the signer's own version is intact — C2). Refused records
// are candidates on purpose: a later revision redefining the signer's dictionary leaves the original ByteRange on the
// refused record, and its prefix is the signer's version. Ends proposed by a well-formed record come first, then the
// rest, each newest first.
//
// capped is true when `maxCandidateRecords` stopped it with naming records unconsidered: nothing it returns is then the
// whole set, and the caller must not reason as if it were.
func revisionCandidates(pdf []byte, revs []Revision, err error, fingerprint string) (out []candidate, capped bool) {
	if fingerprint == "" {
		return nil, false
	}
	at := map[int64]int{}
	considered := 0
	for i := range revs {
		r := &revs[i]
		// A record the library checked and FAILED is no candidate while the whole file verified: the prefix would hash
		// the same bytes. A record the library never enumerated (`libPos < 0` — e.g. a later revision dropped
		// `/SigFlags`) was not checked at all, so it is screened like any name on the error path.
		if r.named != fingerprint || r.Timestamp || (err == nil && r.libPos >= 0 && !r.Verified) {
			continue
		}
		end, ok := lastPairEnd(r.ByteRange, len(pdf))
		if !ok {
			continue
		}
		if considered == maxCandidateRecords {
			capped = true
			break
		}
		considered++
		checked := err == nil && r.Verified
		if j, seen := at[end]; seen {
			if r.Cause == "" {
				out[j].onlyRefused = false
			}
			out[j].checked = out[j].checked || checked
			out[j].proposers = append(out[j].proposers, r)
			continue
		}
		at[end] = len(out)
		out = append(out, candidate{end: end, obj: r.Obj, onlyRefused: r.Cause != "", checked: checked, proposers: []*Revision{r}})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].onlyRefused != out[j].onlyRefused {
			return !out[i].onlyRefused
		}
		return out[i].end > out[j].end
	})
	return out, capped
}

// lastPairEnd is where a raw ByteRange's last pair ends (D5), refused for anything a slice could not take.
func lastPairEnd(br []int64, size int) (int64, bool) {
	if len(br) < 2 || len(br)%2 != 0 {
		return 0, false
	}
	a, b := br[len(br)-2], br[len(br)-1]
	if a < 0 || b < 0 {
		return 0, false
	}
	end := satAdd(a, b)
	return end, end > 0 && end <= int64(size)
}

// holder is the record inside a cut-out prefix that proves it: a verified, well-formed signer with this fingerprint
// whose coverage is the whole prefix (I2 — the check that can fail; the length matches by construction of the cut).
func holder(revs []Revision, fingerprint string, end int64) (Revision, bool) {
	for _, r := range revs {
		if r.Fingerprint == fingerprint && r.Verified && r.countsAsSigner() && r.CoverageEnd == end {
			return r, true
		}
	}
	return Revision{}, false
}

// redefinedIn reports whether the file as it stands no longer holds the record that re-verified in a prefix: no
// well-formed record of that object number carries its ranges (plan-review W8). It is read from the whole file's
// records, never inferred from which candidates were tried — the candidate cap can drop the record that would say the
// object is unchanged (P02's phase-close review).
func redefinedIn(revs []Revision, held Revision) bool {
	for _, r := range revs {
		if r.Obj == held.Obj && r.Cause == "" && slices.Equal(r.ByteRange, held.ByteRange) {
			return false
		}
	}
	return true
}

// laterNaming lists records naming the fingerprint whose signature claims to reach past end, or out of the file.
func laterNaming(pdf []byte, revs []Revision, fingerprint string, end int64) []uint32 {
	var out []uint32
	for _, r := range revs {
		if r.named != fingerprint || r.Timestamp {
			continue
		}
		if e, ok := lastPairEnd(r.ByteRange, len(pdf)); !ok || e > end {
			out = append(out, r.Obj)
		}
	}
	return out
}

// signedAttribute mirrors pkcs7's attribute so the SET can be re-marshalled as it was signed.
type signedAttribute struct {
	Type  asn1.ObjectIdentifier
	Value asn1.RawValue `asn1:"set"`
}

var (
	oidDigestSHA1    = asn1.ObjectIdentifier{1, 3, 14, 3, 2, 26}
	oidDigestSHA256  = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 1}
	oidDigestSHA384  = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 2}
	oidDigestSHA512  = asn1.ObjectIdentifier{2, 16, 840, 1, 101, 3, 4, 2, 3}
	oidMessageDigest = asn1.ObjectIdentifier{1, 2, 840, 113549, 1, 9, 4}
)

// hashOf is the hash a SignerInfo's digest algorithm names — the one the library itself uses for both the content and
// the signed attributes. SHA-1 is in: third-party signatures still use it, and the question here is whether bytes were
// signed with a key, not whether the algorithm is fit for new signatures.
func hashOf(d asn1.ObjectIdentifier) (crypto.Hash, bool) {
	switch {
	case d.Equal(oidDigestSHA1):
		return crypto.SHA1, true
	case d.Equal(oidDigestSHA256):
		return crypto.SHA256, true
	case d.Equal(oidDigestSHA384):
		return crypto.SHA384, true
	case d.Equal(oidDigestSHA512):
		return crypto.SHA512, true
	}
	return 0, false
}

// signerProof is what `SignedRevisionFor` keeps of a signature blob, COPIED out of it so nothing aliases `/Contents`.
type signerProof struct {
	cert         *x509.Certificate // the one certificate the SignerInfo names
	signedAttrs  []byte            // the DER of the signed attributes as a SET OF — the bytes the signature covers
	signature    []byte            // EncryptedDigest
	digest       asn1.ObjectIdentifier
	encryption   asn1.ObjectIdentifier // DigestEncryptionAlgorithm
	messageDigst []byte                // the messageDigest attribute; nil when absent
	encapsulated bool                  // the SignedData carries its content (the `adbe.pkcs7.sha1` shape)
	content      []byte                // a COPY of that content, kept only up to `maxEncapsulatedContent` bytes
}

// proofOf copies the single SignerInfo's proof out of p7, or nil when there is not exactly one named signer.
func proofOf(p7 *pkcs7.PKCS7) *signerProof {
	cert := p7.GetOnlySigner()
	if cert == nil || len(p7.Signers) != 1 {
		return nil
	}
	own, err := x509.ParseCertificate(bytes.Clone(cert.Raw))
	if err != nil {
		return nil
	}
	si := p7.Signers[0]
	pr := &signerProof{cert: own, signature: bytes.Clone(si.EncryptedDigest), digest: append(asn1.ObjectIdentifier(nil), si.DigestAlgorithm.Algorithm...),
		encryption: append(asn1.ObjectIdentifier(nil), si.DigestEncryptionAlgorithm.Algorithm...), encapsulated: len(p7.Content) > 0}
	if pr.encapsulated && len(p7.Content) <= maxEncapsulatedContent {
		pr.content = bytes.Clone(p7.Content)
	}
	if len(si.AuthenticatedAttributes) == 0 {
		return pr
	}
	attrs := make([]signedAttribute, len(si.AuthenticatedAttributes))
	for i, a := range si.AuthenticatedAttributes {
		attrs[i] = signedAttribute{a.Type, a.Value}
		if a.Type.Equal(oidMessageDigest) {
			var md []byte
			if _, err := asn1.Unmarshal(a.Value.Bytes, &md); err == nil {
				pr.messageDigst = bytes.Clone(md)
			}
		}
	}
	// The signature is over the attributes' DER as a SET OF (RFC 5652 §5.4), which asn1 sorts on Marshal — the same
	// re-marshal the library verifies against.
	enc, err := asn1.Marshal(struct {
		A []signedAttribute `asn1:"set"`
	}{attrs})
	if err != nil {
		return pr
	}
	var set asn1.RawValue
	if _, err := asn1.Unmarshal(enc, &set); err != nil {
		return pr
	}
	pr.signedAttrs = bytes.Clone(set.Bytes)
	return pr
}

// signatureAlgorithm maps a SignerInfo's digest and encryption algorithms EXACTLY as the library does (`pkcs7`
// `getSignatureAlgorithm`, verify.go): the ECDSA-with-hash OIDs fix the hash; every RSA OID — `sha256WithRSA` included
// — and every curve OID take it from the digest algorithm; Ed25519 signs its message whole. A shape the library
// verifies and this refuses throws a genuine version away on the screened path, so the two must not drift.
func (pr *signerProof) signatureAlgorithm() (x509.SignatureAlgorithm, bool) {
	enc, d := pr.encryption, pr.digest
	byDigest := func(sha1, sha256, sha384, sha512 x509.SignatureAlgorithm) (x509.SignatureAlgorithm, bool) {
		switch {
		case d.Equal(pkcs7.OIDDigestAlgorithmSHA1):
			return sha1, true
		case d.Equal(pkcs7.OIDDigestAlgorithmSHA256):
			return sha256, true
		case d.Equal(pkcs7.OIDDigestAlgorithmSHA384):
			return sha384, true
		case d.Equal(pkcs7.OIDDigestAlgorithmSHA512):
			return sha512, true
		}
		return 0, false
	}
	switch {
	case enc.Equal(pkcs7.OIDDigestAlgorithmECDSASHA1):
		return x509.ECDSAWithSHA1, true
	case enc.Equal(pkcs7.OIDDigestAlgorithmECDSASHA256):
		return x509.ECDSAWithSHA256, true
	case enc.Equal(pkcs7.OIDDigestAlgorithmECDSASHA384):
		return x509.ECDSAWithSHA384, true
	case enc.Equal(pkcs7.OIDDigestAlgorithmECDSASHA512):
		return x509.ECDSAWithSHA512, true
	case enc.Equal(pkcs7.OIDEncryptionAlgorithmRSA), enc.Equal(pkcs7.OIDEncryptionAlgorithmRSASHA1),
		enc.Equal(pkcs7.OIDEncryptionAlgorithmRSASHA256), enc.Equal(pkcs7.OIDEncryptionAlgorithmRSASHA384),
		enc.Equal(pkcs7.OIDEncryptionAlgorithmRSASHA512):
		return byDigest(x509.SHA1WithRSA, x509.SHA256WithRSA, x509.SHA384WithRSA, x509.SHA512WithRSA)
	case enc.Equal(pkcs7.OIDEncryptionAlgorithmECDSAP256), enc.Equal(pkcs7.OIDEncryptionAlgorithmECDSAP384),
		enc.Equal(pkcs7.OIDEncryptionAlgorithmECDSAP521):
		return byDigest(x509.ECDSAWithSHA1, x509.ECDSAWithSHA256, x509.ECDSAWithSHA384, x509.ECDSAWithSHA512)
	case enc.Equal(pkcs7.OIDEncryptionAlgorithmEDDSA25519):
		return x509.PureEd25519, true
	}
	return 0, false
}

// checks reports whether the SignerInfo's signature verifies over signed with the named certificate's key.
func (pr *signerProof) checks(signed []byte) bool {
	alg, ok := pr.signatureAlgorithm()
	return ok && pr.cert.CheckSignature(alg, signed, pr.signature) == nil
}

// attributed reports whether the SignerInfo's signature over its signed attributes checks against the key of the
// certificate it names (plan-review C1) — "made with that key", over bytes this file may no longer hold. It is computed
// only where `SignedRevisionFor` screens a candidate or is about to say `resaved`, never on the verdict path. A
// SignerInfo with NO signed attributes signs the content itself, so there is nothing to attribute apart from bytes
// this file holds: it is false here, and the screen checks it over the bytes its ranges select instead.
func (pr *signerProof) attributed() bool {
	return pr != nil && pr.signedAttrs != nil && pr.checks(pr.signedAttrs)
}

// rangeCost is how many bytes a ByteRange selects, refused where a pair is negative, runs past the file, or does not
// begin after the previous pair ends — so one record costs at most the file's length to hash (P02.S01's review: one
// copy listing `0 L` sixty-four times spent the whole budget).
func rangeCost(br []int64, size int) (int64, bool) {
	if len(br) == 0 || len(br)%2 != 0 {
		return 0, false
	}
	var cost, prev int64
	for i := 0; i < len(br); i += 2 {
		a, n := br[i], br[i+1]
		if a < prev || n < 0 || satAdd(a, n) > int64(size) {
			return 0, false
		}
		cost = satAdd(cost, n)
		prev = a + n
	}
	return cost, true
}

// selected is head followed by the bytes a ByteRange selects, joined into a new slice; the ranges must already have
// passed `rangeCost`.
func selected(head, pdf []byte, br []int64) []byte {
	out := append([]byte(nil), head...)
	for i := 0; i < len(br); i += 2 {
		out = append(out, pdf[br[i]:br[i]+br[i+1]]...)
	}
	return out
}

// digestMatches reports whether the bytes a ByteRange selects hash to the messageDigest the signed attributes carry.
// A SignerInfo that encapsulates its content (the `adbe.pkcs7.sha1` shape), carries no messageDigest, or names a digest
// this does not know cannot be screened this way and passes, so it costs a full re-verify like any candidate. The
// ranges must already have passed `rangeCost`.
func (pr *signerProof) digestMatches(pdf []byte, br []int64) bool {
	if pr == nil {
		return false
	}
	if pr.encapsulated || pr.messageDigst == nil {
		return true
	}
	h, ok := hashOf(pr.digest)
	if !ok {
		return true
	}
	if _, ok := rangeCost(br, len(pdf)); !ok {
		return false
	}
	hw := h.New()
	for i := 0; i < len(br); i += 2 {
		hw.Write(pdf[br[i] : br[i]+br[i+1]])
	}
	return bytes.Equal(hw.Sum(nil), pr.messageDigst)
}
