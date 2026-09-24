package fontcode

// Codespace cuts a Type 0 font's shown bytes into character codes, as veraPDF-parser's
// `CMap.getCodeFromStream` does.
//
// A simple font reads one byte per code and needs no Codespace (`PDFont.readCode`).
type Codespace struct {
	spaces   []codeRange
	shortest int
	// Invalid is a range veraPDF keeps with no bytes (its two ends differ in length, or a begin byte is
	// above its end byte): checking a partial match against it throws inside the reader, and nib does not
	// model where that leaves the stream. A caller treats such a font's codes as unread.
	Invalid bool
	// UsesCMap names another CMap (`/Name usecmap`) whose ranges veraPDF adds AT that point of the parse. Identity
	// is merged here; any other name is the caller's to resolve.
	UsesCMap string
	// Malformed is a CMap veraPDF-parser throws on, and so reads as holding no range at all.
	Malformed bool
}

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

// Identity is Identity-H's and Identity-V's one range, `<0000> <FFFF>`.
func Identity() *Codespace {
	return &Codespace{spaces: []codeRange{{lo: []byte{0, 0}, hi: []byte{0xFF, 0xFF}}}, shortest: 2}
}

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
	for _, s := range c.spaces {
		if s.overlaps(r) {
			return
		}
	}
	if !valid {
		c.Invalid = true
	}
	c.spaces = append(c.spaces, r)
	if c.shortest == 0 || len(lo) < c.shortest {
		c.shortest = len(lo)
	}
}

// Merge adds another CMap's ranges, as `CMap.useCMap` does — with no overlap check, and WITHOUT touching the
// shortest length, which only a CMap's own `codespacerange` sets (`CMapParser.java:179`). The shortest decides how
// far a byte no range admits is skipped, so a merge that moved it cut the string differently (measured).
func (c *Codespace) Merge(o *Codespace) {
	c.spaces = append(c.spaces, o.spaces...)
	c.Invalid = c.Invalid || o.Invalid
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
		c.UsesCMap = name
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
					// `CodeSpace.isPartialMatch` THROWS for a position past the range's length, and the throw
					// ends the whole scan: the ranges after it are not consulted. A byte that fails to match
					// first breaks out before the position is reached, so it does not throw.
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
