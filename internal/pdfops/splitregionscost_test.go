package pdfops

import (
	"bytes"
	"image"
	"image/png"
	"math/rand"
	"runtime"
	"testing"
)

// noisePage is a one-page document whose page draws an incompressible image, so what a split costs per region
// shows up as bytes: a region that re-reads, or re-carries, the page pays for the image again.
func noisePage(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 1000, 1000))
	r := rand.New(rand.NewSource(1))
	r.Read(img.Pix)
	var b bytes.Buffer
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	pdf, err := ImagesToPDF([]RasterPage{{Image: b.Bytes(), W: 300, H: 300}})
	if err != nil {
		t.Fatal(err)
	}
	return pdf
}

// SplitRegions reads the page it splits once, not once per region, and its regions share the page's resources
// (`/pending 603`). Each region used to be its own read of the normalized page and its own one-page document, merged
// one at a time onto a growing result — so the page's image was parsed, written and re-merged per region, and the
// output carried a copy of it per region. Counted in bytes (output, and allocated), not timed.
func TestSplitRegionsReadsThePageOnce(t *testing.T) {
	src := noisePage(t)
	regions := func(n int) [][4]float64 {
		rs := make([][4]float64, n)
		for i := range rs {
			x := float64(i%8) * 30
			rs[i] = [4]float64{x, 0, x + 30, 300}
		}
		return rs
	}
	measure := func(n int) (out int, alloc uint64) {
		var m0, m1 runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&m0)
		b, err := SplitRegions(src, 1, regions(n))
		runtime.ReadMemStats(&m1)
		if err != nil {
			t.Fatal(err)
		}
		if got, err := PageCount(b); err != nil || got != n {
			t.Fatalf("%d regions: %d pages (%v)", n, got, err)
		}
		return len(b), m1.TotalAlloc - m0.TotalAlloc
	}
	out1, alloc1 := measure(1)
	out16, alloc16 := measure(16)
	t.Logf("source %d B; 1 region: %d B out, %d B allocated; 16 regions: %d B out, %d B allocated", len(src), out1, alloc1, out16, alloc16)
	// The page's image is ~all of the source. Sixteen regions of one page are one image and sixteen small content
	// streams; the floor of 2× leaves room for those and nothing like a copy per region.
	if out16 > 2*out1 {
		t.Errorf("16 regions wrote %d B against %d B for one — the page's resources are carried once per region", out16, out1)
	}
	// One read of this page allocates ~5 MB (measured: a read per region added 78 MB over 15 regions), and the
	// fifteen regions past the first cost ~14 MB when the page is read once, whatever the image's size; the old
	// one-at-a-time merge cost ~120 MB on a page a tenth this size. Eight reads' worth separates them.
	if marginal := alloc16 - alloc1; marginal > uint64(8*len(src)) {
		t.Errorf("15 more regions allocated %d B over one region's %d B, on a %d B page — the page is read once per region", marginal, alloc1, len(src))
	}
}
