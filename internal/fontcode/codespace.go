// SPDX-License-Identifier: MPL-2.0 OR AGPL-3.0-only
//
// Parts of this file are derived from veraPDF 1.30.2 (veraPDF-validation and veraPDF-parser),
// Copyright (c) 2015-2026 veraPDF Consortium, which is offered under GPLv3+ or MPLv2+. nib takes the
// MPL-2.0 option: this file is available under MPL-2.0, and is distributed as part of nib under the
// AGPL-3.0 (MPL 2.0 section 3.3). See THIRD-PARTY-NOTICES.md, "veraPDF".

package fontcode

// Codespace cuts a Type 0 font's shown bytes into character codes, as veraPDF-parser's
// `CMap.getCodeFromStream` does.
//
// A simple font reads one byte per code and needs no Codespace (`PDFont.readCode`).
//
// # An index, not veraPDF's list (`/pending 724`)
//
// veraPDF keeps its ranges in one list: `readLineCodeSpaceRange` compares each new range with every earlier one,
// and `getCodeFromStream` scans every range per byte — 9.6 s to parse a 32k-range CMap and 3.3 s per 8 KB shown
// (measured). nib keeps the same ranges as byte TRIES and answers every question the list answers, with the
// list's answers; `TestTheCodespaceIndexCutsExactlyAsTheListDid` holds the old list as its reference.
//
// The list's ORDER is observable in exactly one place, and the tries carry it as the order of `groups`: the
// partial scan ends at the first range the code has run past (see `Codes`). Within one group no two ranges
// overlap — every range a program declares was checked against all before it, and a merged CMap's were checked
// against their own — so a code matches either ONE range at most its own length, or only ranges longer than it.
// Order inside a group therefore decides nothing, and a group is a prefix-free set of byte strings: a trie.
type Codespace struct {
	// groups are the list's runs of mutually non-overlapping ranges, in list order: a program's own ranges up to
	// a merge, the merged CMap's groups, the program's ranges after it. A group already held is not held twice,
	// which no answer can see: an earlier copy answers first and identically.
	groups []*csNode
	// tailOwn is whether the last group is this Codespace's own, the one `add` extends. A merge ends it.
	tailOwn bool
	// count is the list's length, duplicates and kept invalid ranges included: `Codes` asks only whether it is 0.
	count    int
	shortest int
	// Invalid is a range veraPDF keeps with no bytes (its two ends differ in length, or a begin byte is
	// above its end byte): checking a partial match against it throws inside the reader, and nib does not
	// model where that leaves the stream. A caller treats such a font's codes as unread.
	Invalid bool
	// UsesCMaps names every other CMap (`/Name usecmap`) whose ranges veraPDF adds AT that point of the parse, in
	// order. Identity is merged here; any other name is the caller's to resolve — ALL of them, since a CMap may use
	// several and veraPDF merges each.
	UsesCMaps []string
	// Malformed is a CMap veraPDF-parser throws on, and so reads as holding no range at all.
	Malformed bool
	// Overspent is a codespace whose ranges cost the index more than `maxCodespaceWork` to hold. Its answers are
	// NOT veraPDF's — ranges past the budget were not read — so a caller must refuse it rather than cut with it.
	Overspent bool
	// Overlong is a codespace range longer than `maxCodeRangeBytes`. The tries recurse once per byte of a range, so
	// one megabyte-long range overflowed the goroutine's stack — fatal, not a panic, so no `recover` held it — on a
	// CMap that compresses to 4 KB (measured). Its answers are not veraPDF's, so a caller refuses it.
	Overlong bool
	// work is what the index has spent so far, in trie nodes visited and intervals rebuilt.
	work int
}

// maxCodeRangeBytes bounds a codespace range's length. ISO 32000-1 9.7.6.2 caps a code at four bytes and nib's
// `bfrange` reader already refuses past four; this bound is looser, so no range a real font writes is refused, and
// tight enough that the tries' recursion is shallow.
const maxCodeRangeBytes = 32

// maxCodespaceWork bounds what one codespace may cost to index (`/pending 724`). The tries answer as veraPDF's list
// does and are near-linear for every CMap shape seen in practice — 32,768 disjoint four-byte ranges spend ~2/5 of it
// — but a range that is wide at its leading bytes must be checked against, and inserted under, every distinct
// subtree it spans, so narrow ranges that fan out followed by wide ones that span them are quadratic here as they
// are in veraPDF (measured: 4,096 of each, 45 s unbudgeted at load ~30). No real CMap is shaped so; one that is, is refused by name rather
// than read for minutes.
const maxCodespaceWork = 1 << 24

type codeRange struct{ lo, hi []byte }

func (r codeRange) contains(code []byte) bool {
	if len(code) != len(r.lo) {
		return false
	}
	for i, c := range code {
		if c < r.lo[i] || c > r.hi[i] {
			return false
		}
	}
	return true
}

// overlaps is `CodeSpace.overlaps`: over the shorter length, no byte position is disjoint.
func (r codeRange) overlaps(o codeRange) bool {
	n := len(r.lo)
	if len(o.lo) < n {
		n = len(o.lo)
	}
	for i := 0; i < n; i++ {
		b1, e1, b2, e2 := r.lo[i], r.hi[i], o.lo[i], o.hi[i]
		if (b2 > e1 && e2 > e1) || (b2 < b1 && e2 < b1) {
			return false
		}
	}
	return true
}

// csNode is one byte position of a group's trie. A node where a range ENDS is terminal (`term` is its length,
// and it has no children, the group being prefix-free); any other node's children are `segs`, disjoint byte
// intervals in order, with a gap meaning no range continues there. `min` is the shortest range through it.
//
// A child may be reached by more than one interval (a range inserted into the middle of an interval splits it);
// such a node is `shared` and is copied before anything is inserted into it. Nodes a merge hands to another
// Codespace are never inserted into again (`tailOwn`), so sharing across Codespaces needs no flag.
type csNode struct {
	term   int
	min    int
	shared bool
	segs   []csSeg
}

type csSeg struct {
	lo, hi byte
	child  *csNode
}

// child is the node byte v leads to, or nil.
func (n *csNode) child(v byte) *csNode {
	lo, hi := 0, len(n.segs)
	for lo < hi {
		m := (lo + hi) / 2
		switch s := n.segs[m]; {
		case v < s.lo:
			hi = m
		case v > s.hi:
			lo = m + 1
		default:
			return s.child
		}
	}
	return nil
}

func (n *csNode) clone() *csNode {
	c := &csNode{term: n.term, min: n.min, segs: append([]csSeg(nil), n.segs...)}
	for _, s := range c.segs {
		s.child.shared = true
	}
	return c
}

// csMemo is one insert's answers for the children several intervals can reach — a shared node, or the empty one
// at a depth — so each is inserted into once. A MAP: one wide range reaches every child of every node it spans,
// and a list searched per child made that quadratic in the children (4,096 of them took 97 s, measured).
type csMemo map[csMemoKey]*csNode

type csMemoKey struct {
	in *csNode
	d  int
}

// insert adds the range lo-hi from byte position d, into n (nil for an empty node), and returns the node. n must
// not be shared. The range must overlap nothing already there — the caller checked.
func (n *csNode) insert(lo, hi []byte, d int, memo csMemo, work *int) *csNode {
	if d == len(lo) {
		return &csNode{term: d, min: d}
	}
	var segs []csSeg
	if n != nil {
		segs = n.segs
	}
	*work += len(segs) + 1
	a, b := int(lo[d]), int(hi[d])
	out := make([]csSeg, 0, len(segs)+3)
	emit := func(l, h int, ch *csNode) {
		if ch == nil || l > h {
			return
		}
		if k := len(out) - 1; k >= 0 && out[k].child == ch && int(out[k].hi)+1 == l {
			out[k].hi = byte(h)
			return
		}
		out = append(out, csSeg{byte(l), byte(h), ch})
	}
	// below is what an interval inside lo[d]-hi[d] leads to once the range is inserted under it. An interval that
	// straddles the range keeps its old child outside, so that child is copied rather than changed; one child
	// reached through several intervals is inserted into once and the result shared.
	below := func(ch *csNode, straddles bool) *csNode {
		// A child neither shared nor empty is reached through this one interval only, and is never asked for again.
		reused := ch == nil || ch.shared
		key := csMemoKey{ch, d + 1}
		if reused {
			if r, ok := memo[key]; ok {
				r.shared = true
				return r
			}
		}
		t := ch
		if ch != nil && (ch.shared || straddles) {
			t = ch.clone()
		}
		r := t.insert(lo, hi, d+1, memo, work)
		if reused {
			memo[key] = r
		}
		return r
	}
	piece := func(l, h int, ch *csNode) {
		ml, mh := max(l, a), min(h, b)
		if ml > mh {
			emit(l, h, ch)
			return
		}
		left, right := l < ml, mh < h
		if left && right && ch != nil {
			ch.shared = true // it stays on both sides
		}
		if left {
			emit(l, ml-1, ch)
		}
		emit(ml, mh, below(ch, left || right))
		if right {
			emit(mh+1, h, ch)
		}
	}
	x := 0
	for _, s := range segs {
		if int(s.lo) > x {
			piece(x, int(s.lo)-1, nil)
		}
		piece(int(s.lo), int(s.hi), s.child)
		x = int(s.hi) + 1
	}
	if x <= 255 {
		piece(x, 255, nil)
	}
	if n == nil {
		n = &csNode{term: -1}
	}
	n.segs = out
	n.min = -1
	for _, s := range out {
		if n.min < 0 || s.child.min < n.min {
			n.min = s.child.min
		}
	}
	return n
}

// overlapsAny is whether some range under n overlaps lo-hi from byte position d: every position both have is
// shared, so the walk meets a range's end, or reaches the range's own end with a longer range still going.
func (n *csNode) overlapsAny(lo, hi []byte, d int, cleared map[*csNode]bool, work *int) bool {
	if n == nil {
		return false
	}
	if n.term >= 0 || d == len(lo) {
		return true
	}
	if cleared[n] {
		return false
	}
	if *work++; *work > maxCodespaceWork {
		return true // the caller sees the budget spent and keeps nothing
	}
	a, b := lo[d], hi[d]
	// The first interval ending at or after a.
	i, j := 0, len(n.segs)
	for i < j {
		if m := (i + j) / 2; n.segs[m].hi < a {
			i = m + 1
		} else {
			j = m
		}
	}
	for ; i < len(n.segs) && n.segs[i].lo <= b; i++ {
		if n.segs[i].child.overlapsAny(lo, hi, d+1, cleared, work) {
			return true
		}
	}
	cleared[n] = true
	return false
}

// identityRoot is `<0000> <FFFF>`'s trie. Nodes are never inserted into once another Codespace holds them, so
// every Identity shares it — which is also what lets a repeated `/Identity-H usecmap` be held once.
var identityRoot = func() *csNode {
	memo := csMemo{}
	var work int
	return (*csNode)(nil).insert([]byte{0, 0}, []byte{0xFF, 0xFF}, 0, memo, &work)
}()

// Identity is Identity-H's and Identity-V's one range, `<0000> <FFFF>`.
func Identity() *Codespace {
	return &Codespace{groups: []*csNode{identityRoot}, count: 1, shortest: 2}
}

// ranges is the list veraPDF would hold's length.
func (c *Codespace) ranges() int { return c.count }

// add is `CMapParser.readLineCodeSpaceRange`: a range overlapping one already read is dropped, with a
// warning, rather than added.
func (c *Codespace) add(lo, hi []byte) {
	r := codeRange{lo: lo, hi: hi}
	valid := len(lo) == len(hi)
	for i := 0; valid && i < len(lo); i++ {
		valid = lo[i] <= hi[i]
	}
	if !valid {
		r = codeRange{lo: []byte{}, hi: []byte{}}
	}
	if c.Overspent || c.Overlong {
		return
	}
	if len(lo) > maxCodeRangeBytes || len(hi) > maxCodeRangeBytes {
		c.Overlong = true
		return
	}
	cleared := map[*csNode]bool{}
	for _, g := range c.groups {
		if g.overlapsAny(r.lo, r.hi, 0, cleared, &c.work) {
			c.Overspent = c.work > maxCodespaceWork
			return
		}
	}
	if !valid {
		c.Invalid = true
	}
	memo := csMemo{}
	if c.tailOwn {
		k := len(c.groups) - 1
		c.groups[k] = c.groups[k].insert(r.lo, r.hi, 0, memo, &c.work)
	} else {
		c.groups = append(c.groups, (*csNode)(nil).insert(r.lo, r.hi, 0, memo, &c.work))
		c.tailOwn = true
	}
	c.Overspent = c.work > maxCodespaceWork
	c.count++
	if c.shortest == 0 || len(lo) < c.shortest {
		c.shortest = len(lo)
	}
}

// Merge adds another CMap's ranges, as `CMap.useCMap` does — with no overlap check, and WITHOUT touching the
// shortest length, which only a CMap's own `codespacerange` sets (`CMapParser.java:179`). The shortest decides how
// far a byte no range admits is skipped, so a merge that moved it cut the string differently (measured).
//
// The merged groups go after this CMap's, and this CMap's next range starts a group after them; o's own last
// group is closed too, because both now hold its trie and neither may insert into it.
func (c *Codespace) Merge(o *Codespace) {
next:
	for _, g := range o.groups {
		for _, h := range c.groups {
			if h == g {
				continue next
			}
		}
		c.groups = append(c.groups, g)
	}
	c.tailOwn, o.tailOwn = false, false
	c.count += o.count
	c.Invalid = c.Invalid || o.Invalid
	c.Overspent = c.Overspent || o.Overspent
	c.Overlong = c.Overlong || o.Overlong
}

// Clone is a copy a caller may Merge into without touching the original — **its shortest length included**, which is
// why a chain is merged into a clone of the font's OWN CMap and never into an empty Codespace: an empty one's shortest
// is unset, so a byte no range admits refused where veraPDF skips it as code 0 (the P07 phase-close review, R3-7).
//
// The two share their tries, so NEITHER may insert into its last group again: each one's next range starts a group
// of its own, which no answer can see (a later range that could reorder against an earlier group's overlaps it and
// is dropped). That is the only field of the original a clone touches.
func (c *Codespace) Clone() *Codespace {
	c.tailOwn = false
	o := *c
	o.groups = append([]*csNode(nil), c.groups...)
	o.UsesCMaps = append([]string(nil), c.UsesCMaps...)
	return &o
}

// ParseCodespace reads an embedded CMap program's `codespacerange` lists, with veraPDF's count rule.
func ParseCodespace(src []byte) *Codespace {
	t := newCMapTokens(src)
	c := &Codespace{}
	ok := t.lists(func(key string) bool {
		if key == "codespacerange" {
			lo, a := t.hex()
			hi, b := t.hex()
			if a && b {
				c.add(lo, hi)
			}
			return a && b
		}
		return t.skipEntry(key)
	}, func(name string) {
		// **Merged where the operator stands**, so every range the program declares AFTER it is overlap-checked
		// against the used CMap's: after `/Identity-H usecmap`, whose one range overlaps every one- or two-byte
		// range, the program's own ranges are dropped (`CMapParser.java:102-105, 170-181`).
		if name == "Identity-H" || name == "Identity-V" {
			c.Merge(Identity())
			return
		}
		c.UsesCMaps = append(c.UsesCMaps, name)
	})
	c.Malformed = !ok
	return c
}

// Codes calls fn with each code shown by b, in order, until fn returns false; `code` is a view the caller must
// copy to keep. It returns false where veraPDF's own reader cannot cut the string — its skip would allocate a
// negative or unbounded array, or the code is the -1 its reader cannot return — so the caller reads no guess.
//
// **This is veraPDF's reading, not the specification's**, and it differs in two places a lenient reader
// would not: bytes matching no range are skipped as far as the shortest range that last partially matched
// (the CMap's own shortest at the first byte), and yield code 0; and a code the string ends inside is
// completed with 0xFF bytes, because `InputStream.read` returns -1 there and the reader stores it as a byte.
// Both are glyphs veraPDF judges, so both are glyphs here. A code is veraPDF's Java `int` (`javaInt`).
func (c *Codespace) Codes(b []byte, fn func(code []byte, value int) bool) bool {
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
			exact, shortestPartial := c.match(cur)
			if exact {
				if v := javaInt(numberFromBytes(cur)); v != -1 {
					value, done = v, true
				}
			}
			if done {
				break
			}
			if shortestPartial < 0 && c.count > 0 {
				// `new byte[previous - i - 1]`: with no range of the CMap's own, `previous` is Integer.MAX_VALUE (0 here,
				// so the skip is negative), and past a -1 code it goes negative too — veraPDF throws either way.
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

// match is the list's two scans of one code, answered from the tries.
//
// `exact` is whether some range of the code's own length contains it (`CodeSpace.isFullMatch`, over the whole
// list). `shortestPartial` is the shortest range, at least the code's length, whose first bytes all admit the
// code — over the list only as far as the first range SHORTER than the code whose bytes the code matches:
// `CodeSpace.isPartialMatch` throws at a position past a range's length, and the throw ends the scan. -1 is none.
//
// Within a group such a shorter range is the only range the code matches, so the scan's end is the first group
// holding one, and a group's answer is its trie walked as far as the code goes.
func (c *Codespace) match(code []byte) (exact bool, shortestPartial int) {
	shortestPartial = -1
	ended := false
	for _, n := range c.groups {
		d := 0
		for ; n != nil && n.term < 0 && d < len(code); d++ {
			n = n.child(code[d])
		}
		switch {
		case n == nil:
			continue
		case n.term >= 0 && d < len(code): // a shorter range the code has run past
			ended = true
			continue
		case n.term >= 0:
			exact = true
		}
		if !ended && (shortestPartial < 0 || n.min < shortestPartial) {
			shortestPartial = n.min
		}
	}
	return exact, shortestPartial
}
