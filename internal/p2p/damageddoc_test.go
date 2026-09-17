package p2p

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

// childMarker makes the process below re-enter this test as the damaged-document child.
const childMarker = "NIB_DAMAGED_DOC_CHILD"

// TestADamagedDocumentFromARemotePartyIsRefusedRatherThanFatal — /pending 509, ADR-041.
//
// A contribution arriving from a remote party reaches `ContributionProgress`, which asks
// `sign.Verify` what is already on the document. Before ADR-041 an object stream whose `/Filter`
// name was damaged sent `digitorus/pdf`'s lexer past the end of the stream, where `readByte`
// returns `'\n'` forever: `readLiteralString` appends without bound and the process dies with
// `fatal error: out of memory`, and `readHexString` spins instead — neither is a panic, so
// `internal/safe.Recover` and the library's own recover are both irrelevant. **Measured: 12 of
// 7,438 single-bit flips of a signed fixture.**
//
// # Why this test forks
//
// The failure it guards against **takes the process**, so a test that drove it in-process would
// take the test binary with it — the suite would not report a failure, it would stop existing.
// So the dangerous call runs in a child: this same binary, re-run with `childMarker` set, which
// is the ordinary Go shape for a test whose subject is a process death.
//
// The child bounds ITSELF rather than trusting the parent's timeout, because the two failure
// modes need different bounds and neither may be allowed to reach the machine: an allocation
// watcher for the OOM (which would otherwise grow until the kernel starts killing other
// processes) and a deadline for the spin (which allocates nothing, so the allocation watcher
// cannot see it). Both are portable — no `RLIMIT_AS`, so this runs on every platform nib ships to.
//
// # What makes it fall over if the gate goes
//
// Nothing here is a source-text assertion. The child either returns a refusal or it does not come
// back, and the parent distinguishes the two by exit status, which is the fact at issue.
func TestADamagedDocumentFromARemotePartyIsRefusedRatherThanFatal(t *testing.T) {
	if os.Getenv(childMarker) != "" {
		damagedDocChild(t)
		return
	}
	for _, payload := range []string{"literal", "hex"} {
		t.Run(payload, func(t *testing.T) {
			cmd := exec.Command(os.Args[0], "-test.run", "^"+strings.Split(t.Name(), "/")[0]+"$", "-test.v")
			cmd.Env = append(os.Environ(), childMarker+"="+payload)
			var out bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &out
			if err := cmd.Start(); err != nil {
				t.Fatal(err)
			}
			done := make(chan error, 1)
			go func() { done <- cmd.Wait() }()
			var werr error
			select {
			case werr = <-done:
			case <-time.After(90 * time.Second):
				_ = cmd.Process.Kill()
				<-done
				t.Fatalf("the child never came back: %s payload took the process past even its own "+
					"self-imposed deadline\n%s", payload, out.String())
			}
			text := out.String()
			if werr != nil {
				t.Fatalf("a damaged %s object stream arriving from a remote party TOOK THE PROCESS "+
					"(%v) instead of being refused — this is the crash ADR-041 exists to prevent, and "+
					"no recover reaches it\n%s", payload, werr, text)
			}
			if !strings.Contains(text, "REFUSED-UNPROVEN") {
				t.Fatalf("the child survived but did not refuse the document through the receive "+
					"path's own error taxonomy\n%s", text)
			}
			// The CONTROL, and it is why a gate that refuses everything cannot pass this: the same
			// child admits the undamaged document it built the damaged one from.
			if !strings.Contains(text, "CONTROL-ADMITTED") {
				t.Fatalf("the child refused the UNDAMAGED document too — the gate is not "+
					"discriminating, it is just closed\n%s", text)
			}
		})
	}
}

// damagedDocChild is the half that runs in the forked process. It never uses `t` to report the
// dangerous call's outcome, because the outcome under test is whether the process survives it.
func damagedDocChild(t *testing.T) {
	// The allocation watcher: `readLiteralString` appends without bound, so heap growth is the
	// observable. 256 MiB is far above anything this fixture legitimately needs (the document is
	// under 30 KB) and far below anything that would disturb the machine.
	go func() {
		for {
			time.Sleep(20 * time.Millisecond)
			var ms runtime.MemStats
			runtime.ReadMemStats(&ms)
			if ms.HeapAlloc > 256<<20 {
				fmt.Println("CHILD-RUNAWAY-ALLOCATION")
				os.Exit(9)
			}
		}
	}()
	// The deadline: `readHexString` spins on `'\n'` at EOF and allocates NOTHING, so the watcher
	// above is blind to it. Without this the child would sit at 100% of a core until the parent's
	// timeout, and the parent could not tell a hang from a slow machine.
	go func() {
		time.Sleep(45 * time.Second)
		fmt.Println("CHILD-RUNAWAY-SPIN")
		os.Exit(10)
	}()

	a, b := l3Identity(t, "A"), l3Identity(t, "B")
	r := l3Roster(a, b)
	clean := l3Chain(t, l3Prepared(t), []l3Party{a}, []l3Party{a}, "")

	// The control runs FIRST and on the undamaged bytes, so a gate that refuses every document
	// cannot reach the refusal below and call itself proven.
	if _, err := ContributionProgress(clean, r); err != nil {
		fmt.Printf("CONTROL-REFUSED: %v\n", err)
		os.Exit(11)
	}
	fmt.Println("CONTROL-ADMITTED")

	damaged := damageObjectStream(t, clean, os.Getenv(childMarker))
	_, err := ContributionProgress(damaged, r)
	if err == nil {
		fmt.Println("ADMITTED-A-DOCUMENT-NOBODY-CAN-READ")
		os.Exit(12)
	}
	if !errors.Is(err, ErrPrefixUnproven) {
		fmt.Printf("REFUSED-BY-THE-WRONG-RULE: %v\n", err)
		os.Exit(13)
	}
	fmt.Printf("REFUSED-UNPROVEN: %v\n", err)
}

// damageObjectStream reproduces the measured failure DETERMINISTICALLY, which a bit flip cannot:
// whether a flipped fixture runs away depends on whether its compressed bytes happen to contain an
// unbalanced `(`, and the fixture is freshly signed with a random key on every run. Measured: the
// same semantic damage crashed one fixture and was shrugged off by the next.
//
// Both edits keep the file's byte length, so no cross-reference offset moves and the document
// stays exactly as reachable as the one it came from:
//
//   - the object stream's `/Filter` becomes `/Filuer`, an unknown name, so the library hands the
//     lexer the stream's RAW bytes instead of the inflated ones — which is what every one of the
//     12 measured flips did, all six of them landing inside the word `Filter`;
//   - those bytes become a payload that cannot terminate. `(aaa…` reaches `readLiteralString`,
//     which appends `'\n'` forever; `<444…` reaches `readHexString`, which spins forever. The
//     payload is read by `readToken` itself, before any object id is matched, so neither depends
//     on which object the verifier happens to be resolving.
func damageObjectStream(t *testing.T, base []byte, payload string) []byte {
	t.Helper()
	doc := append([]byte(nil), base...)
	typ := bytes.Index(doc, []byte("/Type/ObjStm"))
	if typ < 0 {
		t.Fatal("setup: the fixture has no object stream to damage, so this test proves nothing")
	}
	dict := bytes.LastIndex(doc[:typ], []byte("<<"))
	filter := bytes.Index(doc[dict:typ], []byte("/Filter"))
	if filter < 0 {
		t.Fatal("setup: the object stream dict has no /Filter")
	}
	copy(doc[dict+filter:], "/Filuer")

	kw := bytes.Index(doc[typ:], []byte("stream"))
	if kw < 0 {
		t.Fatal("setup: no stream keyword after the object stream dict")
	}
	start := typ + kw + len("stream")
	for start < len(doc) && (doc[start] == '\r' || doc[start] == '\n') {
		start++
	}
	end := bytes.Index(doc[start:], []byte("endstream"))
	if end < 0 {
		t.Fatal("setup: no endstream")
	}
	body := doc[start : start+end]
	if len(body) < 2 {
		t.Fatalf("setup: the object stream is %d bytes, too small to carry a payload", len(body))
	}
	var fill byte
	switch payload {
	case "literal":
		body[0], fill = '(', 'a'
	case "hex":
		body[0], fill = '<', '4'
	default:
		t.Fatalf("setup: unknown payload %q", payload)
	}
	for i := 1; i < len(body); i++ {
		body[i] = fill
	}
	return doc
}
