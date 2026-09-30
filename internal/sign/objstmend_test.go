//go:build unix

package sign

import (
	"bytes"
	"compress/zlib"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	dpdf "github.com/digitorus/pdf"
)

// openAtEndDoc is a one-page document whose catalog names `/AcroForm 5 0 R`, object 5 being the LAST
// member of flate object stream 4, written as member. The page dictionary carries a `/ByteRange` name,
// so `Verify` passes its byte scan and reaches ADR-041's pdfcpu gate and the sweep. A member written
// "plain:<obj>" makes object 5 an ordinary indirect object instead, outside the stream.
func openAtEndDoc(member string) []byte {
	plain, isPlain := strings.CutPrefix(member, "plain:")
	if isPlain {
		member = "null"
	}
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n")
	off := map[int]int{}
	for _, o := range []struct {
		n int
		s string
	}{
		{1, "<</Type/Catalog/Pages 2 0 R/AcroForm 5 0 R>>"},
		{2, "<</Type/Pages/Kids[3 0 R]/Count 1>>"},
		{3, "<</Type/Page/Parent 2 0 R/MediaBox[0 0 9 9]/X/ByteRange>>"},
	} {
		off[o.n] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", o.n, o.s)
	}
	hdr := "5 0 "
	var z bytes.Buffer
	w := zlib.NewWriter(&z)
	w.Write([]byte(hdr + member))
	w.Close()
	off[4] = b.Len()
	fmt.Fprintf(&b, "4 0 obj\n<</Type/ObjStm/N 1/First %d/Filter/FlateDecode/Length %d>>\nstream\n", len(hdr), z.Len())
	b.Write(z.Bytes())
	b.WriteString("\nendstream\nendobj\n")
	if isPlain {
		off[5] = b.Len()
		fmt.Fprintf(&b, "5 0 obj\n%s\nendobj\n", plain)
	}
	off[6] = b.Len()
	var x bytes.Buffer
	for n := 0; n <= 6; n++ {
		switch {
		case n == 0:
			x.Write([]byte{0, 0, 0, 0, 0, 0xff, 0xff})
		case n == 5 && !isPlain:
			x.Write([]byte{2, 0, 0, 0, 4, 0, 0})
		default:
			o := off[n]
			x.Write([]byte{1, byte(o >> 24), byte(o >> 16), byte(o >> 8), byte(o), 0, 0})
		}
	}
	fmt.Fprintf(&b, "6 0 obj\n<</Type/XRef/Size 7/W[1 4 2]/Root 1 0 R/Length %d>>\nstream\n", x.Len())
	b.Write(x.Bytes())
	fmt.Fprintf(&b, "\nendstream\nendobj\nstartxref\n%d\n%%%%EOF\n", off[6])
	return b.Bytes()
}

// openAtEndChild runs in a re-executed test binary under an address-space cap, so a reader that does
// not terminate is OBSERVED (killed, or out of memory) rather than suffered by the test process.
func openAtEndChild() {
	lim := &syscall.Rlimit{Cur: 2 << 30, Max: 2 << 30}
	_ = syscall.Setrlimit(syscall.RLIMIT_AS, lim)
	doc := openAtEndDoc(os.Getenv("NIB_OPENATEND_MEMBER"))
	switch os.Getenv("NIB_OPENATEND_PATH") {
	case "HasSignatureBlob":
		fmt.Printf("RESULT %v\n", HasSignatureBlob(doc))
	case "Verify":
		st := Verify(doc)
		fmt.Printf("RESULT %v %v\n", st.State, st.Unchecked)
	case "Revisions":
		_, err := Revisions(doc)
		fmt.Printf("RESULT %v\n", err)
	case "Sign", "SignApproval":
		cert, key, err := GenerateIdentity("End")
		if err != nil {
			fmt.Printf("RESULT identity: %v\n", err)
			break
		}
		sign := Sign
		if os.Getenv("NIB_OPENATEND_PATH") == "SignApproval" {
			sign = SignApproval
		}
		_, err = sign(doc, cert, key, Options{Name: "E", When: time.Now()})
		fmt.Printf("RESULT %v\n", err)
	}
	os.Exit(0)
}

// TestAMemberLeftOpenAtTheStreamsEndIsRefusedNotRead — /pending 761. `digitorus/pdf`'s readByte
// answers '\n' for ever past an object stream's end, so a hex string, literal string or array left
// open at the stream's clean end never terminates (the hex string spins; the other two allocate until
// the process dies). pdfcpu READS all three documents, so ADR-041's gate does not keep them from the
// library: measured before the bound, `HasSignatureBlob`, `Verify`, `Revisions`, `Sign` and `SignApproval` each
// died or were killed at 15 s on every one. Bounded, `Sign` still took the process, because the
// library's own walk had no recover (`containedLibrarySign`). The patched reader refuses a view told of the end more than
// objStmMaxEndReads times (`ErrObjStmRunsPastEnd`, NOTICE.nib divergence 3).
//
// An `endobj` inside an array — anywhere, not only at a stream's end — is the same outcome from
// lex.go: readObject unreads it and answers null, and readArray appended that null until the process
// died, on all five paths. pdfcpu reads that document too. readArray now refuses it (divergence 4).
func TestAMemberLeftOpenAtTheStreamsEndIsRefusedNotRead(t *testing.T) {
	if os.Getenv("NIB_OPENATEND_PATH") != "" {
		openAtEndChild()
		return
	}
	// STIMULUS: an honest member that ENDS the stream (an integer and a dictionary, no trailing
	// delimiter) reads the end a few times — well under the bound — and resolves.
	for _, honest := range []string{"<</Fields[]>>", "42"} {
		doc := openAtEndDoc(honest)
		if HasSignatureBlob(doc) {
			t.Errorf("STIMULUS: honest member %q: HasSignatureBlob = true, want false", honest)
		}
		if _, err := Revisions(doc); err != nil {
			t.Errorf("STIMULUS: honest member %q: Revisions: %v — the end-read bound refuses an honest read", honest, err)
		}
	}
	// The last three are an object's end inside an array — in the stream, as an ordinary object, and
	// in a field's value — which lex.go's readArray appended null for, for ever (divergence 4).
	for _, member := range []string{"(aaa", "<444", "[1 2", "[ endobj", "plain:[ endobj", "plain:<</Fields[1 endobj"} {
		doc := openAtEndDoc(member)
		// STIMULUS: ADR-041's gate admits it, so only the reader's own bound can stop it.
		if err := pdfcpuCanRead(doc); err != nil {
			t.Fatalf("STIMULUS: pdfcpu refuses %q (%v), so this is not the shape the gate lets through", member, err)
		}
		for _, path := range []string{"HasSignatureBlob", "Verify", "Revisions", "Sign", "SignApproval"} {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestAMemberLeftOpenAtTheStreamsEndIsRefusedNotRead$", "-test.count=1")
			cmd.Env = append(os.Environ(), "NIB_OPENATEND_PATH="+path, "NIB_OPENATEND_MEMBER="+member)
			start := time.Now()
			out, err := cmd.CombinedOutput()
			timedOut := ctx.Err() == context.DeadlineExceeded
			cancel()
			i := bytes.Index(out, []byte("RESULT "))
			if timedOut || err != nil || i < 0 {
				tail := out
				if len(tail) > 200 {
					tail = tail[:200]
				}
				t.Errorf("member %q, %s: no answer came back — the process was killed or died (killed at timeout: %v, exit: %v, after %v): %q",
					member, path, timedOut, err, time.Since(start).Round(time.Millisecond), tail)
				continue
			}
			got := strings.TrimSpace(string(bytes.SplitN(out[i+7:], []byte("\n"), 2)[0]))
			switch path {
			case "HasSignatureBlob":
				// The reader's refusal is a panic; the recover answers by byte scan, and the page's
				// `/ByteRange` makes that true — "something is wrong with a signed document".
				if got != "true" {
					t.Errorf("member %q: HasSignatureBlob = %s, want true (the byte scan's answer)", member, got)
				}
			case "Verify":
				if !strings.HasPrefix(got, string(Invalid)) {
					t.Errorf("member %q: Verify = %s, want %s", member, got, Invalid)
				}
			default: // Revisions, Sign, SignApproval: the reader's refusal, said as an error
				want := dpdf.ErrObjStmRunsPastEnd.Error()
				if strings.Contains(member, "endobj") {
					want = `unexpected keyword "endobj" parsing array`
				}
				if !strings.Contains(got, want) {
					t.Errorf("member %q, %s: error = %s, want the reader's refusal (%s)", member, path, got, want)
				}
			}
		}
	}
}
