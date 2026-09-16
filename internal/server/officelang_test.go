package server

import (
	"bytes"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// officeFormWithLang is officeForm plus the Document language field, omitted when lang is empty —
// which is what the client does when the user clears it.
func officeFormWithLang(t *testing.T, name string, data []byte, lang string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", name)
	fw.Write(data)
	if lang != "" {
		mw.WriteField("lang", lang)
	}
	mw.Close()
	return &buf, mw.FormDataContentType()
}

// workingDocLang reads the catalog /Lang of the document /api/pdf serves, "" where there is none.
func workingDocLang(t *testing.T, c *http.Client, url string) string {
	t.Helper()
	resp, err := c.Get(url + "/api/pdf")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	pdf, _ := io.ReadAll(resp.Body)
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("the working document does not read back (%d bytes): %v", len(pdf), err)
	}
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	s, _ := root["Lang"].(types.StringLiteral)
	return string(s)
}

// TestConvertingDeclaresTheDocumentLanguageField — `/pending 471`'s GUI door. Markdown, because it
// needs no LibreOffice and because without the field nib is told no language at all, so the control
// is clean.
func TestConvertingDeclaresTheDocumentLanguageField(t *testing.T) {
	md := []byte("# Notizen\n\nGuten Tag.\n")
	for _, c := range []struct {
		name, lang, want string
	}{
		{"cleared field", "", ""},
		{"a chosen language, canonicalised", "DE-at", "de-AT"},
	} {
		ts, _ := startServer(t)
		cl, csrf := authedClient(t, ts)
		body, ct := officeFormWithLang(t, "notizen.md", md, c.lang)
		resp := write(t, cl, csrf, http.MethodPost, ts.URL+"/api/office", ct, body)
		out, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("%s: office status %d: %s", c.name, resp.StatusCode, out)
		}
		if got := workingDocLang(t, cl, ts.URL); got != c.want {
			t.Errorf("%s: the converted document declares /Lang %q, want %q", c.name, got, c.want)
		}
	}
}

// TestConvertingRefusesALanguageItCannotDeclare — and installs nothing.
func TestConvertingRefusesALanguageItCannotDeclare(t *testing.T) {
	ts, _ := startServer(t)
	cl, csrf := authedClient(t, ts)
	body, ct := officeFormWithLang(t, "notes.md", []byte("# Notes\n"), "german")
	resp := write(t, cl, csrf, http.MethodPost, ts.URL+"/api/office", ct, body)
	out, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("office with lang=german answered %d (%s), want 400", resp.StatusCode, out)
	}
	dr, err := cl.Get(ts.URL + "/api/docs")
	if err != nil {
		t.Fatal(err)
	}
	defer dr.Body.Close()
	var docs struct {
		Docs []json.RawMessage `json:"docs"`
	}
	if err := json.NewDecoder(dr.Body).Decode(&docs); err != nil {
		t.Fatal(err)
	}
	if len(docs.Docs) != 0 {
		t.Errorf("a refused language still installed %d document(s)", len(docs.Docs))
	}
}
