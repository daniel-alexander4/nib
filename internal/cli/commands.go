package cli

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

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

func cmdPagenum(args []string) int {
	fs := flag.NewFlagSet("nib pagenum", flag.ContinueOnError)
	var out string
	var inPlace, total bool
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
	fs.Usage = usageFunc(fs, "nib pagenum IN -o OUT [--position br --start 1 --prefix P --pad N --total]", "Stamp running page numbers (or Bates numbering) onto every page.")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	st.OfTotal = total
	return runTransform(fs, out, inPlace, func(b []byte) ([]byte, error) {
		return pdfops.StampPageNumbers(b, st)
	})
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
	fs.Usage = usageFunc(fs, "nib sign IN -o OUT --cert C.p12", "Certify a PDF with an imported .p12 identity. The passphrase comes from\n--password-file or $NIB_P12_PASSWORD, never the command line.")
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
	return "", errors.New("no passphrase: set NIB_P12_PASSWORD or use --password-file")
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
	fs.Usage = usageFunc(fs, "nib verify [--json] FILE...", "Report each file's signature integrity. Exit 2 if any file is unsigned or modified.")
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
		if asJSON {
			b, _ := json.Marshal(struct {
				File string `json:"file"`
				sign.Status
			}{p, st})
			fmt.Println(string(b))
		} else {
			fmt.Printf("%s: %s\n", p, describeStatus(st))
		}
		if st.State != sign.Valid {
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
	fs.BoolVar(&doVerify, "verify", false, "check each file against its .ots proof instead of creating one")
	fs.Usage = usageFunc(fs, "nib timestamp [--verify] FILE...", "Create an OpenTimestamps proof (FILE.ots) for each file, or with --verify\ncheck each file against its existing FILE.ots.")
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
	return timestampCreate(fs.Args())
}

func timestampCreate(files []string) int {
	client := safeClient()
	worst := 0
	for _, p := range files {
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
		proofPath := p + ".ots"
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

// transformInPlace applies fn to each file and rewrites it atomically. A file
// that fails to read or transform is reported and skipped; the rest continue,
// and the worst per-file outcome becomes the exit code.
func transformInPlace(files []string, fn func([]byte) ([]byte, error)) int {
	worst := 0
	for _, p := range files {
		data, err := os.ReadFile(p)
		if err != nil {
			errf("%v", err)
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
