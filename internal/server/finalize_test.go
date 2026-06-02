package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"

	"nib/internal/sign"
	"nib/internal/testpdf"
)

func TestFinalizeSignsAndVerifies(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	pdf, _ := testpdf.Form()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
	fw.Write(pdf)
	mw.WriteField("params", `{"reason":"Finalized in Nib"}`)
	mw.Close()

	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/finalize", mw.FormDataContentType(), &buf)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("finalize status = %d: %s", resp.StatusCode, body)
	}
	signed, _ := io.ReadAll(resp.Body)
	if st := sign.Verify(signed); st.State != sign.Valid {
		t.Errorf("finalized doc verify = %q, want valid", st.State)
	}
}

func TestFormDataExport(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path)

	for _, format := range []string{"json", "csv"} {
		resp, err := c.Get(ts.URL + "/api/form-data?format=" + format)
		if err != nil {
			t.Fatal(err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if !strings.Contains(string(body), "fullName") {
			t.Errorf("%s export missing field name: %s", format, body)
		}
	}
}

func TestProfileRoundTrip(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	in := map[string]string{"fullName": "Dan", "email": "dan@example.com"}
	body, _ := json.Marshal(in)
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/profile", "application/json", bytes.NewReader(body))
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("set profile status = %d", resp.StatusCode)
	}

	resp, _ = c.Get(ts.URL + "/api/profile")
	var out map[string]string
	json.NewDecoder(resp.Body).Decode(&out)
	resp.Body.Close()
	if out["fullName"] != "Dan" || out["email"] != "dan@example.com" {
		t.Errorf("profile = %v, want the values set", out)
	}
}

func TestIdentityExport(t *testing.T) {
	ts, _ := startServer(t)
	c, _ := authedClient(t, ts)
	resp, err := c.Get(ts.URL + "/api/identity")
	if err != nil {
		t.Fatal(err)
	}
	pem, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if !bytes.Contains(pem, []byte("BEGIN CERTIFICATE")) {
		t.Errorf("identity export is not a PEM certificate: %.40q", pem)
	}
}
