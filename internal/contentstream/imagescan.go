package contentstream

import (
	"bytes"
	"sort"
)

// imageScanner carries, across one Tokenize pass, what the inline-image end rules have already learnt about
// the stream, so that no image pays again for a search an earlier one made (`/pending 817`).
//
// # Why it exists
//
// Two of the end rules ask a question whose answer lies OUTSIDE the image: the ASCII filters' rule asks
// where the next `>` or `~>` is, and `imageEndsAt` asks where the whitespace after a proposed end stops.
// Asked afresh per image, each walked the rest of the stream whenever the answer was far away or absent —
// and an attacker chooses both. Measured before this existed: 3 MB of `BI /F /AHx ID 00 EI` with no `>`
// took 4.8 s (0.1 s unfiltered); one `>` followed by 1.5 MB of whitespace and no `EI`, named by every
// image, took 2 m 20 s; a `/L` per image pointing into one far whitespace run took 3 m 18 s. A content
// stream Flate-compresses that to a few KB and is bounded only by the 512 MiB page content cap.
//
// **It changes what the searches COST, never what they answer.** Every method returns exactly what the
// direct search it replaced returned; `TestTheMemoisedImageEndsAgreeWithTheDirectSearch` holds that
// against the direct search on varied streams.
type imageScanner struct {
	src []byte

	hexEOD eodMemo // ASCIIHexDecode's `>`
	a85EOD eodMemo // ASCII85Decode's `~>`

	// runs is every maximal whitespace run of at least longRun bytes, in stream order, built on first need.
	runs      []wsRun
	runsBuilt bool

	// searched counts the bytes the searches above examined. Read only by tests: it is the work count the
	// linear-cost test asserts, which a wall clock on a loaded machine could not.
	searched int
}

// eodMemo remembers the last answer to "where is the first `marker` at or after `from`": `at`, or -1 for
// none.
//
// One answer covers every later question up to it. No marker STARTS in [from, at), so for any from' in
// [from, at] the first one at or after from' is still `at`; and when there is none after `from` there is
// none after any later offset. The images ask in stream order — each one's data starts past the previous
// image's end — so each byte is searched at most once per marker.
type eodMemo struct {
	marker   []byte
	from, at int
	valid    bool
}

// newImageScanner is the scanner for one stream, its memos empty.
func newImageScanner(src []byte) *imageScanner {
	return &imageScanner{src: src, hexEOD: eodMemo{marker: hexEODMarker}, a85EOD: eodMemo{marker: a85EODMarker}}
}

var hexEODMarker, a85EODMarker = []byte(">"), []byte("~>")

// wsRun is a maximal whitespace run [start, end): src[start-1] (if any) and src[end] (if any) are not
// whitespace.
type wsRun struct{ start, end int }

// longRun is the whitespace a skip walks byte by byte before it asks the run index instead. It bounds
// each image's share of a skip to longRun bytes — a few times the smallest image that can ask — and the
// index to one 16-byte entry per longRun+1 bytes of stream, less than the whitespace tokens those same bytes
// already cost. It is built only by a stream holding such a run where an image's end is looked for.
const longRun = 64

// nextEOD is the offset of the first m.marker at or after `from`, or -1.
func (s *imageScanner) nextEOD(m *eodMemo, from int) int {
	if m.valid && from >= m.from && (m.at < 0 || from <= m.at) {
		return m.at
	}
	at := -1
	if from <= len(s.src) {
		if i := bytes.Index(s.src[from:], m.marker); i >= 0 {
			at = from + i
			s.searched += i + len(m.marker)
		} else {
			s.searched += len(s.src) - from
		}
	}
	*m = eodMemo{marker: m.marker, from: from, at: at, valid: true}
	return at
}

// skipWhite is the first offset at or after p that is not whitespace, or len(src).
//
// Up to longRun bytes are walked directly — the ordinary case, a newline or two before `EI`. A skip that
// has walked that far is inside a maximal run of at least longRun bytes, and every such run is in the
// index, so its end is read there instead of walked to.
func (s *imageScanner) skipWhite(p int) int {
	src := s.src
	limit := min(len(src), p+longRun)
	m := p
	for m < limit && isWhite(src[m]) {
		m++
	}
	s.searched += m - p
	if m < limit || m == len(src) {
		return m
	}
	if !s.runsBuilt {
		s.buildRuns()
	}
	k := sort.Search(len(s.runs), func(k int) bool { return s.runs[k].end > p })
	if k < len(s.runs) && s.runs[k].start <= p {
		return s.runs[k].end
	}
	// Unreachable: longRun whitespace bytes from p lie inside one maximal run of at least that length. Walk
	// rather than trust the index, so a broken invariant costs time and never a wrong answer.
	for m < len(src) && isWhite(src[m]) {
		m++
	}
	return m
}

// buildRuns indexes every maximal whitespace run of at least longRun bytes: one pass over the stream.
func (s *imageScanner) buildRuns() {
	s.runsBuilt = true
	src := s.src
	for i := 0; i < len(src); {
		if !isWhite(src[i]) {
			i++
			continue
		}
		j := i
		for j < len(src) && isWhite(src[j]) {
			j++
		}
		if j-i >= longRun {
			s.runs = append(s.runs, wsRun{i, j})
		}
		i = j
	}
	s.searched += len(src)
}
