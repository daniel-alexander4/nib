package pdfops

import (
	"bytes"
	"context"
	"errors"
	"sort"
	"strings"
	"unicode/utf8"

	"nib/internal/docmodel"
	"nib/internal/pdfread"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/font"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Inspection is what a document says about itself, shown to the person holding it (/pending 406): its
// document information, its XMP packet, the fonts it names, and the hidden-content scan beside them.
//
// **Every string here is one the DOCUMENT chose**, so each passes through `docmodel.Clean` (ADR-193's one
// text door), which drops the bidirectional overrides and controls that would let an Author line read as
// something else, and each is capped. What either step took is said (`Removed`, `Cut`), never silent. The
// answer goes in a JSON body only: no value of it may ride in a header (ADR-193) or a log line.
//
// It names the fields it shows and claims nothing about the rest; it is not "everything the file knows".
// Not shown yet (declared): incremental-update history, font licensing, per-page resources, XMP history.
type Inspection struct {
	Info     []InfoEntry `json:"info"`
	InfoCut  bool        `json:"infoCut,omitempty"` // more than inspectMaxInfo entries; the rest are not listed
	XMP      XMPPacket   `json:"xmp"`
	Fonts    []FontRow   `json:"fonts"`
	FontsCut bool        `json:"fontsCut,omitempty"` // more than inspectMaxFonts distinct fonts
	Scan     ScanReport  `json:"scan"`
}

// InfoEntry is one key of the document information dictionary.
type InfoEntry struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Cut     bool   `json:"cut,omitempty"`     // the value was longer than inspectMaxValue characters
	Removed int    `json:"removed,omitempty"` // characters Clean dropped from the key and value
	NotText bool   `json:"notText,omitempty"` // the value is not a string or a name, so none is shown
}

// XMPPacket is the catalog's metadata packet, as text.
type XMPPacket struct {
	Present    bool   `json:"present"`
	Text       string `json:"text,omitempty"`
	Cut        bool   `json:"cut,omitempty"`        // shown up to inspectMaxXMP bytes
	TooLarge   bool   `json:"tooLarge,omitempty"`   // it decodes past pdfread.MaxPacketBytes (decodePacket's cap); nothing is shown
	Unreadable bool   `json:"unreadable,omitempty"` // present but could not be decoded
	Removed    int    `json:"removed,omitempty"`
	// Elsewhere counts the other holders of an XMP packet — pages, images, forms, fonts (`metadataHolders`).
	Elsewhere int `json:"elsewhere,omitempty"`
}

// FontRow is one distinct font the document names.
type FontRow struct {
	Name     string `json:"name"`
	Subtype  string `json:"subtype"`
	Embedded bool   `json:"embedded"`
	Unknown  bool   `json:"unknown,omitempty"` // whether it is embedded could not be read
}

const (
	inspectMaxInfo  = 64
	inspectMaxValue = 2000
	inspectMaxXMP   = 64 << 10
	inspectMaxFonts = 500
)

// InspectDocument reads the document as Scan does — unvalidated, so it answers on the malformed files it is most
// useful on — and never writes anything back.
func InspectDocument(pdf []byte) (Inspection, error) {
	rep, err := Scan(pdf)
	if err != nil {
		return Inspection{}, err
	}
	ctx, err := api.ReadContext(context.Background(), bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		return Inspection{}, err
	}
	xt := ctx.XRefTable
	root, err := xt.Catalog()
	if err != nil {
		return Inspection{}, err
	}
	out := Inspection{Scan: rep, Info: []InfoEntry{}, Fonts: []FontRow{}}
	out.Info, out.InfoCut = inspectInfo(xt)
	out.XMP = inspectXMP(xt, root)
	out.Fonts, out.FontsCut = inspectFonts(xt)
	return out, nil
}

// inspectShown is s as the window may show it: cleaned, then capped at max characters.
func inspectShown(s string, max int) (out string, cut bool, removed int) {
	c := docmodel.Clean(s)
	removed = utf8.RuneCountInString(s) - utf8.RuneCountInString(c)
	if removed < 0 { // invalid bytes counted one rune each become one U+FFFD each: never negative in practice
		removed = 0
	}
	if r := []rune(c); len(r) > max {
		return string(r[:max]), true, removed
	}
	return c, false, removed
}

func inspectInfo(xt *model.XRefTable) ([]InfoEntry, bool) {
	rows := []InfoEntry{}
	if xt.Info == nil {
		return rows, false
	}
	info := derefDict(xt, *xt.Info)
	if info == nil {
		return rows, false
	}
	keys := make([]string, 0, len(info))
	for k := range info {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	cut := false
	if len(keys) > inspectMaxInfo {
		keys, cut = keys[:inspectMaxInfo], true
	}
	for _, k := range keys {
		key, _, kRemoved := inspectShown(k, 200)
		e := InfoEntry{Key: key, Removed: kRemoved}
		o, _ := xt.Dereference(info[k])
		switch v := o.(type) {
		case types.Name:
			e.Value, e.Cut, e.Removed = shownWith(string(v), kRemoved)
		case types.StringLiteral, types.HexLiteral:
			text, err := xt.DereferenceText(v)
			if err != nil {
				e.NotText = true
				break
			}
			e.Value, e.Cut, e.Removed = shownWith(strings.TrimSpace(text), kRemoved)
		default:
			e.NotText = true
		}
		rows = append(rows, e)
	}
	return rows, cut
}

func shownWith(s string, already int) (string, bool, int) {
	v, cut, removed := inspectShown(s, inspectMaxValue)
	return v, cut, removed + already
}

func inspectXMP(xt *model.XRefTable, root types.Dict) XMPPacket {
	var p XMPPacket
	holders := len(metadataHolders(xt))
	raw, ok := root.Find("Metadata")
	if !ok {
		p.Elsewhere = holders
		return p
	}
	p.Present = true
	p.Elsewhere = max(holders-1, 0)
	o, err := xt.Dereference(raw)
	sd, isStream := o.(types.StreamDict)
	if err != nil || !isStream {
		p.Unreadable = true
		return p
	}
	sd.Content = nil // decode into a copy, never into the table's own object
	// The catalog packet's one decode (`decodePacket`, uaid.go), so its cap is the one every packet reader keeps.
	if err := decodePacket(&sd); err != nil {
		if errors.Is(err, pdfread.ErrDecodeLimit) {
			p.TooLarge = true
		} else {
			p.Unreadable = true
		}
		return p
	}
	text := string(sd.Content)
	if len(text) > inspectMaxXMP {
		text, p.Cut = text[:inspectMaxXMP], true
	}
	p.Text, _, p.Removed = inspectShown(text, len(text)+1)
	return p
}

func inspectFonts(xt *model.XRefTable) ([]FontRow, bool) {
	seen := map[FontRow]bool{}
	rows := []FontRow{}
	cut := false
	for objNr, e := range xt.Table {
		if e == nil || e.Free {
			continue
		}
		d, ok := e.Object.(types.Dict)
		if !ok {
			continue
		}
		if t := d.NameEntry("Type"); t == nil || *t != "Font" {
			continue
		}
		row := FontRow{Name: "(unnamed)"}
		if n := d.NameEntry("BaseFont"); n != nil {
			row.Name, _, _ = inspectShown(*n, 200)
		}
		if st := d.NameEntry("Subtype"); st != nil {
			row.Subtype, _, _ = inspectShown(*st, 40)
		}
		if emb, err := font.Embedded(xt, d, objNr); err != nil {
			row.Unknown = true
		} else {
			row.Embedded = emb
		}
		if seen[row] {
			continue
		}
		if len(rows) == inspectMaxFonts {
			cut = true
			continue
		}
		seen[row] = true
		rows = append(rows, row)
	}
	// Sorted, because xt.Table is a map: the same document must list its fonts in the same order every time.
	sort.Slice(rows, func(i, j int) bool {
		a, b := rows[i], rows[j]
		if a.Name != b.Name {
			return a.Name < b.Name
		}
		if a.Subtype != b.Subtype {
			return a.Subtype < b.Subtype
		}
		return !a.Embedded && b.Embedded
	})
	return rows, cut
}
