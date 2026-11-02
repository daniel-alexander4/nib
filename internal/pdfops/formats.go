package pdfops

import "strings"

// The format table (PLAN-import-export P01.S01, ADR-184): the ONE list of the document types Nib turns
// into a PDF. Every other place that names an extension or a MIME type — the open door, the folder
// listing, the pickers' `accept`, the .desktop file's MimeType — is a view of this table, and
// TestTheFormatTableIsTheOnlyList fails on a second list (ADR-009: a rule gets one door).
//
// A row is added in the slice that builds its producer, never ahead of it: a format listed here is a
// format Nib converts today, so the pickers and the folder listing never offer what the doors refuse.
// A format whose tool is absent stays in the table with the tool named (L4) and is dropped only where
// a picker would offer a file Nib cannot convert on this machine.

// Format is one row. Exts are lowercase with no dot; MIME types are what a browser and a desktop file
// say for it.
type Format struct {
	Key   string
	Label string
	Exts  []string
	MIME  []string
	Tool  MissingTool
	// Image marks a raster image the open door reads by magic bytes (sniffImage) on all four routes.
	Image bool
	// Family is what the format's BYTES look like (D32): the table classifies by content, and an extension
	// that disagrees with the sniffed family is a named refusal, except where bytes cannot say (text).
	Family Family
	// Door marks a format every route (path, upload, fetch, OS hand-off) opens as it stands today, with
	// no second step. The folder listing and the .desktop file offer exactly these (and PDF); the rest
	// arrive through Open & convert until the import door (P01.S02) makes them Door too.
	Door bool
}

// Family is the kind of bytes a format is made of, as the sniffer (sniffFamily) can tell.
type Family int

const (
	FamilyNone  Family = iota // not classified by bytes
	FamilyImage               // a raster image, decided by sniffImage
	FamilyText                // text: bytes cannot name the format, the extension does
	FamilyZip                 // an OOXML or OpenDocument package
	FamilyOLE                 // a legacy Office compound file
	FamilyRTF                 // Rich Text
)

const (
	mimeOOXML = "application/vnd.openxmlformats-officedocument."
	mimeODF   = "application/vnd.oasis.opendocument."
)

var formats = []Format{
	{Key: "png", Label: "PNG image", Exts: []string{"png"}, MIME: []string{"image/png"}, Image: true, Door: true, Family: FamilyImage},
	{Key: "jpeg", Label: "JPEG image", Exts: []string{"jpg", "jpeg"}, MIME: []string{"image/jpeg"}, Image: true, Door: true, Family: FamilyImage},
	{Key: "tiff", Label: "TIFF image", Exts: []string{"tif", "tiff"}, MIME: []string{"image/tiff"}, Image: true, Door: true, Family: FamilyImage},
	{Key: "webp", Label: "WebP image", Exts: []string{"webp"}, MIME: []string{"image/webp"}, Image: true, Door: true, Family: FamilyImage},

	{Key: "markdown", Label: "Markdown", Exts: []string{"md", "markdown"}, MIME: []string{"text/markdown"}, Family: FamilyText},

	{Key: "doc", Label: "Word (.doc)", Exts: []string{"doc"}, MIME: []string{"application/msword"}, Tool: ToolLibreOffice, Family: FamilyOLE},
	{Key: "docx", Label: "Word", Exts: []string{"docx"}, MIME: []string{mimeOOXML + "wordprocessingml.document"}, Tool: ToolLibreOffice, Family: FamilyZip},
	{Key: "odt", Label: "OpenDocument text", Exts: []string{"odt"}, MIME: []string{mimeODF + "text"}, Tool: ToolLibreOffice, Family: FamilyZip},
	{Key: "rtf", Label: "Rich Text", Exts: []string{"rtf"}, MIME: []string{"application/rtf"}, Tool: ToolLibreOffice, Family: FamilyRTF},
	{Key: "txt", Label: "Plain text", Exts: []string{"txt"}, MIME: []string{"text/plain"}, Tool: ToolLibreOffice, Family: FamilyText},
	{Key: "xls", Label: "Excel (.xls)", Exts: []string{"xls"}, MIME: []string{"application/vnd.ms-excel"}, Tool: ToolLibreOffice, Family: FamilyOLE},
	{Key: "xlsx", Label: "Excel", Exts: []string{"xlsx"}, MIME: []string{mimeOOXML + "spreadsheetml.sheet"}, Tool: ToolLibreOffice, Family: FamilyZip},
	{Key: "ods", Label: "OpenDocument spreadsheet", Exts: []string{"ods"}, MIME: []string{mimeODF + "spreadsheet"}, Tool: ToolLibreOffice, Family: FamilyZip},
	{Key: "csv", Label: "CSV", Exts: []string{"csv"}, MIME: []string{"text/csv"}, Tool: ToolLibreOffice, Family: FamilyText},
	{Key: "ppt", Label: "PowerPoint (.ppt)", Exts: []string{"ppt"}, MIME: []string{"application/vnd.ms-powerpoint"}, Tool: ToolLibreOffice, Family: FamilyOLE},
	{Key: "pptx", Label: "PowerPoint", Exts: []string{"pptx"}, MIME: []string{mimeOOXML + "presentationml.presentation"}, Tool: ToolLibreOffice, Family: FamilyZip},
	{Key: "odp", Label: "OpenDocument presentation", Exts: []string{"odp"}, MIME: []string{mimeODF + "presentation"}, Tool: ToolLibreOffice, Family: FamilyZip},
}

// Formats returns a copy of the table, so no caller can edit the one list.
func Formats() []Format {
	out := make([]Format, len(formats))
	for i, f := range formats {
		f.Exts = append([]string(nil), f.Exts...)
		f.MIME = append([]string(nil), f.MIME...)
		out[i] = f
	}
	return out
}

func normalExt(ext string) string { return strings.ToLower(strings.TrimPrefix(ext, ".")) }

// FormatForExt returns the row an extension (any case, with or without the dot) belongs to.
func FormatForExt(ext string) (Format, bool) {
	ext = normalExt(ext)
	for _, f := range Formats() {
		for _, e := range f.Exts {
			if e == ext {
				return f, true
			}
		}
	}
	return Format{}, false
}

// FormatAvailable reports whether this machine can convert the format: its tool, if it has one, is found.
func FormatAvailable(f Format) bool {
	switch f.Tool {
	case ToolNone:
		return true
	case ToolLibreOffice:
		return LibreOfficeAvailable()
	case ToolGhostscript:
		return GhostscriptAvailable()
	}
	return false
}

// FormatMIMEs lists the MIME types of every row keep accepts, in table order.
func FormatMIMEs(keep func(Format) bool) []string {
	var out []string
	for _, f := range Formats() {
		if keep(f) {
			out = append(out, f.MIME...)
		}
	}
	return out
}

// IsDoorExt reports whether an extension names a format every open route opens as it stands.
func IsDoorExt(ext string) bool {
	f, ok := FormatForExt(ext)
	return ok && f.Door
}
