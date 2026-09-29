package sign

import (
	"bufio"
	"errors"
	"fmt"
	"io"

	dpdf "github.com/digitorus/pdf"
)

// # The signature reader re-reads an object stream for every member it resolves (/pending 751)
//
// `digitorus/pdf`'s `resolve` (`read.go:874-905`) finds an object-stream member by decoding its stream
// FROM THE START and lexing the header's `id offset` pairs until the id matches, then reading forward to
// the member — every time, with nothing cached. Both the revision sweep and the library's own verify
// pass resolve EVERY xref object (ADR-058: they are the same enumeration), so a stream of N members costs
// N²/2 header pairs per pass. Measured on a 561 KB signed file with one 20,000-member stream: the sweep
// 60 s and `Verify` 129 s; pdfcpu reads it in milliseconds.
//
// **The quadratic is inside the library and not reachable from here.** The sweep could decode each
// stream once, but the library's pass — which `Verify` must run — would still pay it, and the library
// takes a byte reader, not a parse. So the cost is COMPUTED before either pays it: `libraryLookupCost`
// walks the xref once, decodes each object stream's header once, and adds up what the library's
// `resolve` would read — header pairs lexed, and decoded bytes read up to each member. Past the
// ceiling the document is refused before the sweep resolves anything and before the library is called:
// `errLookupCostCeiling`, which `Verify` routes to `Invalid` with `could-not-check`, as it does every
// sweep that cannot finish.
//
// **The figure is an upper bound on the library's work, never an estimate below it.** A header this
// parser cannot read as plain `int int` pairs, a member the header does not list, and a `/First` past
// the header ceiling are all charged as the library's worst case for that lookup (the whole header and
// the whole stream, and its `/Extends` chain); a `/Extends` cycle — on which the library's loop never
// terminates — is refused outright.
//
// # The ceiling, measured
//
// One figure, in units of about a nanosecond of the library's time on the development machine: a
// lexed header pair weighs `lookupPairWeight` and a decoded byte read on the way to a member
// `lookupByteWeight`. Fitted on the corpus and checked against it — the 36 files in `~/nib/producers`
// resolve in the time the figure predicts to within ~5% on the dearest of them, InDesign's
// `census-p60-280.pdf` (2.7 MB; 1.8 M pairs and 310 MB read; predicted 3.3 s a pass, measured 3.2 s).
// That file is why the ceiling is not lower: it is an honest document, and it already costs the verdict
// two such passes (the sweep, the library). `maxLookupWork` admits it with ~1.8x to spare and refuses
// everything past about six seconds a pass; `TestNoProducerDocumentReachesTheLookupCeiling` holds the
// corpus under it. A 100-member stream, the common producer choice, weighs ~1.5 M units.
//
// **This is a bound, not a fix, and the fix is declared residue**: the library caching each decoded
// object stream per `Reader` takes `census-p60-280.pdf` from 3.2 s to 0.53 s a pass and a 4,000
// member stream from 2.3 s to 59 ms (measured on a patched copy), but it is a change to a
// third-party module, which nib does not carry. Signing meets the same ceiling: `signedAsIntended`
// sweeps its input, so a document past it is refused `ErrSignedOutputUnreadable`.
//
// Declared: the object stream's own DICTIONARY is re-parsed per lookup too, and is not charged — it is
// a few keys in every producer's output.
const (
	lookupPairWeight = 300
	lookupByteWeight = 9
	maxLookupWork    = 6_000_000_000
)

// errLookupCostCeiling refuses a document whose object streams would cost the signature reader more
// than `maxLookupWork`.
var errLookupCostCeiling = errors.New("the document's object streams are too costly for the signature reader to read")

// objStmInfo is what one object stream costs a lookup: where each id sits in its header (first listing
// wins, as the library's loop takes it), the header's claimed `/N` and `/First`, and — decoded only when a
// lookup misses the header — the stream's decoded length.
type objStmInfo struct {
	ok       bool // an `/Type /ObjStm` stream; otherwise the library panics on the lookup and the sweep errors
	n, first int64
	at       map[uint32]objStmSlot
	length   int64
	lengthOK bool
	extends  dpdf.Value
	v        dpdf.Value
}

type objStmSlot struct{ pair, off int64 }

// libraryLookupCost reports errLookupCostCeiling when resolving every xref object through r — what the
// sweep and the library each do — would cost the library more than `maxLookupWork`. It resolves no member: it reads each object stream's dictionary once and decodes its
// header once, so its own cost is linear in the document.
func libraryLookupCost(r *dpdf.Reader) (pairs, bytesRead int64, err error) {
	streams := map[uint32]*objStmInfo{}
	info := func(v dpdf.Value, id uint32) *objStmInfo {
		if s, ok := streams[id]; ok {
			return s
		}
		s := readObjStm(v)
		streams[id] = s
		return s
	}
	over := func() bool {
		return pairs > maxLookupWork/lookupPairWeight || bytesRead > maxLookupWork/lookupByteWeight ||
			pairs*lookupPairWeight+bytesRead*lookupByteWeight > maxLookupWork
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
			if !s.ok {
				break // the library panics here, and the sweep recovers it as an error
			}
			if visited[sid] {
				return pairs, bytesRead, fmt.Errorf("%w: object stream %d extends itself", errLookupCostCeiling, sid)
			}
			visited[sid] = true
			if slot, found := s.at[id]; found {
				pairs += slot.pair + 1
				bytesRead += satAdd(s.first, slot.off)
				break
			}
			// A miss: the library lexes all N pairs, reads to the end, and follows `/Extends`.
			pairs += s.n
			bytesRead += s.decodedLength()
			if over() {
				return pairs, bytesRead, errLookupCostCeiling
			}
			ext := s.extends
			if ext.Kind() != dpdf.Stream {
				break // the library panics "cannot find object in stream"
			}
			ep := ext.GetPtr()
			sid = ep.GetID()
			s = info(ext, sid)
		}
		if over() {
			return pairs, bytesRead, errLookupCostCeiling
		}
	}
	return pairs, bytesRead, nil
}

// readObjStm decodes v's header once, through the library's own decoder, as far as `/First` or the
// pair ceiling allows.
func readObjStm(v dpdf.Value) (s *objStmInfo) {
	s = &objStmInfo{at: map[uint32]objStmSlot{}, v: v}
	// A damaged filter panics in the library's decoder: the sweep recovers the same panic, so here it
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
	s.ok = s.first != 0 // the library panics "missing First"
	if !s.ok || s.n < 0 {
		s.n = max(s.n, 0)
		return s
	}
	// Read the header's pairs as the library's loop does, stopping at the first token that is not a
	// plain non-negative integer — a token read differently is charged as a miss, the dearer case.
	hdrCap := min(s.first, maxLookupWork/lookupByteWeight)
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
	n, _ = io.Copy(io.Discard, io.LimitReader(s.v.Reader(), maxLookupWork/lookupByteWeight+1))
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
