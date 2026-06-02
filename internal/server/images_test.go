package server

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"testing"
)

// tinyPNG returns a valid 2x2 PNG for use as a library-image fixture.
func tinyPNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	img.Set(0, 0, color.RGBA{255, 0, 0, 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func addImage(t *testing.T, ts string, c *http.Client, csrf string, data []byte) imageMeta {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	mw.WriteField("name", "Signature")
	fw, _ := mw.CreateFormFile("file", "sig.png")
	fw.Write(data)
	mw.Close()

	resp := write(t, c, csrf, http.MethodPost, ts+"/api/images", mw.FormDataContentType(), &buf)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("add image status = %d, want 200", resp.StatusCode)
	}
	var m imageMeta
	if err := json.NewDecoder(resp.Body).Decode(&m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestImageAddListGetDelete(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	pngBytes := tinyPNG(t)

	m := addImage(t, ts.URL, c, csrf, pngBytes)
	if m.ID == "" || m.MIME != "image/png" || m.Name != "Signature" {
		t.Fatalf("unexpected image meta: %+v", m)
	}

	// List shows it.
	resp, err := c.Get(ts.URL + "/api/images")
	if err != nil {
		t.Fatal(err)
	}
	var list []imageMeta
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(list) != 1 || list[0].ID != m.ID {
		t.Fatalf("list = %+v, want one image %s", list, m.ID)
	}

	// Get returns the exact bytes.
	resp, err = c.Get(ts.URL + "/api/images/" + m.ID)
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if !bytes.Equal(got, pngBytes) {
		t.Error("fetched image bytes differ from stored")
	}

	// Delete removes it.
	del := write(t, c, csrf, http.MethodDelete, ts.URL+"/api/images/"+m.ID, "", nil)
	del.Body.Close()
	if del.StatusCode != http.StatusNoContent {
		t.Errorf("delete status = %d, want 204", del.StatusCode)
	}
	resp, err = c.Get(ts.URL + "/api/images")
	if err != nil {
		t.Fatal(err)
	}
	list = nil
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(list) != 0 {
		t.Errorf("list after delete = %+v, want empty", list)
	}
}

func TestImageRejectsNonImage(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("file", "x.txt")
	fw.Write([]byte("definitely not an image"))
	mw.Close()

	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/images", mw.FormDataContentType(), &buf)
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnsupportedMediaType {
		t.Errorf("non-image add status = %d, want 415", resp.StatusCode)
	}
}
