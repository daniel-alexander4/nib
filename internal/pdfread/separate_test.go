package pdfread_test

import (
	"bytes"
	"runtime"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
	"nib/internal/testpdf"
)

// TestSeparateContentsMakesPdfcpusJoinTheDoors — ADR-084. pdfcpu's page operations read a page through pdfcpu's own
// bare join and write it into the output; after `SeparateContents`, that join must be byte-identical to the door's
// join of the page as it was — which is what lets the n-up carries read the source through the door — and a page
// that needed no separator must be left exactly as it was. A second call changes nothing.
func TestSeparateContentsMakesPdfcpusJoinTheDoors(t *testing.T) {
	for _, c := range []struct {
		name  string
		shape testpdf.JoinShape
		fused bool
	}{
		{"`Tj` + `ET`", testpdf.JoinRegular, true},
		{"a comment with no end of line", testpdf.JoinComment, true},
		{"a white-space division", testpdf.JoinSafe, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			pdf, _, err := testpdf.SplitContents("Hello, world", c.shape)
			if err != nil {
				t.Fatal(err)
			}
			ctx, err := pdfread.Validated(pdf, model.NewDefaultConfiguration())
			if err != nil {
				t.Fatal(err)
			}
			page, _, _, err := ctx.PageDict(1, false)
			if err != nil {
				t.Fatal(err)
			}
			door, err := pdfread.PageContent(ctx, page, 1)
			if err != nil {
				t.Fatal(err)
			}
			before, _ := ctx.PageContent(page, 1)
			// The stimulus: the fused shapes are ones pdfcpu's join gets wrong.
			if fused := !bytes.Equal(before, door); fused != c.fused {
				t.Fatalf("pdfcpu's join differs from the door's = %v, want %v — the fixture is not what it is named for", fused, c.fused)
			}
			was := page["Contents"]
			for i := 1; i <= 2; i++ {
				if err := pdfread.SeparateContents(ctx, nil); err != nil {
					t.Fatal(err)
				}
				after, err := ctx.PageContent(page, 1)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(after, door) {
					t.Errorf("call %d: pdfcpu's join after SeparateContents is not the door's join\n  pdfcpu %q\n  door   %q", i, after, door)
				}
				again, err := pdfread.PageContent(ctx, page, 1)
				if err != nil || !bytes.Equal(again, door) {
					t.Errorf("call %d: the door reads the separated page as %q, %v — it must read it as before", i, again, err)
				}
			}
			arr, _ := page["Contents"].(types.Array)
			wasArr, _ := was.(types.Array)
			switch {
			case !c.fused && len(arr) != len(wasArr):
				t.Errorf("a page needing no separator went from %d elements to %d — it must be left alone", len(wasArr), len(arr))
			case c.fused && len(arr) != len(wasArr)+1:
				t.Errorf("the divided page went from %d elements to %d, want exactly one separator (and none on the second call)",
					len(wasArr), len(arr))
			}
		})
	}
}

// TestARepeatedStreamIsDecodedOnce — `/pending 728` (2). A `/Contents` array may name one stream any number of times,
// and the door decoded it every time: a page naming one EMPTY flate stream 100,000 times, in a file of about 1 KB,
// took 5.5 s and allocated 3.9 GB while producing no content — the byte bound cannot see a cost that produces no
// bytes. Measured by allocation, which a loaded machine does not move: 20,000 repeats allocated ~790 MB before the
// fix (~39 KB per decode, the inflater), and each distinct stream is now decoded once per page.
func TestARepeatedStreamIsDecodedOnce(t *testing.T) {
	const n = 20000
	for name, chunk := range map[string]string{"an empty stream": "", "a stream that draws": "q Q "} {
		ctx, page := dividedPage(t, 1, []byte("x"))
		sd, err := ctx.NewStreamDictForBuf([]byte(chunk))
		if err != nil {
			t.Fatal(err)
		}
		if err := sd.Encode(); err != nil {
			t.Fatal(err)
		}
		ref, err := ctx.IndRefForNewObject(*sd)
		if err != nil {
			t.Fatal(err)
		}
		arr := make(types.Array, n)
		for i := range arr {
			arr[i] = *ref
		}
		page["Contents"] = arr
		// Written and read back, so the stream is undecoded as a parsed file's is: an in-memory stream already
		// carries its content and would decode for free.
		var buf bytes.Buffer
		if err := api.WriteContext(ctx, &buf); err != nil {
			t.Fatal(err)
		}
		read, err := pdfread.Validated(buf.Bytes(), model.NewDefaultConfiguration())
		if err != nil {
			t.Fatal(err)
		}
		readPage, _, _, err := read.PageDict(1, false)
		if err != nil {
			t.Fatal(err)
		}
		var m0, m1 runtime.MemStats
		runtime.GC()
		runtime.ReadMemStats(&m0)
		out, err := pdfread.PageContent(read, readPage, 1)
		runtime.ReadMemStats(&m1)
		if chunk == "" && err != model.ErrNoContent {
			t.Errorf("%s: read as %d bytes, %v — want model.ErrNoContent", name, len(out), err)
		}
		if chunk != "" && (err != nil || len(out) != n*len(chunk)) {
			t.Errorf("%s: read as %d bytes, %v — want %d bytes, the stream %d times", name, len(out), err, n*len(chunk), n)
		}
		if alloc := m1.TotalAlloc - m0.TotalAlloc; alloc > 64<<20 {
			t.Errorf("%s named %d times: the read allocated %d MB — the stream is being decoded once per mention",
				name, n, alloc>>20)
		}
	}
}
