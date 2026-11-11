package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nib/internal/ceremony"
	"nib/internal/pdfops"
	"nib/internal/sign"
	"nib/internal/testpdf"
)

// The page objects' routes (PLAN-image-edit P01.S02).

func pageObjectsOpen(t *testing.T, c *http.Client, base string) pdfops.PageObjects {
	t.Helper()
	var got pdfops.PageObjects
	if err := json.Unmarshal(getBytes(t, c, base+"/api/page/objects?page=1"), &got); err != nil {
		t.Fatal(err)
	}
	return got
}

func removeBody(page, ordinal int, digest string) map[string]any {
	return map[string]any{"edits": []map[string]any{{"kind": "delete", "page": page, "ordinal": ordinal, "digest": digest}}}
}

func moveBody(page, ordinal int, digest string, l, t, r, b float64) map[string]any {
	return map[string]any{"edits": []map[string]any{{"kind": "move", "page": page, "ordinal": ordinal, "digest": digest, "box": []float64{l, t, r, b}}}}
}

// The window's move (PLAN-image-edit P02.S03): the route installs it, the picture reads back at the box asked, the
// other picture is untouched, the old address is stale, and Undo brings the document back byte for byte.
func TestThePageObjectsRouteMovesAPictureAndUndoPutsItBack(t *testing.T) {
	src := testpdf.TwoPictures()
	base, c, csrf := openTagsFixture(t, src)
	original := getBytes(t, c, base+"/api/pdf")
	objs := pageObjectsOpen(t, c, base)
	if len(objs.Objects) != 2 || !objs.Objects[0].Movable {
		t.Fatalf("listed %+v", objs.Objects)
	}
	code, body := postTags(t, c, csrf, base+"/api/page/objects", moveBody(1, 0, objs.Objects[0].Digest, 0.5, 0.5, 0.7, 0.6))
	if code != http.StatusOK {
		t.Fatalf("move = %d: %s", code, body)
	}
	var answer struct {
		Removed []pdfops.RemovedObject `json:"removed"`
		Moved   []pdfops.MovedObject   `json:"moved"`
	}
	if err := json.Unmarshal([]byte(body), &answer); err != nil || len(answer.Moved) != 1 || answer.Moved[0].Ordinal != 0 || len(answer.Removed) != 0 {
		t.Fatalf("answer %s (%v)", body, err)
	}
	after := pageObjectsOpen(t, c, base)
	want := pdfops.MapRect{0.5, 0.5, 0.7, 0.6}
	for k := range want {
		if d := after.Objects[0].Box[k] - want[k]; d > 1e-6 || d < -1e-6 {
			t.Fatalf("the picture reads at %v, asked %v", after.Objects[0].Box, want)
		}
	}
	if after.Objects[1].Digest != objs.Objects[1].Digest {
		t.Fatal("the other picture changed")
	}
	if code, body := postTags(t, c, csrf, base+"/api/page/objects", moveBody(1, 0, objs.Objects[0].Digest, 0.1, 0.1, 0.2, 0.2)); code != http.StatusConflict ||
		objectRefusalCause(t, body) != pdfops.CauseObjectChanged {
		t.Fatalf("the old address = %d %s, want 409 object-changed", code, body)
	}
	if code, body := postTags(t, c, csrf, base+"/api/undo", map[string]any{}); code != http.StatusOK {
		t.Fatalf("undo = %d: %s", code, body)
	}
	if !bytes.Equal(original, getBytes(t, c, base+"/api/pdf")) {
		t.Fatal("Undo did not bring back the document as it was")
	}
}

func objectRefusalCause(t *testing.T, body string) string {
	t.Helper()
	var r struct{ Cause, Error string }
	if err := json.Unmarshal([]byte(body), &r); err != nil {
		t.Fatalf("a refusal that is not {cause, error}: %q", body)
	}
	return r.Cause
}

func TestThePageObjectsRouteRemovesAPictureAndUndoBringsItBack(t *testing.T) {
	src := testpdf.TwoPictures()
	base, c, csrf := openTagsFixture(t, src)
	original := getBytes(t, c, base+"/api/pdf")
	objs := pageObjectsOpen(t, c, base)
	if len(objs.Objects) != 2 || !objs.Objects[0].Editable {
		t.Fatalf("listed %+v", objs.Objects)
	}
	if !bytes.Equal(original, getBytes(t, c, base+"/api/pdf")) {
		t.Fatal("listing the pictures changed the document")
	}
	code, body := postTags(t, c, csrf, base+"/api/page/objects", removeBody(1, 0, objs.Objects[0].Digest))
	if code != http.StatusOK {
		t.Fatalf("remove = %d: %s", code, body)
	}
	var answer struct {
		Rev     int                    `json:"rev"`
		Removed []pdfops.RemovedObject `json:"removed"`
	}
	if err := json.Unmarshal([]byte(body), &answer); err != nil || len(answer.Removed) != 1 || answer.Removed[0].Ordinal != 0 {
		t.Fatalf("answer %s (%v)", body, err)
	}
	after := pageObjectsOpen(t, c, base)
	if len(after.Objects) != 1 || after.Objects[0].Digest != objs.Objects[1].Digest {
		t.Fatalf("after the removal the page lists %+v", after.Objects)
	}
	if bytes.Contains(getBytes(t, c, base+"/api/pdf"), []byte("NIBPLANTEDIMAGEA")) {
		t.Fatal("the removed picture's bytes are still served")
	}
	if code, body := postTags(t, c, csrf, base+"/api/undo", map[string]any{}); code != http.StatusOK {
		t.Fatalf("undo = %d: %s", code, body)
	}
	if !bytes.Equal(original, getBytes(t, c, base+"/api/pdf")) {
		t.Fatal("Undo did not bring back the document as it was")
	}
}

func TestThePageObjectsRouteRefusesByCause(t *testing.T) {
	base, c, csrf := openTagsFixture(t, testpdf.TwoPictures())
	objs := pageObjectsOpen(t, c, base)
	before := getBytes(t, c, base+"/api/pdf")
	for _, tc := range []struct {
		name  string
		body  map[string]any
		code  int
		cause string
	}{
		{"a stale digest", removeBody(1, 0, strings.Repeat("0", 64)), http.StatusConflict, pdfops.CauseObjectChanged},
		{"the right digest at the wrong ordinal", removeBody(1, 1, objs.Objects[0].Digest), http.StatusConflict, pdfops.CauseObjectChanged},
		{"no edit", map[string]any{"edits": []any{}}, http.StatusBadRequest, pdfops.CauseBadRequest},
		{"an unknown field", withField(removeBody(1, 0, objs.Objects[0].Digest), "baseRev", 3), http.StatusBadRequest, pdfops.CauseBadRequest},
		{"a move with no box", map[string]any{"edits": []map[string]any{{"kind": "move", "page": 1, "ordinal": 0, "digest": objs.Objects[0].Digest}}}, http.StatusBadRequest, pdfops.CauseBadRequest},
		{"a removal with a box", map[string]any{"edits": []map[string]any{{"kind": "delete", "page": 1, "ordinal": 0, "digest": objs.Objects[0].Digest, "box": []float64{0.1, 0.1, 0.2, 0.2}}}}, http.StatusBadRequest, pdfops.CauseBadRequest},
		{"a move's box off the page", moveBody(1, 0, objs.Objects[0].Digest, 0.1, 0.1, 1.2, 0.2), http.StatusBadRequest, pdfops.CauseBadRequest},
		// Go drops a fifth number into a fixed array without a word: the box is read as a slice and counted.
		{"a box of five numbers", map[string]any{"edits": []map[string]any{{"kind": "move", "page": 1, "ordinal": 0, "digest": objs.Objects[0].Digest, "box": []float64{0.1, 0.1, 0.3, 0.3, 9}}}}, http.StatusBadRequest, pdfops.CauseBadRequest},
		{"a box of three numbers", map[string]any{"edits": []map[string]any{{"kind": "move", "page": 1, "ordinal": 0, "digest": objs.Objects[0].Digest, "box": []float64{0.1, 0.1, 0.3}}}}, http.StatusBadRequest, pdfops.CauseBadRequest},
	} {
		code, body := postTags(t, c, csrf, base+"/api/page/objects", tc.body)
		if code != tc.code || objectRefusalCause(t, body) != tc.cause {
			t.Errorf("%s: %d %s, want %d %s", tc.name, code, body, tc.code, tc.cause)
		}
	}
	if !bytes.Equal(before, getBytes(t, c, base+"/api/pdf")) {
		t.Fatal("a refused removal changed the document")
	}
	for _, q := range []string{"", "?page=0", "?page=x", "?page=9"} {
		if code, _ := getAnswer(t, c, base+"/api/page/objects"+q); code != http.StatusBadRequest {
			t.Errorf("objects%s = %d, want 400", q, code)
		}
	}
}

// TestALostUpdateIsObjectChanged — an install landing between the edit's read and its commit is the route's one 409,
// with the cause the window recovers from; the byte cap keeps the shared answer.
func TestALostUpdateIsObjectChanged(t *testing.T) {
	rec := httptest.NewRecorder()
	if !wroteObjectChanged(rec, errDocChanged) {
		t.Fatal("a lost update was not answered")
	}
	if rec.Code != http.StatusConflict || objectRefusalCause(t, rec.Body.String()) != pdfops.CauseObjectChanged {
		t.Fatalf("a lost update answered %d %s, want 409 %s", rec.Code, rec.Body, pdfops.CauseObjectChanged)
	}
	if wroteObjectChanged(httptest.NewRecorder(), ErrTooManyBytes) || wroteObjectChanged(httptest.NewRecorder(), nil) {
		t.Fatal("a commit refusal that is not a lost update, or no refusal, was answered as object-changed")
	}
}

func TestASignedDocumentIsRefusedAtTheRoute(t *testing.T) {
	certPEM, keyPEM, err := sign.GenerateIdentity("Nib Test")
	if err != nil {
		t.Fatal(err)
	}
	signed, err := sign.Sign(testpdf.TwoPictures(), certPEM, keyPEM, sign.Options{Name: "Nib Test", When: time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	refusedAtTheRoute(t, signed, pdfops.CauseSigned)
}

// TestAConvenedDocumentIsRefusedAtTheRouteWhenReopenedCold — the file carries a record and this server convened
// nothing, so only the record can refuse it (D5's pin: `doc.ceremony` and `hasLiveCeremony` are not the test).
func TestAConvenedDocumentIsRefusedAtTheRouteWhenReopenedCold(t *testing.T) {
	pdf := testpdf.TwoPictures()
	hash, err := ceremony.DocumentHash(pdf)
	if err != nil {
		t.Fatal(err)
	}
	fp := sha256.Sum256([]byte("convener"))
	convened, err := ceremony.Embed(pdf, ceremony.Record{ID: "picture-ceremony", DocHash: hash, Intent: "sign the lease",
		Expires: time.Now().Add(time.Hour), Roster: []ceremony.Party{{Fingerprint: hex.EncodeToString(fp[:]), Label: "Convener", Signs: true}}})
	if err != nil {
		t.Fatal(err)
	}
	refusedAtTheRoute(t, convened, pdfops.CauseCeremony)
}

// replaceDoorImports is how many imports the replace route has made, by every outcome (`import.ok{door=replace,…}`,
// `import.refused{door=replace,…}`).
func replaceDoorImports() uint64 {
	var n uint64
	for k, v := range pdfops.ImportCounts() {
		if strings.Contains(k, "{door=replace,") {
			n += v
		}
	}
	return n
}

// postReplace posts a replace: the picture as the `file` part, the edit as the `edits` field.
func postReplace(t *testing.T, c *http.Client, csrf, base string, file []byte, name string, edit map[string]any) (int, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", name)
	fw.Write(file)
	edits, _ := json.Marshal(map[string]any{"edits": []map[string]any{edit}})
	mw.WriteField("edits", string(edits))
	mw.Close()
	r := write(t, c, csrf, http.MethodPost, base+"/api/page/objects/replace", mw.FormDataContentType(), &buf)
	defer r.Body.Close()
	b, _ := io.ReadAll(r.Body)
	return r.StatusCode, string(b)
}

func replacePNG(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 20, G: uint8(x), B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestThePageObjectsRouteReplacesAPictureAndUndoPutsItBack — the multipart route: the new picture installs, the other
// picture is untouched, the answer says what was done, and Undo brings the document back byte for byte.
func TestThePageObjectsRouteReplacesAPictureAndUndoPutsItBack(t *testing.T) {
	base, c, csrf := openTagsFixture(t, testpdf.TwoPictures())
	original := getBytes(t, c, base+"/api/pdf")
	objs := pageObjectsOpen(t, c, base)
	code, body := postReplace(t, c, csrf, base, replacePNG(t, 40, 20), "new.png",
		map[string]any{"kind": "replace", "page": 1, "ordinal": 0, "digest": objs.Objects[0].Digest})
	if code != http.StatusOK {
		t.Fatalf("replace = %d: %s", code, body)
	}
	if replaceDoorImports() == 0 {
		t.Fatal("the replace door's import count did not move: the refused-document test's unmoved count would prove nothing")
	}
	var answer struct {
		Replaced []pdfops.ReplacedObject `json:"replaced"`
	}
	if err := json.Unmarshal([]byte(body), &answer); err != nil || len(answer.Replaced) != 1 || answer.Replaced[0].Ordinal != 0 {
		t.Fatalf("answer %s (%v)", body, err)
	}
	after := pageObjectsOpen(t, c, base)
	if len(after.Objects) != 2 || after.Objects[0].Digest == objs.Objects[0].Digest || after.Objects[1].Digest != objs.Objects[1].Digest {
		t.Fatalf("after: %+v", after.Objects)
	}
	if code, body := postTags(t, c, csrf, base+"/api/undo", map[string]any{}); code != http.StatusOK {
		t.Fatalf("undo = %d: %s", code, body)
	}
	if !bytes.Equal(original, getBytes(t, c, base+"/api/pdf")) {
		t.Fatal("Undo did not bring back the document as it was")
	}
}

// TestTheReplaceRouteRefusesByCause — a PDF named as a picture, a move posted here, a stale address; none changes the
// document.
func TestTheReplaceRouteRefusesByCause(t *testing.T) {
	base, c, csrf := openTagsFixture(t, testpdf.TwoPictures())
	before := getBytes(t, c, base+"/api/pdf")
	d := pageObjectsOpen(t, c, base).Objects[0].Digest
	edit := map[string]any{"kind": "replace", "page": 1, "ordinal": 0, "digest": d}
	if code, body := postReplace(t, c, csrf, base, before, "sneaky.png", edit); code != http.StatusUnsupportedMediaType || !strings.Contains(body, "kind of picture") {
		t.Errorf("a PDF named .png = %d %s, want 415", code, body)
	}
	if code, body := postReplace(t, c, csrf, base, replacePNG(t, 8, 8), "x.png", map[string]any{"kind": "move", "page": 1, "ordinal": 0, "digest": d, "box": []float64{0.1, 0.1, 0.2, 0.2}}); code != http.StatusBadRequest || objectRefusalCause(t, body) != pdfops.CauseBadRequest {
		t.Errorf("a move here = %d %s, want 400", code, body)
	}
	if code, body := postReplace(t, c, csrf, base, replacePNG(t, 8, 8), "x.png", map[string]any{"kind": "replace", "page": 1, "ordinal": 0, "digest": strings.Repeat("0", 64)}); code != http.StatusConflict || objectRefusalCause(t, body) != pdfops.CauseObjectChanged {
		t.Errorf("a stale address = %d %s, want 409", code, body)
	}
	if !bytes.Equal(before, getBytes(t, c, base+"/api/pdf")) {
		t.Fatal("a refused replace changed the document")
	}
}

func refusedAtTheRoute(t *testing.T, pdf []byte, cause string) {
	t.Helper()
	base, c, csrf := openTagsFixture(t, pdf)
	objs := pageObjectsOpen(t, c, base)
	if len(objs.Objects) != 2 || objs.Objects[0].Editable || objs.Objects[0].Why != cause {
		t.Fatalf("listed %+v, want every picture refused as %s", objs.Objects, cause)
	}
	before := getBytes(t, c, base+"/api/pdf")
	code, body := postTags(t, c, csrf, base+"/api/page/objects", removeBody(1, 0, objs.Objects[0].Digest))
	if code != http.StatusUnprocessableEntity || objectRefusalCause(t, body) != cause {
		t.Fatalf("remove = %d %s, want 422 %s", code, body, cause)
	}
	// A replace is refused before its picture is imported: the replace door's import count does not move.
	imports := replaceDoorImports()
	code, body = postReplace(t, c, csrf, base, replacePNG(t, 8, 8), "x.png",
		map[string]any{"kind": "replace", "page": 1, "ordinal": 0, "digest": objs.Objects[0].Digest})
	if code != http.StatusUnprocessableEntity || objectRefusalCause(t, body) != cause {
		t.Fatalf("replace = %d %s, want 422 %s", code, body, cause)
	}
	if got := replaceDoorImports(); got != imports {
		t.Fatalf("a refused document's picture was imported (%d imports, was %d)", got, imports)
	}
	if !bytes.Equal(before, getBytes(t, c, base+"/api/pdf")) {
		t.Fatal("the refused document changed")
	}
}

// withField is body with one more top-level field: a valid edit beside a field the route does not read, so only the
// unknown-field refusal can answer it.
func withField(body map[string]any, k string, v any) map[string]any {
	out := map[string]any{k: v}
	for key, val := range body {
		out[key] = val
	}
	return out
}

// Drawings at the route (PLAN-image-edit P03.S03): while `drawingsInWindow` is off the listing offers the window none,
// and an edit naming a drawing by the address `nib objects` hands out is still made, as one Undo step.
func TestTheRouteOffersNoDrawingsWhileTheGroupingIsBelowItsThresholdAndStillEditsOne(t *testing.T) {
	src := testpdf.PictureAndDrawings()
	base, c, csrf := openTagsFixture(t, src)
	original := getBytes(t, c, base+"/api/pdf")
	listed := pageObjectsOpen(t, c, base)
	if drawingsInWindow || len(listed.Objects) != 1 || len(listed.Drawings) != 0 {
		t.Fatalf("with the switch %v the window was offered %d drawing(s) and %d picture(s)", drawingsInWindow, len(listed.Drawings), len(listed.Objects))
	}
	objs, err := pdfops.ReadPageObjects(src, 1)
	if err != nil || len(objs.Drawings) != 2 {
		t.Fatalf("setup: the document draws %d drawings (%v)", len(objs.Drawings), err)
	}
	body := map[string]any{"edits": []map[string]any{{"kind": "delete", "object": "drawing", "page": 1, "ordinal": 0, "digest": objs.Drawings[0].Digest}}}
	code, out := postTags(t, c, csrf, base+"/api/page/objects", body)
	var answer struct {
		Removed []pdfops.RemovedObject `json:"removed"`
	}
	if code != http.StatusOK || json.Unmarshal([]byte(out), &answer) != nil || len(answer.Removed) != 1 || answer.Removed[0].Object != pdfops.ObjectDrawing {
		t.Fatalf("removing drawing 0 = %d: %s", code, out)
	}
	if after, err := pdfops.ReadPageObjects(getBytes(t, c, base+"/api/pdf"), 1); err != nil || len(after.Drawings) != 1 || len(after.Objects) != 1 {
		t.Fatalf("after the removal the page draws %+v (%v)", after, err)
	}
	if code, body := postTags(t, c, csrf, base+"/api/undo", map[string]any{}); code != http.StatusOK {
		t.Fatalf("undo = %d: %s", code, body)
	}
	if !bytes.Equal(original, getBytes(t, c, base+"/api/pdf")) {
		t.Fatal("Undo did not bring back the document as it was")
	}
}
