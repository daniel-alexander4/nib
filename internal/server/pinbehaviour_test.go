package server

import (
	"bytes"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"regexp"
	"testing"

	"nib/internal/testpdf"
)

// TestEveryMutatingRouteRefusesAMisaddressedDocument — /pending 15's named residual, built.
//
// The pinning inventory in `test/jsdom/pinning.test.mjs` is a SOURCE scan: it proves every
// mutating call site in app.js passes a document id, and it pins the route list against the mux
// so the list cannot drift. What it cannot prove is that the SERVER refuses a request addressed
// to a document it no longer holds — that is behaviour, and behaviour needs driving. The entry
// said so: "what is missing is each route being DRIVEN individually, which is a new behavioural
// suite rather than a fix to this guard."
//
// Before this, the Go side drove that law for **two** of the eleven routes (undo and redo, in a
// hand-written literal). ADR-004 makes it 409 and never 404, because the client branches on the
// difference: 404 is "nothing is open" and 409 is "not that one, drop the stale tab".
//
// **The route list is READ from the JS inventory rather than copied.** A second list would be a
// second door onto one rule (ADR-009), and the failure mode is silent: a route added to one list
// and not the other is covered by whichever guard happens to hold it.
func TestEveryMutatingRouteRefusesAMisaddressedDocument(t *testing.T) {
	routes := mutatingInventory(t)

	ts, srv := startServerWith(t)
	c, csrf := authedClient(t, ts)
	pdf, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	srv.setDoc(&document{data: pdf})

	// An id from THIS process's epoch with a sequence no document ever had. Not a random epoch:
	// docFor checks the epoch first, so a foreign epoch would be refused before the per-document
	// lookup ran and this suite would prove only that the epoch check works.
	stale := docID{Epoch: srv.epoch, Seq: 9999}

	for _, route := range routes {
		t.Run(route, func(t *testing.T) {
			body, ctype := requestBodyFor(t, route)
			req, err := http.NewRequest(http.MethodPost, ts.URL+route, body)
			if err != nil {
				t.Fatal(err)
			}
			req.Header.Set("X-CSRF-Token", csrf)
			req.Header.Set("X-Nib-Doc", stale.String())
			if ctype != "" {
				req.Header.Set("Content-Type", ctype)
			}
			resp, err := c.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()

			// A 400 means the request never got far enough to be refused for the RIGHT reason,
			// which makes the row prove nothing. It is a setup failure, not a finding.
			if resp.StatusCode == http.StatusBadRequest {
				t.Fatalf("%s answered 400 — the body this suite sent was rejected before the pin "+
					"was reached, so this row is not exercising the law it names", route)
			}

			if resp.StatusCode == http.StatusConflict {
				return
			}
			// 404 is the specific wrong answer ADR-004 names, so it gets its own sentence: the
			// client drops a stale tab on 409 and shows its empty state on 404, and a document
			// that exists but is not this one must never produce the second.
			if resp.StatusCode == http.StatusNotFound {
				t.Errorf("%s addressed to an unknown document = 404, want 409 — ADR-004: a document "+
					"the server no longer holds is 409, and the client shows its EMPTY STATE on 404",
					route)
				return
			}
			t.Errorf("%s addressed to an unknown document = %d, want 409 — the request was answered "+
				"without the pin being checked, so a misaddressed mutation reached the handler",
				route, resp.StatusCode)
		})
	}
}

// mutatingInventory reads the MUTATING list out of the jsdom pinning guard, which is the one
// place that list lives and is already reconciled against the mux there.
func mutatingInventory(t *testing.T) []string {
	t.Helper()
	src, err := os.ReadFile("../../test/jsdom/pinning.test.mjs")
	if err != nil {
		t.Fatalf("the pinning inventory is unreadable, so this suite has no route list: %v", err)
	}
	block := regexp.MustCompile(`(?s)const MUTATING = \[(.*?)\];`).FindSubmatch(src)
	if block == nil {
		t.Fatal("the MUTATING inventory was not found in pinning.test.mjs — it was renamed or " +
			"restructured, and this suite would silently drive nothing")
	}
	var routes []string
	for _, m := range regexp.MustCompile(`'(/api/[^']+)'`).FindAllSubmatch(block[1], -1) {
		routes = append(routes, string(m[1]))
	}
	// STIMULUS: the parse must actually have produced the list. A regex that stopped matching
	// yields zero routes, and a table-driven test over zero rows is green.
	if len(routes) < 10 {
		t.Fatalf("parsed only %d mutating routes from the inventory, which holds eleven — the "+
			"parse is broken and this suite is driving almost nothing", len(routes))
	}
	return routes
}

// requestBodyFor builds a body the route will actually accept, so the refusal under test is the
// PIN and never a malformed request.
//
// **Four of the twelve routes need one**, because they work from posted bytes rather than from the
// open document: `/api/pages`, `/api/redact`, `/api/outline` and `/api/assemble`. They used to
// parse the whole body and run the PDF operation before resolving the document the result is
// installed into — which is how this suite found them, since every other route answered a
// bodyless request straight away and these returned 400. They refuse early now (/pending 261);
// the bodies stay, because a row that cannot reach the check proves nothing about it.
func requestBodyFor(t *testing.T, route string) (io.Reader, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	switch route {
	case "/api/pages":
		fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
		fw.Write(threePagePDF(t))
		mw.WriteField("op", "delete")
		mw.WriteField("pages", "2")
	case "/api/outline":
		pdf, err := testpdf.Form()
		if err != nil {
			t.Fatal(err)
		}
		fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
		fw.Write(pdf)
		mw.WriteField("outline", `[{"title":"Intro","page":1,"level":0}]`)
	case "/api/assemble":
		// reload=1 is the branch that COMMITS (commitBarrier); without it the route is a
		// download and the pin is not its business. The row would prove nothing on the other
		// branch, so it drives the one that installs into a document.
		iw, _ := mw.CreateFormFile("page", "page-1.png")
		png.Encode(iw, image.NewRGBA(image.Rect(0, 0, 612, 792)))
		mw.WriteField("pageW", "612")
		mw.WriteField("pageH", "792")
		mw.WriteField("reload", "1")
	case "/api/redact":
		pdf, err := testpdf.Form()
		if err != nil {
			t.Fatal(err)
		}
		fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
		fw.Write(pdf)
		iw, _ := mw.CreateFormFile("page", "page-1.png")
		png.Encode(iw, image.NewRGBA(image.Rect(0, 0, 612, 792)))
		mw.WriteField("pageNum", "1")
		mw.WriteField("pageW", "612")
		mw.WriteField("pageH", "792")
	default:
		// The bodyless case, and it is deliberate: these routes resolve the document before
		// they read anything, so a request with nothing in it still reaches the pin.
		return nil, ""
	}
	mw.Close()
	return &buf, mw.FormDataContentType()
}
