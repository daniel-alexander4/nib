package server

import (
	"bytes"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// `/pending 400`'s server-side readers — an opened image is a document, and it is PATHLESS.

func testPNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 90, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestAPathOpenedImageIsPathlessSoSaveCannotDestroyIt — the hazard this whole route turns on.
//
// `readInstallablePDF`'s own comment names it: a path-opened document reports canSave, and
// "a non-PDF installed here would be overwritten with PDF bytes by the next Save — the one open
// surface where getting this wrong destroys the file." An image is exactly that case: the bytes on
// disk are a PNG and the document in memory is a PDF built from them.
//
// The assertion is on the document's PATH, which is what grants canSave — not on a save having been
// attempted, because a test that only checked "the file is still a PNG afterwards" would pass on a
// build where Save was simply never called.
func TestAPathOpenedImageIsPathlessSoSaveCannotDestroyIt(t *testing.T) {
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)

	dir := t.TempDir()
	imgPath := filepath.Join(dir, "screenshot.png")
	original := testPNG(t, 192, 96)
	if err := os.WriteFile(imgPath, original, 0o644); err != nil {
		t.Fatal(err)
	}

	code, body := postForCode(t, c, csrf, ts.URL+"/api/open", openRequest{Path: imgPath})
	if code != http.StatusOK {
		t.Fatalf("opening a PNG was refused: %d %s", code, body)
	}

	srv.mu.Lock()
	defer srv.mu.Unlock()
	if len(srv.docs) == 0 {
		t.Fatal("the open reported success and installed no document")
	}
	var found *document
	for _, d := range srv.docs {
		if d.name == "screenshot.png" {
			found = d
		}
	}
	if found == nil {
		t.Fatal("no document carries the opened file's name, so the tab would read Untitled")
	}
	if found.path != "" {
		t.Errorf("the opened image kept path %q. A document with a path reports canSave, and the "+
			"next Save would write PDF bytes over the user's PNG", found.path)
	}
	if !bytes.HasPrefix(found.data, []byte("%PDF-")) {
		t.Error("the installed document is not a PDF, so the conversion did not run")
	}

	// The file on disk is untouched — the other half of the same claim, and the one a user checks.
	after, err := os.ReadFile(imgPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, original) {
		t.Error("the source image changed on disk merely by being opened")
	}
}

// TestANonImageNonPDFIsStillRefusedWithTheExistingSentence — the acceptance's fourth clause.
//
// The new route must widen what opens, not what is accepted. A text file named `.png` reaches the
// sniff and is refused by its CONTENT, with the sentence the route already printed.
func TestANonImageNonPDFIsStillRefusedWithTheExistingSentence(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	dir := t.TempDir()
	p := filepath.Join(dir, "not-really.png")
	if err := os.WriteFile(p, []byte("plain text wearing a .png extension\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	code, body := postForCode(t, c, csrf, ts.URL+"/api/open", openRequest{Path: p})
	if code != http.StatusUnsupportedMediaType {
		t.Fatalf("code = %d, want 415 — a file is judged by its bytes, never its extension (body %s)", code, body)
	}
	if !bytes.Contains([]byte(body), []byte("isn't a PDF")) {
		t.Errorf("the refusal reads %q; the existing sentence is what a user of this route already knows", body)
	}
}

// TestAnImageTooLargeToDecodeIsRefusedWithItsOwnSentence — an image nib recognises and declines is
// not "that file isn't a PDF", which would be a lie about a PNG.
func TestAnImageTooLargeToDecodeIsRefusedWithItsOwnSentence(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	huge := append([]byte{}, testPNG(t, 4, 4)...)
	binary.BigEndian.PutUint32(huge[16:20], 30000)
	binary.BigEndian.PutUint32(huge[20:24], 30000)
	ih := crc32.NewIEEE()
	ih.Write(huge[12:29])
	binary.BigEndian.PutUint32(huge[29:33], ih.Sum32())

	dir := t.TempDir()
	p := filepath.Join(dir, "enormous.png")
	if err := os.WriteFile(p, huge, 0o644); err != nil {
		t.Fatal(err)
	}
	code, body := postForCode(t, c, csrf, ts.URL+"/api/open", openRequest{Path: p})
	if code != http.StatusUnsupportedMediaType {
		t.Fatalf("code = %d, want 415 (body %s)", code, body)
	}
	if bytes.Contains([]byte(body), []byte("isn't a PDF")) {
		t.Errorf("a PNG nib recognised and declined was refused as not-a-PDF: %q — the user picked "+
			"something nib says it opens, and that sentence is untrue of it", body)
	}
	if !bytes.Contains([]byte(body), []byte("megapixel")) {
		t.Errorf("the refusal reads %q and does not name the size", body)
	}
}
