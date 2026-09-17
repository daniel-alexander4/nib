package server

import (
	"bytes"
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

// TestAConvertibleDocumentIsRefusedByNAMEnotByTheWrongSentence — /pending 541.
//
// A `.docx` reaching this route was refused with "that file isn't a PDF": true, useless, and
// **identical whether or not LibreOffice is installed**, so it read as a missing-converter problem
// when it is a routing one. nib converts that type through Open & convert to PDF…, and the server
// is in a position to say so — it holds the name, and `pdfops.SupportedDocExt` is the same
// predicate `/api/office` routes on.
func TestAConvertibleDocumentIsRefusedByNAMEnotByTheWrongSentence(t *testing.T) {
	ts, _ := startServerWith(t)
	c, csrf := authedClient(t, ts)
	dir := t.TempDir()

	for _, c2 := range []struct {
		name       string
		body       []byte
		wantSaysNo bool // true: the plain not-a-PDF sentence is correct for it
	}{
		{"report.docx", []byte("PK\x03\x04 not really a docx, but the NAME is what routes"), false},
		{"notes.md", []byte("# A heading\n\nSome prose.\n"), false},
		{"sheet.xlsx", []byte("PK\x03\x04"), false},
		{"mystery.bin", []byte("\x00\x01\x02 nothing nib opens"), true},
	} {
		t.Run(c2.name, func(t *testing.T) {
			p := filepath.Join(dir, c2.name)
			if err := os.WriteFile(p, c2.body, 0o644); err != nil {
				t.Fatal(err)
			}
			code, body := postForCode(t, c, csrf, ts.URL+"/api/open", openRequest{Path: p})
			if code != http.StatusUnsupportedMediaType {
				t.Fatalf("code = %d, want 415 (body %s)", code, body)
			}
			plain := bytes.Contains([]byte(body), []byte("isn't a PDF"))
			if c2.wantSaysNo {
				if !plain {
					t.Errorf("a file nib has no route for was refused with %q; the plain sentence is "+
						"the right one for it", body)
				}
				return
			}
			if plain {
				t.Errorf("%s was refused with %q. nib converts that type through another door, and "+
					"this sentence is both true and useless — it is the same one a user gets for a "+
					"file nib cannot open at all", c2.name, body)
			}
			// "convert to PDF", not the full label: Go's JSON encoder escapes & as \u0026, so
			// matching the label verbatim asserts the encoding rather than the sentence.
			if !bytes.Contains([]byte(body), []byte("convert to PDF")) {
				t.Errorf("the refusal for %s reads %q and names no route the user can take", c2.name, body)
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

	for _, name := range []string{"report.docx", "notes.md", "mystery.bin"} {
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

// TestTheHandOffSpeaksForAConvertibleDocumentRatherThanFallingThrough — 541's named trap.
//
// `openHandedOff` switches on `ref.kind` with a `default` arm, and Go has no exhaustive switch —
// so a new `refusalKind` does not fail to compile there. It would silently degrade to "that file
// could not be opened" for a document nib actually converts, and every existing test would still
// pass. The item that proposed this kind called that arm "the trap in this item", which is why the
// arm gets an assertion of its own rather than being assumed correct because it was written.
func TestTheHandOffSpeaksForAConvertibleDocumentRatherThanFallingThrough(t *testing.T) {
	_, srv := startServerWith(t)
	dir := t.TempDir()
	p := filepath.Join(dir, "handed-over.docx")
	if err := os.WriteFile(p, []byte("PK\x03\x04 a document the OS handed to nib"), 0o644); err != nil {
		t.Fatal(err)
	}

	// The hand-off's own door, not the HTTP route: that is the surface with the `default` arm.
	err := srv.openHandedOff(p)
	if err == nil {
		t.Fatal("the hand-off accepted a .docx as a document")
	}
	got := err.Error()
	if strings.Contains(got, "could not be opened") {
		t.Errorf("the hand-off fell through to its default arm: %q. That is the sentence for a file "+
			"nothing could read, and this is a document nib converts — the new kind reached the "+
			"default rather than an arm of its own", got)
	}
	if !strings.Contains(got, "convert to PDF") {
		t.Errorf("the hand-off says %q and names no route the user can take", got)
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
		if d.name == "receipt.png" {
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
