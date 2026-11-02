package pdfops

import (
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"strings"
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
	}
	return FamilyNone
}

// looksLikeText: no NUL in the head. Markdown, plain text and CSV have no signature, so "is text" is the
// only thing the bytes can be held to.
func looksLikeText(head []byte) bool {
	// A UTF-16 or UTF-32 text file (Notepad's "Unicode", a Windows CSV export) is full of NULs and starts with a
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
	case FamilyZip, FamilyOLE, FamilyRTF:
		if !hasRow {
			return Format{}, fmt.Errorf("%w: it is a packaged document but its name does not say which kind", ErrExtensionDisagrees)
		}
		if named.Family != fam {
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
	zr, err := zip.NewReader(r, size)
	if zr == nil || (err != nil && !errors.Is(err, zip.ErrInsecurePath)) {
		return fmt.Errorf("%w: the %s could not be read as a package", ErrCorruptDocument, f.Label)
	}
	// An insecure member name is not a refusal for a package LibreOffice reads itself; it IS one in the CBZ
	// extractor that writes members out (S03). The reader's own verdict is kept out of the way here.
	if len(zr.File) > importArchiveEntries {
		return fmt.Errorf("%w: it declares %d parts, and Nib opens up to %d", ErrArchiveRefused, len(zr.File), importArchiveEntries)
	}
	var total, packed uint64
	seen := map[string]bool{}
	for _, m := range zr.File {
		if seen[m.Name] {
			return fmt.Errorf("%w: it names the part %q twice", ErrArchiveRefused, m.Name)
		}
		seen[m.Name] = true
		total += m.UncompressedSize64
		packed += m.CompressedSize64
		if total > importDecodedBytes {
			return fmt.Errorf("%w: it expands to more than %d MiB", ErrArchiveRefused, importDecodedBytes>>20)
		}
	}
	if total > importRatioFloor && packed > 0 && total/packed > importRatioMax {
		return fmt.Errorf("%w: it expands %d to one", ErrArchiveRefused, total/packed)
	}
	switch f.Key {
	case "docx":
		return wantPart(seen, f, "word/")
	case "xlsx":
		return wantPart(seen, f, "xl/")
	case "pptx":
		return wantPart(seen, f, "ppt/")
	case "odt", "ods", "odp":
		return wantMimetype(zr, f)
	}
	return nil
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

func wantMimetype(zr *zip.Reader, f Format) error {
	for _, m := range zr.File {
		if m.Name != "mimetype" {
			continue
		}
		rc, err := m.Open()
		if err != nil {
			break
		}
		b, _ := io.ReadAll(io.LimitReader(rc, 256))
		rc.Close()
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
