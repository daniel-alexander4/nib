package server

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

// postFlags posts a PDF (and optionally a flags JSON) to /api/flags and returns
// the resulting bytes. An empty flags string exercises the strip path.
func postFlags(t *testing.T, ts string, c *http.Client, csrf string, pdf []byte, flags string) []byte {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
	fw.Write(pdf)
	if flags != "" {
		mw.WriteField("flags", flags)
	}
	mw.Close()
	resp := write(t, c, csrf, http.MethodPost, ts+"/api/flags", mw.FormDataContentType(), &buf)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/api/flags status = %d, want 200", resp.StatusCode)
	}
	out := new(bytes.Buffer)
	out.ReadFrom(resp.Body)
	return out.Bytes()
}

// openFlags opens a PDF by path and returns the docResponse's Flags field.
func openFlags(t *testing.T, ts string, c *http.Client, csrf string, pdf []byte) json.RawMessage {
	t.Helper()
	path := filepath.Join(t.TempDir(), "doc.pdf")
	if err := os.WriteFile(path, pdf, 0o600); err != nil {
		t.Fatal(err)
	}
	resp := write(t, c, csrf, http.MethodPost, ts+"/api/open", "application/json", jsonBody(openRequest{Path: path}))
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("/api/open status = %d, want 200", resp.StatusCode)
	}
	var dr docResponse
	json.NewDecoder(resp.Body).Decode(&dr)
	return dr.Flags
}

// TestFlagsEndpointRoundTrip drives the recipient-facing wiring: embed flags via
// /api/flags, confirm /api/open reports them back in docResponse, then strip them
// and confirm a reopened document reports none.
func TestFlagsEndpointRoundTrip(t *testing.T) {
	ts, pdfPath := startServer(t)
	c, csrf := authedClient(t, ts)
	base, err := os.ReadFile(pdfPath)
	if err != nil {
		t.Fatal(err)
	}
	flags := `[{"page":1,"frac":[0.1,0.2,0.32,0.25],"type":"sign"}]`

	withFlags := postFlags(t, ts.URL, c, csrf, base, flags)
	got := openFlags(t, ts.URL, c, csrf, withFlags)
	if len(got) == 0 {
		t.Fatal("docResponse reported no flags after embed")
	}
	// Compare structurally so insignificant whitespace doesn't fail the test.
	var want, have any
	json.Unmarshal([]byte(flags), &want)
	json.Unmarshal(got, &have)
	if b1, _ := json.Marshal(want); !bytes.Equal(b1, mustMarshal(have)) {
		t.Fatalf("flags = %s, want %s", got, flags)
	}

	stripped := postFlags(t, ts.URL, c, csrf, withFlags, "")
	if got := openFlags(t, ts.URL, c, csrf, stripped); len(got) != 0 {
		t.Fatalf("flags survived strip: %s", got)
	}
}

func mustMarshal(v any) []byte { b, _ := json.Marshal(v); return b }
