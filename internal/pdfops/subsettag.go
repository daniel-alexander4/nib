package pdfops

import (
	"crypto/sha256"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// ownFacesOf is what every door that embeds a nib-supplied face runs over the faces it drew, inside a
// rewrite the door is already paying for: no `/CIDSet` nib cannot support (`dropCIDSetsOf`), and a
// subset tag that is a function of the subset rather than of the clock (`retagSubsetsOf`). ONE door
// (ADR-009) — `embeddedFacesAreHonest` for the doors that author or overlay a document, and the text
// stamps' rewrite — so a door that drops the one cannot forget the other.
func ownFacesOf(ctx *model.Context, faces []string) {
	dropCIDSetsOf(ctx, faces)
	retagSubsetsOf(ctx, faces)
}

// retagSubsetsOf replaces the six-letter subset tag of every embedded subset of the named faces (every
// subset, with none named) with one DERIVED FROM THE SUBSET: the face name and its font program — /pending
// 757.
//
// **pdfcpu draws the tag from the clock.** `font/fontDict.go` `subFontPrefix` seeds `math/rand` with
// `time.Now().UnixNano()` and has no configuration, so two renders of the same page carried
// `KPGGTK+Roboto-Bold` and `VWUKPP+Roboto-Bold` over byte-identical font programs and content streams.
// `ContentDigest` folds each page's font dictionaries in (it must: the font decides what the glyphs look
// like), so the readme — a page with NO inputs — had a different digest on every call, and so did every
// document `PrepareDocument` produced from the same bytes: its DocHash could not be reproduced from what
// went into it. Measured on base: two `RenderReadme` calls, digests `5428f3da…` and `b8b58a39…`, the tags
// the only difference inside any object `ContentDigest` reads.
//
// **Why a hash and not a constant.** ISO 32000-1 §9.6.4 asks only for six uppercase letters, but a reader
// may treat two fonts with one tag as one font, so two DIFFERENT subsets of one face must not share a tag.
// A hash of the program keeps that (a collision needs 26^6 luck) and gives identical subsets the same
// tag, which is the truth about them.
//
// It touches only names of the tagged form (`ABCDEF+Face`), rewrites the descriptor's `/FontName` and every
// font dictionary's `/BaseFont` that named the old one, and leaves anything it cannot read as it was: a
// program it cannot decode keeps pdfcpu's tag, which is what shipped before.
func retagSubsetsOf(ctx *model.Context, faces []string) {
	only := map[string]bool{}
	for _, f := range faces {
		only[f] = true
	}
	renamed := map[string]string{}
	for _, e := range ctx.XRefTable.Table {
		if e == nil || e.Object == nil {
			continue
		}
		d, ok := e.Object.(types.Dict)
		if !ok {
			continue
		}
		if ty, _ := d["Type"].(types.Name); ty != "FontDescriptor" {
			continue
		}
		name, _ := d["FontName"].(types.Name)
		base, ok := subsetFace(string(name))
		if !ok || (len(only) > 0 && !only[base]) {
			continue
		}
		prog := fontProgramBytes(ctx, d)
		if prog == nil {
			continue
		}
		to := subsetTag(base, prog) + "+" + base
		if to == string(name) {
			continue
		}
		d["FontName"] = types.Name(to)
		renamed[string(name)] = to
	}
	if len(renamed) == 0 {
		return
	}
	for _, e := range ctx.XRefTable.Table {
		if e == nil || e.Object == nil {
			continue
		}
		d, ok := e.Object.(types.Dict)
		if !ok {
			continue
		}
		if ty, _ := d["Type"].(types.Name); ty != "Font" {
			continue
		}
		if bf, _ := d["BaseFont"].(types.Name); renamed[string(bf)] != "" {
			d["BaseFont"] = types.Name(renamed[string(bf)])
		}
	}
}

// subsetFace splits `ABCDEF+Face` into Face, reporting whether name has that form at all.
func subsetFace(name string) (string, bool) {
	if len(name) < 8 || name[6] != '+' {
		return "", false
	}
	for i := 0; i < 6; i++ {
		if name[i] < 'A' || name[i] > 'Z' {
			return "", false
		}
	}
	return name[7:], true
}

// subsetTag is six uppercase letters drawn from a SHA-256 over the face name and its program.
func subsetTag(face string, prog []byte) string {
	h := sha256.New()
	h.Write([]byte(face))
	h.Write([]byte{0})
	h.Write(prog)
	sum := h.Sum(nil)
	var b strings.Builder
	for i := 0; i < 6; i++ {
		b.WriteByte('A' + sum[i]%26)
	}
	return b.String()
}

// fontProgramBytes is the DECODED embedded program a descriptor names, or nil where it names none or it
// cannot be read. Decoded, so the tag does not move with how a writer chose to compress it.
func fontProgramBytes(ctx *model.Context, d types.Dict) []byte {
	for _, k := range []string{"FontFile2", "FontFile3", "FontFile"} {
		o, ok := d[k]
		if !ok {
			continue
		}
		sd, _, err := ctx.DereferenceStreamDict(o)
		if err != nil || sd == nil {
			return nil
		}
		if sd.Content == nil {
			c := *sd // decode a copy: the stream in the table stays as it was read
			if err := c.Decode(); err != nil {
				return nil
			}
			return c.Content
		}
		return sd.Content
	}
	return nil
}
