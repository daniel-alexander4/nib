package pdfops

// Signing flags ("place a flag, email it, the recipient fills it") are stored as
// a single custom Info-dict property, NibFlags, holding a base64-encoded JSON
// array of {page, frac, type} placeholders. The property travels inside the one
// PDF (no sidecar to lose in the mail) and survives a later watermark bake, so it
// must be stripped explicitly once the recipient has filled the flags. base64
// keeps the value pure ASCII, sidestepping PDF string-literal escaping.

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
