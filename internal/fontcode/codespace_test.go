package fontcode

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"
	"time"

	"nib/internal/scaling"
)

// refCodespace is the codespace as it stood before /pending 724 — every range in one list, `add` checking each
// against every earlier one and `Codes` scanning every range per byte — kept verbatim as the reference the index
// must agree with. It is veraPDF's own shape (`CMapParser.readLineCodeSpaceRange`, `CMap.getCodeFromStream`),
// so any disagreement is the index's error.
type refCodespace struct {
	spaces   []codeRange
	shortest int
	invalid  bool
}

func (c *refCodespace) add(lo, hi []byte) {
	r := codeRange{lo: lo, hi: hi}
	valid := len(lo) == len(hi)
	for i := 0; valid && i < len(lo); i++ {
		valid = lo[i] <= hi[i]
	}
	if !valid {
		r = codeRange{lo: []byte{}, hi: []byte{}}
	}
	for _, s := range c.spaces {
		if s.overlaps(r) {
			return
		}
	}
	if !valid {
		c.invalid = true
	}
	c.spaces = append(c.spaces, r)
	if c.shortest == 0 || len(lo) < c.shortest {
		c.shortest = len(lo)
	}
}

func (c *refCodespace) merge(o *refCodespace) {
	c.spaces = append(c.spaces, o.spaces...)
	c.invalid = c.invalid || o.invalid
}

func (c *refCodespace) codes(b []byte, fn func(code []byte, value int) bool) bool {
	pos := 0
	var code [5]byte
	for pos < len(b) {
		prev := c.shortest
		value, n, done := 0, 0, false
		for i := 0; i <= 4 && !done; i++ {
			if pos < len(b) {
				code[i] = b[pos]
				pos++
			} else {
				code[i] = 0xFF
			}
			n = i + 1
			cur := code[:i+1]
			for _, s := range c.spaces {
				if s.contains(cur) {
					if v := javaInt(numberFromBytes(cur)); v != -1 {
						value, done = v, true
					}
					break
				}
			}
			if done {
				break
			}
			shortestPartial := -1
		scan:
			for _, s := range c.spaces {
				match := true
				for j := 0; j <= i; j++ {
					if j >= len(s.lo) {
						break scan
					}
					if code[j] < s.lo[j] || code[j] > s.hi[j] {
						match = false
						break
					}
				}
				if match && (shortestPartial < 0 || len(s.lo) < shortestPartial) {
					shortestPartial = len(s.lo)
				}
			}
			if shortestPartial < 0 && len(c.spaces) > 0 {
				skip := prev - i - 1
				if skip < 0 {
					return false
				}
				if skip > len(b)-pos {
					skip = len(b) - pos
				}
				pos += skip
				value, done = 0, true
				break
			}
			prev = shortestPartial
		}
		if !fn(code[:n], value) {
			return true
		}
	}
	return true
}

func (c *refCodespace) clone() *refCodespace {
	o := *c
	o.spaces = append([]codeRange(nil), c.spaces...)
	return &o
}

func refIdentity() *refCodespace {
	return &refCodespace{spaces: []codeRange{{lo: []byte{0, 0}, hi: []byte{0xFF, 0xFF}}}, shortest: 2}
}

// cut renders a whole cut, so two readings compare as one string.
func cut(codes func([]byte, func([]byte, int) bool) bool, b []byte) string {
	var out strings.Builder
	if !codes(b, func(code []byte, v int) bool {
		fmt.Fprintf(&out, "%X=%d ", code, v)
		return true
	}) {
		out.WriteString("UNREAD")
	}
	return out.String()
}

// pair is one codespace built twice, through the index and through the reference, by the same operations.
type pair struct {
	got  *Codespace
	want *refCodespace
}

func (p pair) add(lo, hi []byte) { p.got.add(lo, hi); p.want.add(lo, hi) }

func (p pair) merge(o pair) { p.got.Merge(o.got); p.want.merge(o.want) }

// A small alphabet makes overlaps, shared prefixes and partial matches common rather than rare.
var alphabet = []byte{0x00, 0x01, 0x40, 0x41, 0x7F, 0x80, 0x81, 0xFE, 0xFF}

func randByte(r *rand.Rand) byte {
	if r.Intn(4) == 0 {
		return byte(r.Intn(256))
	}
	return alphabet[r.Intn(len(alphabet))]
}

func randRange(r *rand.Rand) ([]byte, []byte) {
	n := r.Intn(6) // 0..5, and 5 is past the longest code a reader cuts
	if r.Intn(10) == 0 {
		n = 6 + r.Intn(3)
	}
	m := n
	if r.Intn(25) == 0 { // ends of different lengths
		m = r.Intn(6)
	}
	lo, hi := make([]byte, n), make([]byte, m)
	for i := range lo {
		lo[i] = randByte(r)
	}
	for i := range hi {
		switch {
		case i < n && r.Intn(3) == 0:
			hi[i] = lo[i]
		case i < n && r.Intn(30) != 0: // rarely, a begin byte above its end
			hi[i] = lo[i] + byte(r.Intn(256-int(lo[i])))
		default:
			hi[i] = randByte(r)
		}
	}
	return lo, hi
}

func randPair(r *rand.Rand, depth int) pair {
	p := pair{&Codespace{}, &refCodespace{}}
	for k, ops := 0, r.Intn(12); k < ops; k++ {
		switch x := r.Intn(20); {
		case x == 0:
			p.merge(pair{Identity(), refIdentity()})
		case x == 1 && depth < 2:
			p.merge(randPair(r, depth+1))
		default:
			p.add(randRange(r))
		}
	}
	return p
}

func randShown(r *rand.Rand) []byte {
	b := make([]byte, r.Intn(14))
	for i := range b {
		b[i] = randByte(r)
	}
	return b
}

// TestTheCodespaceIndexCutsExactlyAsTheListDid — /pending 724. The index replaced a list veraPDF's own reader
// walks, so it must agree with that list on every observable: which ranges `add` keeps (and so what the index
// holds), the invalid flag, the shortest length, and every cut `Codes` makes — including the orders a list makes
// visible (a range that throws inside the partial scan ends it; a merge adds unchecked ranges after its own).
func TestTheCodespaceIndexCutsExactlyAsTheListDid(t *testing.T) {
	r := rand.New(rand.NewSource(724))
	for trial := 0; trial < 20000; trial++ {
		p := randPair(r, 0)
		if p.got.Invalid != p.want.invalid || p.got.shortest != p.want.shortest || p.got.ranges() != len(p.want.spaces) {
			t.Fatalf("trial %d: invalid/shortest/count %v/%d/%d, want %v/%d/%d", trial,
				p.got.Invalid, p.got.shortest, p.got.ranges(), p.want.invalid, p.want.shortest, len(p.want.spaces))
		}
		for q := 0; q < 20; q++ {
			b := randShown(r)
			if got, want := cut(p.got.Codes, b), cut(p.want.codes, b); got != want {
				t.Fatalf("trial %d: % X → %s, want %s\nranges: %v", trial, b, got, want, p.want.spaces)
			}
		}
		// What `add` keeps is observable through every later add: probe with ranges the reference would keep. A
		// clone shares the original's tries, so both are grown apart and both must still agree with their lists.
		c := pair{p.got.Clone(), p.want.clone()}
		for k := 0; k < 3; k++ {
			for _, x := range []pair{p, c} {
				lo, hi := randRange(r)
				x.add(lo, hi)
				if x.got.ranges() != len(x.want.spaces) {
					t.Fatalf("trial %d: after adding % X-% X the index holds %d ranges, the list %d", trial, lo, hi, x.got.ranges(), len(x.want.spaces))
				}
			}
		}
		for _, x := range []pair{p, c} {
			for q := 0; q < 5; q++ {
				b := randShown(r)
				if got, want := cut(x.got.Codes, b), cut(x.want.codes, b); got != want {
					t.Fatalf("trial %d, after a clone: % X → %s, want %s\nranges: %v", trial, b, got, want, x.want.spaces)
				}
			}
		}
	}
}

// bigCMap is the shape the P05 review measured: tens of thousands of four-byte ranges, each written as
// `<XXXXXXXX> <XXXXXXXX>`, none overlapping.
func bigCMap(n int) []byte {
	var src strings.Builder
	fmt.Fprintf(&src, "%d begincodespacerange\n", n)
	for i := 0; i < n; i++ {
		fmt.Fprintf(&src, "<%04X2000> <%04X7FFF>\n", i, i)
	}
	src.WriteString("endcodespacerange\n")
	return []byte(src.String())
}

// readCost returns a measurement of parsing an n-range CMap and cutting 8 KB of text with it; the fixture is built
// once per size, outside the clock.
func readCost(t *testing.T) func(n int) time.Duration {
	type fixture struct{ src, shown []byte }
	built := map[int]fixture{}
	return func(n int) time.Duration {
		f, ok := built[n]
		if !ok {
			f = fixture{bigCMap(n), make([]byte, 8192)}
			r := rand.New(rand.NewSource(1))
			for i := 0; i < len(f.shown); i += 4 {
				v := uint32(r.Intn(n))<<16 | uint32(0x2000+r.Intn(0x6000))
				if r.Intn(2) == 0 {
					v = r.Uint32() // mostly a miss, which is the partial scan's worst case
				}
				f.shown[i], f.shown[i+1], f.shown[i+2], f.shown[i+3] = byte(v>>24), byte(v>>16), byte(v>>8), byte(v)
			}
			built[n] = f
		}
		return scaling.TimeOnce(func() {
			cs := ParseCodespace(f.src)
			if cs.Overspent || cs.ranges() != n {
				t.Fatalf("an %d-range CMap kept %d of its disjoint ranges (overspent %v)", n, cs.ranges(), cs.Overspent)
			}
			cs.Codes(f.shown, func([]byte, int) bool { return true })
		})
	}
}

// TestAWideCodespaceIsReadInLinearTime — /pending 724's own case. A 32k-range CMap took seconds to parse and to cut
// 8 KB with, because `add` compared each range with every earlier one and `Codes` scanned every range per byte.
//
// Bounded by SCALING, not by the clock, because the clock is the machine's load: at load ~10 the list read 32k ranges
// and 8 KB in 4.2 s and at load ~30 in 12.9 s, while the index reads them in ~0.16 s. Eight times the ranges cost the
// list ~64 times the time and cost the index ~8; the bound is 20. Measured one size after the other it ranged
// ×5.7-×12.6 at load ~15, so the sizes are interleaved through `scaling` (/pending 799).
func TestAWideCodespaceIsReadInLinearTime(t *testing.T) {
	cost := readCost(t)
	scaling.GrowsLinearly(t, "codespace read", 4096, 32768, 20, cost)

	// The shape the tries are quadratic in, as veraPDF's list is: narrow ranges fanning out into distinct subtrees,
	// then wide ones spanning every one of them. Refused by the budget, in bounded time, rather than read for
	// minutes (45 s unbudgeted at this size and load ~30, measured). "Bounded" is against the honest 32k-range read in the
	// same rounds: an absolute 6 s took 9.96 s at load ~8 and 1.15 s alone (/pending 799).
	var adv strings.Builder
	fmt.Fprintf(&adv, "%d begincodespacerange\n", 2*4096)
	for i := 0; i < 4096; i++ {
		fmt.Fprintf(&adv, "<%04X%02X%02X> <%04X%02X%02X>\n", i*3, i%128, i/128, i*3, i%128, i/128)
	}
	for i := 0; i < 4096; i++ {
		fmt.Fprintf(&adv, "<0000%02X%02X> <FFFF%02X%02X>\n", 0x80+i/256, i%256, 0x80+i/256, i%256)
	}
	adv.WriteString("endcodespacerange\n")
	advSrc, wide := []byte(adv.String()), (*Codespace)(nil)
	scaling.WithinFactor(t, "refusing an overspent codespace", 12,
		func() time.Duration { return cost(32768) },
		func() time.Duration { return scaling.TimeOnce(func() { wide = ParseCodespace(advSrc) }) })
	if !wide.Overspent {
		t.Fatalf("a codespace quadratic in the index was read whole (%d ranges, work %d)", wide.ranges(), wide.work)
	}
	// Refused wherever it travels: into a chain, and into the clone a chain starts from.
	chain := ParseCodespace([]byte("1 begincodespacerange <00> <7F> endcodespacerange")).Clone()
	chain.Merge(wide)
	if !chain.Overspent || !wide.Clone().Overspent {
		t.Fatal("an overspent codespace stopped being refused once merged or cloned")
	}
	// The budget spent INSIDE an overlap check: the walk stops, and a range it stopped on is refused with the
	// codespace — never dropped as though it overlapped, which is an answer.
	near := ParseCodespace([]byte("2 begincodespacerange <0000> <00FF> <0100> <01FF> endcodespacerange"))
	near.work = maxCodespaceWork
	near.add([]byte{0x02, 0x00}, []byte{0x02, 0xFF}) // overlaps nothing, and the walk's first node spends the budget
	if !near.Overspent {
		t.Fatalf("a budget spent inside an overlap check read as an overlap (%d ranges, work %d)", near.ranges(), near.work)
	}
	// The one list order the index keeps is its groups, and a repeated `/Identity-H usecmap` would make one per
	// operator — every code then walked 40,000 times. A group already held is held once.
	rep := ParseCodespace([]byte(strings.Repeat("/Identity-H usecmap\n", 40000)))
	if len(rep.groups) != 1 || rep.ranges() != 40000 {
		t.Fatalf("40,000 Identity merges: %d groups for %d ranges, want 1 group", len(rep.groups), rep.ranges())
	}
}

// TestAnOverlongCodespaceRangeIsRefused — the tries recurse once per byte of a range, so one megabyte-long range
// overflowed the goroutine stack (fatal: no `recover` holds it) on a CMap that compresses to 4 KB. It is refused by
// name, in bounded time, wherever the codespace travels; a range at the bound still reads.
func TestAnOverlongCodespaceRangeIsRefused(t *testing.T) {
	const n = 1 << 20
	long := ParseCodespace([]byte("1 begincodespacerange <" + strings.Repeat("00", n) + "> <" + strings.Repeat("FF", n) +
		"> endcodespacerange"))
	if !long.Overlong || long.ranges() != 0 {
		t.Fatalf("a %d-byte range was read (overlong %v, %d ranges)", n, long.Overlong, long.ranges())
	}
	if c := long.Clone(); !c.Overlong {
		t.Fatal("an overlong codespace stopped being refused once cloned")
	}
	chain := ParseCodespace([]byte("1 begincodespacerange <00> <7F> endcodespacerange")).Clone()
	chain.Merge(long)
	if !chain.Overlong {
		t.Fatal("an overlong codespace stopped being refused once merged")
	}
	atBound := ParseCodespace([]byte("1 begincodespacerange <" + strings.Repeat("00", maxCodeRangeBytes) + "> <" +
		strings.Repeat("FF", maxCodeRangeBytes) + "> endcodespacerange"))
	if atBound.Overlong || atBound.ranges() != 1 {
		t.Fatalf("a %d-byte range was refused (overlong %v, %d ranges)", maxCodeRangeBytes, atBound.Overlong, atBound.ranges())
	}
}
