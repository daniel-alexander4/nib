package sign

import (
	"errors"
	"fmt"
	"runtime/debug"
	"strings"
	"testing"
	"time"

	dpdf "github.com/digitorus/pdf"
)

// nestedObjStmDoc is a file whose object 1 is its xref stream; `direct` objects are written as bodies, `inStm` entries
// are type 2 (stream id, index), and a padding comment carries extra text (a `/ByteRange` for the scan to find).
func nestedObjStmDoc(direct map[int]string, inStm map[int][2]int, size int, pad string) []byte {
	var b strings.Builder
	b.WriteString("%PDF-1.7\n%" + strings.Repeat("x", 256) + pad + "\n")
	offs := map[int]int{}
	for id := 2; id < size; id++ {
		if body, ok := direct[id]; ok {
			offs[id] = b.Len()
			fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", id, body)
		}
	}
	xoff := b.Len()
	var ent []byte
	for id := 0; id < size; id++ {
		switch s, in := inStm[id]; {
		case id == 1:
			ent = append(ent, 1, byte(xoff>>8), byte(xoff), 0)
		case offs[id] != 0:
			ent = append(ent, 1, byte(offs[id]>>8), byte(offs[id]), 0)
		case in:
			ent = append(ent, 2, byte(s[0]>>8), byte(s[0]), byte(s[1]))
		default:
			ent = append(ent, 0, 0, 0, 0)
		}
	}
	fmt.Fprintf(&b, "1 0 obj<</Type/XRef/Size %d/W[1 2 1]/Length %d/Root 2 0 R>>stream\n", size, len(ent))
	b.Write(ent)
	fmt.Fprintf(&b, "\nendstream\nendobj\nstartxref\n%d\n%%%%EOF\n", xoff)
	return []byte(b.String())
}

// TestAnObjectStreamReachedThroughItselfIsRefusedNotRecursed — the patched reader's `resolve` found a member by
// resolving its stream, and opened that stream by resolving its /Length, with no guard on either: a stream listed
// inside itself, two streams inside each other, or a stream whose /Length is its own member recursed until Go's
// 1 GB stack limit — `fatal error: stack overflow`, which no recover holds (/pending 803, measured: 1.3-8.7 s, then
// the process is gone). `HasSignatureBlob` runs ungated on every save and every ceremony arrival, so a ~400-byte
// file took Nib down. pdfcpu refuses all three, so `Verify` was gated; the blob check and the sweep are not.
// NOTICE.nib divergence 8 bounds the nesting; the blob check answers from its byte scan.
func TestAnObjectStreamReachedThroughItselfIsRefusedNotRecursed(t *testing.T) {
	// A pre-fix run fails fast rather than growing a 1 GB stack: still fatal, which is the red.
	defer debug.SetMaxStack(debug.SetMaxStack(64 << 20))
	shapes := map[string]func(pad string) []byte{
		"a stream listed inside itself": func(pad string) []byte {
			return nestedObjStmDoc(nil, map[int][2]int{2: {2, 0}}, 3, pad)
		},
		"two streams inside each other": func(pad string) []byte {
			return nestedObjStmDoc(nil, map[int][2]int{2: {3, 0}, 3: {2, 0}}, 4, pad)
		},
		"a /Length that is its own stream's member": func(pad string) []byte {
			return nestedObjStmDoc(map[int]string{3: "<</Type/ObjStm/N 2/First 8/Length 4 0 R>>stream\n4 0 2 3\n10 <<>> \nendstream"},
				map[int][2]int{2: {3, 1}, 4: {3, 0}}, 5, pad)
		},
	}
	for name, shape := range shapes {
		t.Run(name, func(t *testing.T) {
			if HasSignatureBlob(shape("")) {
				t.Error("an unsigned file with no /ByteRange answered as carrying a signature")
			}
			if !HasSignatureBlob(shape(" /ByteRange")) {
				t.Error("the byte scan's answer was lost: the file carries a /ByteRange")
			}
			_, _, err := sweep(shape(""))
			if err == nil || !strings.Contains(err.Error(), dpdf.ErrObjStmNested.Error()) {
				t.Fatalf("sweep: %v, want the reader's nesting refusal", err)
			}
		})
	}
}

// TestAnHonestlyNestedObjectStreamStillReads — the control: a member whose stream's /Length is itself a member of a
// SECOND object stream (forbidden by ISO 32000-1 7.5.7, read by upstream, two lookups deep) still resolves.
func TestAnHonestlyNestedObjectStreamStillReads(t *testing.T) {
	doc := nestedObjStmDoc(map[int]string{
		3: "<</Type/ObjStm/N 1/First 4/Length 6 0 R>>stream\n2 0 <</Ok true>> \nendstream",
		5: "<</Type/ObjStm/N 1/First 4/Length 16>>stream\n6 0 17              \nendstream",
	}, map[int][2]int{2: {3, 0}, 6: {5, 0}}, 7, "")
	r, err := dpdf.NewReader(strings.NewReader(string(doc)), int64(len(doc)))
	if err != nil {
		t.Fatal(err)
	}
	var got dpdf.Value
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				if e, ok := rec.(error); ok && errors.Is(e, dpdf.ErrObjStmNested) {
					t.Fatalf("a two-deep lookup was refused as nested: %v", e)
				}
				t.Fatalf("lookup panicked: %v", rec)
			}
		}()
		got = r.Trailer().Key("Root").Key("Ok")
	}()
	if !got.Bool() {
		t.Fatalf("the member did not resolve through its stream's referenced /Length: %v", got)
	}
}

// TestAnExtendsCycleReachedBeforeTheScreenIsRefusedNotSpun — `libraryLookupCost` refuses a `/Extends` cycle, but only
// once a reader is open, and `NewReader` resolves the trailer's `/Encrypt` before that: an `/Encrypt` naming a member
// of a stream that `/Extends` itself (or a ring of two) spun the patched reader's search loop for ever inside
// `HasSignatureBlob` (/pending 803, measured: killed at 20 s, a ~400-byte file). pdfcpu refuses it, so only the
// ungated blob check was exposed. NOTICE.nib divergence 9 refuses the revisit.
func TestAnExtendsCycleReachedBeforeTheScreenIsRefusedNotSpun(t *testing.T) {
	encrypted := func(d []byte) []byte {
		return []byte(strings.Replace(string(d), "/Root 2 0 R>>", "/Root 2 0 R/Encrypt 5 0 R/ID[<00><00>]>>", 1))
	}
	shapes := map[string]func(pad string) []byte{
		"a stream extending itself": func(pad string) []byte {
			return encrypted(nestedObjStmDoc(map[int]string{
				3: "<</Type/ObjStm/N 1/First 4/Extends 3 0 R/Length 6>>stream\n9 0 1 \nendstream",
			}, map[int][2]int{5: {3, 0}}, 6, pad))
		},
		"two streams extending each other": func(pad string) []byte {
			return encrypted(nestedObjStmDoc(map[int]string{
				3: "<</Type/ObjStm/N 1/First 4/Extends 4 0 R/Length 6>>stream\n9 0 1 \nendstream",
				4: "<</Type/ObjStm/N 1/First 4/Extends 3 0 R/Length 6>>stream\n9 0 1 \nendstream",
			}, map[int][2]int{5: {3, 0}}, 6, pad))
		},
	}
	for name, shape := range shapes {
		t.Run(name, func(t *testing.T) {
			done := make(chan [2]bool, 1)
			go func() { done <- [2]bool{HasSignatureBlob(shape("")), HasSignatureBlob(shape(" /ByteRange"))} }()
			select {
			case got := <-done:
				if got[0] || !got[1] {
					t.Errorf("HasSignatureBlob = %v without and %v with a /ByteRange, want the byte scan's false, true", got[0], got[1])
				}
			case <-time.After(10 * time.Second):
				t.Fatal("HasSignatureBlob ran 10 s: the reader is searching the /Extends ring again and again")
			}
			_, _, err := sweep(shape(""))
			if err == nil || !strings.Contains(err.Error(), dpdf.ErrObjStmExtendsCycle.Error()) {
				t.Fatalf("sweep: %v, want the reader's /Extends-cycle refusal", err)
			}
		})
	}
}
