package server

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/testpdf"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// `/pending 929` — the route that inserts a contents page.

func TestContentsRouteInsertsThePageAndRefusesWhatItCannot(t *testing.T) {
	ts, path := startServer(t)
	c, csrf := authedClient(t, ts)
	openByPath(t, ts.URL, c, csrf, path)

	plain, err := testpdf.Text("one", "two", "three")
	if err != nil {
		t.Fatal(err)
	}
	marked, err := pdfops.SetOutline(plain, []pdfops.OutlineItem{{Title: "Start", Page: 1}, {Title: "End", Page: 3}})
	if err != nil {
		t.Fatal(err)
	}
	post := func(pdf []byte, fields map[string]string) (int, string) {
		return postPageOp(t, ts.URL, c, csrf, pdf, "contents", fields)
	}

	if code, body := post(marked, map[string]string{"after": "0", "depth": "2", "title": "Inside"}); code != http.StatusOK {
		t.Fatalf("a document with bookmarks answered %d: %s", code, body)
	}
	pdfResp, _ := c.Get(ts.URL + "/api/pdf")
	got, _ := io.ReadAll(pdfResp.Body)
	pdfResp.Body.Close()
	if n, err := api.PageCount(context.Background(), bytes.NewReader(got), model.NewDefaultConfiguration()); err != nil || n != 4 {
		t.Fatalf("the document has %d pages after the insertion (%v), want 4", n, err)
	}

	for name, tc := range map[string]struct {
		pdf    []byte
		fields map[string]string
		want   int
	}{
		"nothing to list":         {plain, nil, http.StatusUnprocessableEntity},
		"a page past the end":     {marked, map[string]string{"after": "9"}, http.StatusUnprocessableEntity},
		"a position not a number": {marked, map[string]string{"after": "x"}, http.StatusBadRequest},
		"a depth not a number":    {marked, map[string]string{"depth": "deep"}, http.StatusBadRequest},
		"a list that is not json": {marked, map[string]string{"items": "{"}, http.StatusBadRequest},
	} {
		if code, body := post(tc.pdf, tc.fields); code != tc.want {
			t.Errorf("%s answered %d (%s), want %d", name, code, strings.TrimSpace(body), tc.want)
		}
	}
}

// Every field the contents case READS is one pageOp APPENDS — /pending 448's lesson: a knob the form never carries does nothing.
func TestEveryContentsFieldTheServerReadsIsOneTheClientSends(t *testing.T) {
	srv, err := os.ReadFile("pages.go")
	if err != nil {
		t.Fatal(err)
	}
	s := string(srv)
	from := strings.Index(s, `case "contents":`)
	to := strings.Index(s[from:], `case "normalize":`)
	if from < 0 || to < 0 {
		t.Fatal("the contents case is gone from pages.go")
	}
	reads := map[string]bool{}
	for _, m := range regexp.MustCompile(`FormValue\("([a-zA-Z]+)"\)`).FindAllStringSubmatch(s[from:from+to], -1) {
		reads[m[1]] = true
	}
	if len(reads) < 4 {
		t.Fatalf("found %d FormValue reads in the contents case; it takes four, so the scan is broken", len(reads))
	}
	cli, err := os.ReadFile(filepath.Join("..", "..", "web", "app.js"))
	if err != nil {
		t.Fatal(err)
	}
	js := string(cli)
	oi := strings.Index(js, "async function pageOp(")
	if oi < 0 {
		t.Fatal("pageOp is gone")
	}
	body := js[oi : oi+strings.Index(js[oi:], "\n}")]
	for field := range reads {
		if !strings.Contains(body, "form.append('"+field+"'") {
			t.Errorf("the server reads %q for a contents page and pageOp never appends it", field)
		}
	}
}
