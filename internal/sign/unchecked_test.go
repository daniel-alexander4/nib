package sign

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestAnUncheckedSignatureSaysWhy is /pending 741's own case: a signed hybrid-reference file reads
// `Invalid` (since /pending 733) and, until this, said nothing else — no signer, no refused record,
// `AddedAfter` false — so every reader was told "invalid" and the CLI said "modified since signing" of
// a document nobody had checked. It must carry ADR-059's could-not-check and name the reading that
// could not be made. The two non-hybrid shapes that reach the same silent verdict are held too.
func TestAnUncheckedSignatureSaysWhy(t *testing.T) {
	a := newIdentity(t, "Alice")
	want := func(t *testing.T, st Status, cause UncheckedCause) {
		t.Helper()
		if st.State != Invalid {
			t.Fatalf("STIMULUS: state %q, want invalid — this fixture no longer reaches the silent verdict", st.State)
		}
		if st.Unchecked != cause || !st.AddedAfter || st.AddedAfterCause != AddedAfterCouldNotCheck {
			t.Errorf("unchecked=%q addedAfter=%v cause=%q, want %q/true/%q (/pending 741)",
				st.Unchecked, st.AddedAfter, st.AddedAfterCause, cause, AddedAfterCouldNotCheck)
		}
	}
	for _, c := range []struct {
		name string
		stm  map[int]bool
	}{
		{"catalog only in the hybrid stream", map[int]bool{1: true}},
		// (The field alone in the stream verifies `Valid`: the library's xref sweep finds the signature
		// dictionary in the classic table and never needs the field. Not this item's shape.)
		{"catalog and signature dictionary only in the hybrid stream", map[int]bool{1: true, 5: true}},
	} {
		t.Run("hybrid: "+c.name, func(t *testing.T) {
			st := Verify(signedHybrid(t, a, c.stm))
			if len(st.Refused) > 0 || len(st.Timestamps) > 0 {
				t.Fatalf("STIMULUS: refused=%v timestamps=%v — something already names why", st.Refused, st.Timestamps)
			}
			want(t, st, UncheckedHybridReference)
		})
	}
	t.Run("not hybrid: the trailer's /Root is not a catalog", func(t *testing.T) {
		doc := append([]byte(nil), synthSigned(t, a)...)
		i := bytes.LastIndex(doc, []byte("/Root 1 0 R"))
		copy(doc[i:], "/Root 3 0 R")
		want(t, Verify(doc), UncheckedUnread)
	})
	t.Run("not a PDF nib can read, carrying /ByteRange", func(t *testing.T) {
		want(t, Verify([]byte("%PDF-1.7\n1 0 obj <</ByteRange[0 1 2 3]>> endobj\n")), UncheckedUnreadable)
	})
	t.Run("control: an honest signature and a refusal say nothing unchecked", func(t *testing.T) {
		if st := Verify(synthSigned(t, a)); st.Unchecked != "" || st.AddedAfter {
			t.Errorf("honest: unchecked=%q addedAfter=%v, want empty/false", st.Unchecked, st.AddedAfter)
		}
		// A lone signature whose PKCS#7 does not parse: Invalid, no signer — and `Refused` already
		// names why, so nothing is added beside it.
		garbage := func([]byte) []byte { return []byte{0x30, 0x03, 1, 2, 3} }
		st := Verify(fillSig(t, synthRevision(t, nil, baseObjs(sigDict("1", "")), 1), "1", nil, garbage))
		if st.State != Invalid || len(st.Refused) != 1 {
			t.Fatalf("STIMULUS: state=%q refused=%v, want invalid with one refusal", st.State, st.Refused)
		}
		if st.Unchecked != "" {
			t.Errorf("a refused lone signature: unchecked=%q, want empty — Refused names it", st.Unchecked)
		}
	})
}

// TestEveryUncheckedCauseIsSaid holds `web/app.js`'s UNCHECKED_WORDS to the `UncheckedCause`
// constants in both directions, as TestEveryRefusalCauseIsSaidToTheUser does REFUSAL_WORDS: a cause
// with no sentence shows the user its code. The CLI's map is held by its own package's twin.
func TestEveryUncheckedCauseIsSaid(t *testing.T) {
	causes := uncheckedCauses(t, "verify.go")
	b, err := os.ReadFile(filepath.Join("..", "..", "web", "app.js"))
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	start := strings.Index(src, "const UNCHECKED_WORDS = {")
	if start < 0 {
		t.Fatal("web/app.js has no UNCHECKED_WORDS block")
	}
	end := strings.Index(src[start:], "\n};")
	keys := map[string]bool{}
	for _, m := range regexp.MustCompile(`(?m)^\s*'?([a-z][a-z-]*)'?:`).FindAllStringSubmatch(src[start:start+end], -1) {
		keys[m[1]] = true
	}
	if len(causes) < 3 || len(keys) == 0 {
		t.Fatalf("setup: %d causes and %d sentences read", len(causes), len(keys))
	}
	for c := range causes {
		if !keys[c] {
			t.Errorf("the unchecked cause %q has no sentence in UNCHECKED_WORDS", c)
		}
	}
	for k := range keys {
		if !causes[k] {
			t.Errorf("UNCHECKED_WORDS has a sentence for %q, which is no UncheckedCause", k)
		}
	}
}

// uncheckedCauses reads the `UncheckedCause` constants' values from path's source.
func uncheckedCauses(t *testing.T, path string) map[string]bool {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]bool{}
	for _, d := range file.Decls {
		g, ok := d.(*ast.GenDecl)
		if !ok || g.Tok != token.CONST {
			continue
		}
		for _, sp := range g.Specs {
			vs := sp.(*ast.ValueSpec)
			if id, ok := vs.Type.(*ast.Ident); !ok || id.Name != "UncheckedCause" {
				continue
			}
			for _, v := range vs.Values {
				if lit, ok := v.(*ast.BasicLit); ok {
					c, _ := strconv.Unquote(lit.Value)
					out[c] = true
				}
			}
		}
	}
	return out
}
