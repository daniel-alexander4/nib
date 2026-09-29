package sign

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"

	dpdf "github.com/digitorus/pdf"
	"github.com/digitorus/pdfsign/verify"
	"github.com/digitorus/pkcs7"

	"nib/internal/fontcode"
)

// RefusalCause names why a signature-shaped dictionary is not a well-formed signature. The empty
// cause is a well-formed one. Five causes and no more: each is a different sentence to a reader,
// and a lumped refusal reads backwards exactly when it matters (PLAN-returned-document D7). A
// document timestamp is never refused on account of its imprint (ADR-059): that check is the
// dispute surface's, on demand, never the verdict path's.
type RefusalCause string

const (
	// CauseMalformedByteRange: the `/ByteRange` is not one gap around this dictionary's own
	// `/Contents` — structure conjuncts (1)-(5) and (7)-(10).
	CauseMalformedByteRange RefusalCause = "malformed-byterange"
	// CauseByteRangeOutsideFile: the last pair ends past the end of the file — conjunct (6). The
	// library would have truncated the read silently (`processByteRange`, `io.NewSectionReader`).
	CauseByteRangeOutsideFile RefusalCause = "byterange-outside-file"
	// CauseContentsElsewhere: the gap is a hex string, but not the one this object wrote — the
	// nearest object header before it names another object — conjunct (11).
	CauseContentsElsewhere RefusalCause = "contents-elsewhere"
	// CauseUnsupportedFilter: the dictionary claims to be a signature and its `/Filter` is not
	// `Adobe.PPKLite`, so the library never looks at it (/pending 661's first decoy).
	CauseUnsupportedFilter RefusalCause = "unsupported-filter"
	// CauseUnparseableContents: `/Filter /Adobe.PPKLite`, and `pkcs7.Parse` rejects `/Contents` —
	// the library skips it silently (`pdfsign verify/verify.go:97-101`, /pending 661's second decoy).
	CauseUnparseableContents RefusalCause = "unparseable-contents"
)

const ppkLite = "Adobe.PPKLite"

// gapScanWindow bounds conjunct (11)'s backward scan. The header of the dictionary that owns a gap
// sits a few hundred bytes before it on every producer measured (199 B on nib's, 501 B on the IRS
// files); 64 KiB is the plan-review's bound, and the previous gap's end tightens it further.
const gapScanWindow = 64 << 10

// Revision is one signature-shaped dictionary in the document: who signed it, how far the
// signature reaches, and whether it is well-formed. It is the one home of "who signed" and
// "well-formed" (ADR-058) — `SignerInfo.Fingerprint` is read from here, and nothing else computes
// it — and of "how far" (P01.S02 deleted the `/Fields` ByteRange walk that was its second home):
// `AddedAfter` reads `CoverageEnd` from the records that `bounds`.
//
// The revision a signature covers is `[0, CoverageEnd)`: every signature but the last
// legitimately ends before EOF.
type Revision struct {
	Obj uint32 // object number of the signature dictionary
	Gen uint16
	// Type, Filter and SubFilter are the dictionary's own names, recorded so a reader can say what
	// the object CLAIMED to be (`/DocTimeStamp` is S02's to judge).
	Type, Filter, SubFilter string
	// ByteRange is the raw array, read the way the library reads it (`Index(i).Int64()`).
	ByteRange []int64
	// Cause is empty for a well-formed signature and names the refusal otherwise.
	Cause RefusalCause
	// CoverageEnd is the end of the last pair (D5) — set only when Cause is empty.
	CoverageEnd int64
	// Fingerprint is the hex SHA-256 SPKI of the certificate this signature's SignerInfo NAMES
	// (ADR-051), and it is set only when Verified AND Cause is empty: nothing reports an identity it
	// has not established, and a refused copy of the victim's dictionary verifies as the victim. A
	// verified record with no fingerprint still bounds coverage — the hash was checked, whoever made
	// it — and never stands for a person.
	Fingerprint string
	// Verified is the library's `ValidSignature` for this record's position in its enumeration.
	// A record outside that enumeration is false by definition, never by index.
	Verified bool
	// Timestamp marks a document timestamp: `/Type /DocTimeStamp` or `/SubFilter /ETSI.RFC3161`.
	// It names no signer, and anyone can obtain one over any bytes, so it NEVER bounds coverage
	// (ADR-059). Its `messageImprint` is not read here: the verdict path never pays for it.
	Timestamp bool

	// hasContents is a non-empty `/Contents` — the rule `signatureBlobPresent` uses to tell a real
	// signature from a prepare-for-signing placeholder, read here over the xref sweep so a
	// `/Kids`-nested blob is seen (the zero-signer path in `verifyIndexed`). A record without one is
	// never published as refused (`refusedOf`).
	hasContents bool
	// underPerms is whether the catalog's `/Perms` names this dictionary — a usage-rights or DocMDP
	// signature, which lives outside the form and may lawfully have no `/SigFlags` beside it.
	underPerms bool

	// conjunct is which of the eleven structure conjuncts refused the record (0 = none). The
	// public vocabulary is Cause; `malformed-byterange` lumps nine conjuncts, so a fixture built
	// for one and refused by another would pass on the cause alone. Read by tests only.
	conjunct int
	// libPos is this record's ordinal in the library's enumeration — `/Filter /Adobe.PPKLite`
	// whose `/Contents` `pkcs7.Parse` accepts, in xref order, and only when `/SigFlags` exists —
	// or -1 when the library never reports it.
	libPos int
	// bag is `bagKey` over the certificate bag, for the per-position cross-check.
	bag string
	// named is the fingerprint the SignerInfo names, before anything has verified it.
	named string
	// brIndirect is a `/ByteRange` that resolves to an ARRAY held in another object: the library
	// re-parses it on every `Key` call, three times per pair (`libraryWouldOverread`).
	brIndirect bool
	// gapStart and gapEnd are the gap's offsets, valid once conjuncts (1)-(6) hold.
	gapStart, gapEnd int64
}

// sweepStats is what the sweep can say about its own cost; read by the budget tests.
type sweepStats struct {
	// scanned counts the bytes conjunct (11)'s backward scans examined, over the whole document.
	scanned int
	// forward counts the bytes conjuncts (7) and (8) examined: each distinct gap's token walked
	// once and decoded at most once, plus the bytes of every `/Contents` compared against a
	// decoding of equal length.
	forward int
}

// gapMemo holds, per distinct gap start, what conjuncts (7) and (8) learned by walking the token
// there — so M dictionaries pointing at one G-byte token cost G once, not M×G.
type gapMemo struct {
	at      map[int64]*gapToken
	forward int
}

// gapToken is the walk from a `<` at a gap start: `end` is the first byte after it that is neither
// a hex digit nor white space, and `digits` how many hex digits lie between. `decoded` is filled
// the first time a record's `/Contents` is the right length to be compared with it.
type gapToken struct {
	end, digits int64
	decoded     []byte
}

// sweepRevisions is the one walk over a document's signature-shaped dictionaries.
func sweepRevisions(pdf []byte) ([]Revision, error) {
	revs, _, err := sweep(pdf)
	return revs, err
}

// sweep performs the library's own enumeration and records every signature-shaped dictionary.
//
// # This must be the SAME walk the library performs (ADR-051, kept by ADR-058)
//
// The library enumerates signatures by resolving every object in the xref and testing it for
// `/Filter /Adobe.PPKLite` (`pdfsign verify/verify.go:87-94`), and it never consults
// `AcroForm/Fields`, needing only that `/SigFlags` exists (`:81-84`). A `/Fields` walk was once
// used here because it is two orders of magnitude cheaper, and it let the attacker write both sides
// of the join key: a real signature reachable only through the xref, plus a decoy `/Fields` entry
// supplying the key. The xref sweep also sees a signature nested under `/Kids` without recursing
// (D6): it never walks fields at all.
//
// # The record set is wider than the library's, on purpose
//
// A record is made for EVERY xref object that is a dictionary carrying `/ByteRange`, `/Type /Sig` or `/Type
// /DocTimeStamp`, and for every `/Filter /Adobe.PPKLite` one (the library reports those whether or
// not they carry a `/ByteRange`, and the positional join needs each of them to be a record). So an
// examiner can say "object 31 claims to be a signature and is not one" — /pending 661's decoys are
// records with causes, never silent skips.
//
// **A panic is an error, never a crash and never a partial answer** — `digitorus/pdf` panics from
// its lazy dereferences on ordinary corruption (/pending 502), and `Verify` routes an error here to
// its fail-closed arms.
func sweep(pdf []byte) (revs []Revision, st sweepStats, err error) {
	defer func() {
		if rec := recover(); rec != nil {
			revs, err = nil, fmt.Errorf("read pdf: %v", rec)
		}
	}()
	r, err := dpdf.NewReader(bytes.NewReader(pdf), int64(len(pdf)))
	if err != nil {
		return nil, st, err
	}
	// The library's own gate: without `/SigFlags` it reports no signer at all, so no record is at
	// any position in its enumeration.
	sigFlags := !r.Trailer().Key("Root").Key("AcroForm").Key("SigFlags").IsNull()
	// The signatures the catalog's `/Perms` names (`/UR3` usage rights, `/DocMDP`): the one class of
	// signature a document carries OUTSIDE its form, legitimately without `/SigFlags`.
	perms := map[uint32]bool{}
	pd := r.Trailer().Key("Root").Key("Perms")
	for _, k := range pd.Keys() {
		if p := pd.Key(k).GetPtr(); p.GetID() != 0 {
			perms[p.GetID()] = true
		}
	}
	lib := 0
	memo := &gapMemo{at: map[int64]*gapToken{}}
	for _, x := range r.Xref() {
		ptr := x.Ptr()
		v := r.Resolve(ptr, ptr)
		filter := v.Key("Filter").Name()
		typ := v.Key("Type").Name()
		br := v.Key("ByteRange")
		if filter != ppkLite && br.Kind() == dpdf.Null && typ != "Sig" && typ != "DocTimeStamp" {
			continue
		}
		stream := x.Stream()
		rev := Revision{
			Obj: ptr.GetID(), Gen: ptr.GetGen(),
			Type: typ, Filter: filter, SubFilter: v.Key("SubFilter").Name(),
			libPos:      -1,
			hasContents: len(v.Key("Contents").RawString()) > 0,
			underPerms:  perms[ptr.GetID()],
		}
		rev.Timestamp = rev.Type == "DocTimeStamp" || rev.SubFilter == "ETSI.RFC3161"
		var allInts bool
		rev.ByteRange, allInts = byteRangeOf(br)
		rev.brIndirect = br.Kind() == dpdf.Array && br.GetPtr() != v.GetPtr()
		if filter != ppkLite {
			rev.Cause = CauseUnsupportedFilter
			revs = append(revs, rev)
			continue
		}
		p7, perr := pkcs7.Parse([]byte(v.Key("Contents").RawString()))
		if perr != nil {
			rev.Cause = CauseUnparseableContents
			revs = append(revs, rev)
			continue
		}
		if sigFlags {
			rev.libPos = lib
			lib++
		}
		rev.bag = bagKey(rawsOf(p7.Certificates))
		// GetOnlySigner is nil for a bag that names no certificate it carries, and for the
		// multi-SignerInfo shape a PDF signature never has. Either way nib cannot say who signed.
		if cert := p7.GetOnlySigner(); cert != nil {
			rev.named = hex.EncodeToString(fingerprintOf(cert))
		}
		rev.conjunct, rev.gapStart, rev.gapEnd = structureOf(pdf, v, br, rev.ByteRange, allInts, stream.GetID() != 0, memo)
		revs = append(revs, rev)
	}
	st.scanned = assignGapOwners(pdf, revs)
	st.forward = memo.forward
	for i := range revs {
		rv := &revs[i]
		if rv.Cause != "" {
			continue
		}
		switch {
		case rv.conjunct == 0:
			n := len(rv.ByteRange)
			rv.CoverageEnd = rv.ByteRange[n-2] + rv.ByteRange[n-1]
		case rv.conjunct == 6:
			rv.Cause = CauseByteRangeOutsideFile
		case rv.conjunct == 11:
			rv.Cause = CauseContentsElsewhere
		default:
			rv.Cause = CauseMalformedByteRange
		}
	}
	return revs, st, nil
}

// bounds reports whether this record's `CoverageEnd` counts towards `AddedAfter`: the library
// verified it, it is well-formed, and it is a signature rather than a document timestamp. Nothing
// else measures coverage (P01.S02, ADR-059).
func (r *Revision) bounds() bool { return r.Verified && r.Cause == "" && !r.Timestamp }

// byteRangeOf reads the array exactly as the library does, and says whether every element was an
// integer — conjunct (1) needs to know, and the library does not ask.
func byteRangeOf(br dpdf.Value) ([]int64, bool) {
	if br.Kind() != dpdf.Array {
		return nil, false
	}
	n := br.Len()
	out := make([]int64, n)
	all := true
	for i := 0; i < n; i++ {
		e := br.Index(i)
		if e.Kind() != dpdf.Integer {
			all = false
		}
		out[i] = e.Int64()
	}
	return out, all
}

// structureOf evaluates conjuncts (1)-(10) of the structure rule, in the plan's order, and returns
// the first that fails (0 when all ten hold) with the gap's bounds. Conjunct (11) needs every
// record's gap and is `assignGapOwners`'. The rule, whole: a signature is well-formed only if its
// `/ByteRange` covers the entire revision less its own `/Contents` hex string, and nothing else.
//
// **A copied dictionary is what this refuses** (/pending 687): a new dictionary carrying the
// victim's `/Contents` and `/ByteRange` verifies as a second signer under the victim's fingerprint,
// and a last-pair rule alone would let it end coverage anywhere the attacker likes.
func structureOf(data []byte, v, br dpdf.Value, vals []int64, allInts, inObjStm bool, memo *gapMemo) (conj int, gs, ge int64) {
	n := len(vals)
	// (1) a direct array of integers, even length >= 4. An indirect array resolves with the
	// referenced object as its parent, so a parent other than this dictionary is an indirection.
	if br.Kind() != dpdf.Array || br.GetPtr() != v.GetPtr() || !allInts || n < 4 || n%2 != 0 {
		return 1, 0, 0
	}
	// (2) the first start is 0.
	if vals[0] != 0 {
		return 2, 0, 0
	}
	// (3) every length is positive.
	for i := 1; i < n; i += 2 {
		if vals[i] <= 0 {
			return 3, 0, 0
		}
	}
	// (4) starts strictly ascend.
	for i := 2; i < n; i += 2 {
		if vals[i] <= vals[i-2] {
			return 4, 0, 0
		}
	}
	// (5) every pair begins where the previous one ended, except exactly one gap.
	gaps := 0
	for i := 2; i < n; i += 2 {
		prevEnd := satAdd(vals[i-2], vals[i-1])
		switch {
		case vals[i] < prevEnd:
			return 5, 0, 0
		case vals[i] > prevEnd:
			gaps++
			gs, ge = prevEnd, vals[i]
		}
	}
	if gaps != 1 {
		return 5, 0, 0
	}
	// (6) the last pair ends inside the file — before any byte of it is read.
	if satAdd(vals[n-2], vals[n-1]) > int64(len(data)) {
		return 6, gs, ge
	}
	// (7) the gap is exactly one hex-string token: `<`, hex digits and white space, `>`. A `>`
	// anywhere before the last byte is a second token, and whatever follows it (a `/Reason`, say)
	// would sit inside the hole, signed by nobody.
	//
	// **The token is walked once per distinct gap start, never once per record** (memo): M
	// dictionaries naming one G-byte token would otherwise cost M×G. Walks from different `<`
	// bytes are disjoint — each stops at the first byte that is neither hex nor white space, and
	// `<` is such a byte — so all of them together read the file at most once.
	if ge-gs < 2 || data[gs] != '<' || data[ge-1] != '>' {
		return 7, gs, ge
	}
	tok := memo.token(data, gs)
	if tok.end != ge-1 {
		return 7, gs, ge
	}
	// (8) its strict decoding is this dictionary's `/Contents`. The decoded LENGTH is known from
	// the walk, so a `/Contents` of any other length is refused without reading the token again;
	// one of the right length is compared against the token's decoding, made once per gap.
	want := v.Key("Contents").RawString()
	if int64(len(want)) != (tok.digits+1)/2 {
		return 8, gs, ge
	}
	if tok.decoded == nil {
		tok.decoded = fontcode.Hex(data[gs:tok.end]) // the one hex door (ADR-052); (7) left only digits and white space
		memo.forward += int(tok.end - gs - 1)
	}
	memo.forward += len(want)
	if string(tok.decoded) != want {
		return 8, gs, ge
	}
	contents := v.Key("Contents")
	// (9) `/Contents` is a direct string: an indirect one is some other object's bytes.
	if contents.Kind() != dpdf.String || contents.GetPtr() != v.GetPtr() {
		return 9, gs, ge
	}
	// (10) the dictionary is not in an object stream: a compressed `/Contents` has no file offset,
	// so a gap that decodes to it is a copy somewhere else in the file.
	if inObjStm {
		return 10, gs, ge
	}
	return 0, gs, ge
}

// assignGapOwners is conjunct (11): the gap belongs to THIS object. It returns the bytes scanned.
//
// Scanning back from the gap through bytes the signature itself covers, the nearest `N G obj`
// header must be the record's own object number, and only `/Contents` and white space may sit
// immediately before the `<`. A verbatim copy of the victim's dictionary under any other number
// finds the victim's header, because those bytes are the victim's signed bytes; a copy REUSING the
// victim's number replaces the victim in the xref (one signer, harmless). The digitorus reader
// exposes no offsets (`read.go:118-131`), which is why this is a backward scan of signed bytes and
// not an offset lookup — and nothing searches the whole file.
//
// **Nearest, never "our own number anywhere"**: nearest makes gap → owner a FUNCTION, so no gap
// has two well-formed owners. **Linear, never a regex or a lexer**: a regex over the window
// measured 3.2 ms per IRS signature and a candidate-by-candidate lexer went quadratic (609 ms for
// one 52 KB record). **Bounded below by the previous distinct gap's end**, and memoised per gap,
// so total work across every record is O(len(data)) however many records the attacker writes: a
// header between an earlier gap and this one belongs to a later object than the earlier gap's.
//
// **Declared residual (G10, /pending 736)**: where a third-party producer writes attacker-
// influenced text (`/Reason`, `/Name`) BEFORE `/Contents` — the IRS files do, nib does not —
// header-shaped text there can move which object a gap is attributed to. Coverage, identity and
// the signer count cannot move: the hash binds the ranges and the SignerInfo the key.
func assignGapOwners(data []byte, revs []Revision) (scanned int) {
	var starts []int64
	ends := map[int64]int64{}
	for i := range revs {
		rv := &revs[i]
		if rv.Cause != "" || rv.conjunct != 0 {
			continue
		}
		if _, seen := ends[rv.gapStart]; !seen {
			starts = append(starts, rv.gapStart)
		}
		// Distinct gaps are disjoint — a gap that passed (7) is one hex token, so no other
		// token's `<` falls inside it, and one start has one end.
		ends[rv.gapStart] = rv.gapEnd
	}
	sort.Slice(starts, func(a, b int) bool { return starts[a] < starts[b] })
	type owned struct {
		obj uint32
		ok  bool
	}
	owner := make(map[int64]owned, len(starts))
	var prevEnd int64
	for _, gs := range starts {
		floor := gs - gapScanWindow
		if floor < prevEnd {
			floor = prevEnd
		}
		if floor < 0 {
			floor = 0
		}
		if floor > gs {
			floor = gs
		}
		obj, ok, n := gapOwner(data, int(gs), int(floor))
		scanned += n
		owner[gs] = owned{obj, ok}
		prevEnd = ends[gs]
	}
	for i := range revs {
		rv := &revs[i]
		if rv.Cause != "" || rv.conjunct != 0 {
			continue
		}
		if o := owner[rv.gapStart]; !o.ok || o.obj != rv.Obj {
			rv.conjunct = 11
		}
	}
	return scanned
}

// gapOwner finds the object that wrote the hex string starting at gs: `/Contents` and white space
// immediately before it, then the nearest `N G obj` header, looking no lower than floor. It never
// reads below floor, and the bytes it examines are returned for the budget tests.
func gapOwner(data []byte, gs, floor int) (obj uint32, ok bool, examined int) {
	const key = "/Contents"
	i := gs - 1
	for i >= floor && isPDFSpace(data[i]) {
		i--
		examined++
	}
	keyAt := i + 1 - len(key)
	if keyAt < floor || string(data[keyAt:i+1]) != key {
		return 0, false, examined
	}
	for j := keyAt - 3; j >= floor; j-- {
		examined++
		if data[j] != 'o' || data[j+1] != 'b' || data[j+2] != 'j' {
			continue
		}
		if c := data[j+3]; !isPDFSpace(c) && !isPDFDelimiter(c) {
			continue // `objx` is not the keyword
		}
		// Walk back over `N ws+ G ws+`. The runs are disjoint across candidates (a candidate's
		// run holds no `j`), so the walks add up to at most the window once more. Every exit
		// from the walk counts the bytes it read, so `examined` is what the scan cost.
		k := j - 1
		mark := k
		for k >= floor && isPDFSpace(data[k]) {
			k--
		}
		if k == mark {
			continue
		}
		mark = k
		for k >= floor && isDigit(data[k]) {
			k--
		}
		if k == mark {
			examined += j - 1 - k
			continue
		}
		mark = k
		for k >= floor && isPDFSpace(data[k]) {
			k--
		}
		if k == mark {
			examined += j - 1 - k
			continue
		}
		numEnd := k
		for k >= floor && isDigit(data[k]) {
			k--
		}
		examined += j - 1 - k
		if k == numEnd || numEnd-k > 10 {
			continue
		}
		// The number must start at a token boundary; a digit below the floor means the header
		// straddles it and is not wholly in the window.
		if k >= 0 && !isPDFSpace(data[k]) && !isPDFDelimiter(data[k]) {
			continue
		}
		var n uint64
		for _, c := range data[k+1 : numEnd+1] {
			n = n*10 + uint64(c-'0')
		}
		if n > 1<<32-1 {
			continue
		}
		return uint32(n), true, examined
	}
	return 0, false, examined
}

// errLibraryWouldOverread refuses a document before the library copies its byte ranges.
var errLibraryWouldOverread = errors.New("a signature's /ByteRange would make the library read more than the file holds")

// libraryWouldOverread reports whether any record the library WILL process names a `/ByteRange`
// the library must not be handed. It refuses exactly three shapes, each a cost the library pays
// before any hash is compared, and nothing else:
//
//   - **An indirect `/ByteRange` array.** `processByteRange` (`signature.go:69-84`) calls
//     `v.Key("ByteRange")` three times per pair, and an indirect array is re-resolved and re-parsed
//     from the file on every call — measured 7.86 s for K=4000 pairs on a 29 KB file, still ending
//     `valid`. Honest producers write it direct; conjunct (1) refuses it as a record too.
//   - **Any negative length.** `io.NewSectionReader` (`io/io.go:486-497`) computes `off+n` guarded
//     against overflow only for positive `n`: a negative `n` takes the overflow arm and the section
//     becomes "to the end of the file", so `[0 -1 0 -1 …]` copies the whole file per pair —
//     measured 371 MB from a 16 KB file.
//   - **An effective read summing past the file.** The library copies every pair into memory, so
//     `[0 S 0 S … ×K]` allocates K×S, and a Go out-of-memory is `fatal`, not a panic, so no
//     `recover` contains it. A pair reads `min(l, size-off)` bytes when `0 <= off < size`
//     (`SectionReader.Read` clamps to the underlying reader, `io/io.go:509-519`), and nothing
//     otherwise.
//
// **Three hostile-looking shapes are deliberately NOT refused here, because they read nothing**, and
// refusing them would make the whole document `Invalid` and blank its real signers — the S03
// plan-review pin: a structural refusal is a refused RECORD, never a refused document. The structure
// rule refuses each of them as a record, and the co-signers keep their verdict:
//
//   - an odd element count: the library reads the missing length as `Index(n)` of an `n`-element
//     array, which is null, and `Int64()` of null is 0 — an empty section;
//   - a negative offset: `bytes.Reader.ReadAt` returns an error before copying (`bytes/reader.go:52-54`),
//     and the library returns that error for the signer;
//   - a pair starting at or past EOF: `bytes.Reader.ReadAt` answers `io.EOF` with nothing copied
//     (`bytes/reader.go:55-57`) — whatever its length, it contributes 0 to the sum above.
//
// A pair that STARTS inside the file and runs past EOF is refused here only if its clamped read
// pushes the sum past the file; otherwise it is conjunct (6)'s record refusal, since the library
// would hash a silently truncated read.
//
// Only records in the library's enumeration are asked: one whose PKCS#7 does not parse is dropped by
// the library before any range is read, and refusing on it would turn /pending 661's second decoy
// into an `Invalid` document instead of a refused record.
//
// The arithmetic cannot overflow: every effective read is in `[0, limit]`, and it is compared with
// `limit-sum`, which never goes negative because `sum` never exceeds `limit`.
func libraryWouldOverread(revs []Revision, size int) bool {
	limit := int64(size)
	for i := range revs {
		rv := &revs[i]
		if rv.libPos < 0 {
			continue
		}
		if rv.brIndirect {
			return true
		}
		br := rv.ByteRange
		var sum int64
		for j := 0; j+1 < len(br); j += 2 {
			off, l := br[j], br[j+1]
			if l < 0 {
				return true
			}
			var eff int64
			if off >= 0 && off < limit {
				eff = min(l, limit-off)
			}
			if eff > limit-sum {
				return true
			}
			sum += eff
		}
	}
	return false
}

// errJoin is a disagreement between the library's enumeration and the sweep's. Every caller
// routes it fail-closed: no fingerprint is reported, and `AddedAfter` warns.
var errJoin = errors.New("the library's signers do not line up with the document's signatures")

// joinLibrary lines the library's signers up with the records, POSITIONALLY: ordinal i of the
// records the library processes (PPKLite, parseable, `/SigFlags` present, in xref order) is the
// library's signer i. It sets `Verified` from `ValidSignature` at that position and, only where it
// verified AND the record is well-formed, `Fingerprint`. It returns, per library signer, the index of its record.
//
// **Positional, not a re-hash** (plan-review, performance): the library is 13-23 ms per signature
// at 10 MB and a per-signature `p7.Verify` here would roughly double `Verify`'s dominant cost.
//
// **The bag is the cross-check, not the key** (ADR-058, superseding ADR-051's join key). The count
// must agree, and at every position a NON-EMPTY library bag must equal the record's. An EMPTY
// library bag is what the library returns on every failure path (`signature.go:36-63`; the bag is
// filled only at `certificate.go:345`), so it must come with `ValidSignature=false` — requiring the
// record's bag to be empty too would make every tampered document a join error and blank its valid
// co-signers.
func joinLibrary(revs []Revision, signers []verify.Signer) ([]int, error) {
	idx := make([]int, 0, len(signers))
	for j := range revs {
		if revs[j].libPos < 0 {
			continue
		}
		if revs[j].libPos != len(idx) {
			return nil, fmt.Errorf("%w: record %d is at library position %d, want %d", errJoin, revs[j].Obj, revs[j].libPos, len(idx))
		}
		idx = append(idx, j)
	}
	if len(idx) != len(signers) {
		return nil, fmt.Errorf("%w: the library reported %d signers and the document holds %d it would process", errJoin, len(signers), len(idx))
	}
	for i, j := range idx {
		s := &signers[i]
		if len(s.Certificates) == 0 {
			if s.ValidSignature {
				return nil, fmt.Errorf("%w: signer %d verified with no certificates", errJoin, i)
			}
			continue
		}
		if bagKeyOfSigner(s) != revs[j].bag {
			return nil, fmt.Errorf("%w: signer %d's certificates are not object %d's", errJoin, i, revs[j].Obj)
		}
	}
	for i, j := range idx {
		revs[j].Verified = signers[i].ValidSignature
		// **A structurally refused record carries no fingerprint, even where the library verified
		// it**: a copied dictionary (/pending 687) verifies as the victim, and P02 selects records
		// by fingerprint — a refused copy carrying the victim's would be selected as the victim.
		if revs[j].Verified && revs[j].Cause == "" {
			revs[j].Fingerprint = revs[j].named
		}
	}
	return idx, nil
}

// Revisions returns one record per signature-shaped dictionary in pdf, in xref order, each saying
// who signed it (where that is established), how far it reaches, and whether it is well-formed. It
// runs the same door `Verify` does — the readability gate, the sweep, the library, the join — and
// an error means some part of that could not be trusted; the records are then not to be believed.
// An unsigned document has none.
func Revisions(pdf []byte) ([]Revision, error) {
	_, revs, err := verifyIndexed(pdf)
	if err != nil {
		return nil, err
	}
	return revs, nil
}

func satAdd(a, b int64) int64 {
	if b > 0 && a > (1<<63-1)-b {
		return 1<<63 - 1
	}
	return a + b
}

func isPDFSpace(c byte) bool {
	return c == 0 || c == '\t' || c == '\n' || c == '\f' || c == '\r' || c == ' '
}

func isPDFDelimiter(c byte) bool {
	switch c {
	case '(', ')', '<', '>', '[', ']', '{', '}', '/', '%':
		return true
	}
	return false
}

func isDigit(c byte) bool { return c >= '0' && c <= '9' }

func isHexDigit(c byte) bool {
	return isDigit(c) || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

// token walks the hex-string token whose `<` is at gs, once: later records naming the same gap
// start read the memo.
func (m *gapMemo) token(data []byte, gs int64) *gapToken {
	if t, ok := m.at[gs]; ok {
		return t
	}
	t := &gapToken{end: gs + 1}
	for t.end < int64(len(data)) {
		c := data[t.end]
		if isHexDigit(c) {
			t.digits++
		} else if !isPDFSpace(c) {
			break
		}
		t.end++
	}
	m.forward += int(t.end - gs)
	m.at[gs] = t
	return t
}
