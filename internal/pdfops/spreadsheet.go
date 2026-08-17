package pdfops

import (
	"archive/zip"
	"bytes"
	"encoding/csv"
	"encoding/xml"
	"fmt"
	"hash/crc32"
	"strings"
)

// A grid of cell text (rows of columns) extracted from a PDF page's table by the
// browser, serialized here into spreadsheet formats. Generation is deliberately
// dependency-free: CSV via encoding/csv, and XLSX hand-rolled as the minimal
// OOXML package (zip + a few XML parts), since a Go office library would be a
// heavy/licence-encumbered dependency the project declined.

// csvSafe neutralizes a cell that a spreadsheet would execute as a formula.
//
// The grid is table text extracted from an UNTRUSTED PDF, and CSV is the one output
// format here that carries no type information: XLSX marks cells t="inlineStr" and
// ODS office:value-type="string", so both are inert by construction, while a CSV
// cell beginning = + - @ (or a leading tab/CR before one) becomes a live formula the
// moment the file is opened in Excel or LibreOffice — =HYPERLINK, DDE, and the rest.
//
// Prefixing with an apostrophe is the conventional fix: spreadsheets read it as
// "treat the rest as text" and strip it on display, and a plain text reader sees one
// leading quote rather than a mangled value. Leading control characters are dropped
// first, since they would otherwise carry the trigger past the check.
func csvSafe(cell string) string {
	trimmed := strings.TrimLeft(cell, "\t\r\n ")
	if trimmed == "" {
		return cell
	}
	switch trimmed[0] {
	case '=', '+', '-', '@':
		return "'" + trimmed
	}
	return cell
}

// GridToCSV serializes the grid as RFC-4180 CSV (encoding/csv handles quoting).
func GridToCSV(grid [][]string) ([]byte, error) {
	var buf bytes.Buffer
	cw := csv.NewWriter(&buf)
	safe := make([][]string, len(grid))
	for i, row := range grid {
		safe[i] = make([]string, len(row))
		for j, cell := range row {
			safe[i][j] = csvSafe(cell)
		}
	}
	if err := cw.WriteAll(safe); err != nil {
		return nil, err
	}
	return buf.Bytes(), cw.Error()
}

// GridToODS serializes the grid as a minimal OpenDocument Spreadsheet (.ods) — the
// ODF counterpart of GridToXLSX, hand-rolled (zip + XML), pure Go. ODF requires the
// uncompressed "mimetype" entry to be first in the package; the rest are ordinary
// deflated parts.
func GridToODS(grid [][]string) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	// mimetype MUST be the first entry, stored uncompressed, with its size/CRC in the
	// local header (no data descriptor) so an ODF reader can sniff it at byte 38.
	// CreateRaw (not Create) is required — Create streams with a data-descriptor flag
	// that zeroes the local-header size, which strict readers (LibreOffice) reject.
	mtBytes := []byte(odsMimetype)
	mt, err := zw.CreateRaw(&zip.FileHeader{
		Name:               "mimetype",
		Method:             zip.Store,
		CRC32:              crc32.ChecksumIEEE(mtBytes),
		CompressedSize64:   uint64(len(mtBytes)),
		UncompressedSize64: uint64(len(mtBytes)),
	})
	if err != nil {
		return nil, err
	}
	if _, err := mt.Write(mtBytes); err != nil {
		return nil, err
	}
	for _, p := range []struct{ name, body string }{
		{"META-INF/manifest.xml", odsManifestXML},
		{"content.xml", odsContentXML(grid)},
	} {
		w, err := zw.Create(p.name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write([]byte(p.body)); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// odsContentXML builds the ODF body: one <table:table-row> per grid row, a
// string <table:table-cell> per non-empty cell (empty cells are self-closing).
func odsContentXML(grid [][]string) string {
	var b strings.Builder
	b.WriteString(xmlHeader)
	b.WriteString(`<office:document-content xmlns:office="urn:oasis:names:tc:opendocument:xmlns:office:1.0" xmlns:table="urn:oasis:names:tc:opendocument:xmlns:table:1.0" xmlns:text="urn:oasis:names:tc:opendocument:xmlns:text:1.0" office:version="1.2"><office:body><office:spreadsheet><table:table table:name="Sheet1">`)
	for _, row := range grid {
		b.WriteString(`<table:table-row>`)
		for _, cell := range row {
			if cell == "" {
				b.WriteString(`<table:table-cell/>`)
				continue
			}
			fmt.Fprintf(&b, `<table:table-cell office:value-type="string"><text:p>%s</text:p></table:table-cell>`, xmlEsc(cell))
		}
		b.WriteString(`</table:table-row>`)
	}
	b.WriteString(`</table:table></office:spreadsheet></office:body></office:document-content>`)
	return b.String()
}

const odsMimetype = "application/vnd.oasis.opendocument.spreadsheet"

const odsManifestXML = xmlHeader + `<manifest:manifest xmlns:manifest="urn:oasis:names:tc:opendocument:xmlns:manifest:1.0" manifest:version="1.2"><manifest:file-entry manifest:full-path="/" manifest:media-type="application/vnd.oasis.opendocument.spreadsheet"/><manifest:file-entry manifest:full-path="content.xml" manifest:media-type="text/xml"/></manifest:manifest>`

// GridToXLSX serializes the grid as a minimal .xlsx (OOXML SpreadsheetML) with one
// worksheet, using inline strings (no shared-string table, no styles) — the
// smallest structure Excel and LibreOffice both open.
func GridToXLSX(grid [][]string) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	parts := []struct{ name, body string }{
		{"[Content_Types].xml", contentTypesXML},
		{"_rels/.rels", relsXML},
		{"xl/workbook.xml", workbookXML},
		{"xl/_rels/workbook.xml.rels", workbookRelsXML},
		{"xl/worksheets/sheet1.xml", sheetXML(grid)},
	}
	for _, p := range parts {
		w, err := zw.Create(p.name)
		if err != nil {
			return nil, err
		}
		if _, err := w.Write([]byte(p.body)); err != nil {
			return nil, err
		}
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// sheetXML builds the worksheet part: one <row> per grid row, one inline-string
// <c> per cell. Empty trailing cells are omitted. xml:space="preserve" keeps
// leading/trailing spaces.
func sheetXML(grid [][]string) string {
	var b strings.Builder
	b.WriteString(xmlHeader)
	b.WriteString(`<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>`)
	for r, row := range grid {
		fmt.Fprintf(&b, `<row r="%d">`, r+1)
		for c, cell := range row {
			if cell == "" {
				continue
			}
			fmt.Fprintf(&b, `<c r="%s%d" t="inlineStr"><is><t xml:space="preserve">%s</t></is></c>`, colName(c), r+1, xmlEsc(cell))
		}
		b.WriteString(`</row>`)
	}
	b.WriteString(`</sheetData></worksheet>`)
	return b.String()
}

// colName converts a 0-based column index to its spreadsheet letter (0→A, 25→Z,
// 26→AA).
func colName(n int) string {
	s := ""
	for n >= 0 {
		s = string(rune('A'+n%26)) + s
		n = n/26 - 1
	}
	return s
}

func xmlEsc(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

const xmlHeader = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>`

const contentTypesXML = xmlHeader + `<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/><Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/></Types>`

const relsXML = xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/></Relationships>`

const workbookXML = xmlHeader + `<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><sheets><sheet name="Sheet1" sheetId="1" r:id="rId1"/></sheets></workbook>`

const workbookRelsXML = xmlHeader + `<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/></Relationships>`
