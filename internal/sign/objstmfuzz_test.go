package sign

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	dpdf "github.com/digitorus/pdf"
	orig "nib/internal/sign/testdata/digitorus-v0.1.2"
)

// # The patched object-stream reader against the original (/pending 759)
//
// `third_party/digitorus-pdf` (ADR-066) replaces v0.1.2's object-stream lookup, and was proved
// behaviour-identical only on enumerated malformed cases and a verdict-table differential. This fuzz
// test is the differential over the lookup itself: random object streams — header pairs, `/N`,
// `/First`, `/Extends`, flate bodies truncated anywhere — inside a minimal PDF, every member resolved
// through BOTH readers, in both orders and twice over (the patched reader caches between lookups),
// and every value and every panic compared.
//
// Two divergence classes are declared, and nothing else may differ:
//
//   - the original panics on a backward seek (`/First` behind the header buffer's current 4 KB chunk
//     sets a negative index: "index out of range [-"), and the patched reader resolves — NOTICE.nib;
//   - the patched reader refuses member reads that cost more than the stream holds
//     (`dpdf.ErrObjStmTooCostly`, /pending 760), which the original would pay.
//
// The generator emits BALANCED content (every `[` and `<<` closed, strings whole, no `endobj`) and truncates only
// flate bodies, because both readers hang or allocate without bound on an unterminated array, hex or
// literal string at a clean end of stream (`lex.go` readByte returns '\n' at EOF forever; ADR-041's
// pdfcpu gate keeps those from Verify). A truncated flate body ends in an unexpected-EOF error, which
// both readers raise, so it is safe to cut anywhere. A watchdog fails any input that runs 20 s.

// fuzzGen reads choices from the fuzzer's bytes; past the end every choice is 0.
type fuzzGen struct {
	b []byte
	i int
}

func (g *fuzzGen) next() int {
	if g.i >= len(g.b) {
		return 0
	}
	g.i++
	return int(g.b[g.i-1])
}

func (g *fuzzGen) done() bool { return g.i >= len(g.b) }

// content emits balanced member text from g's next k choices.
func (g *fuzzGen) content(k int) string {
	var s strings.Builder
	var open []string
	for ; k > 0 && !g.done() && s.Len() < 2000; k-- {
		switch c := g.next(); c % 14 {
		case 0:
			fmt.Fprintf(&s, "%d ", int(int8(g.next())))
		case 1:
			s.WriteString("1.5 ")
		case 2:
			fmt.Fprintf(&s, "/%c ", 'A'+g.next()%26)
		case 3:
			s.WriteString("(ab) ")
		case 4:
			s.WriteString("<4142> ")
		case 5:
			s.WriteString("[ ")
			open = append(open, "] ")
		case 6:
			s.WriteString("<< ")
			open = append(open, ">> ")
		case 7:
			if len(open) > 0 {
				s.WriteString(open[len(open)-1])
				open = open[:len(open)-1]
			}
		case 8:
			fmt.Fprintf(&s, "%d 0 R ", g.next()%20)
		case 9:
			// Never `endobj`: inside an array both readers loop forever appending nil (readObject
			// unreads it and readArray never advances) — an upstream exposure, not a divergence.
			s.WriteString([]string{"true ", "null ", "false ", "-0.5 "}[g.next()%4])
		case 10:
			s.WriteString([]string{"\n", "\r\n", "%c\n", "  "}[g.next()%4])
		case 11:
			fmt.Fprintf(&s, "/K %d ", g.next())
		case 12:
			s.WriteString("stream ")
		case 13:
			s.WriteString("<</Type/X/N 1>>\n")
		}
	}
	for i := len(open) - 1; i >= 0; i-- {
		s.WriteString(open[i])
	}
	return s.String()
}

// header emits `id offset` pairs naming ids 7..16 at offsets into (and just past) content, with the
// odd token that is not a plain pair.
func (g *fuzzGen) header(contentLen int) (string, int) {
	var s strings.Builder
	pairs := g.next() % 12
	for i := 0; i < pairs; i++ {
		id := 7 + g.next()%10
		off := g.next()<<8 | g.next()
		off %= contentLen + 5
		switch g.next() % 16 {
		case 0:
			fmt.Fprintf(&s, "%d -%d ", id, off)
		case 1:
			fmt.Fprintf(&s, "%d 1.5 ", id)
		case 2:
			fmt.Fprintf(&s, "/x %d ", off)
		case 3:
			fmt.Fprintf(&s, "%d %d ", id+1<<32, off)
		default:
			fmt.Fprintf(&s, "%d %d ", id, off)
		}
	}
	return s.String(), pairs
}

// objStmFuzzDoc builds the document: object streams 4 and 6 (6 the `/Extends` target when flags say
// so), ids 7..16 all named by the xref as members of stream 4.
func objStmFuzzDoc(a, b []byte, flags uint8, trunc uint16) []byte {
	spec := func(src []byte, flate, pad bool) stmSpec {
		g := &fuzzGen{b: src}
		content := g.content(g.next())
		hdr, pairs := g.header(len(content))
		n := pairs + int(int8(g.next()))%4 // /N a little over or under the pairs written
		delta := 0
		if g.next()%4 == 0 {
			delta = int(int8(g.next())) % 9 // /First a little off the header's end
		}
		if pad {
			hdr += strings.Repeat(" ", 4200) // past one 4 KB chunk: a /First into it seeks backward
		}
		first := len(hdr) + delta
		if pad && delta < 0 {
			first = -delta * 16 // well behind the chunk the header lexer ends in
		}
		return stmSpec{hdr: hdr, content: content, n: n, first: max(first, 0), flate: flate}
	}
	s4 := spec(a, flags&1 != 0, flags&4 != 0)
	s6 := spec(b, flags&16 != 0, false)
	if flags&2 != 0 {
		s4.extends = 6
	}
	if s4.flate && flags&32 != 0 {
		s4.mangle = func(d []byte) []byte { return d[:len(d)-int(trunc)%(len(d)+1)] }
	}
	members := map[int][2]int{}
	for id := 7; id <= 16; id++ {
		members[id] = [2]int{4, id - 7}
	}
	return rawObjStmDoc(map[int]stmSpec{4: s4, 6: s6}, members)
}

// resolved is one lookup's outcome: the value's text, or the panic's.
func resolvedBy(resolve func() string) (out string, panicked bool) {
	defer func() {
		if rec := recover(); rec != nil {
			out, panicked = fmt.Sprint(rec), true
		}
	}()
	return resolve(), false
}

func FuzzThePatchedObjectStreamReaderMatchesTheOriginal(f *testing.F) {
	// Seeds: honest pairs over a small dictionary content, each flag alone, a truncation, the pad.
	// 4 content choices (three dictionaries and an array), then 4 pairs naming ids 7..10 at offsets
	// 0, 16, 32 and 48, then /N and /First exact.
	honest := []byte{4, 13, 13, 13, 5, 4, 0, 0, 0, 4, 1, 0, 16, 4, 2, 0, 32, 4, 3, 0, 48, 4, 4, 1}
	for _, fl := range []uint8{0, 1, 2, 3, 4, 8, 16, 1 | 32, 1 | 2 | 8 | 16 | 32, 4 | 2} {
		f.Add(honest, honest, fl, uint16(7))
	}
	// The same with /First one past the header's end, and with a backward /First behind a padded header.
	f.Add(append(honest[:len(honest)-1:len(honest)-1], 0, 1), honest, uint8(1), uint16(0))
	f.Add(append(honest[:len(honest)-1:len(honest)-1], 0, 0xfd), honest, uint8(4), uint16(0))
	f.Add([]byte{200, 250}, []byte{}, uint8(4), uint16(0))
	f.Add([]byte("\x05\x00\x05\x05\x05\x06\x02\x00\x07\x07\x07\x07\x07"), []byte("\x02"), uint8(3), uint16(1))
	f.Fuzz(func(t *testing.T, a, b []byte, flags uint8, trunc uint16) {
		doc := objStmFuzzDoc(a, b, flags, trunc)
		done := make(chan string, 1)
		go func() { done <- compareObjStmReaders(doc, flags&8 != 0) }()
		select {
		case diff := <-done:
			if diff != "" {
				t.Fatalf("the patched reader diverges from v0.1.2 outside the declared classes:\n%s\ndocument %q", diff, doc)
			}
		case <-time.After(20 * time.Second):
			t.Fatalf("a lookup ran 20 s (a hang in either reader): document %q", doc)
		}
	})
}

// compareObjStmReaders resolves ids 7..16 through both readers, twice over, and returns every
// divergence outside the declared classes.
func compareObjStmReaders(doc []byte, reverse bool) string {
	po, errO := orig.NewReader(bytes.NewReader(doc), int64(len(doc)))
	pp, errP := dpdf.NewReader(bytes.NewReader(doc), int64(len(doc)))
	if (errO == nil) != (errP == nil) {
		return fmt.Sprintf("NewReader: original err=%v, patched err=%v", errO, errP)
	}
	if errO != nil {
		return ""
	}
	var diffs []string
	costly := false
	for pass := 0; pass < 2; pass++ {
		for i := 0; i < 10; i++ {
			id := 7 + i
			if reverse {
				id = 16 - i
			}
			o, oPanic := resolvedBy(func() string {
				if id >= len(po.Xref()) {
					return "<no entry>"
				}
				p := po.Xref()[id].Ptr()
				return po.Resolve(p, p).String()
			})
			p, pPanic := resolvedBy(func() string {
				if id >= len(pp.Xref()) {
					return "<no entry>"
				}
				ptr := pp.Xref()[id].Ptr()
				return pp.Resolve(ptr, ptr).String()
			})
			switch {
			case o == p && oPanic == pPanic:
			case oPanic && strings.Contains(o, "index out of range [-"):
				// Declared: the original's backward seek (NOTICE.nib).
			case pPanic && strings.Contains(p, dpdf.ErrObjStmTooCostly.Error()):
				costly = true // declared: the patched reader's own cost refusal (/pending 760)
			case costly && pPanic:
			default:
				diffs = append(diffs, fmt.Sprintf("pass %d id %d: original %s %q; patched %s %q", pass, id, kind(oPanic), o, kind(pPanic), p))
			}
		}
	}
	return strings.Join(diffs, "\n")
}

func kind(panicked bool) string {
	if panicked {
		return "panics"
	}
	return "returns"
}

// TestTheOriginalReaderCopyIsUpstreamUnchanged holds the fuzz test's original to what it claims to be:
// v0.1.2's read.go, lex.go and text.go byte for byte (testdata/digitorus-v0.1.2/NOTICE.nib).
func TestTheOriginalReaderCopyIsUpstreamUnchanged(t *testing.T) {
	out, err := exec.Command("go", "env", "GOMODCACHE").Output()
	cache := strings.TrimSpace(string(out))
	if err != nil || cache == "" {
		t.Skipf("SKIP (not a pass): go env GOMODCACHE: %v", err)
	}
	up := filepath.Join(cache, "github.com", "digitorus", "pdf@v0.1.2")
	if _, err := os.Stat(filepath.Join(up, "go.mod")); err != nil {
		t.Skipf("SKIP (not a pass): github.com/digitorus/pdf@v0.1.2 is not in the module cache (%v)", err)
	}
	for _, name := range []string{"read.go", "lex.go", "text.go", "LICENSE"} {
		want, err := os.ReadFile(filepath.Join(up, name))
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join("testdata", "digitorus-v0.1.2", name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(got, want) {
			t.Errorf("testdata/digitorus-v0.1.2/%s differs from the module cache's v0.1.2 — the fuzz test's original is not the original", name)
		}
	}
}
