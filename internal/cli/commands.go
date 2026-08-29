package cli

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"nib/mdpdf"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"golang.org/x/term"

	"nib/internal/ots"
	"nib/internal/pdfops"
	"nib/internal/sign"
)

// --- PDF-output transforms: IN [...] -o OUT -----------------------------------

func cmdOptimize(args []string) int {
	fs := flag.NewFlagSet("nib optimize", flag.ContinueOnError)
	var out string
	var inPlace bool
	outFlag(fs, &out)
	inPlaceFlag(fs, &inPlace)
	fs.Usage = usageFunc(fs, "nib optimize IN -o OUT  |  nib optimize -w FILE...", "Losslessly shrink a PDF (dedupe fonts/images, compress streams).")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	return runTransform(fs, out, inPlace, pdfops.Optimize)
}

func cmdSanitize(args []string) int {
	fs := flag.NewFlagSet("nib sanitize", flag.ContinueOnError)
	var out string
	var inPlace bool
	outFlag(fs, &out)
	inPlaceFlag(fs, &inPlace)
	fs.Usage = usageFunc(fs, "nib sanitize IN -o OUT  |  nib sanitize -w FILE...", "Strip identifying metadata and active content (JavaScript, auto-actions, embedded files).")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	return runTransform(fs, out, inPlace, sanitize)
}

// cmdPDFA converts a PDF to a PDF/A-2b archival candidate. It refuses (exit 1,
// reasons on stderr) a document Nib can't make conformant — non-embedded fonts or
// encryption — rather than emit a file that falsely claims PDF/A. The result must
// be verified with veraPDF; Nib cannot self-validate conformance.
func cmdPDFA(args []string) int {
	fs := flag.NewFlagSet("nib pdfa", flag.ContinueOnError)
	var out string
	var useGS bool
	outFlag(fs, &out)
	fs.BoolVar(&useGS, "gs", false, "convert via Ghostscript — the general path that re-embeds fonts and converts colour (requires Ghostscript installed)")
	fs.Usage = usageFunc(fs, "nib pdfa IN -o OUT [--gs]", "Convert to a PDF/A-2b archival candidate (verify with veraPDF). Pure-Go by default — refuses documents whose fonts aren't embedded; --gs uses Ghostscript to convert those too.")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	in, code := singleInput(fs, out)
	if code != 0 {
		return code
	}
	pdf, err := readInput(in)
	if err != nil {
		errf("%v", err)
		return 1
	}
	if useGS {
		result, err := pdfops.ConvertPDFAGhostscript(pdf)
		if err != nil {
			if errors.Is(err, pdfops.ErrGhostscriptMissing) {
				errf("Ghostscript not found — install it, or omit --gs for the pure-Go converter")
			} else {
				errf("%v", err)
			}
			return 1
		}
		return writeOut(out, result)
	}
	result, blockers, err := pdfops.PreparePDFA(pdf)
	if err != nil {
		errf("%v", err)
		return 1
	}
	if len(blockers) > 0 {
		for _, b := range blockers {
			errf("cannot convert to PDF/A: %s", b)
		}
		if pdfops.GhostscriptAvailable() {
			errf("retry with --gs to convert via Ghostscript (re-embeds fonts, converts colour)")
		}
		return 1
	}
	return writeOut(out, result)
}

// cmdOffice converts a document to PDF: Markdown natively, office documents
// (Word/Excel/PowerPoint/OpenDocument) via installed LibreOffice. The input's
// extension selects the conversion, so it needs a real file path (not stdin).
func cmdOffice(args []string) int {
	fs := flag.NewFlagSet("nib office", flag.ContinueOnError)
	var out string
	outFlag(fs, &out)
	fs.Usage = usageFunc(fs, "nib office IN -o OUT",
		"Convert a document (.md/.docx/.xlsx/.odt/.pptx/…) to PDF. Markdown converts natively; office formats need LibreOffice installed.")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	in, code := singleInput(fs, out)
	if code != 0 {
		return code
	}
	ext := filepath.Ext(in)
	if !pdfops.SupportedDocExt(ext) {
		errf("unsupported document type %q — give a Markdown, Word, Excel, PowerPoint, or OpenDocument file", ext)
		return 1
	}
	data, err := readInput(in)
	if err != nil {
		errf("%v", err)
		return 1
	}
	// Warn BEFORE the conversion becomes a record. pdfcpu maps a rune it cannot encode to
	// a SPACE rather than erroring, so a name or a heading silently loses characters in a
	// perfectly valid PDF — there is nothing to catch afterwards, which is why this is a
	// warning at the point of conversion rather than an error anywhere.
	//
	// A warning, not a refusal: a document with one unprintable rune is still worth
	// converting, and the person running the command is the one who can judge that. On
	// stderr, so `nib office in.md -o -` piped to a file still emits a clean PDF.
	if bad := pdfops.UnprintableMarkdown(data); len(bad) > 0 {
		errf("warning: %d character(s) cannot be printed and will render as blanks: %s",
			len(bad), mdpdf.FormatRunes(bad))
	}
	pdf, err := pdfops.ConvertDocToPDF(data, ext)
	if err != nil {
		if errors.Is(err, pdfops.ErrLibreOfficeMissing) {
			errf("LibreOffice not found — install it to convert office documents")
		} else {
			errf("%v", err)
		}
		return 1
	}
	return writeOut(out, pdf)
}

// sanitize runs both scrubs the GUI's Secure tab offers: active content first,
// then identifying metadata.
func sanitize(pdf []byte) ([]byte, error) {
	out, err := pdfops.StripActive(pdf)
	if err != nil {
		return nil, err
	}
	return pdfops.StripMetadata(out)
}

func cmdMerge(args []string) int {
	fs := flag.NewFlagSet("nib merge", flag.ContinueOnError)
	var out string
	outFlag(fs, &out)
	fs.Usage = usageFunc(fs, "nib merge IN... -o OUT", "Concatenate two or more PDFs, in the order given, into one.")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	if out == "" {
		errf("missing -o/--out (the output PDF)")
		return 1
	}
	if fs.NArg() < 2 {
		errf("merge needs at least two input PDFs")
		return 1
	}
	pdfs := make([][]byte, 0, fs.NArg())
	for _, p := range fs.Args() {
		b, err := readInput(p)
		if err != nil {
			errf("%v", err)
			return 1
		}
		pdfs = append(pdfs, b)
	}
	res, err := pdfops.Combine(pdfs)
	if err != nil {
		errf("%v", err)
		return 1
	}
	return writeOut(out, res)
}

func cmdRotate(args []string) int {
	fs := flag.NewFlagSet("nib rotate", flag.ContinueOnError)
	var out, pages string
	var inPlace bool
	var deg int
	outFlag(fs, &out)
	inPlaceFlag(fs, &inPlace)
	fs.IntVar(&deg, "deg", 0, "rotation in degrees, a non-zero multiple of 90 (90/180/270)")
	fs.StringVar(&pages, "pages", "", "page selection to rotate, e.g. 1-3,5 (default: all)")
	fs.Usage = usageFunc(fs, "nib rotate IN -o OUT --deg N [--pages SEL]  |  nib rotate -w FILE... --deg N", "Rotate pages by a multiple of 90 degrees.")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	if deg == 0 || deg%90 != 0 {
		errf("--deg must be a non-zero multiple of 90 (90, 180, or 270)")
		return 1
	}
	sel := splitSel(pages)
	return runTransform(fs, out, inPlace, func(b []byte) ([]byte, error) {
		return pdfops.Rotate(b, sel, deg)
	})
}

func cmdPages(args []string) int {
	fs := flag.NewFlagSet("nib pages", flag.ContinueOnError)
	var out, keep, remove string
	var inPlace bool
	outFlag(fs, &out)
	inPlaceFlag(fs, &inPlace)
	fs.StringVar(&keep, "keep", "", "keep (and reorder to) this selection, e.g. 1-3,5")
	fs.StringVar(&remove, "remove", "", "delete this selection, e.g. 2,4")
	fs.Usage = usageFunc(fs, "nib pages IN -o OUT (--keep SEL | --remove SEL)", "Keep/reorder pages, or delete them, by selection.")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	if (keep == "") == (remove == "") {
		errf("give exactly one of --keep or --remove")
		return 1
	}
	fn := func(b []byte) ([]byte, error) { return pdfops.Collect(b, splitSel(keep)) }
	if remove != "" {
		fn = func(b []byte) ([]byte, error) { return pdfops.RemovePages(b, splitSel(remove)) }
	}
	return runTransform(fs, out, inPlace, fn)
}

func cmdSplit(args []string) int {
	fs := flag.NewFlagSet("nib split", flag.ContinueOnError)
	var outDir, ranges, prefix string
	var every int
	var bookmarks bool
	fs.StringVar(&outDir, "out-dir", "", "write the output PDFs into this folder (required)")
	fs.IntVar(&every, "every", 0, "split into chunks of N pages each")
	fs.StringVar(&ranges, "ranges", "", "split at these page ranges, e.g. 1-3,4-8,9")
	fs.BoolVar(&bookmarks, "bookmarks", false, "split at each top-level bookmark")
	fs.StringVar(&prefix, "prefix", "", "prefix for each output file name")
	fs.Usage = usageFunc(fs, "nib split IN --out-dir DIR (--every N | --ranges SEL | --bookmarks) [--prefix P]", "Burst a PDF into several files — one per chunk, range, or bookmark.")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	if outDir == "" {
		errf("missing --out-dir (the output folder)")
		return 1
	}
	modes := 0
	for _, on := range []bool{every > 0, ranges != "", bookmarks} {
		if on {
			modes++
		}
	}
	if modes != 1 {
		errf("give exactly one of --every, --ranges, or --bookmarks")
		return 1
	}
	if fs.NArg() != 1 {
		errf("expected one input PDF, got %d", fs.NArg())
		return 1
	}
	b, err := readInput(fs.Arg(0))
	if err != nil {
		errf("%v", err)
		return 1
	}
	var parts []pdfops.SplitPart
	if bookmarks {
		parts, err = pdfops.SplitByBookmarks(b, prefix)
	} else {
		n, perr := pdfops.PageCount(b)
		if perr != nil {
			errf("%v", perr)
			return 1
		}
		mode, everyStr := "ranges", ""
		if every > 0 {
			mode, everyStr = "every", strconv.Itoa(every)
		}
		spans, serr := pdfops.PageSpans(mode, everyStr, ranges, n)
		if serr != nil {
			errf("%v", serr)
			return 1
		}
		parts, err = pdfops.SplitBySpans(b, spans, prefix)
	}
	if err != nil {
		errf("%v", err)
		return 1
	}
	return writeSplitFiles(outDir, parts)
}

// splitSel turns a comma-separated page selection ("1-3,5") into pdfcpu tokens;
// empty means all pages (nil).
func splitSel(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// writeSplitFiles writes each split part as <dir>/<name>.pdf, atomically and
// confined to dir (the name is title/range-derived, so the join is re-checked, as
// the GUI's folder split does). Returns a CLI exit code.
func writeSplitFiles(dir string, parts []pdfops.SplitPart) int {
	dir = filepath.Clean(dir)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		errf("%v", err)
		return 1
	}
	for _, p := range parts {
		full := filepath.Join(dir, p.Name+".pdf")
		if filepath.Dir(full) != dir { // containment: name is user/title-derived
			errf("unsafe file name %q", p.Name)
			return 1
		}
		if err := os.WriteFile(full, p.Data, 0o644); err != nil {
			errf("%v", err)
			return 1
		}
		fmt.Println(full)
	}
	fmt.Printf("%d file(s) written to %s\n", len(parts), dir)
	return 0
}

func cmdEncrypt(args []string) int {
	fs := flag.NewFlagSet("nib encrypt", flag.ContinueOnError)
	var out, passFile string
	var inPlace bool
	outFlag(fs, &out)
	inPlaceFlag(fs, &inPlace)
	fs.StringVar(&passFile, "password-file", "", "read the password from `FILE` (else $NIB_PDF_PASSWORD); required")
	fs.Usage = usageFunc(fs, "nib encrypt IN -o OUT --password-file FILE  |  nib encrypt -w FILE...", "Add AES-256 password protection. The same password opens and owns the file; an already-encrypted PDF is reported, not re-encrypted.")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	pw, err := pdfPassword(passFile)
	if err != nil {
		errf("%v", err)
		return 1
	}
	if pw == "" {
		errf("encrypt needs a password: set $NIB_PDF_PASSWORD or use --password-file")
		return 1
	}
	return runTransform(fs, out, inPlace, func(b []byte) ([]byte, error) {
		return pdfops.Encrypt(b, pw)
	})
}

func cmdDecrypt(args []string) int {
	fs := flag.NewFlagSet("nib decrypt", flag.ContinueOnError)
	var out, passFile string
	var inPlace bool
	outFlag(fs, &out)
	inPlaceFlag(fs, &inPlace)
	fs.StringVar(&passFile, "password-file", "", "read the PDF open-password from `FILE` (else $NIB_PDF_PASSWORD; empty drops owner-only restrictions)")
	fs.Usage = usageFunc(fs, "nib decrypt IN -o OUT [--password-file FILE]  |  nib decrypt -w FILE...", "Remove password protection (open password and/or owner restrictions). An already-unprotected PDF passes through unchanged.")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	pw, err := pdfPassword(passFile)
	if err != nil {
		errf("%v", err)
		return 1
	}
	return runTransform(fs, out, inPlace, func(b []byte) ([]byte, error) {
		res, err := pdfops.RemovePassword(b, pw)
		if errors.Is(err, pdfops.ErrNotEncrypted) {
			return b, nil // already plain: pass through, so batch decrypt is idempotent
		}
		return res, err
	})
}

func cmdNup(args []string) int {
	fs := flag.NewFlagSet("nib nup", flag.ContinueOnError)
	var out string
	var inPlace, border bool
	var n int
	outFlag(fs, &out)
	inPlaceFlag(fs, &inPlace)
	fs.IntVar(&n, "n", 0, "pages per sheet: 2, 3, 4, 6, 8, 9, 12, or 16 (required)")
	fs.BoolVar(&border, "border", false, "draw a thin border around each placed page")
	fs.Usage = usageFunc(fs, "nib nup IN -o OUT --n N [--border]  |  nib nup -w FILE... --n N", "Place several pages on each sheet (2-up, 4-up, …) for printing.")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	if n < 2 {
		errf("--n must be at least 2 (2, 3, 4, 6, 8, 9, 12, or 16)")
		return 1
	}
	return runTransform(fs, out, inPlace, func(b []byte) ([]byte, error) {
		return pdfops.NUp(b, n, border)
	})
}

func cmdNormalize(args []string) int {
	fs := flag.NewFlagSet("nib normalize", flag.ContinueOnError)
	var out string
	var inPlace bool
	outFlag(fs, &out)
	inPlaceFlag(fs, &inPlace)
	fs.Usage = usageFunc(fs, "nib normalize IN -o OUT  |  nib normalize -w FILE...", "Resize every page to the document's most common page size (scaled to fit, centred).")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	return runTransform(fs, out, inPlace, pdfops.NormalizePageSizes)
}

func cmdPagenum(args []string) int {
	fs := flag.NewFlagSet("nib pagenum", flag.ContinueOnError)
	var out, outDir string
	var inPlace, total, continuous bool
	st := pdfops.PageNumberStyle{Start: 1, Size: 11}
	outFlag(fs, &out)
	inPlaceFlag(fs, &inPlace)
	fs.StringVar(&st.Position, "position", "bc", "corner: tl/tc/tr/bl/bc/br")
	fs.StringVar(&st.Prefix, "prefix", "", "text before the number (e.g. \"Page \" or a Bates prefix)")
	fs.IntVar(&st.Start, "start", 1, "number printed on the first page")
	fs.IntVar(&st.Pad, "pad", 0, "zero-pad width (e.g. 6 → 000123 for Bates)")
	fs.IntVar(&st.Size, "size", 11, "point size")
	fs.StringVar(&st.Color, "color", "", "hex color #RRGGBB (default black)")
	fs.BoolVar(&total, "total", false, "append \" of N\"")
	fs.BoolVar(&continuous, "continuous", false, "number multiple files as one running sequence (Bates production)")
	fs.StringVar(&outDir, "out-dir", "", "with --continuous: write numbered copies into this directory")
	fs.Usage = usageFunc(fs, "nib pagenum IN -o OUT [--start 1 --prefix P --pad N --total]  |  nib pagenum --continuous (-w | --out-dir DIR) FILE...", "Stamp running page numbers (or Bates numbering). --continuous threads ONE counter across several files.")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	st.OfTotal = total
	if continuous {
		return runContinuousPagenum(fs.Args(), st, inPlace, outDir, out)
	}
	return runTransform(fs, out, inPlace, func(b []byte) ([]byte, error) {
		return pdfops.StampPageNumbers(b, st)
	})
}

// runContinuousPagenum stamps a set of files with ONE running page/Bates counter:
// file 1 starts at --start, each later file continues where the previous ended.
// Output is either in place (-w) or a non-destructive copy set (--out-dir); exactly
// one is required (never an implicit overwrite). With --total every input is counted
// up front, so the grand total behind "of N" is known before anything is written;
// without it each file is read, stamped and written in turn, so a bad PDF partway
// through stops the run with the earlier files already numbered.
func runContinuousPagenum(files []string, st pdfops.PageNumberStyle, inPlace bool, outDir, out string) int {
	switch {
	case out != "":
		errf("--continuous writes many files: use -w or --out-dir DIR, not -o")
		return 1
	case inPlace == (outDir != ""): // neither, or both
		errf("--continuous needs exactly one of -w (rewrite in place) or --out-dir DIR")
		return 1
	case len(files) == 0:
		errf("--continuous needs one or more input PDFs")
		return 1
	}
	if st.Start < 1 {
		st.Start = 1 // match StampPageNumbers' own clamp, so the threaded offset and Total agree
	}
	for _, f := range files {
		if f == "-" {
			errf("--continuous reads files by name; stdin (-) is not supported")
			return 1
		}
	}

	// ONE document in memory at a time, not the whole set.
	//
	// This used to read every input up front, stamp every one, and hold both the inputs
	// and the stamped outputs until the last write — so a Bates run over a few hundred
	// scanned PDFs held all of them at once. The counting pass below re-reads each file
	// and keeps only its page count; the stamping pass re-reads, stamps, writes, and
	// drops. Twice the I/O, bounded memory, and the trade is the right way round for a
	// command whose whole purpose is batches.
	//
	// The counting pass runs only for --total, which is the only thing that needs a
	// number from files it has not reached yet.
	counts := make([]int, len(files))
	if st.OfTotal {
		grand := 0
		for i, f := range files {
			n, err := pageCountOf(f)
			if err != nil {
				errf("%s: %v", f, err)
				return 1
			}
			counts[i] = n
			grand += n
		}
		st.Total = st.Start + grand - 1 // "of N" = the set's last number, not each file's
	}

	if !inPlace {
		dir := filepath.Clean(outDir)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			errf("%v", err)
			return 1
		}
		outDir = dir
	}
	seen := map[string]int{}
	offset := st.Start
	for i, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			errf("%v", err)
			return 1
		}
		n := counts[i]
		if n == 0 { // no counting pass ran (no --total): count it here, from bytes already in hand
			if n, err = pdfops.PageCount(b); err != nil {
				errf("%s: %v", f, err)
				return 1
			}
		}
		if inPlace && signedInPlace(b) {
			errf("%s: %s", f, refuseSignedInPlace)
			return 1
		}
		s := st
		s.Start = offset
		res, err := pdfops.StampPageNumbers(b, s)
		if err != nil {
			errf("%s: %v", f, err)
			return 1
		}
		offset += n
		if inPlace {
			if err := writeAtomic(f, res); err != nil {
				errf("%s: %v", f, err)
				return 1
			}
			fmt.Printf("%s: rewritten\n", f)
			continue
		}
		base := pdfops.SanitizeFilename(strings.TrimSuffix(filepath.Base(f), filepath.Ext(f)))
		full := filepath.Join(outDir, pdfops.UniqueName(base, i+1, seen)+".pdf")
		if filepath.Dir(full) != outDir { // containment: the name is input-derived
			errf("unsafe file name %q", base)
			return 1
		}
		if err := os.WriteFile(full, res, 0o644); err != nil {
			errf("%v", err)
			return 1
		}
		fmt.Println(full)
	}
	if !inPlace {
		fmt.Printf("%d file(s) written to %s\n", len(files), outDir)
	}
	return 0
}

// pageCountOf reads a PDF only to count its pages and lets the bytes go immediately —
// the whole point of the counting pass in runContinuousPagenum.
func pageCountOf(path string) (int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return pdfops.PageCount(b)
}

// rangeFlag collects repeatable --range PAGE:STYLE[:START[:PREFIX]] specs into
// page-label ranges. Fields split on ':' (so a prefix cannot itself contain ':').
type rangeFlag []pdfops.PageLabelRange

func (rf *rangeFlag) String() string { return "" }

func (rf *rangeFlag) Set(s string) error {
	parts := strings.SplitN(s, ":", 4)
	if len(parts) < 2 {
		return fmt.Errorf("range %q: want PAGE:STYLE[:START[:PREFIX]]", s)
	}
	page, err := strconv.Atoi(parts[0])
	if err != nil {
		return fmt.Errorf("range %q: page must be a number", s)
	}
	r := pdfops.PageLabelRange{Start: page, Style: parts[1], First: 1}
	if len(parts) >= 3 && parts[2] != "" {
		if r.First, err = strconv.Atoi(parts[2]); err != nil {
			return fmt.Errorf("range %q: start must be a number", s)
		}
	}
	if len(parts) == 4 {
		r.Prefix = parts[3]
	}
	*rf = append(*rf, r)
	return nil
}

// cmdPagelabels sets the document's logical page labels (the i, ii, iii / 1, 2, 3
// a viewer shows), one --range per section. Pages before the first range carry no
// label. Single transform (pipeable, -w to rewrite in place).
func cmdPagelabels(args []string) int {
	fs := flag.NewFlagSet("nib pagelabels", flag.ContinueOnError)
	var out string
	var inPlace bool
	var ranges rangeFlag
	outFlag(fs, &out)
	inPlaceFlag(fs, &inPlace)
	fs.Var(&ranges, "range", "label range PAGE:STYLE[:START[:PREFIX]], repeatable; STYLE = decimal|roman-lower|roman-upper|alpha-lower|alpha-upper|none")
	fs.Usage = usageFunc(fs, "nib pagelabels IN -o OUT --range 1:roman-lower --range 5:decimal", "Set logical page labels (front-matter i,ii,iii then body 1,2,3). Repeat --range per section.")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	if len(ranges) == 0 {
		errf("give at least one --range PAGE:STYLE[:START[:PREFIX]]")
		return 1
	}
	return runTransform(fs, out, inPlace, func(b []byte) ([]byte, error) {
		return pdfops.SetPageLabels(b, ranges)
	})
}

// cmdFill fills a PDF form from data — a single record (JSON → one PDF, -o OUT,
// pipeable) or a CSV mail-merge (one filled PDF per row → --out-dir). The data
// format is chosen by the --data file's extension. Filling removes any existing
// signature, since the content changes.
func cmdFill(args []string) int {
	fs := flag.NewFlagSet("nib fill", flag.ContinueOnError)
	var out, outDir, dataPath, nameCol string
	outFlag(fs, &out)
	fs.StringVar(&dataPath, "data", "", "form data: a .json or .xfdf record (single fill) or a .csv (one row → one PDF) (required)")
	fs.StringVar(&outDir, "out-dir", "", "with a CSV: write one filled PDF per row into this folder")
	fs.StringVar(&nameCol, "name-col", "", "with a CSV: name each output from this column's value")
	fs.Usage = usageFunc(fs, "nib fill IN --data DATA.json|.xfdf -o OUT  |  nib fill IN --data DATA.csv --out-dir DIR [--name-col COL]", "Fill a form from a JSON or XFDF record, or mail-merge a CSV (one filled PDF per row).")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	if dataPath == "" {
		errf("missing --data (the form-data JSON or CSV file)")
		return 1
	}
	data, err := os.ReadFile(dataPath)
	if err != nil {
		errf("%v", err)
		return 1
	}

	if strings.EqualFold(filepath.Ext(dataPath), ".csv") { // mail-merge
		if out != "" {
			errf("a CSV mail-merge writes many files: use --out-dir DIR, not -o")
			return 1
		}
		if outDir == "" {
			errf("missing --out-dir (where to write the filled PDFs)")
			return 1
		}
		if fs.NArg() != 1 {
			errf("expected one input PDF, got %d", fs.NArg())
			return 1
		}
		pdf, err := readInput(fs.Arg(0))
		if err != nil {
			errf("%v", err)
			return 1
		}
		parts, err := pdfops.FillFormCSV(pdf, data, nameCol)
		if err != nil {
			errf("%v", err)
			return 1
		}
		return writeSplitFiles(outDir, parts)
	}

	// Single fill from a JSON or XFDF record (pipeable via - / -o -).
	if outDir != "" {
		errf("--out-dir is for a CSV mail-merge; a JSON/XFDF record fills one PDF — use -o")
		return 1
	}
	if nameCol != "" {
		errf("--name-col is only for a CSV mail-merge")
		return 1
	}
	fillFn := func(b []byte) ([]byte, error) { return pdfops.FillFormJSON(b, data) }
	if strings.EqualFold(filepath.Ext(dataPath), ".xfdf") {
		fillFn = func(b []byte) ([]byte, error) { return pdfops.FillFormXFDF(b, data) }
	}
	in, code := singleInput(fs, out)
	if code != 0 {
		return code
	}
	return transform(in, out, fillFn)
}

// cmdExportXFDF exports a form's field data as XFDF — the XML interchange format
// Acrobat and Foxit read and write — the inverse of `nib fill --data DATA.xfdf`.
func cmdExportXFDF(args []string) int {
	fs := flag.NewFlagSet("nib export-xfdf", flag.ContinueOnError)
	var out string
	outFlag(fs, &out)
	fs.Usage = usageFunc(fs, "nib export-xfdf IN -o OUT.xfdf", "Export a form's field data as XFDF (the Acrobat/Foxit interchange format).")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	in, code := singleInput(fs, out)
	if code != 0 {
		return code
	}
	return transform(in, out, pdfops.ExportFormXFDF)
}

// cmdAttachments lists embedded files (default), extracts one to -o, or embeds one
// and writes a new PDF to -o. One mode at a time; single input PDF (— for stdin).
func cmdAttachments(args []string) int {
	fs := flag.NewFlagSet("nib attachments", flag.ContinueOnError)
	var out, extract, add, name string
	var asJSON bool
	outFlag(fs, &out)
	fs.BoolVar(&asJSON, "json", false, "emit the listing as JSON (list mode only)")
	fs.StringVar(&extract, "extract", "", "extract the embedded file named `NAME` to -o")
	fs.StringVar(&add, "add", "", "embed `FILE` as an attachment, writing a new PDF to -o")
	fs.StringVar(&name, "name", "", "attachment name for --add (default: the file's basename)")
	fs.Usage = usageFunc(fs, "nib attachments IN [--json]  |  --extract NAME -o OUT  |  --add FILE [--name N] -o OUT", "List, extract, or add embedded file attachments.")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	if extract != "" && add != "" {
		errf("give only one of --extract or --add")
		return 1
	}
	if fs.NArg() != 1 {
		errf("expected one input PDF, got %d", fs.NArg())
		return 1
	}
	pdf, err := readInput(fs.Arg(0))
	if err != nil {
		errf("%v", err)
		return 1
	}
	switch {
	case extract != "":
		if out == "" {
			errf("missing -o/--out (where to write the extracted file)")
			return 1
		}
		data, err := pdfops.ExtractAttachment(pdf, extract)
		if err != nil {
			errf("%v", err)
			return 1
		}
		return writeOut(out, data)
	case add != "":
		if out == "" {
			errf("missing -o/--out (the output PDF)")
			return 1
		}
		data, err := os.ReadFile(add)
		if err != nil {
			errf("%v", err)
			return 1
		}
		nm := name
		if nm == "" {
			nm = filepath.Base(add)
		}
		res, err := pdfops.AddAttachment(pdf, nm, data)
		if err != nil {
			errf("%v", err)
			return 1
		}
		return writeOut(out, res)
	default: // list
		aa, err := pdfops.Attachments(pdf)
		if err != nil {
			errf("%v", err)
			return 1
		}
		if asJSON {
			if aa == nil {
				aa = []pdfops.AttachmentInfo{}
			}
			b, _ := json.Marshal(aa)
			fmt.Println(string(b))
			return 0
		}
		if len(aa) == 0 {
			fmt.Println("(no attachments)")
			return 0
		}
		for _, a := range aa {
			if a.Desc != "" {
				fmt.Printf("%s — %s\n", a.Name, a.Desc)
			} else {
				fmt.Println(a.Name)
			}
		}
		return 0
	}
}

// cmdOutline prints the document's bookmark outline, indented by nesting level
// (or as JSON). A bookmark-less PDF prints nothing and exits 0.
func cmdOutline(args []string) int {
	fs := flag.NewFlagSet("nib outline", flag.ContinueOnError)
	var asJSON bool
	fs.BoolVar(&asJSON, "json", false, "emit the outline as JSON")
	fs.Usage = usageFunc(fs, "nib outline IN [--json]", "List the document's bookmark outline.")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	if fs.NArg() != 1 {
		errf("expected one input PDF, got %d", fs.NArg())
		return 1
	}
	pdf, err := readInput(fs.Arg(0))
	if err != nil {
		errf("%v", err)
		return 1
	}
	items, err := pdfops.Outline(pdf)
	if err != nil {
		errf("%v", err)
		return 1
	}
	if asJSON {
		if items == nil {
			items = []pdfops.OutlineItem{}
		}
		b, _ := json.Marshal(items)
		fmt.Println(string(b))
		return 0
	}
	if len(items) == 0 {
		fmt.Println("(no bookmarks)")
		return 0
	}
	for _, it := range items {
		fmt.Printf("%s%s (p %d)\n", strings.Repeat("  ", it.Level), it.Title, it.Page)
	}
	return 0
}

func cmdSign(args []string) int {
	fs := flag.NewFlagSet("nib sign", flag.ContinueOnError)
	var out, cert, passFile, reason, name, tsa string
	outFlag(fs, &out)
	fs.StringVar(&cert, "cert", "", "PKCS#12 (.p12/.pfx) identity `FILE` (required)")
	fs.StringVar(&passFile, "password-file", "", "read the .p12 passphrase from `FILE` (else $NIB_P12_PASSWORD)")
	fs.StringVar(&reason, "reason", "Signed with Nib", "signature reason")
	fs.StringVar(&name, "name", "", "signer name (default: the certificate's common name)")
	fs.StringVar(&tsa, "tsa", "", "RFC3161 timestamp authority `URL` to fix the signing time")
	fs.Usage = usageFunc(fs, "nib sign IN -o OUT --cert C.p12", "Certify a PDF with an imported .p12 identity. The passphrase comes from\n--password-file or $NIB_P12_PASSWORD, or a no-echo terminal prompt — never the\ncommand line.")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	in, code := singleInput(fs, out)
	if code != 0 {
		return code
	}
	if cert == "" {
		errf("missing --cert (the .p12 identity)")
		return 1
	}
	pass, err := passphrase(passFile)
	if err != nil {
		errf("%v", err)
		return 1
	}
	pdf, err := readInput(in)
	if err != nil {
		errf("%v", err)
		return 1
	}
	p12, err := os.ReadFile(cert)
	if err != nil {
		errf("%v", err)
		return 1
	}
	signed, err := sign.SignExternal(pdf, p12, pass, sign.Options{Name: name, Reason: reason, When: time.Now().UTC(), TSAURL: tsa})
	if err != nil {
		if errors.Is(err, sign.ErrWrongPassphrase) {
			errf("wrong passphrase for %s", cert)
		} else {
			errf("%v", err)
		}
		return 1
	}
	return writeOut(out, signed)
}

// passphrase sources the .p12 passphrase, never from argv: --password-file wins,
// else the NIB_P12_PASSWORD environment variable. A file's trailing newline is
// dropped (interior characters, including spaces, are preserved).
func passphrase(passFile string) (string, error) {
	if passFile != "" {
		b, err := os.ReadFile(passFile)
		if err != nil {
			return "", err
		}
		return strings.TrimRight(string(b), "\r\n"), nil
	}
	if v, ok := os.LookupEnv("NIB_P12_PASSWORD"); ok {
		return v, nil
	}
	// Last resort for an interactive run: prompt on the terminal with no echo, so
	// the passphrase never lands in argv, an env var, or a file. Skipped when stdin
	// isn't a terminal (piped / automation), which must use --password-file or
	// $NIB_P12_PASSWORD. The prompt goes to stderr, keeping stdout clean for -o -.
	if fd := int(os.Stdin.Fd()); term.IsTerminal(fd) {
		fmt.Fprint(os.Stderr, "Passphrase for the .p12 identity: ")
		b, err := term.ReadPassword(fd)
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return "", err
		}
		return string(b), nil
	}
	return "", errors.New("no passphrase: set NIB_P12_PASSWORD, use --password-file, or run in a terminal to be prompted")
}

// pdfPassword sources an OPTIONAL PDF open-password, never from argv: --password-file
// wins, else $NIB_PDF_PASSWORD, else "" (empty is valid — it drops owner-only
// restrictions). Unlike passphrase, an absent secret is not an error: not every
// protected PDF has an open password.
func pdfPassword(passFile string) (string, error) {
	if passFile != "" {
		b, err := os.ReadFile(passFile)
		if err != nil {
			return "", err
		}
		return strings.TrimRight(string(b), "\r\n"), nil
	}
	if v, ok := os.LookupEnv("NIB_PDF_PASSWORD"); ok {
		return v, nil
	}
	return "", nil
}

// --- verify: signature integrity ----------------------------------------------

func cmdVerify(args []string) int {
	fs := flag.NewFlagSet("nib verify", flag.ContinueOnError)
	var asJSON bool
	fs.BoolVar(&asJSON, "json", false, "emit one JSON object per file instead of a text report")
	fs.Usage = usageFunc(fs, "nib verify [--json] FILE...", "Report each file's signature integrity, and the ceremony it belongs to if it has one.\nExit 2 if any file is unsigned, modified, has content added after its last\nsignature, or belongs to a ceremony some obliged party has not signed.")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	if fs.NArg() == 0 {
		errf("verify needs at least one PDF")
		return 1
	}
	worst := 0
	for _, p := range fs.Args() {
		data, err := os.ReadFile(p)
		if err != nil {
			errf("%v", err)
			worst = max(worst, 1)
			continue
		}
		st := sign.Verify(data)
		cer := ceremonyReportOf(data, st, time.Now())
		if asJSON {
			b, _ := json.Marshal(struct {
				File string `json:"file"`
				sign.Status
				Ceremony *ceremonyReportJSON `json:"ceremony,omitempty"`
			}{p, st, cer.json()})
			fmt.Println(string(b))
		} else {
			fmt.Printf("%s: %s\n", p, describeStatus(st))
			for _, line := range cer.lines() {
				fmt.Printf("  %s\n", line)
			}
		}
		// **An unfinished ceremony exits non-zero too (P07.S10).**
		//
		// The README ships `nib verify contract.pdf && echo "signature intact"`, and a nine-party
		// deed that four obliged parties never signed is not a document a script should wave
		// through. Every signature on it is valid and `State` is Valid, so without this the
		// machine-readable channel says "fine" about a document the human-readable channel
		// describes as unfinished.
		//
		// That is the same divergence `AddedAfter` was added to this condition to close, recorded
		// three paragraphs down: "the CLI was the one surface where the machine-readable channel
		// disagreed with the human one". The help text moves with it.
		if st.State != sign.Valid || st.AddedAfter || cer.incomplete() {
			// AddedAfter too, and it is the case that mattered.
			//
			// `sign.Verify` reports State=Valid with AddedAfter=true for a document
			// carrying content in a revision LATER than its last signature — each
			// signature still proves its own content intact, and the final document is not
			// wholly signed. Exit was driven by State alone, so `nib verify` returned 0 for
			// it while its own help says "Exit 2 if any file is unsigned or modified" and
			// README ships `nib verify contract.pdf && echo "signature intact"`.
			//
			// The counterparty who returns your signed contract having appended pages —
			// an ordinary, tool-supported PDF operation — is the actor. The text line does
			// say "content added after the last signature" and --json carries addedAfter,
			// so a human reading each line catches it; the CLI was the one surface where
			// the machine-readable channel disagreed with the human one.
			worst = max(worst, 2)
		}
	}
	return worst
}

func describeStatus(st sign.Status) string {
	switch st.State {
	case sign.Valid:
		s := fmt.Sprintf("valid (%d signer(s))", len(st.Signers))
		if st.AddedAfter {
			s += "; content added after the last signature"
		}
		return s
	case sign.Invalid:
		return "INVALID — modified since signing"
	default:
		return "unsigned"
	}
}

// --- timestamp: create / verify OpenTimestamps proofs -------------------------

func cmdTimestamp(args []string) int {
	fs := flag.NewFlagSet("nib timestamp", flag.ContinueOnError)
	var doVerify bool
	var force bool
	fs.BoolVar(&doVerify, "verify", false, "check each file against its .ots proof instead of creating one")
	fs.BoolVar(&force, "force", false, "re-stamp even where a .ots proof already exists (discards it)")
	fs.Usage = usageFunc(fs, "nib timestamp [--verify] [--force] FILE...", "Create an OpenTimestamps proof (FILE.ots) for each file, or with --verify\ncheck each file against its existing FILE.ots.\n\nA file that already has a proof is skipped, so re-running over a directory is\nsafe; --force re-stamps and discards the existing proof.")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	if fs.NArg() == 0 {
		errf("timestamp needs at least one PDF")
		return 1
	}
	if doVerify {
		return timestampVerify(fs.Args())
	}
	return timestampCreate(fs.Args(), force)
}

// timestampCreate stamps each file, skipping one that already has a proof unless
// force is set.
//
// The skip is the important part, and it is not politeness. A .ots proof starts
// PENDING and becomes anchored to a Bitcoin block hours to days later; re-stamping
// replaces a confirmed proof with a fresh pending one and the anchoring is not
// recoverable. The README documents `for f in *.pdf; do nib timestamp "$f"; done`,
// which a user will naturally run twice — so the unguarded write turned an ordinary
// repeat into silent destruction of every proof in the directory.
//
// watchTimestamp has always had this guard; the two paths simply disagreed about
// the same invariant. Skipping (rather than erroring) also matches the
// ErrNotEncrypted-passes-through precedent for idempotent batch runs: a second pass
// over a directory should be a no-op that exits 0, not a failure.
func timestampCreate(files []string, force bool) int {
	client := safeClient()
	worst := 0
	for _, p := range files {
		proofPath := p + ".ots"
		if !force {
			if _, err := os.Stat(proofPath); err == nil {
				fmt.Printf("%s: skipped (%s exists; --force to re-stamp)\n", p, proofPath)
				continue
			}
		}
		data, err := os.ReadFile(p)
		if err != nil {
			errf("%v", err)
			worst = max(worst, 1)
			continue
		}
		digest := sha256.Sum256(data)
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		proof, err := ots.Stamp(ctx, client, digest, ots.DefaultCalendars)
		cancel()
		if err != nil {
			errf("%s: could not reach an OpenTimestamps calendar server: %v", p, err)
			worst = max(worst, 1)
			continue
		}
		if err := os.WriteFile(proofPath, proof, 0o644); err != nil {
			errf("%v", err)
			worst = max(worst, 1)
			continue
		}
		fmt.Printf("%s: wrote %s\n", p, proofPath)
	}
	return worst
}

func timestampVerify(files []string) int {
	client := safeClient()
	// minAgree 2: at least two of the independent public explorers must agree on
	// the attested block before the result is trusted (mirrors the GUI default).
	sources := make([]ots.BlockSource, len(ots.DefaultExplorers))
	for i, e := range ots.DefaultExplorers {
		sources[i] = ots.NewEsplora(e, client)
	}
	worst := 0
	for _, p := range files {
		data, err := os.ReadFile(p)
		if err != nil {
			errf("%v", err)
			worst = max(worst, 1)
			continue
		}
		proof, err := os.ReadFile(p + ".ots")
		if err != nil {
			errf("%s: no proof file at %s", p, p+".ots")
			worst = max(worst, 1)
			continue
		}
		digest := sha256.Sum256(data)
		ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		res, err := ots.VerifyProof(ctx, client, sources, 2, proof, digest)
		cancel()
		if err != nil {
			errf("%s: %v", p, err)
			worst = max(worst, 1)
			continue
		}
		fmt.Printf("%s: %s\n", p, describeProof(res))
		if res.State != ots.StateConfirmed {
			worst = max(worst, 2)
		}
	}
	return worst
}

func describeProof(res *ots.VerifyResult) string {
	switch res.State {
	case ots.StateConfirmed:
		return fmt.Sprintf("confirmed — Bitcoin block %d, %s", res.Height, res.Time.UTC().Format("2006-01-02"))
	case ots.StatePending:
		return "pending — not yet anchored to a Bitcoin block"
	case ots.StateMismatch:
		return "MISMATCH — proof is for a different document"
	default:
		return "INVALID — proof does not verify"
	}
}

// --- shared helpers -----------------------------------------------------------

// singleInput validates the one-input + required -o shape shared by the
// single-output transforms, returning the input path or a non-zero exit code.
func singleInput(fs *flag.FlagSet, out string) (string, int) {
	if out == "" {
		errf("missing -o/--out (the output PDF)")
		return "", 1
	}
	if fs.NArg() != 1 {
		errf("expected one input PDF, got %d", fs.NArg())
		return "", 1
	}
	return fs.Arg(0), 0
}

// runTransform dispatches a single-output transform between its two modes: with
// -w/--in-place it rewrites each of N files in place; otherwise it takes one
// input and the required -o output. The two are mutually exclusive.
func runTransform(fs *flag.FlagSet, out string, inPlace bool, fn func([]byte) ([]byte, error)) int {
	if inPlace {
		if out != "" {
			errf("-o/--out cannot be combined with -w/--in-place")
			return 1
		}
		if fs.NArg() == 0 {
			errf("nothing to do: give one or more PDFs to rewrite with -w")
			return 1
		}
		return transformInPlace(fs.Args(), fn)
	}
	in, code := singleInput(fs, out)
	if code != 0 {
		return code
	}
	return transform(in, out, fn)
}

// signedInPlace reports whether rewriting pdf in place would destroy a
// signature. Any structural rewrite invalidates every signature on a document —
// the byte ranges each signature covers cease to exist — and in place there is
// no undo and no second copy. The loss is silent, too: the result is a perfectly
// valid PDF that simply no longer proves anything.
//
// So Nib refuses, and says why. This is the destructive twin of the judgment
// `nib watch` already makes about unattended `sign` (watch.go): an operation
// whose consequence the user cannot see coming is not a silent default.
//
// Writing to a NEW file is deliberately unaffected — the original survives, and
// deliberately stripping a signature into a copy is a legitimate thing to want.
// A document with an empty placeholder signature field verifies as Unsigned, so
// it rewrites normally; there is nothing there to lose.
func signedInPlace(pdf []byte) bool {
	return sign.Verify(pdf).State != sign.Unsigned
}

// refuseSignedInPlace is the one message every in-place door gives, so the
// remedy reads the same wherever the user meets it.
const refuseSignedInPlace = "refusing to rewrite a signed PDF in place — that invalidates every signature on it; write the result to a new file instead"

// transformInPlace applies fn to each file and rewrites it atomically. A file
// that fails to read or transform is reported and skipped; the rest continue,
// and the worst per-file outcome becomes the exit code. This is the -w door for
// every runTransform command — optimize, sanitize, rotate, normalize, pagenum,
// pagelabels and the rest — so the signed-document guard sits here rather than
// in any one of them.
func transformInPlace(files []string, fn func([]byte) ([]byte, error)) int {
	worst := 0
	for _, p := range files {
		data, err := os.ReadFile(p)
		if err != nil {
			errf("%v", err)
			worst = max(worst, 1)
			continue
		}
		if signedInPlace(data) {
			errf("%s: %s", p, refuseSignedInPlace)
			worst = max(worst, 1)
			continue
		}
		res, err := fn(data)
		if err != nil {
			errf("%s: %v", p, err)
			worst = max(worst, 1)
			continue
		}
		if err := writeAtomic(p, res); err != nil {
			errf("%s: %v", p, err)
			worst = max(worst, 1)
			continue
		}
		fmt.Printf("%s: rewritten\n", p)
	}
	return worst
}

// transform reads in, applies fn, and writes the result to out. in and out may
// be "-" for stdin / stdout, so commands compose in a pipeline.
func transform(in, out string, fn func([]byte) ([]byte, error)) int {
	data, err := readInput(in)
	if err != nil {
		errf("%v", err)
		return 1
	}
	res, err := fn(data)
	if err != nil {
		errf("%s: %v", inputName(in), err)
		return 1
	}
	return writeOut(out, res)
}

func writeOut(out string, data []byte) int {
	if err := writeOutput(out, data); err != nil {
		errf("%v", err)
		return 1
	}
	return 0
}

// readInput reads a PDF from path, or from stdin when path is "-".
func readInput(path string) ([]byte, error) {
	if path == "-" {
		return io.ReadAll(os.Stdin)
	}
	return os.ReadFile(path)
}

// writeOutput writes data to out, or to stdout when out is "-". It refuses to
// dump a PDF onto a terminal, where the raw bytes would be unreadable garbage.
func writeOutput(out string, data []byte) error {
	if out == "-" {
		if info, err := os.Stdout.Stat(); err == nil && info.Mode()&os.ModeCharDevice != 0 {
			return errors.New("refusing to write a PDF to the terminal; redirect with > or pass -o FILE")
		}
		_, err := os.Stdout.Write(data)
		return err
	}
	return os.WriteFile(out, data, 0o644)
}

// inputName labels an input in messages: "-" reads as "<stdin>".
func inputName(path string) string {
	if path == "-" {
		return "<stdin>"
	}
	return path
}
