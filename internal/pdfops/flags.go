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
// `frac` is a fraction of the page, so a flag says WHERE on the page, never WHAT it was placed
// beside. Nothing in this tree cross-checks one against page content: the named grep
// `NibFlags|FlagsJSON|flagsKey` over `internal/` non-test returns this file and a slice-identity
// cache in `internal/server/server.go`, and `reconstructFlags` in the client clamps to `[0,1]`
// and to the page count and validates nothing else.
//
// **Measured**: after a rewrite that changed a page's text — `ContentDigest` moved —
// `FlagsJSON` returned a byte-identical flag set. It survives because `writeMutated` and
// `api.WriteContext` carry `ctx.Info` through unchanged. So a flag placed on "sign here" points
// at whatever now occupies that fraction of the page.
//
// **Live today at small scale**, on any `writeMutated` route reached with a flagged document
// open — `/api/ocr`, `/api/sanitize`, `/api/attachments/add`. Those rewrites move text by a
// little or not at all, so the flag usually still lands somewhere defensible, which is why this
// has not been seen.
//
// **It becomes load-bearing for `PLAN-text-reflow.md`'s P05**, whose whole job is to move the
// text a flag was placed beside. Making it DETECTABLE means carrying a content anchor beside the
// coordinate, and that is a change to a persisted, document-travelling format rather than a
// local fix — so it belongs with the phase that needs it, grilled there, not bolted on here.
// Recorded at the line meanwhile, because the hazard is invisible from the struct.

import (
	"bytes"
	"encoding/base64"
	"errors"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// flagsKey is the Info-dict property holding the encoded flag set.
const flagsKey = "NibFlags"

// errFlagsRoundTrip means the embedded flags didn't read back identically — the
// produced file would have reached the recipient without its placeholders.
var errFlagsRoundTrip = errors.New("flags did not round-trip after embedding")

// FlagsJSON returns the embedded flag set as raw JSON bytes, or nil if the PDF
// carries none. A malformed value reads as none rather than an error, so a
// hand-mangled property can never break opening a document.
func FlagsJSON(pdf []byte) ([]byte, error) {
	props, err := api.Properties(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		return nil, err
	}
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
	var out bytes.Buffer
	enc := base64.StdEncoding.EncodeToString(flagsJSON)
	if err := api.AddProperties(bytes.NewReader(pdf), &out, map[string]string{flagsKey: enc}, model.NewDefaultConfiguration()); err != nil {
		return nil, err
	}
	res := out.Bytes()
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
	var out bytes.Buffer
	if err := api.RemoveProperties(bytes.NewReader(pdf), &out, []string{flagsKey}, model.NewDefaultConfiguration()); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}
