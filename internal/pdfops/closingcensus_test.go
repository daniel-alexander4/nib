package pdfops

// The closing census (PLAN-import-export P06.S01, W18, Y15): every row of the format table has an import
// that comes out as a PDF, a cap that refuses, and, where it has an outside tool, a refusal that names
// the tool when it is missing. The census is EXECUTED, not grepped: it drives each row through the import
// door, and the row and check counts are constants outside the table, so a row added without a fixture, or
// a table emptied, turns it red.
//
// What it cannot see: a present-path check needs the tool, so on a machine without it the check is NAMED as
// not run (W16) and fails under NIB_REQUIRE_TOOLS=1. Where a row's own cap is a measured, producer-specific
// one the census does not rebuild it; it names the test that does, and fails if that test is gone.

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	"image/jpeg"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
	"testing"
)

const (
	// censusRows is the format table's row count, pinned here rather than read from it.
	censusRows = 28
	// censusToolRows is how many of those rows name an outside tool.
	censusToolRows = 18
	// censusChecks is the checks per row the census owes: import, input cap, row cap. A tool row owes one more.
	censusChecks = 3
)

type censusRow struct {
	name string // the file name the import is offered under
	data func(t *testing.T) []byte
	// capTest names the existing test that asserts this row's own cap at cap+1; "" with archive set means
	// the census builds the cap+1 package itself; both empty means the row has only the input cap.
	capTest string
	archive bool
}

func censusFile(file string) func(*testing.T) []byte {
	return func(t *testing.T) []byte {
		t.Helper()
		b, err := os.ReadFile(filepath.Join("testdata", "import", file))
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
}

func censusBytes(b string) func(*testing.T) []byte {
	return func(*testing.T) []byte { return []byte(b) }
}

func censusRowsTable() map[string]censusRow {
	return map[string]censusRow{
		"png": {"a.png", func(t *testing.T) []byte { return pngBytes(t, 24, 16) }, "TestThePixelCapRefusesOnTheHeaderAloneAndNamesItself", false},
		"jpeg": {"a.jpg", func(t *testing.T) []byte {
			var b bytes.Buffer
			if err := jpeg.Encode(&b, image.NewRGBA(image.Rect(0, 0, 24, 16)), nil); err != nil {
				t.Fatal(err)
			}
			return b.Bytes()
		}, "TestThePixelCapRefusesOnTheHeaderAloneAndNamesItself", false},
		"tiff": {"a.tif", func(*testing.T) []byte { return tiffOf([]int{40}, 0, false, false) }, "TestATIFFPageCapRefusesAtCapPlusOneAndAdmitsAtCap", false},
		"webp": {"a.webp", func(t *testing.T) []byte {
			b, err := base64.StdEncoding.DecodeString("UklGRh4AAABXRUJQVlA4TBEAAAAvAUAAAAdQniKXp/+BiOh/AAA=")
			if err != nil {
				t.Fatal(err)
			}
			return b
		}, "TestThePixelCapRefusesOnTheHeaderAloneAndNamesItself", false},
		"gif": {"a.gif", func(t *testing.T) []byte { return gifOf(t, 40) }, "TestAGIFFrameCapRefusesAtCapPlusOneAndAdmitsAtCap", false},
		"bmp": {"a.bmp", func(t *testing.T) []byte { return bmpOf(t, 24, 16) }, "TestThePixelCapRefusesOnTheHeaderAloneAndNamesItself", false},
		"cbz": {"a.cbz", func(t *testing.T) []byte {
			var b bytes.Buffer
			zw := zip.NewWriter(&b)
			for _, n := range []string{"p1.png", "p2.png"} {
				w, _ := zw.Create(n)
				w.Write(pngBytes(t, 24, 16))
			}
			zw.Close()
			return b.Bytes()
		}, "TestACBZEntryCapRefusesAtCapPlusOneAndAdmitsAtCap", false},
		"markdown": {"a.md", censusBytes("# A title\n\nSome prose.\n"), "TestTheTextCapRefusesAtCapPlusOneAndAdmitsAtCap", false},
		"txt":      {"a.txt", censusBytes("A plain line\n"), "TestTextCapsRefuseByName", false},
		"csv":      {"a.csv", censusBytes("name,qty\nbolt,4\n"), "TestTextCapsRefuseByName", false},

		"doc":  {"a.doc", censusFile("hello.doc"), "", false},
		"docx": {"a.docx", censusFile("hello.docx"), "", true},
		"odt":  {"a.odt", censusFile("hello.odt"), "", true},
		"rtf":  {"a.rtf", censusFile("hello.rtf"), "", false},
		"xls":  {"a.xls", censusFile("hello.xls"), "", false},
		"xlsx": {"a.xlsx", censusFile("hello.xlsx"), "", true},
		"ods":  {"a.ods", censusFile("hello.ods"), "", true},
		"ppt":  {"a.ppt", censusFile("hello.ppt"), "", false},
		"pptx": {"a.pptx", censusFile("hello.pptx"), "", true},
		"odp":  {"a.odp", censusFile("hello.odp"), "", true},

		"ps":   {"a.ps", censusBytes(psHello), "TestAPostScriptPastThePageCapIsRefusedByName", false},
		"eps":  {"a.eps", censusBytes(epsHello), "TestAPostScriptPastThePageCapIsRefusedByName", false},
		"heic": {"a.heic", func(*testing.T) []byte { return heifProbeImage }, "TestAPictureOverThePixelCapIsRefusedBeforeAnyDecoderRuns", false},
		"html": {"a.html", censusBytes("<h1>Quarterly</h1><p>Revenue grew</p>"), "TestADataPictureOverTheCapIsDropped", false},
		"svg": {"a.svg", censusBytes(`<svg xmlns="http://www.w3.org/2000/svg" width="200" height="50"><rect width="200" height="50" fill="#eef"/></svg>`),
			"TestSanitizeSVGRefusals", false},
		"mhtml": {"a.mhtml", func(t *testing.T) []byte {
			return mhtml(t, mpart("Content-Type: text/html; charset=utf-8", "<p>Saved page</p>"))
		}, "TestAnMHTMLMessageIsHeldToItsCaps", false},
		"eml": {"a.eml", func(*testing.T) []byte {
			return eml(emlHead+"Content-Type: text/plain; charset=utf-8", "A short note.\r\n")
		}, "TestAMessageIsHeldToItsCaps", false},
		"epub": {"a.epub", func(t *testing.T) []byte { return epubZip(t, nil) }, "TestAnEPUBIsHeldToItsChapterCap", false},
	}
}

func censusImport(t *testing.T, r censusRow, data []byte) (*ImportResult, error) {
	t.Helper()
	return Import(context.Background(), bytes.NewReader(data), int64(len(data)), r.name, ImportOptions{Door: "test"})
}

// A tool the machine has is a present-path check that can run; one it lacks is named, or fatal under
// NIB_REQUIRE_TOOLS=1 (W16).
func censusToolHere(f Format) bool { return FormatAvailable(f) }

func TestEveryFormatRowIsImportedCappedAndNamedWhenItsToolIsMissing(t *testing.T) {
	rows := censusRowsTable()
	table := Formats()
	if len(table) != censusRows {
		t.Fatalf("the format table has %d rows, the census pins %d: add the row to censusRowsTable (a fixture, a cap, a missing-tool check) and move the constant", len(table), censusRows)
	}
	if len(rows) != censusRows {
		t.Fatalf("the census table has %d rows, want %d", len(rows), censusRows)
	}
	tools := 0
	for _, f := range table {
		if _, ok := rows[f.Key]; !ok {
			t.Fatalf("the format table has a row %q the census has no fixture for", f.Key)
		}
		if f.Tool != ToolNone {
			tools++
		}
	}
	if tools != censusToolRows {
		t.Fatalf("%d rows name an outside tool, the census pins %d", tools, censusToolRows)
	}

	srcs := censusTestSources(t)
	ran := map[string]int{}       // key -> checks that executed
	notRun := map[string]string{} // key+check -> reason
	assertions := 0

	for _, f := range table {
		r := rows[f.Key]
		t.Run(f.Key, func(t *testing.T) {
			data := r.data(t)
			if len(data) == 0 {
				t.Fatal("empty fixture")
			}

			// 1. The import comes out as a PDF.
			if censusToolHere(f) {
				res, err := censusImport(t, r, data)
				if err != nil {
					t.Fatalf("import of a real %s: %v", f.Key, err)
				}
				if n, perr := PageCount(res.PDF); perr != nil || n < 1 {
					t.Fatalf("%s imported to %d pages (%v)", f.Key, n, perr)
				}
				if err := Validate(res.PDF); err != nil {
					t.Fatalf("%s imported to an invalid PDF: %v", f.Key, err)
				}
				assertions += 2
				ran[f.Key]++
			} else if os.Getenv("NIB_REQUIRE_TOOLS") == "1" {
				t.Fatalf("NIB_REQUIRE_TOOLS=1 and the tool for %s is missing", f.Key)
			} else {
				notRun[f.Key+"/import"] = "tool absent"
			}

			// 2. The input cap: one byte past it is refused for size, whatever the format (the head decides
			// nothing before the size does).
			_, err := Import(context.Background(), fillThen(data), MaxImportBytes+1, r.name, ImportOptions{Door: "test"})
			if !isTooLarge(err) {
				t.Fatalf("%s at MaxImportBytes+1 = %v, want ErrImportTooLarge", f.Key, err)
			}
			assertions++
			ran[f.Key]++

			// 3. The row's own cap: built here for a package, or the named test that builds it.
			switch {
			case r.archive:
				pkg := censusOverfullPackage(t, data)
				rr, n := asReader(pkg)
				if err := inspectPackage(rr, n, f); err == nil {
					t.Fatalf("%s package one entry past the cap was not refused: %v", f.Key, err)
				}
				assertions++
			case r.capTest != "":
				if !srcs[r.capTest] {
					t.Fatalf("%s names %s as its cap test and no such test exists", f.Key, r.capTest)
				}
				assertions++
			default:
				// doc, xls, ppt, rtf: not packages, no producer-specific cap; the input cap above is theirs.
				assertions++
			}
			ran[f.Key]++
		})
	}

	// 4. Each tool row, on a machine without its tool, is refused with the tool named. A child process with
	// an empty PATH, because the lookups cache a hit for the life of the process.
	missing := censusMissingToolChild(t)
	for _, f := range table {
		if f.Tool == ToolNone {
			continue
		}
		switch missing[f.Key] {
		case "OK":
			assertions++
			ran[f.Key]++
		case "FOUND":
			notRun[f.Key+"/missing-tool"] = "the tool is still found with an empty PATH"
			if os.Getenv("NIB_REQUIRE_TOOLS") == "1" {
				t.Errorf("%s: the tool could not be taken away", f.Key)
			}
		default:
			t.Errorf("%s: with its tool missing the import answered %q", f.Key, missing[f.Key])
		}
	}

	// The census itself: an executed assertion count above zero, and the not-run checks named.
	if assertions < censusRows*censusChecks-len(notRun) {
		t.Errorf("only %d assertions executed over %d rows (%d checks named not run): the census is not reading the table", assertions, censusRows, len(notRun))
	}
	if len(notRun) > 0 {
		keys := make([]string, 0, len(notRun))
		for k := range notRun {
			keys = append(keys, k+" ("+notRun[k]+")")
		}
		sort.Strings(keys)
		t.Logf("%d present-path checks not run on this machine: %s", len(notRun), strings.Join(keys, "; "))
	}
}

func isTooLarge(err error) bool {
	return err != nil && strings.Contains(err.Error(), ErrImportTooLarge.Error())
}

// censusOverfullPackage returns the fixture's zip with importArchiveEntries+1 members in all.
func censusOverfullPackage(t *testing.T, data []byte) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatal(err)
	}
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	n := 0
	for _, f := range zr.File {
		w, _ := zw.Create(f.Name)
		w.Write([]byte("x"))
		n++
	}
	for ; n <= importArchiveEntries; n++ {
		w, _ := zw.Create(fmt.Sprintf("pad/%05d.xml", n))
		w.Write([]byte("x"))
	}
	zw.Close()
	return b.Bytes()
}

// censusTestSources is the set of test function names in this package's test files.
func censusTestSources(t *testing.T) map[string]bool {
	t.Helper()
	files, _ := filepath.Glob("*_test.go")
	re := regexp.MustCompile(`(?m)^func (Test\w+)\(`)
	out := map[string]bool{}
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for _, m := range re.FindAllStringSubmatch(string(b), -1) {
			out[m[1]] = true
		}
	}
	if len(out) < 100 {
		t.Fatalf("read only %d test names: the scan is not reading the package", len(out))
	}
	return out
}

func censusMissingToolChild(t *testing.T) map[string]string {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("an empty PATH still finds a stock install on Windows")
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestClosingCensusChild$")
	cmd.Env = []string{"NIB_CENSUS_CHILD=1", "PATH=" + t.TempDir(), "TMPDIR=" + t.TempDir(), "HOME=" + t.TempDir()}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("the missing-tool child failed: %v\n%s", err, out)
	}
	got := map[string]string{}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "CENSUS ") {
			p := strings.SplitN(line, " ", 3)
			if len(p) == 3 {
				got[p[1]] = p[2]
			}
		}
	}
	if len(got) != censusToolRows {
		t.Fatalf("the child answered for %d rows, want %d:\n%s", len(got), censusToolRows, out)
	}
	return got
}

// The child half: with nothing on PATH, each tool row's real fixture is refused naming the row's tool.
func TestClosingCensusChild(t *testing.T) {
	if os.Getenv("NIB_CENSUS_CHILD") != "1" {
		t.Skip("run by TestEveryFormatRowIsImportedCappedAndNamedWhenItsToolIsMissing")
	}
	rows := censusRowsTable()
	for _, f := range Formats() {
		if f.Tool == ToolNone {
			continue
		}
		r := rows[f.Key]
		if FormatAvailable(f) {
			fmt.Printf("CENSUS %s FOUND\n", f.Key)
			continue
		}
		_, err := censusImport(t, r, r.data(t))
		if tool, ok := MissingToolFor(err); ok && tool == f.Tool {
			fmt.Printf("CENSUS %s OK\n", f.Key)
		} else {
			fmt.Printf("CENSUS %s WRONG:%v\n", f.Key, err)
		}
	}
}
