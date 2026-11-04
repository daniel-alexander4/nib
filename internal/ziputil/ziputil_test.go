package ziputil

import (
	"archive/zip"
	"bytes"
	"errors"
	"strings"
	"testing"
)

var testLim = Limits{Entries: 4, DecodedBytes: 1 << 20, RatioFloor: 1 << 10, RatioMax: 50}

func build(t *testing.T, names []string, bodies [][]byte) []byte {
	t.Helper()
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	for i, n := range names {
		w, _ := zw.Create(n)
		w.Write(bodies[i])
	}
	zw.Close()
	return b.Bytes()
}

func open(t *testing.T, b []byte, l Limits) (*Archive, error) {
	t.Helper()
	return Open(bytes.NewReader(b), int64(len(b)), l)
}

func TestOpenRefusesAnUnsetLimit(t *testing.T) {
	if _, err := open(t, build(t, []string{"a"}, [][]byte{[]byte("x")}), Limits{}); err == nil {
		t.Fatal("a Limits with nothing set opened an archive")
	}
}

func TestEntryCapRefusesAtCapPlusOneAndAdmitsAtCap(t *testing.T) {
	mk := func(n int) []byte {
		names, bodies := make([]string, n), make([][]byte, n)
		for i := range names {
			names[i], bodies[i] = string(rune('a'+i)), []byte("x")
		}
		return build(t, names, bodies)
	}
	if _, err := open(t, mk(testLim.Entries), testLim); err != nil {
		t.Fatalf("at the cap: %v", err)
	}
	if _, err := open(t, mk(testLim.Entries+1), testLim); !errors.Is(err, ErrTooManyEntries) {
		t.Fatalf("cap+1: %v", err)
	}
}

func TestDecodedCapAndRatioAreSeparateAndNamed(t *testing.T) {
	zeros := func(n int) []byte { return bytes.Repeat([]byte{0}, n) }
	l := testLim
	l.RatioMax = 1 << 30 // ratio out of the way
	if _, err := open(t, build(t, []string{"a"}, [][]byte{zeros(int(l.DecodedBytes))}), l); err != nil {
		t.Fatalf("at the decoded cap: %v", err)
	}
	if _, err := open(t, build(t, []string{"a"}, [][]byte{zeros(int(l.DecodedBytes) + 1)}), l); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("decoded cap+1: %v", err)
	}
	// Under the decoded cap but past the floor at a ratio no document has.
	if _, err := open(t, build(t, []string{"a"}, [][]byte{zeros(1 << 19)}), testLim); !errors.Is(err, ErrRatio) {
		t.Fatalf("ratio: %v", err)
	}
	// Under the floor, any ratio is fine.
	if _, err := open(t, build(t, []string{"a"}, [][]byte{zeros(int(testLim.RatioFloor))}), testLim); err != nil {
		t.Fatalf("small archive at a high ratio: %v", err)
	}
}

func TestUnsafeAndRepeatedNamesAreRefused(t *testing.T) {
	for _, n := range []string{"../x", "/x", "a\\b", "a/../b", "C:/x", "a\x00b", "a\nb"} {
		if _, err := open(t, build(t, []string{n}, [][]byte{[]byte("x")}), testLim); err == nil {
			t.Errorf("%q opened", n)
		}
	}
	if _, err := open(t, build(t, []string{"a", "a"}, [][]byte{[]byte("x"), []byte("y")}), testLim); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate: %v", err)
	}
	l := testLim
	l.TolerateUnsafeNames = true
	if _, err := open(t, build(t, []string{"word\\document.xml"}, [][]byte{[]byte("x")}), l); err != nil {
		t.Fatalf("a package a converter reads itself, tolerated: %v", err)
	}
}

func TestReadIsCappedOnTheRealStreamAndCountsTowardTheTotal(t *testing.T) {
	a, err := open(t, build(t, []string{"a", "b"}, [][]byte{bytes.Repeat([]byte("x"), 100), bytes.Repeat([]byte("x"), 100)}), testLim)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := a.Read(a.Files()[0], 99); !errors.Is(err, ErrTooLarge) || !strings.Contains(err.Error(), "99") {
		t.Fatalf("one byte past a member's own cap: %v", err)
	}
	b, err := a.Read(a.Files()[0], 100)
	if err != nil || len(b) != 100 {
		t.Fatalf("at the cap: %v len %d", err, len(b))
	}
	a.lim.DecodedBytes = 150
	if _, err := a.Read(a.Files()[1], 100); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("two reads each under their cap, together over the total: %v", err)
	}
}

func TestWriteStoresMimetypeFirstAtByte38AndRefusesUnsafeNames(t *testing.T) {
	b, err := Write([]Entry{{Name: "mimetype", Data: []byte("application/epub+zip"), Store: true}, {Name: "a.xml", Data: []byte("<a/>")}})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(b[30:], []byte("mimetype")) || !bytes.HasPrefix(b[38:], []byte("application/epub+zip")) {
		t.Fatalf("mimetype is not stored first at byte 38: %q", b[:80])
	}
	if _, err := Write([]Entry{{Name: "../x", Data: nil}}); !errors.Is(err, ErrUnsafeName) {
		t.Fatalf("unsafe: %v", err)
	}
	if _, err := Write([]Entry{{Name: "a"}, {Name: "a"}}); !errors.Is(err, ErrDuplicate) {
		t.Fatalf("duplicate: %v", err)
	}
}

// The entry count is read from the end-of-central-directory record before zip.NewReader builds an entry per record:
// a record that claims more entries than the cap is refused as TOO MANY, where reading the directory first would
// have called the archive unreadable (its real directory is shorter than the claim).
func TestTheDeclaredEntryCountIsRefusedBeforeTheDirectoryIsRead(t *testing.T) {
	b := build(t, []string{"a"}, [][]byte{[]byte("x")})
	i := bytes.LastIndex(b, []byte{'P', 'K', 5, 6})
	b[i+10], b[i+11] = 0x10, 0x27 // 10,000 entries declared
	_, err := open(t, b, testLim)
	if !errors.Is(err, ErrTooManyEntries) {
		t.Fatalf("err = %v, want ErrTooManyEntries from the declared count", err)
	}
}

// forged returns an archive whose one member inflates to n bytes while BOTH its local and its central header
// claim declared: the sizes Open sums and a lying producer controls.
func forged(t *testing.T, n int, declared uint32) []byte {
	t.Helper()
	b := build(t, []string{"a"}, [][]byte{bytes.Repeat([]byte{0}, n)})
	put := func(sig []byte, off int) {
		i := bytes.Index(b, sig)
		if i < 0 {
			t.Fatalf("no %x record", sig)
		}
		b[i+off], b[i+off+1], b[i+off+2], b[i+off+3] = byte(declared), byte(declared>>8), byte(declared>>16), byte(declared>>24)
	}
	put([]byte{'P', 'K', 3, 4}, 22) // local header: uncompressed size
	put([]byte{'P', 'K', 1, 2}, 24) // central directory: uncompressed size
	return b
}

func TestReadHoldsItsCapAgainstAForgedHeader(t *testing.T) {
	lim := testLim
	lim.RatioMax = 1 << 30 // the ratio guard is not the one under test
	a, err := open(t, forged(t, 5000, 10), lim)
	if err != nil {
		t.Fatalf("the forged archive must open — Open trusts the declared 10 bytes: %v", err)
	}
	if got := a.Files()[0].UncompressedSize64; got != 10 {
		t.Fatalf("setup: header says %d, want the forged 10", got)
	}
	// archive/zip itself stops a stream at its header's size (ErrFormat), so the refusal here is ErrUnreadable;
	// the LimitReader is the second wall. What must never happen is the bytes coming back.
	b, err := a.Read(a.Files()[0], 100)
	if err == nil || len(b) != 0 || !(errors.Is(err, ErrTooLarge) || errors.Is(err, ErrUnreadable)) {
		t.Fatalf("a 10-byte member that inflates to 5000: len %d err %v, want a refusal and no bytes", len(b), err)
	}
	// And a header that agrees with the stream but not with the caller's cap is ErrTooLarge, from the stream.
	h, err := open(t, forged(t, 5000, 5000), lim)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.Read(h.Files()[0], 100); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("5000 real bytes against a cap of 100: %v", err)
	}
	if b, err := a.Peek(a.Files()[0], 5); err != nil || len(b) != 5 {
		t.Fatalf("Peek reads a bounded prefix of the real stream: len %d err %v", len(b), err)
	}
}
