package server

import (
	"bytes"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	"nib/internal/testpdf"
)

// TestConvertingLabelsOnlyALanguageTheUserChose — `/pending 486`'s GUI door. The Document language field is
// pre-filled with this computer's language; a conformance claim rests only on one the user picked.
func TestConvertingLabelsOnlyALanguageTheUserChose(t *testing.T) {
	md := []byte("# Notes\n\nA paragraph.\n")
	for _, c := range []struct {
		name   string
		chosen bool
		want   bool
	}{
		{"the pre-filled language, never touched", false, false},
		{"a language the user chose", true, true},
	} {
		ts, _ := startServer(t)
		cl, csrf := authedClient(t, ts)
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("file", "notes.md")
		fw.Write(md)
		mw.WriteField("lang", "en")
		if c.chosen {
			mw.WriteField("langChosen", "1")
		}
		mw.Close()
		resp := write(t, cl, csrf, http.MethodPost, ts.URL+"/api/office", mw.FormDataContentType(), &buf)
		out, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: office status %d: %s", c.name, resp.StatusCode, out)
		}
		pr, err := cl.Get(ts.URL + "/api/pdf")
		if err != nil {
			t.Fatal(err)
		}
		pdf, _ := io.ReadAll(pr.Body)
		pr.Body.Close()
		// Control on the other condition: the language is declared in both rows, so the only difference
		// between them is whether the user chose it.
		if got := workingDocLang(t, cl, ts.URL); got != "en" {
			t.Fatalf("%s: setup: the conversion declares /Lang %q, want en", c.name, got)
		}
		got, err := testpdf.ClaimsUA(pdf)
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("%s: the converted document claims PDF/UA = %v, want %v", c.name, got, c.want)
		}
	}
}
