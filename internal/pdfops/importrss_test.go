package pdfops

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"sync"
	"testing"
	"time"

	"nib/internal/testpdf"
)

// TestImportPeakMemoryAtGateTwo — the measured peak resident memory of two imports running at once
// (R5: S02a does not close without the number). It is an INSTRUMENT, not a tier: it builds two 200 MiB
// inputs and runs two real LibreOffice conversions, so it runs only with NIB_RSS_MEASURE=1 and prints
// what it saw. Run it: `NIB_RSS_MEASURE=1 go test ./internal/pdfops -run ImportPeakMemory -v -timeout 30m`.
//
// What is summed: this process's own resident set plus every registered converter's process group, sampled
// every 50 ms. A renderer that leaves its group is not counted (stated, Y5).
func TestImportPeakMemoryAtGateTwo(t *testing.T) {
	if os.Getenv("NIB_RSS_MEASURE") == "" {
		t.Skip("NOT RUN (an instrument, not a tier): set NIB_RSS_MEASURE=1 to measure peak memory at gate = 2")
	}
	if !LibreOfficeAvailable() || !CeilingEnforced {
		t.Skip("NOT RUN (not a pass): needs LibreOffice and /proc")
	}
	cases := []struct {
		name string
		doc  func() []byte
	}{
		{"two 200 MiB packages", func() []byte { return testpdf.PackageOfSize(MaxImportBytes - 1<<20) }},
		{"two documents of 60 large pictures", func() []byte { return testpdf.DocxWithImages(60, 800) }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			// The fixture goes to a FILE and out of memory, as an upload spilled to disk or a path is: Import
			// reads from the file after the gate, so the fixture's own copy must not be in the measurement.
			path := filepath.Join(t.TempDir(), "big.docx")
			doc := c.doc()
			size := int64(len(doc))
			if err := os.WriteFile(path, doc, 0o600); err != nil {
				t.Fatal(err)
			}
			doc = nil
			debug.FreeOSMemory()
			baseline := groupRSSOfPid(os.Getpid())

			var mu sync.Mutex
			var peakSelf, peakConv, peakTotal int64
			stop := make(chan struct{})
			sampled := make(chan struct{})
			go func() {
				defer close(sampled)
				for {
					select {
					case <-stop:
						return
					case <-time.After(50 * time.Millisecond):
					}
					self := groupRSSOfPid(os.Getpid())
					var conv int64
					converters.mu.Lock()
					for pgid := range converters.groups {
						conv += groupRSS(pgid)
					}
					converters.mu.Unlock()
					mu.Lock()
					if self > peakSelf {
						peakSelf = self
					}
					if conv > peakConv {
						peakConv = conv
					}
					if self+conv > peakTotal {
						peakTotal = self + conv
					}
					mu.Unlock()
				}
			}()
			var wg sync.WaitGroup
			errs := make([]error, 2)
			start := time.Now()
			for i := 0; i < 2; i++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					f, err := os.Open(path)
					if err != nil {
						errs[i] = err
						return
					}
					defer f.Close()
					res, err := Import(context.Background(), f, size, "big.docx", ImportOptions{Door: "measure"})
					if err == nil && !LooksLikePDF(res.PDF) {
						err = fmt.Errorf("no PDF")
					}
					errs[i] = err
				}()
			}
			wg.Wait()
			close(stop)
			<-sampled
			for i, err := range errs {
				if err != nil {
					// A package LibreOffice declines to load still measures Nib's own side of the import.
					t.Logf("import %d: %v", i, err)
				}
			}
			if peakTotal > importPeakBound {
				t.Errorf("peak total %d MiB passes the pinned bound %d MiB", peakTotal>>20, importPeakBound>>20)
			}
			t.Logf("MEASURED %s: input %d MiB each, wall %v, nib baseline %d MiB, peak nib %d MiB, peak converters %d MiB, peak total %d MiB",
				c.name, size>>20, time.Since(start).Round(time.Second), baseline>>20, peakSelf>>20, peakConv>>20, peakTotal>>20)
		})
	}
}

// groupRSSOfPid is one process's own resident set.
func groupRSSOfPid(pid int) int64 {
	b, err := os.ReadFile("/proc/" + strconv.Itoa(pid) + "/statm")
	if err != nil {
		return 0
	}
	var size, res int64
	fmt.Sscanf(string(b), "%d %d", &size, &res)
	return res * pageSize
}

// importPeakBound is the pinned ceiling on Nib plus its converters at gate = 2 with two inputs near the cap:
// measured 2026-11-01 at 1227 MiB (60 large pictures, 110 MiB each) and, for Nib's side alone with two
// 199 MiB sources, 942 MiB before readAll preallocated; headroom ~1.6x. A change that doubles the working
// set goes red here (run with NIB_RSS_MEASURE=1).
const importPeakBound = 2000 << 20
