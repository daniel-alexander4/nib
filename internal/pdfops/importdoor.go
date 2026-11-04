package pdfops

import (
	"context"
	"errors"
	"io"
	"log"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"nib/internal/gate"
	"nib/mdpdf"
)

// Import is the ONE function every non-PDF becomes a PDF through (PLAN-import-export D8, ADR-185,
// ADR-009).
//
// Four routes admit bytes — a path, an upload, a URL and the OS hand-off — and two more (Combine and
// the unlock drain) reach them, and until this function each kept its own refusal: the same Office file
// was converted at one door and refused at four, and `handleOffice` and `cmdOffice` each carried their
// own copy of the language and PDF/UA handling. A caller now passes its source and what it knows (the
// name, the language the person chose) and gets a PDF or a named refusal; it decides nothing about
// formats.
//
// # What the order buys
//
//  1. **Size first**, from the declared size: a source past MaxImportBytes is refused before a byte is
//     read, so a queue of them holds no memory.
//  2. **Classify by content** (importsniff.go): a PDF is returned as it is, at once, and never waits for
//     the gate.
//  3. **A missing tool is refused before the gate**, so a machine without LibreOffice does not queue
//     eight documents to fail eight times.
//  4. **The gate is taken only for a conversion that needs an outside tool**, once, here — a native
//     producer never acquires and never counts toward the queue bound, and a tool called inside never
//     re-acquires (R6, X3). The two-minute budget starts at the acquire.
//  5. **`Admit` after the wait**, because the wait is long enough for "is there still room to open it"
//     to change. The server wires it to `roomToOpen`.
//  6. **The source is read only now**, from the ReaderAt, so a queued import holds a file handle and not
//     its bytes (X2).
//
// The source is never written (L1): Import takes a ReaderAt and has no path to write to.

// ImportOptions is everything a caller knows that the bytes do not.
type ImportOptions struct {
	// Door labels the counters: "path", "upload", "url", "handoff", "combine", "cli".
	Door string
	// Lang and LangChosen are the Document language field (/pending 471, ADR-033): Lang replaces the
	// converter's /Lang when set; LangChosen says a person chose it, which is what a PDF/UA label on a
	// Markdown conversion rests on. The automatic open sends neither and so claims nothing.
	Lang       string
	LangChosen bool
	// Markdown is the Markdown conversion's own options (page size, margin, `{{key}}` values, image directory).
	Markdown mdpdf.Options
	// TypeExt overrides the name's extension (".md" for stdin: `nib import - --type md`).
	TypeExt string
	// Admit is called once, after a gated conversion has its slot and before any work is done; a non-nil
	// answer refuses the import with it. Nil admits.
	Admit func(context.Context) error
	// Gate is the gate an outside-tool conversion waits at. Nil is the process's own (ImportGate).
	Gate *gate.Gate
	// WaitForever makes a gated import wait however long the queue is, instead of being refused with
	// gate.ErrQueueFull. The OS hand-off sets it: those files are already "opening" to the person (Y4).
	WaitForever bool
	// PDFOnly refuses anything that is not already a PDF, with ErrNotAPDF, before classifying it. The reload
	// door sets it: a document keeps a path only because the file was a PDF, and a file that has become a
	// Word document must not be converted over that path.
	PDFOnly bool
}

// ImportResult is a PDF and the facts a caller reports about how it was made.
type ImportResult struct {
	PDF []byte
	// Name is what to call the document: the source name for a PDF as it came, `<base>.pdf` for a
	// conversion (D11).
	Name string
	// Converted is false for a PDF returned as it is. A converted document is installed pathless, so Save
	// on it is Save As and the source file is never a destination (L1).
	Converted bool
	// Unprintable are Markdown characters no face can print; they will render as blanks. A warning, not a
	// refusal.
	Unprintable []rune
	// LabelRefused is why a Markdown conversion with a chosen language did not earn the PDF/UA label; nil
	// when it did or was not asked for. A note, not a failure.
	LabelRefused error
	// AttachmentsDropped is how many attachments of a mail message were not imported (Y8): none is ever
	// extracted, decoded or written. The PDF says the same in its own text.
	AttachmentsDropped int
}

// ImportGate is the process's gate for outside-tool conversions: two at a time, sixteen waiting (D9, D31(4);
// the figures are assumptions, measured at the end of P01.S02a).
var ImportGate = gate.New(2, 16)

// ErrNotAPDF is a non-PDF offered to a PDFOnly import.
var ErrNotAPDF = errors.New("that file isn't a PDF")

// ConversionError is a conversion that was attempted and failed — as opposed to a file that was refused
// before one. Image is true for the native image producer, whose failures are the person's picture, not a
// converter's.
type ConversionError struct {
	Format string
	Image  bool
	Err    error
}

func (e *ConversionError) Error() string { return e.Err.Error() }
func (e *ConversionError) Unwrap() error { return e.Err }

// Import turns src (size bytes, called name) into a PDF. See the package comment above for the order it
// works in. ctx ends the conversion, killing the converter's process tree.
func Import(ctx context.Context, src io.ReaderAt, size int64, name string, o ImportOptions) (res *ImportResult, err error) {
	fkey := "" // the table key of what was imported, for the counters
	defer func() {
		if err != nil {
			countRefused(o.Door, refusalCause(err))
		} else {
			countOK(o.Door, fkey)
		}
	}()
	if size < 0 {
		return nil, ErrSourceChanged
	}
	if size > MaxImportBytes {
		return nil, ErrImportTooLarge
	}
	head := make([]byte, min(size, int64(importSniffBytes)))
	n, rerr := src.ReadAt(head, 0)
	if rerr != nil && !errors.Is(rerr, io.EOF) {
		return nil, rerr
	}
	head = head[:n] // a source that shrank since it was measured is not zero-padded into a different file
	if LooksLikePDF(head[:min(int64(len(head)), int64(pdfHeaderWindow))]) {
		data, err := readAll(src, size)
		if err != nil {
			return nil, err
		}
		fkey = "pdf"
		return &ImportResult{PDF: data, Name: name}, nil
	}

	if o.PDFOnly {
		return nil, ErrNotAPDF
	}
	nm := name
	if o.TypeExt != "" {
		nm = "x." + strings.TrimPrefix(o.TypeExt, ".")
	}
	f, err := classify(head, nm)
	if err != nil {
		return nil, err
	}
	fkey = f.Key
	if f.Family == FamilyText && size > importTextBytes {
		return nil, ErrImportTooLarge
	}
	if f.Family == FamilyZip && f.Key != "cbz" { // a CBZ is opened by its own producer, under the stricter archive limits
		if err := inspectPackage(src, size, f); err != nil {
			return nil, err
		}
	}
	// Options that are Markdown's own, asked of a document a converter lays out: refused before a tool is looked
	// for, so the answer does not depend on what is installed.
	if f.Family != FamilyText && f.Key != "markdown" && (o.Markdown.PageSize != "" || o.Markdown.Margin != 0 || o.Markdown.Vars != nil || o.Markdown.ImageDir != "") && !f.Image {
		return nil, ErrOptionsNeedMarkdown
	}
	if f.Tool != ToolNone && !FormatAvailable(f) {
		return nil, missingToolFor(f)
	}

	convert := func(ctx context.Context) (*ImportResult, error) {
		data, err := readAll(src, size)
		if err != nil {
			return nil, err
		}
		// The package was inspected before the wait for a slot; a file replaced during it is held to the same caps now,
		// on the bytes the converter will actually read.
		if f.Family == FamilyZip && f.Key != "cbz" {
			rd, n := BytesSource(data)
			if err := inspectPackage(rd, n, f); err != nil {
				return nil, err
			}
		}
		return convertImport(ctx, data, name, f, o)
	}
	if f.Tool == ToolNone {
		if o.Admit != nil {
			if err := o.Admit(ctx); err != nil {
				return nil, err
			}
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return convert(ctx)
	}
	g := o.Gate
	if g == nil {
		g = ImportGate
	}
	run := g.Run
	if o.WaitForever {
		run = g.RunWait
	}
	err = run(ctx, officeConvertTimeout, o.Admit, func(ctx context.Context) error {
		r, cerr := convert(ctx)
		res = r
		return cerr
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

// ErrSourceChanged: the source is not the size it was when it was measured.
var ErrSourceChanged = errors.New("that file changed while it was being opened; try again")

func readAll(src io.ReaderAt, size int64) ([]byte, error) {
	// Exactly size bytes in one allocation: io.ReadAll grows by doubling and held ~4x a 200 MiB source (measured).
	buf := make([]byte, size)
	n, err := src.ReadAt(buf, 0)
	if err != nil && !(errors.Is(err, io.EOF) && int64(n) == size) {
		if errors.Is(err, io.EOF) {
			return nil, ErrSourceChanged
		}
		return nil, err
	}
	return buf, nil
}

// BytesSource is the ReaderAt and size of bytes already in memory — what the URL and Combine doors hold.
func BytesSource(b []byte) (io.ReaderAt, int64) { return bytesReaderAt(b), int64(len(b)) }

type bytesReaderAt []byte

func (b bytesReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if off >= int64(len(b)) {
		return 0, io.EOF
	}
	n := copy(p, b[off:])
	if n < len(p) {
		return n, io.EOF
	}
	return n, nil
}

// ImportName is the name a converted document is given: the source's base name with `.pdf` (D11). A source
// with no usable base is "document.pdf".
func ImportName(name string) string {
	base := filepath.Base(name)
	base = strings.TrimSuffix(base, filepath.Ext(base))
	if base == "" || base == "." || base == string(filepath.Separator) {
		base = "document"
	}
	return base + ".pdf"
}

func convertImport(ctx context.Context, data []byte, name string, f Format, o ImportOptions) (*ImportResult, error) {
	res := &ImportResult{Name: ImportName(name), Converted: true}
	var pdf []byte
	var err error
	switch {
	case f.Key == "cbz":
		pdf, err = CBZToDocument(ctx, data, name)
	case f.Image:
		pdf, err = ImageToDocumentCtx(ctx, data, name)
	case f.Family == FamilyPostScript:
		pdf, err = ConvertPostScriptToPDF(ctx, data, f.Key == "eps")
	case f.Family == FamilyHTML:
		pdf, err = ConvertHTMLToPDF(ctx, data)
	case f.Family == FamilySVG:
		pdf, err = ConvertSVGToPDF(ctx, data)
	case f.Family == FamilyMHTML:
		pdf, err = ConvertMHTMLToPDF(ctx, data)
	case f.Family == FamilyEmail:
		pdf, res.AttachmentsDropped, err = ConvertEMLToPDF(ctx, data)
	case f.Key == "epub":
		pdf, err = ConvertEPUBToPDF(ctx, data)
	case f.Family == FamilyHEIF:
		pdf, err = heicToDocument(ctx, data, name)
	case f.Key == "txt" || f.Key == "csv":
		pdf, res.Unprintable, err = textImport(data, f, o)
	default:
		ext := "." + f.Exts[0]
		if o.TypeExt != "" {
			ext = o.TypeExt
		}
		if SupportedMarkdownExt(ext) {
			res.Unprintable = UnprintableMarkdown(data)
		}
		pdf, err = ConvertDocToPDFWith(ctx, data, ext, o.Markdown)
	}
	if err != nil {
		if _, tool := MissingToolFor(err); tool || errors.Is(err, ErrShuttingDown) || ctx.Err() != nil || isNativeRefusal(err) {
			return nil, err
		}
		return nil, &ConversionError{Format: f.Key, Image: f.Image || f.Family == FamilyHEIF || f.Family == FamilySVG, Err: err}
	}
	// The converted document's title is the source's name — best-effort, and the door guarantees it: a failed
	// metadata write must not turn a conversion that succeeded into a refusal.
	if t, terr := TitleFromName(pdf, name); terr != nil {
		log.Printf("import: %v", terr)
	} else {
		pdf = t
	}
	if o.Lang != "" {
		if pdf, err = SetLang(pdf, o.Lang); err != nil {
			return nil, err
		}
		// The PDF/UA identification (ADR-033) — on nib's own Markdown conversion only, and only when the
		// language was CHOSEN, because the field's pre-fill is this computer's language and a conformance
		// claim cannot rest on a guess. A refusal costs nothing but the label.
		if f.Family == FamilyText && f.Key == "markdown" {
			labelled, lerr := LabelUA(pdf, o.LangChosen)
			switch {
			case lerr == nil:
				pdf = labelled
			case !errors.Is(lerr, ErrUALanguageNotAsserted):
				res.LabelRefused = lerr
				log.Printf("import: %s was not labelled PDF/UA: %v", name, lerr)
			}
		}
	}
	res.PDF = pdf
	return res, nil
}

// missingToolFor is the sentinel a gated format's absent tool is refused with; MissingToolFor classifies it.
func missingToolFor(f Format) error {
	switch f.Tool {
	case ToolLibreOffice:
		return ErrLibreOfficeMissing
	case ToolGhostscript:
		return ErrGhostscriptMissing
	case ToolHEIF:
		return ErrHEIFMissing
	case ToolBrowser:
		return ErrBrowserMissing
	}
	return ErrNotADocument
}

// refusalCause is the counter label for a refusal.
func refusalCause(err error) string {
	switch {
	case errors.Is(err, ErrNotAPDF):
		return "not-a-pdf"
	case errors.Is(err, ErrImportTooLarge), errors.Is(err, ErrConvertedTooLarge):
		return "cap:input-bytes"
	case errors.Is(err, ErrConverterOverMemory):
		return "cap:memory"
	case errors.Is(err, ErrArchiveRefused):
		return "cap:archive"
	case errors.Is(err, ErrPackageNamesOutside):
		return "names-outside"
	case errors.Is(err, ErrImagePixels):
		return "cap:pixels"
	case errors.Is(err, ErrTooManyPages):
		return "cap:pages"
	case errors.Is(err, ErrTextTooLong), errors.Is(err, ErrCSVTooLong), errors.Is(err, ErrCSVTooWide), errors.Is(err, ErrCSVTooManyCells):
		return "cap:text"
	case errors.Is(err, ErrCSVUnreadable):
		return "unreadable"
	case errors.Is(err, ErrNoPages):
		return "no-pages"
	case errors.Is(err, ErrGhostscriptTooOld), errors.Is(err, ErrGhostscriptNotConfined):
		return "tool-unsafe"
	case errors.Is(err, ErrPackageTooBig):
		return "cap:archive"
	case errors.Is(err, ErrPostScriptUnreadable), errors.Is(err, ErrHEIFUnreadable), errors.Is(err, ErrHTMLUnreadable), errors.Is(err, ErrPackageUnreadable):
		return "unreadable"
	case errors.Is(err, ErrHEIFCannotRead), errors.Is(err, ErrBrowserCannotRender):
		return "tool-cannot-read"
	case errors.Is(err, ErrTIFFCompression):
		return "tiff-compression"
	case errors.Is(err, ErrExtensionDisagrees):
		return "extension-disagrees"
	case errors.Is(err, ErrNotText):
		return "not-text"
	case errors.Is(err, ErrCorruptDocument):
		return "unreadable"
	case errors.Is(err, ErrNotADocument), errors.Is(err, ErrMsgNotSupported):
		return "unsupported"
	case errors.Is(err, gate.ErrQueueFull):
		return "queue-full"
	case errors.Is(err, ErrShuttingDown):
		return "shutting-down"
	case errors.Is(err, context.Canceled):
		return "cancelled"
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	}
	if _, ok := MissingToolFor(err); ok {
		return "missing-tool"
	}
	return "unreadable"
}

// The counters (L8): atomics only, because the open routes are request handling. `import.ok{door,format}`
// and `import.refused{cause}`; read by the per-door census and the status view (P2, P4–P6 of the inventory).
var importCounts struct {
	mu sync.Mutex
	m  map[string]*atomic.Uint64
}

func bump(key string) {
	importCounts.mu.Lock()
	if importCounts.m == nil {
		importCounts.m = map[string]*atomic.Uint64{}
	}
	c := importCounts.m[key]
	if c == nil {
		c = new(atomic.Uint64)
		importCounts.m[key] = c
	}
	importCounts.mu.Unlock()
	c.Add(1)
}

func countOK(door, format string)     { bump("import.ok{door=" + door + ",format=" + format + "}") }
func countRefused(door, cause string) { bump("import.refused{door=" + door + ",cause=" + cause + "}") }

// ImportCounts is a snapshot of the counters, keyed `import.ok{door=…,format=…}` / `import.refused{door=…,cause=…}`.
func ImportCounts() map[string]uint64 {
	importCounts.mu.Lock()
	defer importCounts.mu.Unlock()
	out := make(map[string]uint64, len(importCounts.m))
	for k, v := range importCounts.m {
		out[k] = v.Load()
	}
	return out
}

// ImportCountKeys is ImportCounts' keys, sorted. For tests that read a door's counter.
func ImportCountKeys() []string {
	m := ImportCounts()
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// isNativeRefusal reports whether err is a native producer REFUSING a file by a named cap or rule, which a caller
// maps itself, as opposed to a conversion that failed (ConversionError).
func isNativeRefusal(err error) bool {
	for _, t := range []error{ErrTextTooLong, ErrCSVTooLong, ErrCSVTooWide, ErrCSVTooManyCells, ErrCSVUnreadable, ErrImagePixels, ErrTooManyPages, ErrNoPages, ErrTIFFCompression, ErrArchiveRefused, ErrPackageNamesOutside, ErrCorruptDocument, ErrConvertedTooLarge, ErrGhostscriptTooOld, ErrGhostscriptNotConfined, ErrPostScriptUnreadable, ErrHEIFCannotRead, ErrHEIFUnreadable, ErrBrowserCannotRender, ErrHTMLUnreadable, ErrPackageTooBig, ErrPackageUnreadable, ErrMsgNotSupported} {
		if errors.Is(err, t) {
			return true
		}
	}
	return false
}
