package pdfops

import (
	"bytes"
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"nib/mdpdf"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"golang.org/x/text/encoding/charmap"
)

// Plain text and CSV are native (PLAN-import-export D10, P01.S03b): Nib writes the Markdown that says what the
// file says and lays it out with its own renderer, so these two types share Markdown's faces, its tagging and
// its font honesty, and need no outside tool.
//
// Plain text is a fenced block (preformatted, wrapped); a CSV is a GFM table, split into several tables of
// at most csvRowsPerTable rows because the renderer refuses a longer one, the header repeated on each so
// every table reads on its own.
//
// Both caps REFUSE, and both are taken from the source before any layout is done (Y6): a layout that
// has begun has already spent what the cap exists to protect.

const (
	// textRowRunes is the characters one laid-out row of text is taken to hold, for the pre-layout row estimate.
	textRowRunes = 90
	// sniffLines is how many leading lines the delimiter sniff looks through.
	sniffLines = 20
)

var (
	// ErrTextTooLong: a plain-text source with more lines than importTextLines.
	ErrTextTooLong = errors.New("that text has more lines than Nib sets")
	// ErrCSVTooLong: a CSV with more rows than importCSVRows.
	ErrCSVTooLong = errors.New("that CSV has more rows than Nib sets")
	// ErrCSVTooWide: a CSV with more columns than the page can set.
	ErrCSVTooWide = errors.New("that CSV has more columns than a page can set")
	// ErrCSVTooManyCells: more cells than importCSVCells.
	ErrCSVTooManyCells = errors.New("that CSV has more cells than Nib sets")
	// ErrCSVUnreadable: text that is not a CSV at all (an unterminated quote swallowing the file is read as a
	// cell; this is for what encoding/csv itself will not parse).
	ErrCSVUnreadable = errors.New("that CSV could not be read")
)

// decodeText turns a text source into UTF-8: a byte-order mark names UTF-8 or UTF-16, valid UTF-8 is itself,
// and anything else is read as Windows-1252, which is what a legacy Windows export is.
func decodeText(b []byte) string {
	switch {
	case bytes.HasPrefix(b, []byte{0xEF, 0xBB, 0xBF}):
		return string(b[3:])
	case bytes.HasPrefix(b, []byte{0xFF, 0xFE}):
		return utf16String(b[2:], false)
	case bytes.HasPrefix(b, []byte{0xFE, 0xFF}):
		return utf16String(b[2:], true)
	}
	if utf8.Valid(b) {
		return string(b)
	}
	if s, err := charmap.Windows1252.NewDecoder().Bytes(b); err == nil {
		return string(s)
	}
	return string(bytes.ToValidUTF8(b, []byte("�")))
}

func utf16String(b []byte, bigEndian bool) string {
	u := make([]uint16, 0, len(b)/2)
	for i := 0; i+1 < len(b); i += 2 {
		if bigEndian {
			u = append(u, uint16(b[i])<<8|uint16(b[i+1]))
		} else {
			u = append(u, uint16(b[i+1])<<8|uint16(b[i]))
		}
	}
	return string(utf16.Decode(u))
}

// textToMarkdown is a plain-text file as a fenced block. The fence is one backtick longer than any run of
// them in the text (and at least three), so nothing in the text can end the block. Carriage returns go: a
// Windows file would otherwise hold a stray one at every line end.
func textToMarkdown(text string) (string, error) {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	text = strings.TrimRight(text, "\n")
	// The cap is on LAID-OUT rows, not source lines: one 20 MiB line wraps into hundreds of thousands of them. A row
	// is taken as textRowRunes characters at the narrowest page Nib sets, so the estimate errs towards refusing.
	rows := 0
	for _, ln := range strings.Split(text, "\n") {
		rows += max(1, (utf8.RuneCountInString(ln)+textRowRunes-1)/textRowRunes)
		if rows > importTextLines {
			return "", fmt.Errorf("%w: Nib sets up to %d lines", ErrTextTooLong, importTextLines)
		}
	}
	longest, run := 0, 0
	for _, r := range text {
		if r == '`' {
			run++
			longest = max(longest, run)
		} else {
			run = 0
		}
	}
	fence := strings.Repeat("`", max(3, longest+1))
	return fence + "\n" + text + "\n" + fence + "\n", nil
}

// sniffDelimiter picks the comma, semicolon or tab with the most occurrences outside quotes on the first of the
// first sniffLines lines that has any (a title line with none is passed over), comma on a tie or when none does.
func sniffDelimiter(text string) rune {
	rest := text
	for i := 0; i < sniffLines && rest != ""; i++ {
		line := rest
		if j := strings.IndexByte(rest, '\n'); j >= 0 {
			line, rest = rest[:j], rest[j+1:]
		} else {
			rest = ""
		}
		best, bestN := ',', 0
		for _, d := range []rune{',', ';', '\t'} {
			n, inQuote := 0, false
			for _, r := range line {
				switch {
				case r == '"':
					inQuote = !inQuote
				case r == d && !inQuote:
					n++
				}
			}
			if n > bestN {
				best, bestN = d, n
			}
		}
		if bestN > 0 {
			return best
		}
	}
	return ','
}

// mdCell makes a CSV cell literal Markdown: every character the inline syntax could read is escaped, a line
// break is a space, and a pipe cannot end the cell.
func mdCell(s string) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\n', '\r', '\t':
			b.WriteByte(' ')
		case '\\', '`', '*', '_', '{', '}', '[', ']', '<', '>', '#', '+', '-', '.', '!', '|', '~', '&', '$', '(', ')', ':', '"', '\'', '=', '^', '%', '@':
			b.WriteByte('\\')
			b.WriteRune(r)
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimSpace(b.String())
}

// csvToMarkdown is a CSV as GFM tables. The first record is the header. A short row is padded to the widest.
func csvToMarkdown(text string) (string, error) {
	rd := csv.NewReader(strings.NewReader(text))
	rd.Comma = sniffDelimiter(text)
	rd.FieldsPerRecord = -1
	rd.LazyQuotes = true
	var rows [][]string
	cols, cells := 0, 0
	for {
		rec, err := rd.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return "", fmt.Errorf("%w: %v", ErrCSVUnreadable, err)
		}
		if len(rows) >= importCSVRows {
			return "", fmt.Errorf("%w: Nib sets up to %d rows", ErrCSVTooLong, importCSVRows)
		}
		cols = max(cols, len(rec))
		if cols > csvMaxColumns {
			return "", fmt.Errorf("%w: it has at least %d and Nib sets up to %d", ErrCSVTooWide, cols, csvMaxColumns)
		}
		cells += len(rec)
		if cells > importCSVCells {
			return "", fmt.Errorf("%w: Nib sets up to %d", ErrCSVTooManyCells, importCSVCells)
		}
		rows = append(rows, rec)
	}
	if cols == 0 {
		return "", nil
	}
	line := func(rec []string) string {
		var b strings.Builder
		b.WriteByte('|')
		for i := 0; i < cols; i++ {
			c := ""
			if i < len(rec) {
				c = mdCell(rec[i])
			}
			b.WriteByte(' ')
			b.WriteString(c)
			b.WriteString(" |")
		}
		b.WriteByte('\n')
		return b.String()
	}
	rule := "|" + strings.Repeat(" --- |", cols) + "\n"
	var out strings.Builder
	head := line(rows[0])
	body := rows[1:]
	for start := 0; ; start += csvRowsPerTable {
		if start > 0 {
			out.WriteString("\n")
		}
		out.WriteString(head)
		out.WriteString(rule)
		end := min(start+csvRowsPerTable, len(body))
		for _, rec := range body[start:end] {
			out.WriteString(line(rec))
		}
		if end >= len(body) {
			break
		}
	}
	return out.String(), nil
}

// textImport is the plain-text and CSV producer: decode, write the Markdown that says it, lay it out with Nib's
// renderer, and hold the result to the page cap. The Markdown options that name a PAGE (size, margin) apply;
// placeholders and an image directory do not — a file's own `{{x}}` is its text, not a template's.
func textImport(data []byte, f Format, o ImportOptions) (pdf []byte, unprintable []rune, err error) {
	text := decodeText(data)
	var md string
	if f.Key == "csv" {
		md, err = csvToMarkdown(text)
	} else {
		md, err = textToMarkdown(text)
	}
	if err != nil {
		return nil, nil, err
	}
	opts := o.Markdown
	opts.Vars, opts.ImageDir = nil, ""
	pdf, err = ConvertDocToPDFWith(context.Background(), []byte(md), ".md", opts)
	if err != nil {
		return nil, nil, textLayoutRefusal(err)
	}
	if n, perr := PageCount(pdf); perr == nil && n > importPages {
		return nil, nil, fmt.Errorf("%w: Nib opens up to %d", ErrTooManyPages, importPages)
	}
	return pdf, UnprintableMarkdown([]byte(text)), nil
}

// textLayoutRefusal names the layout's own refusals in the import door's words.
func textLayoutRefusal(err error) error {
	switch {
	case errors.Is(err, mdpdf.ErrTableTooWide):
		return fmt.Errorf("%w: %v", ErrCSVTooWide, err)
	case errors.Is(err, mdpdf.ErrTableTooLong):
		return fmt.Errorf("%w: %v", ErrCSVTooLong, err)
	case errors.Is(err, mdpdf.ErrTableRowTooTall):
		return fmt.Errorf("%w: %v", ErrCSVTooWide, err)
	}
	return err
}
