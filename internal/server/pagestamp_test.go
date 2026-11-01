package server

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"nib/internal/pdfops"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// `/pending 907` — the route that puts a mark on every page.

func postPageStamp(t *testing.T, ts string, c *http.Client, csrf string, pdf []byte, fields map[string]string, picture []byte) (int, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
	fw.Write(pdf)
	mw.WriteField("op", "pagestamp")
	for k, v := range fields {
		mw.WriteField(k, v)
	}
	if picture != nil {
		iw, _ := mw.CreateFormFile("image", "logo.png")
		iw.Write(picture)
	}
	mw.Close()
	resp := write(t, c, csrf, http.MethodPost, ts+"/api/pages", mw.FormDataContentType(), &buf)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func TestPageStampRouteStampsTheNamedPagesAndRefusesTheRest(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path)
	pdf := threePagePDF(t)

	if code, body := postPageStamp(t, ts.URL, c, csrf, pdf, map[string]string{"kind": "image", "position": "tr", "scale": "0.3", "opacity": "0.5", "pages": "1,3"}, testPNG(t, 60, 30)); code != http.StatusOK {
		t.Fatalf("an image stamp on pages 1,3 answered %d (%s), want 200", code, body)
	}
	pdfResp, _ := c.Get(ts.URL + "/api/pdf")
	got, _ := io.ReadAll(pdfResp.Body)
	pdfResp.Body.Close()
	if n, err := api.PageCount(context.Background(), bytes.NewReader(got), model.NewDefaultConfiguration()); err != nil || n != 3 {
		t.Fatalf("stamped document pages = %d, %v, want 3", n, err)
	}

	for name, tc := range map[string]struct {
		fields  map[string]string
		picture []byte
		want    int
	}{
		"text with no words":          {map[string]string{"kind": "text", "text": "  "}, nil, http.StatusBadRequest},
		"a picture that is not one":   {map[string]string{"kind": "image"}, []byte("not a picture"), http.StatusInternalServerError},
		"an image with no file":       {map[string]string{"kind": "image"}, nil, http.StatusBadRequest},
		"a page the document lacks":   {map[string]string{"kind": "text", "text": "x", "pages": "9"}, nil, http.StatusUnprocessableEntity},
		"a size that is not a number": {map[string]string{"kind": "text", "text": "x", "size": "big"}, nil, http.StatusBadRequest},
		"an opacity that is not one":  {map[string]string{"kind": "text", "text": "x", "opacity": "faint"}, nil, http.StatusBadRequest},
		"an unknown kind":             {map[string]string{"kind": "sticker", "text": "x"}, nil, http.StatusInternalServerError},
	} {
		if code, body := postPageStamp(t, ts.URL, c, csrf, pdf, tc.fields, tc.picture); code != tc.want {
			t.Errorf("%s answered %d (%s), want %d", name, code, body, tc.want)
		}
	}
}

// Every field the pagestamp case READS is one `pageOp` APPENDS — /pending 448's lesson, which this route
// would otherwise repeat: a knob the server reads and the form never carries does nothing, silently.
func TestEveryPageStampFieldTheServerReadsIsOneTheClientSends(t *testing.T) {
	srv, err := os.ReadFile("pages.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(srv)
	from := strings.Index(s, `case "pagestamp":`)
	to := strings.Index(s[from:], `case "normalize":`)
	if from < 0 || to < 0 {
		t.Fatal("the pagestamp case is gone from pages.go — this scan would pass over nothing")
	}
	reads := map[string]bool{}
	for _, m := range regexp.MustCompile(`FormValue\("([a-zA-Z]+)"\)`).FindAllStringSubmatch(s[from:from+to], -1) {
		reads[m[1]] = true
	}
	if len(reads) < 8 {
		t.Fatalf("found %d FormValue reads in the pagestamp case; it takes at least eight, so the scan is broken", len(reads))
	}
	cli, err := os.ReadFile(filepath.Join("..", "..", "web", "app.js"))
	if err != nil {
		t.Fatal(err)
	}
	js := string(cli)
	oi := strings.Index(js, "async function pageOp(")
	if oi < 0 {
		t.Fatal("pageOp is gone or renamed")
	}
	body := js[oi : oi+strings.Index(js[oi:], "\n}")]
	for field := range reads {
		if !strings.Contains(body, "form.append('"+field+"'") {
			t.Errorf("the server reads %q for a page stamp and pageOp never appends it", field)
		}
	}
	if !strings.Contains(body, "form.append('image'") {
		t.Error("the server reads the image file for a picture stamp and pageOp never appends it")
	}
}

// /pending 910 — the route that proposes bookmarks from the headings. It reads, so it must leave the document
// as it found it, and what it answers is the shape the outline editor already takes.
func TestOutlineProposeAnswersTheHeadingsAndWritesNothing(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	pdf, err := pdfops.ConvertDocToPDF([]byte("# Alpha\n\nbody text\n\n## Beta\n\nbody text\n"), ".md")
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "doc.pdf")
	fw.Write(pdf)
	mw.Close()
	if resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/upload", mw.FormDataContentType(), &buf); resp.StatusCode != http.StatusOK {
		t.Fatalf("setup: upload answered %d", resp.StatusCode)
	}
	resp, err := c.Get(ts.URL + "/api/outline/propose")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var got struct {
		Items []pdfops.OutlineItem `json:"items"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(resp.Body).Decode(&got) != nil || len(got.Items) != 2 {
		t.Fatalf("propose answered %d with %+v, want 200 and two bookmarks", resp.StatusCode, got)
	}
	if got.Items[0].Title != "Alpha" || got.Items[0].Level != 0 || got.Items[1].Level != 1 {
		t.Errorf("items = %+v", got.Items)
	}
	cur, err := c.Get(ts.URL + "/api/outline")
	if err != nil {
		t.Fatal(err)
	}
	defer cur.Body.Close()
	var written struct {
		Items []pdfops.OutlineItem `json:"items"`
	}
	json.NewDecoder(cur.Body).Decode(&written)
	if len(written.Items) != 0 {
		t.Errorf("proposing wrote %d bookmark(s) into the document", len(written.Items))
	}
}
