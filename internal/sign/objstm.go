package sign

import (
	"bufio"
	"errors"
	"fmt"
	"io"

	dpdf "github.com/digitorus/pdf"
)

// # What the signature reader pays for an object stream (/pending 751, /pending 758, ADR-066)
//
// Upstream `digitorus/pdf` v0.1.2 found an object-stream member by decoding its stream FROM THE START
// and lexing the header's `id offset` pairs until the id matched — every time, caching nothing — and
// both the revision sweep and the library's verify pass resolve EVERY xref object (ADR-058), so a
// stream of N members cost N²/2 pairs per pass. /pending 751 bounded that from here; /pending 758
// patched the library itself (`third_party/digitorus-pdf`, read.go only, see its NOTICE.nib): the
// patched `resolve` keeps each stream's decoder open and its header lexed SO FAR per `Reader`, so a
// stream is decoded once and its header lexed once per pass, each only as far as a lookup has needed.
//
// **The gate stays, re-fitted to the patched reader.** What a pass still pays, and `libraryLookupCost`
// charges, computed before either pass pays it:
//
//   - header pairs lexed — ONCE per stream, as far as the furthest member looked up (or, on a miss,
//     every pair `/N` claims, but never more than the decoded length can hold: past the header's
//     end the patched lexer reads the rest as one `(0, 0)` pair);
//   - decoded bytes — ONCE per stream, as far as the furthest member's offset (or, on a miss, the whole
//     stream), weighted to cover parsing the members as well as decoding them. These are also what the
//     reader HOLDS for the pass, so the byte term alone caps its memory (`maxDecodedObjStmBytes`);
//   - per lookup, every object stream visited on the way (the stream it names and its `/Extends`
//     chain), each a fresh read of that stream's dictionary — `lookupVisitWeight` a visit, plus
//     `lookupDictByteWeight` for each byte of the dictionary's own text. A long `/Extends` chain times
//     many members is the quadratic the patch did not remove, and it is charged here.
//
// Past the ceiling the document is refused before the sweep resolves anything and before the library
// is called: `errLookupCostCeiling`, which `Verify` routes to `Invalid` with `could-not-check`, as it
// does every sweep that cannot finish.
//
// **The figure is an upper bound on the reader's work, never an estimate below it.** A header this
// parser cannot read as plain `int int` pairs, a member the header does not list, and a `/First` past
// the header ceiling are all charged as a miss (every pair, the whole stream, and the `/Extends`
// chain); a `/Extends` cycle — on which the reader's loop never terminates — is refused outright.
//
// # The ceiling, measured
//
// One figure, in units of about a nanosecond of the patched reader's time on the development machine.
// The weights were fitted on the synthetic N-member files (1,000-20,000 members), `/Extends` chains
// (100x100 to 2,000x500) and the 36 files in `~/nib/producers`, timing only the in-stream lookups of a
// fresh Reader, best of three, and the figure is ABOVE the measured time on every one of them (1.1x to
// 3.6x; ADR-066 has the table). A lookup's fixed cost — a fresh read of the stream's dictionary and a
// buffer for the member — dominates (~4-10 µs), so the visit weight carries most of it; the byte weight
// covers parsing a member as well as decoding it (a 16 MiB array member: 1.2 s). InDesign's
// `census-p60-280.pdf`, the dearest real file, went from 3.2 s a pass to 0.50 s, at 0.21 of the ceiling;
// `maxLookupWork` is kept as the backstop it was, and a 1,000-stream chain under 1,000 members (6.2 s a
// pass) is past it. `TestNoProducerDocumentReachesTheLookupCeiling` holds the corpus under it.
//
// Declared: a member's own extent is not charged (a lookup reads the member, however large, and so
// did upstream); nor is anything a dictionary key REFERS to (`/Type 7 0 R` re-reads object 7 per
// lookup) — only the dictionary's own text is.
const (
	lookupPairWeight     = 200
	lookupByteWeight     = 100
	lookupVisitWeight    = 15_000
	lookupDictByteWeight = 50
	maxLookupWork        = 6_000_000_000
	// maxDecodedObjStmBytes is what the byte term alone admits, and so the most the patched reader can
	// hold decoded for a pass: about 57 MiB.
	maxDecodedObjStmBytes = maxLookupWork / lookupByteWeight
)

// errLookupCostCeiling refuses a document whose object streams would cost the signature reader more
// than `maxLookupWork`, or hold more than `maxDecodedObjStmBytes` decoded.
var errLookupCostCeiling = errors.New("the document's object streams are too costly for the signature reader to read")

// lookupCost is what one pass over the xref costs the patched reader.
type lookupCost struct {
	pairs     int64 // header pairs lexed, once per stream
	bytes     int64 // decoded bytes, once per stream
	visits    int64 // object streams visited by a lookup, each a fresh read of its dictionary
	dictBytes int64 // the text of those dictionaries
}

func (c lookupCost) work() int64 {
	return c.pairs*lookupPairWeight + c.bytes*lookupByteWeight + c.visits*lookupVisitWeight + c.dictBytes*lookupDictByteWeight
}

func (c lookupCost) over() bool {
	// Each term alone first, so no product can overflow before the sum is compared.
	return c.bytes > maxDecodedObjStmBytes || c.pairs > maxLookupWork/lookupPairWeight ||
		c.visits > maxLookupWork/lookupVisitWeight || c.dictBytes > maxLookupWork/lookupDictByteWeight ||
		c.work() > maxLookupWork
}

// objStmInfo is what one object stream costs: where each id sits in its header (first listing wins, as
// the reader takes it), the header's claimed `/N` and `/First`, its dictionary's size, how far the
// pass has been charged into it, and — decoded only when a lookup misses the header — its decoded length.
type objStmInfo struct {
	ok       bool // an `/Type /ObjStm` stream; otherwise the reader panics on the lookup and the sweep errors
	n, first int64
	at       map[uint32]objStmSlot
	dict     int64 // the dictionary's own text, re-read per visit
	length   int64
	lengthOK bool
	extends  dpdf.Value
	v        dpdf.Value
	// Charged so far: the reader lexes and decodes each stream once, as far as it has been asked.
	pairsDone, bytesDone int64
}

type objStmSlot struct{ pair, off int64 }

// charge adds what reaching pair p and decoded byte b of s costs beyond what is already charged.
func (s *objStmInfo) charge(c *lookupCost, p, b int64) {
	if p > s.pairsDone {
		c.pairs += p - s.pairsDone
		s.pairsDone = p
	}
	if b > s.bytesDone {
		c.bytes += b - s.bytesDone
		s.bytesDone = b
	}
}

// libraryLookupCost reports errLookupCostCeiling when resolving every xref object through r — what the
// sweep and the library each do — would cost the patched reader more than `maxLookupWork` or hold more
// than `maxDecodedObjStmBytes`. It resolves no member: it reads each object stream's dictionary once and
// decodes its header once, so its own cost is linear in the document.
func libraryLookupCost(r *dpdf.Reader) (c lookupCost, err error) {
	streams := map[uint32]*objStmInfo{}
	info := func(v dpdf.Value, id uint32) *objStmInfo {
		if s, ok := streams[id]; ok {
			return s
		}
		s := readObjStm(v)
		streams[id] = s
		return s
	}
	for _, x := range r.Xref() { //sigwalk:exempt libraryLookupCost
		sp := x.Stream()
		if sp.GetID() == 0 {
			continue
		}
		ptr := x.Ptr()
		id := ptr.GetID()
		sid := sp.GetID()
		s := info(r.Resolve(sp, sp), sid)
		visited := map[uint32]bool{}
		for {
			c.visits++
			c.dictBytes += s.dict
			if !s.ok {
				break // the reader panics here, and the sweep recovers it as an error
			}
			if visited[sid] {
				return c, fmt.Errorf("%w: object stream %d extends itself", errLookupCostCeiling, sid)
			}
			visited[sid] = true
			if slot, found := s.at[id]; found {
				s.charge(&c, slot.pair+1, satAdd(s.first, slot.off))
				break
			}
			// A miss: the reader lexes every pair /N claims — but past the header's end the rest read
			// as one (0, 0), so never more pairs than the decoded bytes hold — decodes to the end, and
			// follows `/Extends`.
			l := s.decodedLength()
			s.charge(&c, min(s.n, l/2+1), l)
			if c.over() {
				return c, errLookupCostCeiling
			}
			ext := s.extends
			if ext.Kind() != dpdf.Stream {
				break // the reader panics "cannot find object in stream"
			}
			ep := ext.GetPtr()
			sid = ep.GetID()
			s = info(ext, sid)
		}
		if c.over() {
			return c, errLookupCostCeiling
		}
	}
	return c, nil
}

// readObjStm decodes v's header once, through the reader's own decoder, as far as `/First` or the
// pair ceiling allows.
func readObjStm(v dpdf.Value) (s *objStmInfo) {
	s = &objStmInfo{at: map[uint32]objStmSlot{}, v: v, dict: int64(len(v.String()))}
	// A damaged filter panics in the reader's decoder: the sweep recovers the same panic, so here it
	// is only "not an object stream the cost can be read for".
	defer func() {
		if recover() != nil {
			s.ok = false
		}
	}()
	if v.Kind() != dpdf.Stream || v.Key("Type").Name() != "ObjStm" {
		return s
	}
	s.n, s.first = v.Key("N").Int64(), v.Key("First").Int64()
	s.extends = v.Key("Extends")
	s.ok = s.first != 0 // the reader panics "missing First"
	if !s.ok || s.n < 0 {
		s.n = max(s.n, 0)
		return s
	}
	// Read the header's pairs as the reader's loop does, stopping at the first token that is not a
	// plain non-negative integer — a token read differently is charged as a miss, the dearer case.
	hdrCap := min(s.first, maxDecodedObjStmBytes)
	br := bufio.NewReader(io.LimitReader(v.Reader(), hdrCap))
	for i := int64(0); i < s.n; i++ {
		idv, ok1 := readUint(br)
		off, ok2 := readUint(br)
		if !ok1 || !ok2 {
			return s
		}
		if _, dup := s.at[uint32(idv)]; !dup && idv <= 0xFFFFFFFF {
			s.at[uint32(idv)] = objStmSlot{pair: i, off: off}
		}
	}
	return s
}

// decodedLength is the stream's decoded length, read once and only when a lookup misses the header,
// capped just past the byte ceiling (anything longer is refused anyway).
func (s *objStmInfo) decodedLength() (n int64) {
	if s.lengthOK {
		return s.length
	}
	s.lengthOK = true
	defer func() {
		if recover() != nil {
			s.length = n
		}
	}()
	n, _ = io.Copy(io.Discard, io.LimitReader(s.v.Reader(), maxDecodedObjStmBytes+1))
	s.length = n
	return n
}

// readUint reads one white-space-delimited plain decimal integer.
func readUint(br *bufio.Reader) (int64, bool) {
	c, err := br.ReadByte()
	for err == nil && isPDFSpace(c) {
		c, err = br.ReadByte()
	}
	if err != nil || !isDigit(c) {
		return 0, false
	}
	var v int64
	for err == nil && isDigit(c) {
		if v > (1<<62)/10 {
			return 0, false
		}
		v = v*10 + int64(c-'0')
		c, err = br.ReadByte()
	}
	if err == nil && !isPDFSpace(c) {
		return 0, false
	}
	return v, true
}
