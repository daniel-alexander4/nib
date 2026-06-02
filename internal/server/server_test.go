package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"nib/internal/testpdf"
)

// openByPath opens a path-based document on an authenticated client.
func openByPath(t *testing.T, baseURL string, c *http.Client, csrf, path string) docResponse {
	t.Helper()
	body, _ := json.Marshal(openRequest{Path: path})
	resp := write(t, c, csrf, http.MethodPost, baseURL+"/api/open", "application/json", bytes.NewReader(body))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("open status = %d, want 200", resp.StatusCode)
	}
	var dr docResponse
	if err := json.NewDecoder(resp.Body).Decode(&dr); err != nil {
		t.Fatal(err)
	}
	return dr
}

func TestOpenAndFetchPDF(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	dr := openByPath(t, ts.URL, c, csrf, path)

	if !dr.CanSave {
		t.Error("path-opened document should be saveable")
	}
	if dr.Path != path {
		t.Errorf("path = %q, want %q", dr.Path, path)
	}
	if dr.Signature.State != "unsigned" {
		t.Errorf("signature = %q, want unsigned", dr.Signature.State)
	}

	resp, err := c.Get(ts.URL + "/api/pdf")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	got, _ := io.ReadAll(resp.Body)
	if !bytes.HasPrefix(got, []byte("%PDF")) {
		t.Errorf("served bytes are not a PDF: %.10q", got)
	}
	if ct := resp.Header.Get("Content-Type"); ct != "application/pdf" {
		t.Errorf("content-type = %q, want application/pdf", ct)
	}
}

func TestSaveOverwritesInPlace(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path)

	newBytes, _ := testpdf.Form()
	newBytes = append(newBytes, "\n% edited\n"...)
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/save", "application/pdf", bytes.NewReader(newBytes))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("save status = %d, want 200", resp.StatusCode)
	}

	onDisk, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(onDisk, newBytes) {
		t.Error("file on disk was not overwritten with the saved bytes")
	}
}

func TestSaveWithoutPathRejected(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	data, _ := testpdf.Form()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "form.pdf")
	fw.Write(data)
	mw.Close()

	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/upload", mw.FormDataContentType(), &buf)
	var dr docResponse
	json.NewDecoder(resp.Body).Decode(&dr)
	resp.Body.Close()
	if dr.CanSave {
		t.Error("uploaded document should not be saveable in place")
	}

	saveResp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/save", "application/pdf", bytes.NewReader(data))
	saveResp.Body.Close()
	if saveResp.StatusCode != http.StatusConflict {
		t.Errorf("save status = %d, want 409", saveResp.StatusCode)
	}
}

func TestOpenMissingFile(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	body, _ := json.Marshal(openRequest{Path: filepath.Join(t.TempDir(), "nope.pdf")})
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/open", "application/json", bytes.NewReader(body))
	resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("open missing status = %d, want 404", resp.StatusCode)
	}
}

func TestLoopbackGuardRejectsForeignHost(t *testing.T) {
	ts, _ := startServer(t)
	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/", nil)
	req.Host = "evil.example.com"
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("foreign-host status = %d, want 403", resp.StatusCode)
	}
}
