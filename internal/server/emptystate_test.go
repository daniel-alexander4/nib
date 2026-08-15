package server

import (
	"bytes"
	"encoding/json"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"testing"

	"nib/internal/testpdf"
)

// Four routes take the bytes to operate on from the request rather than from the
// open document, and only *install* their result as the open document. They used
// to call commitMutation/commitBarrier unchecked: with nothing open the helper
// bailed internally, the route discarded its work, and the reply was still 200
// with an empty docResponse — a success reply for work that was thrown away.
//
// That was near-unreachable while s.doc was nil only before the first open. Close
// (P01.S02) makes it reachable at any moment, so the guards land first.
//
// The with-a-document case is asserted FIRST and with the SAME body, for two
// distinct reasons that are easy to conflate:
//   - a route that answered 404 unconditionally would pass a 404-only test, and
//   - a body malformed enough to be rejected earlier in the handler would 404 or
//     400 before ever reaching the commit, so the test would pass without the
//     guard existing at all.
//
// Only a body that genuinely reaches the commit call proves the guard is what
// answered.
func TestMutatingRoutesRefuseWithNoDocumentOpen(t *testing.T) {
	pdf := threePagePDF(t)
	form, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}

	pageImg := func() []byte {
		var b bytes.Buffer
		if err := png.Encode(&b, image.NewRGBA(image.Rect(0, 0, 120, 160))); err != nil {
			t.Fatal(err)
		}
		return b.Bytes()
	}

	cases := []struct {
		name  string
		route string
		// body is rebuilt per request: a multipart buffer is consumed by the first send.
		body func() (string, io.Reader)
	}{
		{"pages", "/api/pages", func() (string, io.Reader) {
			var buf bytes.Buffer
			mw := multipart.NewWriter(&buf)
			fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
			fw.Write(pdf)
			mw.WriteField("op", "delete")
			mw.WriteField("pages", "2")
			mw.Close()
			return mw.FormDataContentType(), &buf
		}},
		{"outline", "/api/outline", func() (string, io.Reader) {
			var buf bytes.Buffer
			mw := multipart.NewWriter(&buf)
			fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
			fw.Write(pdf)
			mw.WriteField("outline", `[{"title":"Chapter one","page":1,"level":0}]`)
			mw.Close()
			return mw.FormDataContentType(), &buf
		}},
		{"redact", "/api/redact", func() (string, io.Reader) {
			var buf bytes.Buffer
			mw := multipart.NewWriter(&buf)
			fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
			fw.Write(form)
			iw, _ := mw.CreateFormFile("page", "page-1.png")
			png.Encode(iw, image.NewRGBA(image.Rect(0, 0, 1224, 1584)))
			mw.WriteField("pageNum", "1")
			mw.WriteField("pageW", "612")
			mw.WriteField("pageH", "792")
			mw.Close()
			return mw.FormDataContentType(), &buf
		}},
		{"assemble/reload", "/api/assemble?reload=1", func() (string, io.Reader) {
			var buf bytes.Buffer
			mw := multipart.NewWriter(&buf)
			iw, _ := mw.CreateFormFile("image", "page-1.png")
			iw.Write(pageImg())
			mw.WriteField("pageW", "80")
			mw.WriteField("pageH", "110")
			mw.WriteField("reload", "1")
			mw.Close()
			return mw.FormDataContentType(), &buf
		}},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// The non-zero probe: the identical body must succeed while a
			// document is open, or nothing below distinguishes the guard from
			// a route that rejects this request for some other reason.
			t.Run("succeeds with a document open", func(t *testing.T) {
				ts, path := startServer(t)
				c, csrf := authedClient(t, ts)
				openByPath(t, ts.URL, c, csrf, path)

				ct, body := tc.body()
				resp := write(t, c, csrf, http.MethodPost, ts.URL+tc.route, ct, body)
				defer resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					got, _ := io.ReadAll(resp.Body)
					t.Fatalf("status = %d, want 200 (body %s)", resp.StatusCode, got)
				}
			})

			t.Run("refuses with nothing open", func(t *testing.T) {
				ts, _ := startServer(t) // vault unlocked, no document opened
				c, csrf := authedClient(t, ts)

				ct, body := tc.body()
				resp := write(t, c, csrf, http.MethodPost, ts.URL+tc.route, ct, body)
				defer resp.Body.Close()
				if resp.StatusCode != http.StatusNotFound {
					got, _ := io.ReadAll(resp.Body)
					t.Fatalf("status = %d, want 404 (body %s)", resp.StatusCode, got)
				}
				// The message is asserted as the literal string the thirteen
				// already-guarded handlers emit: the client reads it, so a
				// route inventing its own wording makes the empty state
				// ambiguous even though the status agrees.
				var e struct {
					Error string `json:"error"`
				}
				if err := json.NewDecoder(resp.Body).Decode(&e); err != nil {
					t.Fatal(err)
				}
				if e.Error != "no document open" {
					t.Errorf("error = %q, want %q", e.Error, "no document open")
				}
			})
		})
	}
}
