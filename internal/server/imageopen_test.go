package server

import (
	"bytes"
	"context"
	"encoding/binary"
	"hash/crc32"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
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
		if d.name == "screenshot.pdf" {
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
// sniff and is refused by its CONTENT: the name says an image and the bytes are not one.
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
	if !bytes.Contains([]byte(body), []byte("disagree")) {
		t.Errorf("the refusal reads %q; a name that says PNG over text must say the two disagree", body)
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

// TestADocumentIsJudgedByItsBytesAndItsNameTogether — D32, replacing /pending 541's refusal by name.
//
// A `.docx` reaching an open route used to be refused as "convertible, through another door": true,
// and useless, which is why the door became the same function everywhere (D8). Now the open route
// converts it; what is refused is a file whose BYTES contradict its name or are not what its name says.
// Text with a Markdown name is a document; a "docx" that is not a package is refused by name; nothing
// Nib has a route for is refused plainly.
func TestADocumentIsJudgedByItsBytesAndItsNameTogether(t *testing.T) {
	ts, _ := startServerWith(t)
	c, csrf := authedClient(t, ts)
	dir := t.TempDir()

	for _, c2 := range []struct {
		name string
		body []byte
		code int
		says string
	}{
		{"report.docx", []byte("PK\x03\x04 not really a docx"), http.StatusUnsupportedMediaType, "package"},
		{"sheet.xlsx", []byte("plain text"), http.StatusUnsupportedMediaType, "disagree"},
		{"notes.md", []byte("# A heading\n\nSome prose.\n"), http.StatusOK, ""},
		{"mystery.bin", []byte("\x00\x01\x02 nothing nib opens"), http.StatusUnsupportedMediaType, "isn't a PDF"},
	} {
		t.Run(c2.name, func(t *testing.T) {
			p := filepath.Join(dir, c2.name)
			if err := os.WriteFile(p, c2.body, 0o644); err != nil {
				t.Fatal(err)
			}
			code, body := postForCode(t, c, csrf, ts.URL+"/api/open", openRequest{Path: p})
			if code != c2.code {
				t.Fatalf("code = %d, want %d (body %s)", code, c2.code, body)
			}
			if c2.says != "" && !bytes.Contains([]byte(body), []byte(c2.says)) {
				t.Errorf("the refusal reads %q, want it to say %q", body, c2.says)
			}
		})
	}
}

// TestTheUploadRouteDecidesThroughTheSameDoorAsThePathRoute — 541's second half.
//
// `readInstallablePDF` is "THE door onto 'this file may become a document'" (ADR-009), and
// `handleUpload` had copied its check rather than sharing it — so the door had a declared
// population of three and a fourth site that could drift. Asserting the OUTCOMES agree is what
// catches a drift; asserting that one function calls another would pass over two copies that
// happen to agree today.
func TestTheUploadRouteDecidesThroughTheSameDoorAsThePathRoute(t *testing.T) {
	ts, _ := startServerWith(t)
	c, csrf := authedClient(t, ts)
	dir := t.TempDir()

	for _, name := range []string{"report.docx", "sheet.xlsx", "mystery.bin"} {
		body := []byte("PK\x03\x04 or whatever; the name is what routes")
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, body, 0o644); err != nil {
			t.Fatal(err)
		}
		_, viaPath := postForCode(t, c, csrf, ts.URL+"/api/open", openRequest{Path: p})
		viaUpload := uploadForBody(t, c, csrf, ts.URL, name, body)
		if viaPath != viaUpload {
			t.Errorf("%s: the path route says %q and the upload route says %q — one decision, two "+
				"doors, and they have drifted", name, viaPath, viaUpload)
		}
	}
}

// uploadForBody posts one file to /api/upload and returns the response body, whatever the status.
func uploadForBody(t *testing.T, c *http.Client, csrf, base, name string, body []byte) string {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, err := mw.CreateFormFile("file", name)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(body); err != nil {
		t.Fatal(err)
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, base+"/api/upload", &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-CSRF-Token", csrf)
	res, err := c.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	out, _ := io.ReadAll(res.Body)
	return strings.TrimSpace(string(out))
}

// TestTheHandOffSpeaksForARefusedDocumentRatherThanFallingThrough — 541's named trap, kept.
//
// `openHandedOff` switches on `ref.kind` with a `default` arm, and Go has no exhaustive switch — so a
// new `refusalKind` does not fail to compile there. It would silently degrade to "that file could not be
// opened" for a document Nib actually has an opinion about, and every existing test would still pass.
// The import kind gets an arm of its own and the arm gets an assertion: the sentence is `Import`'s.
func TestTheHandOffSpeaksForARefusedDocumentRatherThanFallingThrough(t *testing.T) {
	_, srv := startServerWith(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "handed-over.docx")
	if err := os.WriteFile(p, []byte("PK\x03\x04 a document the OS handed to nib"), 0o644); err != nil {
		t.Fatal(err)
	}

	// The hand-off's own door, not the HTTP route: that is the surface with the `default` arm.
	_, _, err := srv.openHandedOff(context.Background(), p)
	if err == nil {
		t.Fatal("the hand-off accepted a corrupt .docx as a document")
	}
	got := err.Error()
	if strings.Contains(got, "could not be opened") {
		t.Errorf("the hand-off fell through to its default arm: %q. That is the sentence for a file "+
			"nothing could read, and this is a refusal with a cause of its own", got)
	}
	if !strings.Contains(got, "package") {
		t.Errorf("the hand-off says %q and not the import function's own sentence", got)
	}
}

// TestOpenRecentRecordsTheIMAGEsPathNotTheDocumentsEmptyOne — /pending 539(c).
//
// The two paths are deliberately different and it would be natural to "fix" that: the DOCUMENT is
// pathless, so `canSave` is false and Save routes to Save As rather than writing PDF bytes over the
// user's PNG; **Recent** must still hold the real file, or the image never appears in Open Recent
// and reopening it is impossible. `AddRecent` takes the source path for that reason, and a change
// routing it through `doc.path` — which is empty here by design — would lose the entry silently.
func TestOpenRecentRecordsTheIMAGEsPathNotTheDocumentsEmptyOne(t *testing.T) {
	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)
	dir := t.TempDir()
	imgPath := filepath.Join(dir, "receipt.png")
	if err := os.WriteFile(imgPath, testPNG(t, 64, 32), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, body := postForCode(t, c, csrf, ts.URL+"/api/open", openRequest{Path: imgPath}); code != http.StatusOK {
		t.Fatalf("open: %d %s", code, body)
	}

	// The document is pathless — the other half of the same rule, asserted here so the two cannot
	// drift into agreeing.
	srv.mu.Lock()
	var doc *document
	for _, d := range srv.docs {
		if d.name == "receipt.pdf" {
			doc = d
		}
	}
	srv.mu.Unlock()
	if doc == nil {
		t.Fatal("the opened image installed no document")
	}
	if doc.path != "" {
		t.Errorf("the opened image kept path %q; it must be pathless or Save overwrites the PNG", doc.path)
	}

	found := false
	for _, r := range srv.vault.Recent() {
		if r == imgPath {
			found = true
		}
	}
	if !found {
		t.Errorf("Open Recent does not hold %q after opening it. The document is pathless by design, "+
			"and Recent is what makes the image reachable again — recorded: %v", imgPath, srv.vault.Recent())
	}
}
