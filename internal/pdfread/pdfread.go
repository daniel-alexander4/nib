// Package pdfread is the one door through which nib hands a document's bytes to pdfcpu's validator, and the
// one door to pdfcpu's optimize pass (`optimize.go`). It sits below every package that reads a PDF through
// pdfcpu — `pdfops`, `uacheck`, `mdpdf`, `testpdf` — so a bound set here reaches all of them (ADR-009).
//
// # Why a door in front of the validator — `/pending 675`, `/pending 764`
//
// pdfcpu's validator recurses through a document's references and, on most chains, remembers nothing it has
// visited. A loop on one of those chains — two CMaps naming each other through `/UseCMap` (675), a tiling pattern
// whose `/Resources` names itself (764) — recurses until Go's stack limit, and **a Go stack overflow is fatal, not a
// panic**: no `recover()` catches it, and the whole nib process dies with every open document's unsaved work.
// Measured on a 2.5 KB file: ~5 s to die, through `pdfops.FlagsJSON` (which the server runs when it answers an
// Open, so the ordinary open killed nib), `PageCount`, `Validate`, and `uacheck.Check` alike. The same chains are
// walked once per PATH, so sharing without a loop hangs the read instead (`refgraph.go`). pdfcpu's UNVALIDATED read
// (`api.ReadContext`) follows none of them, so they are looked for there, in the raw object graph, and the
// document is refused before the validator is reached (`refuseUnboundedReferences`, ADR-069).
//
// `Validated` (and `ReadOptimized` over it) is `api.ReadAndValidate` with the check between its read and its
// validation — the same context, so it costs no second parse. pdfcpu's `api` functions that take a reader and do their
// own read are restated over it (`apiread.go`, ADR-082): nib cannot step between their read and their validation, nor
// budget the optimize pass most of them run, so no reader of a PDF is handed to pdfcpu at all.
//
// It is also the one door to a page's decoded content (`PageContent`, `pagecontent.go`, ADR-056): pdfcpu joins a
// `/Contents` array with no separator, which fuses tokens across a legal division.
//
// `TestEveryValidatingReadRoutesThroughTheDoor` (in this package) is the guard: outside this package no
// source file may call pdfcpu's validating reads or its optimize pass, or any `api` function that reads a document
// itself (except `api.ReadContext`, the unvalidated read, and a call passing nil where the document would go).
package pdfread

import (
	"bytes"
	"errors"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/fault"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// Validated is `api.ReadAndValidate` (pkg/api/api.go:171, v0.13.0) with the reference door between
// the read and the validation. Everything after the check is pdfcpu's own sequence, restated because the
// check must sit inside it.
func Validated(pdf []byte, conf *model.Configuration) (*model.Context, error) {
	return validated(pdf, conf, nil)
}

// validated is Validated with one more step a caller may put between the read and the validation: aside takes
// out of the context what pdfcpu's validator would refuse the whole document over and the caller can read for
// itself, and returns what puts it back. It runs for the validator only, exactly as `escapeInfoKeys` does — the
// context every reader sees afterwards is the document as parsed (`ReadOptimizedOrRefuseSettingAside`).
func validated(pdf []byte, conf *model.Configuration, aside func(*model.Context) (restore func())) (ctx *model.Context, err error) {
	defer fault.Catch(&err)
	if ctx, err = api.ReadContext(bytes.NewReader(pdf), conf); err != nil {
		return nil, err
	}
	if err := refuseUnboundedReferences(ctx); err != nil {
		return nil, err
	}
	restore := escapeInfoKeys(ctx)
	putPacket := setAsideOversizedPacket(ctx)
	putBack := func() {}
	if aside != nil {
		putBack = aside(ctx)
	}
	// What the document says about XFA is read HERE: the validator deletes an XFA-only form (`xfa.go`, ADR-145).
	xfa := xfaIn(ctx)
	// And each catalog name tree is walked HERE, and one nested deeper than the validator follows is made flat
	// for it: the validator drops such a tree and says nothing (`nametree.go`, ADR-164).
	trees := settleNameTrees(ctx)
	// And the structure tree's `/IDTree`, whose `/Limits` the validator empties past that depth (`/pending 883`).
	settleIDTree(ctx)
	// What the validator CHANGES rather than refuses is remembered, for the write to put back (`write.go`, ADR-129).
	err = api.ValidateContext(ctx)
	if err == nil {
		rememberValidatorLosses(ctx, pdf)
		rememberAssociatedFiles(ctx)
		rememberXFA(ctx, xfa)
		rememberNameTrees(ctx, trees)
	}
	putBack()
	putPacket()
	restore()
	if err != nil {
		return nil, err
	}
	if conf.Cmd == model.REMOVESIGNATURES || ctx.RemoveSignatures && conf.Cmd.AllowRemoveSignatures() {
		if len(ctx.Signatures) == 0 {
			if conf.Cmd == model.REMOVESIGNATURES {
				return nil, errors.New("pdfcpu: no signatures to remove")
			}
			return ctx, nil
		}
		if err := ctx.RemoveAllSignatures(); err != nil {
			return nil, err
		}
	}
	return ctx, nil
}

// MaxPacketBytes is the most a catalog `/Metadata` packet may decode to and still be READ — by pdfcpu's validator
// (`setAsideOversizedPacket`) and by nib's own packet readers, which decode within the same figure so the two
// agree on which packets are read. It is one page's form allowance in `pdfops` (`newFormWalkBudget(1)`), which is
// what that package's title read was already held to.
const MaxPacketBytes = 20 << 20

// setAsideOversizedPacket takes the catalog's `/Metadata` out of the validator's sight when it decodes to more than
// MaxPacketBytes, and returns what puts it back (`/pending 795`).
//
// pdfcpu's validator inflates the catalog packet whole and parses it as XML to learn a title and an author
// (`validate/metaData.go:79` and `:88`, `catalogMetaData`, v0.13.0) — with no limit but pdfcpu's own 512 MiB decode
// ceiling, and before any of nib's code has the context. Measured on a one-page file whose packet is flate over
// spaces, every read through this door: 1 / 20 / 64 / 200 MiB of packet allocated 6 / 128 / 384 / 1,024 MiB and
// took 0.04 / 0.46 / 1.4 / 7.2 s, from a file of 1 / 20 / 64 / 199 KB. With the packet set aside the last two read
// in 64 MiB and about 0.1 s.
//
// Set aside, not refused: the document is still read, and the validator sees what it sees for a document with no
// packet — exactly what it makes of a packet that is not XML in the relaxed mode nib reads in (`metadata parse
// error`, no title or author taken from it). The stream itself is untouched and is written back out as it was.
// A packet that does not decode at all is NOT set aside: the validator refuses that document, as it always has.
//
// The size is asked with the capped decode, on a COPY of the stream (`ctx.Dereference`, never
// `DereferenceStreamDict`, which marks the object validated before the validator has seen it — ADR-116), so a
// packet within the figure is inflated twice: once here, once by the validator (measured: the 1 MiB read 6 → 10 MiB,
// the 20 MiB one 128 → 192 MiB and 0.46 → 0.70 s; a packet of a few kilobytes, which is what documents carry, a few
// kilobytes more).
func setAsideOversizedPacket(ctx *model.Context) (restore func()) {
	restore = func() {}
	root, err := ctx.XRefTable.Catalog()
	if err != nil || root == nil {
		return restore // the validator reports a document with no catalog itself
	}
	held, ok := root["Metadata"]
	if !ok {
		return restore
	}
	o, err := ctx.Dereference(held)
	if err != nil {
		return restore
	}
	sd, ok := o.(types.StreamDict)
	if !ok {
		return restore
	}
	if !errors.Is(DecodeWithin(&sd, MaxPacketBytes), ErrDecodeLimit) {
		return restore
	}
	delete(root, "Metadata")
	return func() { root["Metadata"] = held }
}

// escapeInfoKeys re-escapes, for the validator only, every document-information key holding a `#`, and returns what
// puts the dictionary back as it was parsed (/pending 696).
//
// pdfcpu decodes a name's `#xx` escapes when it parses it, so the information dictionary's keys are already decoded;
// its validator then decodes each custom key AGAIN (`validate/info.go` `handleProperties`, v0.13.0) to fill
// `ctx.Properties`. A key whose decoded name holds a `#` — Acrobat PDFMaker writes SharePoint columns such as
// `/Document#20#23`, "Document #" — failed the whole read ("not enough characters after #", or "encoding/hex: invalid
// byte" for "Tags (option #1)"), and one that merely looked escaped ("A#41") was recorded as "AA". Encoding the key
// once more makes the second decode yield the name the document wrote. The keys go back after validation, because
// everything else that reads the dictionary — nib's readers and pdfcpu's writer — expects it as parsed.
func escapeInfoKeys(ctx *model.Context) (restore func()) {
	restore = func() {}
	if ctx.Info == nil {
		return restore
	}
	d, err := ctx.DereferenceDict(*ctx.Info)
	if err != nil || d == nil {
		return restore // the validator reports a malformed /Info itself
	}
	renamed := map[string]string{} // escaped key -> the key as parsed
	for k := range d {
		if !strings.Contains(k, "#") {
			continue
		}
		e := types.EncodeName(k)
		if _, taken := d[e]; taken {
			continue // the escaped spelling is a key of its own; leave both as the validator finds them
		}
		renamed[e] = k
	}
	for e, k := range renamed {
		d[e] = d[k]
		delete(d, k)
	}
	return func() {
		for e, k := range renamed {
			if v, ok := d[e]; ok {
				d[k] = v
				delete(d, e)
			}
		}
	}
}
