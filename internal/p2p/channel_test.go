package p2p

import (
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

// halfChannel builds a Channel with exactly one field missing, over a real pipe so the
// stream is genuinely usable — the refusal must come from the missing field and not from
// a stream that could not have carried anything anyway.
func halfChannel(t *testing.T, omit string) Channel {
	t.Helper()
	c1, c2 := net.Pipe()
	t.Cleanup(func() { c1.Close(); c2.Close() })
	ch := Channel{
		Stream: c1,
		PeerFP: make([]byte, 32),
		Export: func(string, []byte, int) ([]byte, error) { return make([]byte, 32), nil },
	}
	switch omit {
	case "stream":
		ch.Stream = nil
	case "peerfp":
		ch.PeerFP = nil
	case "export":
		ch.Export = nil
	case "none":
	default:
		t.Fatalf("unknown field %q", omit)
	}
	return ch
}

// TestAnIncompleteChannelIsRefusedByEveryEntryPoint.
//
// Each field of a Channel is a security property established BEFORE the core runs: the
// peer's identity was verified by the transport's pinned callback, and the exporter binds
// the spoken check to this channel. That is exactly what makes a zero value dangerous —
// `Channel{Stream: s}` compiles, and it would run a whole session against an
// unauthenticated stranger with a verification string bound to nothing.
//
// The stimulus is asserted first: a COMPLETE channel over the same pipe gets past the
// check and fails later, on I/O. Without that, "it returned an error" is satisfied by a
// pipe with nobody on the other end, and the test would pass with check() deleted.
func TestAnIncompleteChannelIsRefusedByEveryEntryPoint(t *testing.T) {
	pdf := []byte("%PDF-1.7\n")
	fp := make([]byte, 32)

	calls := map[string]func(ch Channel) error{
		"Initiate": func(ch Channel) error {
			_, err := Initiate(ch, pdf, fp, okVerifier{})
			return err
		},
		"Receive": func(ch Channel) error {
			_, err := Receive(ch, nil, nil, "Alice", confirmer{accept: true}, okVerifier{}, nil)
			return err
		},
		"SendDocument": func(ch Channel) error {
			return SendDocument(ch, pdf, fp, okVerifier{})
		},
		"ReceiveDocument": func(ch Channel) error {
			_, err := ReceiveDocument(ch, nil, fp, okVerifier{})
			return err
		},
	}

	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			// Stimulus: a COMPLETE channel gets PAST check. It then fails on the pipe,
			// which is fine — what matters is that the error is not the refusal below.
			done := make(chan error, 1)
			go func() { done <- call(halfChannel(t, "none")) }()
			select {
			case err := <-done:
				if err != nil && strings.Contains(err.Error(), "incomplete channel") {
					t.Fatalf("a COMPLETE channel was refused as incomplete (%v) — every "+
						"assertion below would pass for the wrong reason", err)
				}
			case <-time.After(2 * time.Second):
				// Blocked on the pipe, which also means it got past the check.
			}

			for _, omit := range []string{"stream", "peerfp", "export"} {
				// With a timeout, because the failure mode is a HANG and not a wrong
				// answer: an entry point that gets past the check writes to a pipe with
				// nobody on the other end and blocks forever. A test that hangs reports
				// nothing useful and takes the suite with it — so the hang is named.
				res := make(chan error, 1)
				go func() { res <- call(halfChannel(t, omit)) }()
				var err error
				select {
				case err = <-res:
				case <-time.After(3 * time.Second):
					t.Fatalf("a Channel with no %s did not return — it got past the "+
						"boundary and blocked on the stream", omit)
				}
				if err == nil {
					t.Fatalf("a Channel with no %s ran a session", omit)
				}
				if !strings.Contains(err.Error(), "incomplete channel") {
					t.Errorf("a Channel with no %s failed with %q, not as an incomplete "+
						"channel — it got past the boundary and failed later, by luck", omit, err)
				}
			}
		})
	}
}

// TestTheExporterIsTheOnlyRouteToTheChannelBinding — a Channel whose exporter errors must
// fail the verification rather than silently deriving words from nothing.
func TestAFailingExporterFailsTheVerification(t *testing.T) {
	boom := errors.New("no keying material")
	ch := halfChannel(t, "none")
	ch.Export = func(string, []byte, int) ([]byte, error) { return nil, boom }

	_, err := verificationString(ch.Export, ch.PeerFP, ch.PeerFP, nil, nil)
	if err == nil {
		t.Fatal("a failing exporter produced a verification string")
	}
	if !errors.Is(err, boom) {
		t.Errorf("the exporter's error did not survive: %v", err)
	}
}
