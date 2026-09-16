package server

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/sign"
	"nib/internal/testpdf"
)

// The server's three doors for bytes that change a document outside pdfops — `/pending 492`.

// labelledDoc is a titled document carrying `pdfuaid:part 1`, asserted so a fixture that lost its claim
// cannot make every "dropped" below pass on a build that drops nothing.
func labelledDoc(t *testing.T, text string) []byte {
	t.Helper()
	base, err := testpdf.Text(text)
	if err != nil {
		t.Fatal(err)
	}
	titled, err := pdfops.SetTitle(base, text)
	if err != nil {
		t.Fatal(err)
	}
	out, err := testpdf.WithUAIdentification(titled)
	if err != nil {
		t.Fatal(err)
	}
	if ok, err := testpdf.ClaimsUA(out); err != nil || !ok {
		t.Fatalf("setup: the labelled fixture does not claim PDF/UA (err %v)", err)
	}
	return out
}

func claimsUAFile(t *testing.T, path string) bool {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := testpdf.ClaimsUA(b)
	if err != nil {
		t.Fatalf("reading %s back: %v", path, err)
	}
	return ok
}

// TestSavingKeepsAClaimOnlyOverBytesItDidNotChange — handleSave.
func TestSavingKeepsAClaimOnlyOverBytesItDidNotChange(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	dir := t.TempDir()
	orig := labelledDoc(t, "the original")
	path := filepath.Join(dir, "labelled.pdf")
	if err := os.WriteFile(path, orig, 0o600); err != nil {
		t.Fatal(err)
	}
	dr := openByPath(t, ts.URL, c, csrf, path)

	save := func(body []byte) {
		t.Helper()
		req, err := http.NewRequest(http.MethodPost, ts.URL+"/api/save?overwrite=1", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("X-CSRF-Token", csrf)
		req.Header.Set("X-Nib-Doc", dr.ID)
		res, err := c.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		out, _ := io.ReadAll(res.Body)
		res.Body.Close()
		if res.StatusCode != http.StatusOK {
			t.Fatalf("save status %d: %s", res.StatusCode, out)
		}
	}

	save(orig)
	if !claimsUAFile(t, path) {
		t.Error("an unchanged save lost the document's PDF/UA identification — nothing was edited, so the claim stands")
	}
	// Bytes that differ from the document held: what a pdf.js edit posts. Still labelled as posted, so the
	// server is the only thing that can remove it.
	save(labelledDoc(t, "edited in the browser"))
	if claimsUAFile(t, path) {
		t.Error("an edited save kept a PDF/UA identification nib cannot verify survived the edit")
	}
}

// TestSaveAsKeepsAClaimOnlyForAnOpenDocumentsOwnBytes — handleWriteFile, which names no document.
func TestSaveAsKeepsAClaimOnlyForAnOpenDocumentsOwnBytes(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	dir := t.TempDir()
	orig := labelledDoc(t, "the original")
	src := filepath.Join(dir, "open.pdf")
	if err := os.WriteFile(src, orig, 0o600); err != nil {
		t.Fatal(err)
	}
	openByPath(t, ts.URL, c, csrf, src)

	saveAs := func(name string, data []byte) string {
		t.Helper()
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		mw.WriteField("dir", dir)
		mw.WriteField("name", name)
		fw, _ := mw.CreateFormFile("data", name)
		fw.Write(data)
		mw.Close()
		resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/write", mw.FormDataContentType(), &buf)
		out, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("write %s status %d: %s", name, resp.StatusCode, out)
		}
		return filepath.Join(dir, name)
	}

	if !claimsUAFile(t, saveAs("copy.pdf", orig)) {
		t.Error("Save As of an open document's exact bytes lost its PDF/UA identification")
	}
	if claimsUAFile(t, saveAs("export.pdf", labelledDoc(t, "not a document this server holds"))) {
		t.Error("Save As of bytes no open document holds kept a PDF/UA identification nib cannot vouch for")
	}
}

// TestFinalizingDropsTheClaimBeforeTheSignature — handleFinalize: the claim goes, and the signature over
// what remains is valid, which is the proof the drop came BEFORE it.
func TestFinalizingDropsTheClaimBeforeTheSignature(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
	fw.Write(labelledDoc(t, "to be finalized"))
	mw.WriteField("params", `{"reason":"Finalized in Nib"}`)
	mw.Close()
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/finalize", mw.FormDataContentType(), &buf)
	defer resp.Body.Close()
	signed, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("finalize status %d: %s", resp.StatusCode, signed)
	}
	if st := sign.Verify(signed); st.State != sign.Valid {
		t.Fatalf("the finalized document does not verify (%q), so a write followed the signature", st.State)
	}
	if ok, err := testpdf.ClaimsUA(signed); err != nil || ok {
		t.Errorf("the finalized document still claims PDF/UA (err %v)", err)
	}
}
