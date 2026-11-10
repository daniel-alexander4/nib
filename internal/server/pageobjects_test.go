package server

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
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
