package server

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nib/internal/pdfread"
	"nib/internal/testpdf"
)

// xfaFile writes a one-page document carrying an XFA packet — beside a field (a hybrid form) — and, where
// dynamic, `/NeedsRendering true`, and returns its path and bytes.
func xfaFile(t *testing.T, dynamic bool) (string, []byte) {
	t.Helper()
	extra := ""
	if dynamic {
		extra = " /NeedsRendering true"
	}
	packet := `<xdp:xdp xmlns:xdp="http://ns.adobe.com/xdp/"><template/></xdp:xdp>`
	doc := testpdf.Assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [6 0 R] /XFA 5 0 R >>" + extra + " >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Annots [6 0 R] >>",
		5: "<< /Length " + itoa(len(packet)) + " >>\nstream\n" + packet + "\nendstream",
		6: "<< /Type /Annot /Subtype /Widget /FT /Tx /T (name) /DA (/Helv 10 Tf 0 g) /Rect [10 10 100 30] /P 3 0 R >>",
	})
	path := filepath.Join(t.TempDir(), "form.pdf")
	if err := os.WriteFile(path, doc, 0o600); err != nil {
		t.Fatal(err)
	}
	return path, doc
}

// rotateXFA posts doc to `/api/pages` as a rotation and returns the status and the decoded body.
func rotateXFA(t *testing.T, c *http.Client, csrf, base string, doc []byte) (int, map[string]any) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
	fw.Write(doc)
	for k, v := range map[string]string{"op": "rotate", "deg": "90", "pages": "1"} {
		mw.WriteField(k, v)
	}
	mw.Close()
	resp := write(t, c, csrf, http.MethodPost, base+"/api/pages", mw.FormDataContentType(), &buf)
	defer resp.Body.Close()
	var got map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&got)
	return resp.StatusCode, got
}

// conveneOver asks for a ceremony over the open document.
func conveneOver(t *testing.T, c *http.Client, csrf, base string) (int, map[string]string) {
	t.Helper()
	code, body := postForCode(t, c, csrf, base+"/api/ceremony/convene", conveneRequest{
		Roster:        []convenePartyRequest{{Fingerprint: strings.Repeat("1a", 32), Label: "A", Signs: true}},
		Intent:        "We agree to co-sign",
		Expires:       time.Now().Add(48 * time.Hour).UTC().Format(time.RFC3339),
		ConvenerSigns: true,
	})
	var got map[string]string
	_ = json.Unmarshal([]byte(body), &got)
	return code, got
}

// ceremoniesOnDisk is how many ceremony folders this home holds.
func ceremoniesOnDisk(t *testing.T) int {
	t.Helper()
	entries, _ := os.ReadDir(filepath.Join(defaultOutputDir(), "ceremonies"))
	return len(entries)
}

// TestAStaticXFAFormIsAnnouncedRemovedByAChangeAndNeverConvenedOver — ADR-145, the static half, walked from the
// open to the routes: the open's answer says the document carries an XFA form; a ceremony is refused over it,
// 422 naming `xfa`, with nothing written; a change goes through, the result carries no form, and the document
// goes on saying it had one; after that a ceremony is no longer refused for it.
func TestAStaticXFAFormIsAnnouncedRemovedByAChangeAndNeverConvenedOver(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	path, doc := xfaFile(t, false)

	if dr := openByPath(t, ts.URL, c, csrf, path); dr.XFA != pdfread.XFAStatic {
		t.Fatalf("the open's answer says xfa=%q, want %q", dr.XFA, pdfread.XFAStatic)
	}
	// And every later answer for the same bytes — the cached one — says it too.
	if dr := getDoc(t, ts, c); dr.XFA != pdfread.XFAStatic {
		t.Errorf("a second answer for the same document says xfa=%q, want %q", dr.XFA, pdfread.XFAStatic)
	}
	code, got := conveneOver(t, c, csrf, ts.URL)
	if code != http.StatusUnprocessableEntity || got["cause"] != "xfa" || !strings.Contains(got["error"], "Remove active content") {
		t.Errorf("convene over a static XFA form: %d %v — want 422, cause xfa, and the thing to do", code, got)
	}
	if n := ceremoniesOnDisk(t); n != 0 {
		t.Errorf("a refused convene left %d ceremony folder(s) on disk", n)
	}

	status, body := rotateXFA(t, c, csrf, ts.URL, doc)
	if status != http.StatusOK {
		t.Fatalf("a change to a static XFA document: %d %v", status, body)
	}
	if body["xfa"] != xfaRemoved {
		t.Errorf("after the change the document says xfa=%v, want %q", body["xfa"], xfaRemoved)
	}
	resp, err := c.Get(ts.URL + "/api/pdf?auth=" + csrf)
	if err != nil {
		t.Fatal(err)
	}
	var now bytes.Buffer
	now.ReadFrom(resp.Body)
	resp.Body.Close()
	if read, rerr := pdfread.ReadXFA(now.Bytes()); rerr != nil || read != "" || bytes.Contains(now.Bytes(), []byte("NeedsRendering")) {
		t.Errorf("the changed document reads as XFA %q (%v)", read, rerr)
	}
	// A second answer for the same bytes, and one for bytes changed again, still say it.
	if status, body = rotateXFA(t, c, csrf, ts.URL, now.Bytes()); status != http.StatusOK || body["xfa"] != xfaRemoved {
		t.Errorf("a second change: %d, xfa=%v, want %q", status, body["xfa"], xfaRemoved)
	}
	if code, got := conveneOver(t, c, csrf, ts.URL); got["cause"] == "xfa" {
		t.Errorf("convene after the form was removed is still refused for it: %d %v", code, got)
	}
}

// TestADynamicXFADocumentIsAnnouncedAndNothingChangesIt — ADR-145, the dynamic half: the open's answer says so;
// an operation, a posted save and a ceremony are each a 422 naming the cause; and the document is the bytes it
// was opened with afterwards.
func TestADynamicXFADocumentIsAnnouncedAndNothingChangesIt(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	path, doc := xfaFile(t, true)

	if dr := openByPath(t, ts.URL, c, csrf, path); dr.XFA != pdfread.XFADynamic {
		t.Fatalf("the open's answer says xfa=%q, want %q", dr.XFA, pdfread.XFADynamic)
	}
	status, body := rotateXFA(t, c, csrf, ts.URL, doc)
	if status != http.StatusUnprocessableEntity || body["cause"] != "dynamic-xfa" || body["error"] == "" {
		t.Errorf("an operation on a dynamic XFA document: %d %v — want 422, cause dynamic-xfa, and the sentence", status, body)
	}
	edited := append(append([]byte(nil), doc...), []byte("\n% edited\n")...)
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/save", "application/pdf", bytes.NewReader(edited))
	var saved map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&saved)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity || saved["cause"] != "dynamic-xfa" {
		t.Errorf("a posted save of a dynamic XFA document: %d %v — want 422, cause dynamic-xfa", resp.StatusCode, saved)
	}
	if onDisk, _ := os.ReadFile(path); !bytes.Equal(onDisk, doc) {
		t.Error("the refused save wrote the file")
	}
	// Signing is a change too: a placeholder is not finalized.
	var fbuf bytes.Buffer
	mw := multipart.NewWriter(&fbuf)
	fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
	fw.Write(doc)
	mw.WriteField("params", `{"reason":"Finalized in Nib"}`)
	mw.Close()
	fresp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/finalize", mw.FormDataContentType(), &fbuf)
	var finalized map[string]string
	_ = json.NewDecoder(fresp.Body).Decode(&finalized)
	fresp.Body.Close()
	if fresp.StatusCode != http.StatusUnprocessableEntity || finalized["cause"] != "dynamic-xfa" {
		t.Errorf("finalizing a dynamic XFA document: %d %v — want 422, cause dynamic-xfa", fresp.StatusCode, finalized)
	}
	code, got := conveneOver(t, c, csrf, ts.URL)
	if code != http.StatusUnprocessableEntity || got["cause"] != "xfa" || !strings.Contains(got["error"], "placeholder") {
		t.Errorf("convene over a dynamic XFA document: %d %v — want 422, cause xfa", code, got)
	}
	if n := ceremoniesOnDisk(t); n != 0 {
		t.Errorf("a refused convene left %d ceremony folder(s) on disk", n)
	}
	got2, err := c.Get(ts.URL + "/api/pdf?auth=" + csrf)
	if err != nil {
		t.Fatal(err)
	}
	var now bytes.Buffer
	now.ReadFrom(got2.Body)
	got2.Body.Close()
	if !bytes.Equal(now.Bytes(), doc) {
		t.Error("the document is no longer the bytes it was opened with")
	}
}

// TestXFAAfterSaysRemovedOnlyOfAStaticFormThatIsGone holds the one judgment docResponse.XFA makes.
func TestXFAAfterSaysRemovedOnlyOfAStaticFormThatIsGone(t *testing.T) {
	for _, tc := range []struct {
		was, now string
		failed   bool
		want     string
	}{
		{"", "", false, ""},
		{"", "static", false, "static"},
		{"static", "", false, xfaRemoved},
		{xfaRemoved, "", false, xfaRemoved},
		{xfaRemoved, "static", false, "static"}, // an undo brought the form back
		{"dynamic", "", false, ""},              // nothing of Nib's removes a dynamic form: another file was loaded
		{"static", "", true, "static"},          // a read that failed says nothing new
	} {
		var err error
		if tc.failed {
			err = os.ErrInvalid
		}
		if got := xfaAfter(tc.was, tc.now, err); got != tc.want {
			t.Errorf("xfaAfter(%q, %q, failed=%v) = %q, want %q", tc.was, tc.now, tc.failed, got, tc.want)
		}
	}
}
