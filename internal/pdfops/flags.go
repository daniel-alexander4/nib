package pdfops

// Signing flags ("place a flag, email it, the recipient fills it") are stored as
// a single custom Info-dict property, NibFlags, holding a base64-encoded JSON
// array of {page, frac, type} placeholders. The property travels inside the one
// PDF (no sidecar to lose in the mail) and survives a later watermark bake, so it
// must be stripped explicitly once the recipient has filled the flags. base64
// keeps the value pure ASCII, sidestepping PDF string-literal escaping.
//
// # A flag is a PAGE COORDINATE and is not anchored to content (/pending 457)
//
// `frac` is a fraction of the page as the viewer shows it, so a flag says WHERE on the page, never WHAT it was
// placed beside. Nothing cross-checks one against page content: `reconstructFlags` in the client clamps to
// `[0,1]` and to the page count and validates nothing else.
//
// It survives every rewrite because `writeMutated` and `api.WriteContext` carry `ctx.Info` through unchanged,
// so a flag placed on "sign here" points at whatever occupies that fraction of the page afterwards. That is
// correct exactly when the rewrite moves nothing the page draws, or moves the flag with what it moves.
//
// # No anchor, because every rewrite that reaches a flag keeps its place (/pending 457)
//
// An anchor beside the coordinate would make a moved flag DETECTABLE, at the price of a format change to the
// one persisted, document-travelling coordinate nib has. It is not built, because nothing that rewrites a
// flagged document moves what a flag sits beside — and that is held, not assumed:
//
//   - Measured 2026-10-03: the OCR layer, all three sanitize methods and an added attachment, over twelve
//     documents (two government fillable forms, LaTeX, a scan, a tagged conversion), kept the boxes
//     and rotation of the first three pages and rendered them pixel-identical at 40 dpi, annotations shown
//     and hidden (StripMetadata drops the flag with the rest of the Info dictionary); a crop, a turn
//     and a visible stamp, run as controls, all differed.
//   - The same census then found the one shape that DID move: a turned page whose box does not start at the
//     origin. pdfcpu folds `/Rotate` into the content to stamp it and turned the drawing about the origin, so
//     the OCR layer — and every bake, watermark and page number — carried the whole page out of its box. That
//     is repaired at the stamp door (`stampInPlace`), not here: a flag was the least of what it moved.
//   - `internal/server`'s TestEveryRewriteOfAHeldDocumentIsClassifiedForItsFlags requires every route that
//     commits a rewrite to say how a flag keeps its place (keeps geometry, carries the flag through the reflow
//     door in anchormove.go, never receives one, or installs the file's own), and
//     TestEveryRewriteOfAHeldDocumentKeepsWhatAFlagWasPlacedBeside drives each keeps-geometry rewrite over
//     flagged documents, a turned and cropped page among them.
//
// A rewrite that moves drawn content on one of those routes turns the census red, and that is when the anchor
// is owed: an optional field, an old blob without it read as "unknown" (never invalid), every writer and the
// round-trip guard updated, and an ADR, because it changes a blob that travels in documents already sent.

import (
	"bytes"
	"encoding/base64"
	"errors"
	"nib/internal/pdfread"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// propertyConf is the configuration `api.AddProperties` / `api.RemoveProperties` read with when handed
// one, as nib always did: relaxed validation, and the command named.
func propertyConf(cmd model.CommandMode) *model.Configuration {
	conf := model.NewDefaultConfiguration()
	conf.ValidationMode = model.ValidationRelaxed
	conf.Cmd = cmd
	return conf
}

// flagsKey is the Info-dict property holding the encoded flag set.
const flagsKey = "NibFlags"

// errFlagsRoundTrip means the embedded flags didn't read back identically — the
// produced file would have reached the recipient without its placeholders.
var errFlagsRoundTrip = errors.New("flags did not round-trip after embedding")

// FlagsJSON returns the embedded flag set as raw JSON bytes, or nil if the PDF
// carries none. A malformed value reads as none rather than an error, so a
// hand-mangled property can never break opening a document.
func FlagsJSON(pdf []byte) ([]byte, error) {
	// `api.Properties` restated over the read door (pkg/api/property.go:29, v0.13.0: `LISTPROPERTIES`, then the
	// context's properties), because the server runs this on every document it answers for: `pdfread.Reader`
	// in front of the wrapper would parse each file twice (measured +30-85% here on 100 KB-6 MB producer
	// files), where the door checks for reference loops on the read it already makes (`/pending 675`, `/pending 764`).
	conf := model.NewDefaultConfiguration()
	conf.Cmd = model.LISTPROPERTIES
	ctx, err := pdfread.ReadOptimized(pdf, conf)
	if err != nil {
		return nil, err
	}
	props := ctx.Properties
	enc, ok := props[flagsKey]
	if !ok || enc == "" {
		return nil, nil
	}
	raw, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		return nil, nil // unreadable => treat as no flags
	}
	return raw, nil
}

// SetFlags embeds the given flag-set JSON as the NibFlags property and returns
// the new PDF bytes. It reads the property back to confirm the produced file
// actually carries the flags before handing it to the caller — the emailed
// document is worthless if the placeholders silently failed to embed.
func SetFlags(pdf, flagsJSON []byte) ([]byte, error) {
	enc := base64.StdEncoding.EncodeToString(flagsJSON)
	// `api.AddProperties` also validates keys and values for characters a PDF string cannot hold; the key
	// is a constant and the value base64, so there is nothing for that check to find here.
	res, err := rewriteWithConf(pdf, propertyConf(model.ADDPROPERTIES), func(ctx *model.Context) error {
		return pdfcpu.PropertiesAdd(ctx, map[string]string{flagsKey: enc})
	})
	if err != nil {
		return nil, err
	}
	if got, err := FlagsJSON(res); err != nil || !bytes.Equal(got, flagsJSON) {
		return nil, errFlagsRoundTrip
	}
	return res, nil
}

// ClearFlags removes the NibFlags property. It is a no-op (returns the input
// unchanged) when the PDF carries no flags, so it's safe to call on any save.
func ClearFlags(pdf []byte) ([]byte, error) {
	if raw, err := FlagsJSON(pdf); err != nil || len(raw) == 0 {
		return pdf, err
	}
	return rewriteWithConf(pdf, propertyConf(model.REMOVEPROPERTIES), func(ctx *model.Context) error {
		ok, err := pdfcpu.PropertiesRemove(ctx, []string{flagsKey})
		if err != nil {
			return err
		}
		if !ok {
			return errors.New("no property removed") // api.RemoveProperties' own refusal
		}
		return nil
	})
}
