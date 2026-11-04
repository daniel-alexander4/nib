package pdfops

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"

	"nib/internal/ziputil"
)

// The sniffer (D32): bytes decide what a file is, and the name decides only what bytes cannot say.
//
// Before it, `asOpenableDocument` took the image sniff and then the EXTENSION, so a file named `.docx`
// that was really a PNG was handed to the converter as a document. Now the head is read, the family is
// named from it, and a name that contradicts it is a refusal that says so.

// Refusals named by the sniffer. Each is a sentence a person can act on; a caller maps them to a status
// and a counter cause with `errors.Is`.
var (
	// ErrNotADocument: nothing Nib opens or converts.
	ErrNotADocument = errors.New("that file isn't a PDF, an image or a document Nib converts")
	// ErrCorruptDocument: a package its name says it is, whose directory cannot be read. Not "no route for
	// it": Nib has one, and the file is damaged.
	ErrCorruptDocument = errors.New("that document is damaged")
	// ErrNotText: a name that says text, bytes that are not.
	ErrNotText = errors.New("that file is named as text but is not text")
	// ErrExtensionDisagrees: the name says one kind of file and the bytes are another.
	ErrExtensionDisagrees = errors.New("the file's name and its contents disagree about what it is")
	// ErrImportTooLarge: past MaxImportBytes (or a text source past importTextBytes).
	ErrImportTooLarge = errors.New("that file is too large to open")
	// ErrArchiveRefused: a document package that declares too many parts, too much, or expands too far.
	ErrArchiveRefused = errors.New("that document is packaged in a way Nib will not open")
)

// sniffFamily names the family of a file from its head. FamilyNone is "bytes cannot say": text, or
// anything the table does not know.
func sniffFamily(head []byte) Family {
	switch {
	case LooksLikeImage(head):
		return FamilyImage
	case bytes.HasPrefix(head, []byte("PK\x03\x04")):
		return FamilyZip
	case bytes.HasPrefix(head, []byte{0xD0, 0xCF, 0x11, 0xE0, 0xA1, 0xB1, 0x1A, 0xE1}):
		return FamilyOLE
	case bytes.HasPrefix(head, []byte(`{\rtf`)):
		return FamilyRTF
	case bytes.HasPrefix(head, []byte("%!")), bytes.HasPrefix(head, dosEPSMagic):
		return FamilyPostScript
	case isHEIF(head):
		return FamilyHEIF
	case isMHTML(head):
		return FamilyMHTML
	case isHTML(head):
		return FamilyHTML
	}
	return FamilyNone
}

// dosEPSMagic opens an EPS file with a binary preview header (Adobe's TIFF/WMF-preview wrapper).
var dosEPSMagic = []byte{0xC5, 0xD0, 0xD3, 0xC6}

// looksLikeText: no NUL in the head. Markdown, plain text and CSV have no signature, so "is text" is the
// only thing the bytes can be held to.
func looksLikeText(head []byte) bool {
	// A UTF-16 text file (Notepad's "Unicode", a Windows CSV export) is full of NULs and starts with a
	// byte-order mark; LibreOffice converts it, so it is text.
	for _, bom := range [][]byte{{0xFF, 0xFE}, {0xFE, 0xFF}} {
		if bytes.HasPrefix(head, bom) {
			return true
		}
	}
	return !bytes.Contains(head, []byte{0})
}

// classify decides a file's row from its sniffed family and its name (D32). It returns the row, or a
// named refusal. A PDF is decided by the caller before this runs.
func classify(head []byte, name string) (Format, error) {
	fam := sniffFamily(head)
	ext := extOf(name)
	named, hasRow := FormatForExt(ext)

	switch fam {
	case FamilyImage:
		// An image is an image whatever it is called — a clipboard paste has no name and a URL's path may
		// end in anything. A name that is a DIFFERENT KIND of document is the contradiction.
		if hasRow && named.Family != FamilyImage {
			return Format{}, fmt.Errorf("%w: the name says %s, the contents are an image", ErrExtensionDisagrees, named.Label)
		}
		if k, ok := sniffImage(head); ok {
			for _, f := range Formats() {
				if f.Image && f.Key == string(k) {
					return f, nil
				}
			}
		}
		return Format{}, ErrNotADocument
	case FamilyPostScript:
		// A text file that happens to begin `%!` is text, and is not run: only a name that says PostScript
		// sends it to the interpreter. A `.ps` whose name is missing is not guessed (a URL's path may end in anything).
		if hasRow && named.Family == FamilyText {
			if !looksLikeText(head) {
				return Format{}, ErrNotText
			}
			return named, nil
		}
		if !hasRow {
			return Format{}, fmt.Errorf("%w: it is a PostScript program but its name does not say so", ErrExtensionDisagrees)
		}
		if named.Family != fam {
			return Format{}, fmt.Errorf("%w: the name says %s, the contents are PostScript", ErrExtensionDisagrees, named.Label)
		}
		return named, nil
	case FamilyHEIF:
		// A picture, so (as for any image) it is a picture whatever it is called: phones and mail clients rename
		// them. A name that is a different KIND of document is the contradiction.
		if hasRow && named.Family != FamilyHEIF && named.Family != FamilyImage {
			return Format{}, fmt.Errorf("%w: the name says %s, the contents are a HEIC picture", ErrExtensionDisagrees, named.Label)
		}
		return heifRow(), nil
	case FamilyMHTML:
		if hasRow && named.Family == FamilyText {
			return named, nil
		}
		if !hasRow || named.Family != fam {
			return Format{}, fmt.Errorf("%w: it is a saved web page (MHTML) but its name does not say so", ErrExtensionDisagrees)
		}
		return named, nil
	case FamilyHTML:
		// Markup is read by a browser, so a name that says text keeps it text (a Markdown file may open with a
		// tag) and a name that says nothing is not guessed (a URL's path may end in anything).
		if hasRow && named.Family == FamilyText {
			return named, nil
		}
		if !hasRow {
			return Format{}, fmt.Errorf("%w: it is a web page but its name does not say so", ErrExtensionDisagrees)
		}
		if named.Family != fam {
			return Format{}, fmt.Errorf("%w: the name says %s, the contents are a web page", ErrExtensionDisagrees, named.Label)
		}
		return named, nil
	case FamilyZip, FamilyOLE, FamilyRTF:
		if !hasRow {
			return Format{}, fmt.Errorf("%w: it is a packaged document but its name does not say which kind", ErrExtensionDisagrees)
		}
		if named.Family != fam {
			if fam == FamilyOLE && named.Family == FamilyZip {
				// A password-protected DOCX/XLSX/PPTX is an OLE compound file wrapping the encrypted package.
				return Format{}, fmt.Errorf("%w: the name says %s, but the file is an older Office file — a password-protected document looks like this; remove the password and try again", ErrExtensionDisagrees, named.Label)
			}
			return Format{}, fmt.Errorf("%w: the name says %s, the contents are not", ErrExtensionDisagrees, named.Label)
		}
		return named, nil
	}
	// Bytes cannot say. Only a text row is acceptable, and only for text.
	if hasRow && named.Family == FamilyText {
		if !looksLikeText(head) {
			return Format{}, ErrNotText
		}
		return named, nil
	}
	// A page saved as a fragment (`<p>hello</p>`) has no html, head or body tag to sniff: the name says it.
	if hasRow && named.Family == FamilyHTML && looksLikeText(head) {
		return named, nil
	}
	if hasRow {
		return Format{}, fmt.Errorf("%w: the name says %s, the contents are not", ErrExtensionDisagrees, named.Label)
	}
	return Format{}, ErrNotADocument
}

// inspectPackage reads a document package's central directory only (no member is decompressed) and holds
// it to the archive caps and to its name: a `.docx` carries `word/`, a `.xlsx` `xl/`, a `.pptx` `ppt/`, an
// OpenDocument file a `mimetype` member naming its own type. Returns ErrArchiveRefused or
// ErrExtensionDisagrees; a corrupt directory is ErrNotADocument-like "could not be read".
func inspectPackage(r io.ReaderAt, size int64, f Format) error {
	a, err := ziputil.Open(r, size, packageLimits)
	if err != nil {
		return packageRefusal(err, f)
	}
	seen := a.Names()
	switch f.Key {
	case "docx":
		return wantPart(seen, f, "word/")
	case "xlsx":
		return wantPart(seen, f, "xl/")
	case "pptx":
		return wantPart(seen, f, "ppt/")
	case "epub":
		return wantMimetype(a, f)
	case "odt", "ods", "odp":
		return wantMimetype(a, f)
	}
	return nil
}

// packageLimits are D14's archive caps for a document package a converter reads by itself.
var packageLimits = ziputil.Limits{
	Entries:             importArchiveEntries,
	DecodedBytes:        importDecodedBytes,
	RatioFloor:          importRatioFloor,
	RatioMax:            importRatioMax,
	TolerateUnsafeNames: true,
}

// packageRefusal names an archive door's refusal as the import door's own sentinel.
func packageRefusal(err error, f Format) error {
	if errors.Is(err, ziputil.ErrUnreadable) {
		return fmt.Errorf("%w: the %s could not be read as a package", ErrCorruptDocument, f.Label)
	}
	return fmt.Errorf("%w: %v", ErrArchiveRefused, err)
}

func wantPart(seen map[string]bool, f Format, prefix string) error {
	if !seen["[Content_Types].xml"] {
		return fmt.Errorf("%w: the name says %s, but the package is not one", ErrExtensionDisagrees, f.Label)
	}
	for n := range seen {
		if strings.HasPrefix(n, prefix) {
			return nil
		}
	}
	return fmt.Errorf("%w: the name says %s, but the package holds no %s part", ErrExtensionDisagrees, f.Label, strings.TrimSuffix(prefix, "/"))
}

func wantMimetype(a *ziputil.Archive, f Format) error {
	for _, m := range a.Files() {
		if m.Name != "mimetype" {
			continue
		}
		b, err := a.Read(m, 256)
		if err != nil {
			break
		}
		got := strings.TrimSpace(string(b))
		for _, want := range f.MIME {
			if got == want {
				return nil
			}
		}
		return fmt.Errorf("%w: the name says %s, but the package declares %q", ErrExtensionDisagrees, f.Label, got)
	}
	return fmt.Errorf("%w: the name says %s, but the package declares no type", ErrExtensionDisagrees, f.Label)
}

func extOf(name string) string {
	i := strings.LastIndexByte(name, '.')
	if i < 0 || i < strings.LastIndexAny(name, `/\`) {
		return ""
	}
	return name[i:]
}
