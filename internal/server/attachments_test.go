package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"testing"
)

// TestAttachmentsWiring locks the add→list→extract rail end to end: routes, auth,
// the JSON list shape, the doc-replacing add, and the binary download. The
// per-operation behaviour is covered by the pdfops tests.
func TestAttachmentsWiring(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path)

	// Fixture starts with no attachments.
	resp, err := c.Get(ts.URL + "/api/attachments")
	if err != nil {
		t.Fatal(err)
	}
	var list attachmentsResponse
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatalf("attachments response not JSON: %v", err)
	}
	resp.Body.Close()
	if len(list.Attachments) != 0 {
		t.Fatalf("fresh doc has %d attachments, want 0", len(list.Attachments))
	}

	// Add one.
	payload := []byte("server-side attachment payload\n")
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "note.txt")
	fw.Write(payload)
	mw.WriteField("name", "note.txt")
	mw.Close()
	resp = write(t, c, csrf, http.MethodPost, ts.URL+"/api/attachments/add", mw.FormDataContentType(), &buf)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()

	// List now shows it.
	resp, _ = c.Get(ts.URL + "/api/attachments")
	list = attachmentsResponse{}
	json.NewDecoder(resp.Body).Decode(&list)
	resp.Body.Close()
	if len(list.Attachments) != 1 || list.Attachments[0].Name != "note.txt" {
		t.Fatalf("after add: %+v, want one named note.txt", list.Attachments)
	}

	// Extract returns the original bytes.
	var eb bytes.Buffer
	emw := multipart.NewWriter(&eb)
	emw.WriteField("name", "note.txt")
	emw.Close()
	resp = write(t, c, csrf, http.MethodPost, ts.URL+"/api/attachments/extract", emw.FormDataContentType(), &eb)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("extract status = %d, want 200", resp.StatusCode)
	}
	got, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !bytes.Equal(got, payload) {
		t.Errorf("extracted bytes mismatch: got %q want %q", got, payload)
	}
}
