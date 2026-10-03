package p2p

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"runtime"
	"testing"
)

// TestADeclaredFrameIsNotAllocatedBeforeItArrives — /pending 807 R7, 784 (5).
//
// `readFrameMax` checked the declared length against the cap and then allocated all of it, so a
// pinned peer sending four header bytes declaring 128 MiB made the reader hold 128 MiB for as long
// as it cared to stall. A body that never arrives must cost about what did arrive.
func TestADeclaredFrameIsNotAllocatedBeforeItArrives(t *testing.T) {
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], maxFrame)
	src := io.MultiReader(bytes.NewReader(hdr[:]), bytes.NewReader(make([]byte, 100)))

	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	_, err := readFrameMax(src, maxFrame)
	runtime.ReadMemStats(&after)
	if !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("a frame declaring %d bytes and carrying 100 read as %v, want io.ErrUnexpectedEOF", maxFrame, err)
	}
	if alloc := after.TotalAlloc - before.TotalAlloc; alloc > 1<<20 {
		t.Errorf("reading 100 bytes of a frame that declared %d MiB allocated %d MiB — the declaration "+
			"is the peer's word, and memory must track what it actually sent", maxFrame>>20, alloc>>20)
	}

	// The control: a whole large frame still reads whole and byte-exact, across many growths.
	body := make([]byte, 3<<20+17)
	for i := range body {
		body[i] = byte(i * 7)
	}
	var w bytes.Buffer
	if err := writeFrame(&w, body); err != nil {
		t.Fatal(err)
	}
	got, err := readFrameMax(halfReader{&w}, maxFrame)
	if err != nil || !bytes.Equal(got, body) {
		t.Fatalf("control: a %d-byte frame read back %d bytes (%v), want it whole", len(body), len(got), err)
	}
	// And a declared body with nothing after the header is still io.ReadFull's io.EOF.
	binary.BigEndian.PutUint32(hdr[:], 1<<20)
	if _, err := readFrameMax(bytes.NewReader(hdr[:]), maxFrame); err != io.EOF {
		t.Errorf("a declared body with nothing after the header read as %v, want io.EOF", err)
	}
}

// halfReader returns at most half of what was asked, so the growth loop runs many short reads.
type halfReader struct{ r io.Reader }

func (h halfReader) Read(p []byte) (int, error) { return h.r.Read(p[:(len(p)+1)/2]) }
