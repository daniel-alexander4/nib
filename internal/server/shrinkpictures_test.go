package server

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/jpeg"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"testing"

	"nib/internal/pdfops"
)

// `/pending 908` — the route that shrinks the pictures in the open document.

func noisyJPEG(t *testing.T, w, h int) []byte {
	t.Helper()
	r := rand.New(rand.NewSource(3))
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x*255/w + r.Intn(30)), uint8(y*255/h + r.Intn(30)), uint8(100 + r.Intn(60)), 255})
		}
	}
	var b bytes.Buffer
	if err := jpeg.Encode(&b, img, &jpeg.Options{Quality: 95}); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestShrinkPicturesRouteAnswersASmallerPDFAndSaysWhatItDid(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	doc, err := pdfops.ImagesToPDF([]pdfops.RasterPage{{Image: noisyJPEG(t, 2000, 1500), W: 400, H: 300}})
	if err != nil {
		t.Fatal(err)
	}
	uploadPDF(t, ts.URL, c, csrf, "pics.pdf", doc)

	post := func(query string) (*http.Response, []byte) {
		resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/shrink-pictures"+query, "", nil)
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		return resp, body
	}

	resp, body := post("?level=3")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("level 3 answered %d: %s", resp.StatusCode, body)
	}
	before, _ := strconv.Atoi(resp.Header.Get("X-Original-Size"))
	after, _ := strconv.Atoi(resp.Header.Get("X-New-Size"))
	if before != len(doc) || after != len(body) || after*2 > before {
		t.Errorf("sizes: header %d -> %d, body %d, document %d; want a result under half the original", before, after, len(body), len(doc))
	}
	var rep pdfops.ShrinkReport
	if err := json.Unmarshal([]byte(resp.Header.Get("X-Nib-Shrink")), &rep); err != nil || rep.Shrunk != 1 || rep.Level != 3 {
		t.Errorf("X-Nib-Shrink = %q (%v), want one picture shrunk at level 3", resp.Header.Get("X-Nib-Shrink"), err)
	}
	if !bytes.HasPrefix(body, []byte("%PDF-")) {
		t.Error("the answer is not a PDF")
	}

	// A target: met when the document can fit, and said not to be when it cannot.
	resp, body = post("?target=" + strconv.Itoa(len(doc)))
	_ = json.Unmarshal([]byte(resp.Header.Get("X-Nib-Shrink")), &rep)
	if resp.StatusCode != http.StatusOK || !rep.Met {
		t.Errorf("a target the document already meets answered %d with %+v", resp.StatusCode, rep)
	}
	resp, _ = post("?target=1")
	rep = pdfops.ShrinkReport{}
	_ = json.Unmarshal([]byte(resp.Header.Get("X-Nib-Shrink")), &rep)
	if resp.StatusCode != http.StatusOK || rep.Met || rep.Level != len(pdfops.ShrinkLadder)-1 {
		t.Errorf("an impossible target answered %d with %+v, want the hardest rung and not met", resp.StatusCode, rep)
	}

	for _, bad := range []string{"?level=x", "?level=99", "?level=-1", "?target=x", "?target=0", "?target=-5"} {
		if resp, _ := post(bad); resp.StatusCode != http.StatusBadRequest {
			t.Errorf("%s answered %d, want 400", bad, resp.StatusCode)
		}
	}
}
