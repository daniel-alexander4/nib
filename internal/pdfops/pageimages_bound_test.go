package pdfops

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"hash/crc32"
	"testing"

	"golang.org/x/image/bmp"
)

// The single-picture formats take the request's context and an output cap (/pending 941 #6).

func TestAGIFOrBMPOrTIFFOfACancelledRequestIsNotWritten(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	page := pagePNG(t, 1, 64, 64)
	for _, f := range []PageImageFormat{PageImagesGIF, PageImagesBMP, PageImagesTIFF} {
		res, err := EncodePageImages(ctx, f, [][]byte{page}, nil)
		if !errors.Is(err, context.Canceled) || res.Data != nil {
			t.Errorf("%s of a cancelled request: %d bytes, err %v — want no bytes and context.Canceled", f, len(res.Data), err)
		}
	}
	for _, f := range []PageImageFormat{PageImagesZIP, PageImagesCBZ} {
		if _, err := EncodePageImages(ctx, f, [][]byte{page}, nil); !errors.Is(err, context.Canceled) {
			t.Errorf("%s of a cancelled request: err %v, want context.Canceled", f, err)
		}
	}
}

func TestTheSinkStopsOnACancelAndOnAFilePastTheCap(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	w := &boundedWriter{ctx: ctx, limit: 10}
	if _, err := w.Write([]byte("12345")); err != nil {
		t.Fatalf("a write inside the cap: %v", err)
	}
	if _, err := w.Write([]byte("678901")); !errors.Is(err, ErrImagePixels) {
		t.Errorf("a write that takes the file to 11 bytes against a cap of 10: %v, want ErrImagePixels", err)
	}
	if _, err := w.Write([]byte("6789")); err != nil || w.buf.Len() != 9 {
		t.Errorf("a refused write must leave the file as it was and the next fitting one land: len %d, err %v", w.buf.Len(), err)
	}
	cancel()
	if _, err := w.Write([]byte("x")); !errors.Is(err, context.Canceled) {
		t.Errorf("a write after the cancel: %v, want context.Canceled", err)
	}
}

func TestAGIFPastTheOutputCapIsRefusedNotTruncated(t *testing.T) {
	old := onePageLimit
	onePageLimit = 100
	t.Cleanup(func() { onePageLimit = old })
	res, err := EncodePageImages(context.Background(), PageImagesGIF, [][]byte{pagePNG(t, 1, 200, 200)}, nil)
	if !errors.Is(err, ErrImagePixels) || res.Data != nil {
		t.Errorf("a GIF past the cap: %d bytes, err %v — want the TIFF's refusal (ErrImagePixels) and no bytes", len(res.Data), err)
	}
	onePageLimit = old
	if res, err := EncodePageImages(context.Background(), PageImagesGIF, [][]byte{pagePNG(t, 1, 200, 200)}, nil); err != nil || len(res.Data) <= 100 {
		t.Errorf("the same GIF under the real cap: %d bytes, err %v — the test's page must be bigger than its lowered cap", len(res.Data), err)
	}
}

// headerOnlyPNG is a PNG signature and IHDR and nothing else: DecodeConfig reads it, Decode does not.
func headerOnlyPNG(w, h uint32) []byte {
	var ihdr bytes.Buffer
	binary.Write(&ihdr, binary.BigEndian, w)
	binary.Write(&ihdr, binary.BigEndian, h)
	ihdr.Write([]byte{8, 6, 0, 0, 0})
	var out bytes.Buffer
	out.Write([]byte("\x89PNG\r\n\x1a\n"))
	binary.Write(&out, binary.BigEndian, uint32(ihdr.Len()))
	chunk := append([]byte("IHDR"), ihdr.Bytes()...)
	out.Write(chunk)
	binary.Write(&out, binary.BigEndian, crc32.ChecksumIEEE(chunk))
	return out.Bytes()
}

func TestABMPIsRefusedFromItsHeaderBeforeThePageIsDecoded(t *testing.T) {
	old := onePageLimit
	onePageLimit = 1000
	t.Cleanup(func() { onePageLimit = old })
	// The PNG is a header with no pixels: a refusal that came from the decoder would be ErrPageImageUnreadable.
	_, err := EncodePageImages(context.Background(), PageImagesBMP, [][]byte{headerOnlyPNG(200, 200)}, nil)
	if !errors.Is(err, ErrImagePixels) {
		t.Errorf("a BMP whose size is known from the header to pass the cap: %v, want ErrImagePixels before any decode", err)
	}
}

func TestTheBMPSizeIsTheFilesSize(t *testing.T) {
	for _, d := range [][2]int{{1, 1}, {2, 3}, {5, 7}, {200, 200}, {201, 9}} {
		p := pagePNG(t, 1, d[0], d[1])
		res, err := EncodePageImages(context.Background(), PageImagesBMP, [][]byte{p}, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := bmp.Decode(bytes.NewReader(res.Data)); err != nil {
			t.Fatal(err)
		}
		if got := bmpSize(d[0], d[1]); got != int64(len(res.Data)) {
			t.Errorf("%dx%d: bmpSize %d, the file is %d", d[0], d[1], got, len(res.Data))
		}
	}
}
