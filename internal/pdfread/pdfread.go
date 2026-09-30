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
// Two shapes, one check:
//   - `Validated` (and `ReadOptimized` over it) is `api.ReadAndValidate` with the check between its read and
//     its validation — the same context, so it costs no second parse.
//   - `Reader` is for pdfcpu's `api` functions that take a reader and do their own read: nib cannot step
//     between their read and their validation, so it reads once more, unvalidated, and hands the reader over
//     only when that read passes the check.
//
// It is also the one door to a page's decoded content (`PageContent`, `pagecontent.go`, ADR-056): pdfcpu joins a
// `/Contents` array with no separator, which fuses tokens across a legal division.
//
// `TestEveryValidatingReadRoutesThroughTheDoor` (in this package) is the guard: outside this package no
// source file may call pdfcpu's validating reads or its optimize pass, or hand a reader it built itself to a
// pdfcpu `api` call, except at the named not-a-PDF sites.
package pdfread

import (
	"bytes"
	"errors"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/fault"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// Validated is `api.ReadAndValidate` (pkg/api/api.go:171, v0.13.0) with the reference door between
// the read and the validation. Everything after the check is pdfcpu's own sequence, restated because the
// check must sit inside it.
func Validated(pdf []byte, conf *model.Configuration) (ctx *model.Context, err error) {
	defer fault.Catch(&err)
	if ctx, err = api.ReadContext(bytes.NewReader(pdf), conf); err != nil {
		return nil, err
	}
	if err := refuseUnboundedReferences(ctx); err != nil {
		return nil, err
	}
	if err := api.ValidateContext(ctx); err != nil {
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

// Reader returns a reader over pdf for a pdfcpu `api` function that reads and validates it itself, once an
// unvalidated read under the same configuration passes the reference door. conf may be nil where the `api`
// call's is (pdfcpu then uses its default). When the unvalidated read fails, the reader is returned anyway:
// the `api` call's own read is the same read and fails at the same step, before its validator runs, so the
// caller keeps the error pdfcpu has always given it.
func Reader(pdf []byte, conf *model.Configuration) (*bytes.Reader, error) {
	c := model.NewDefaultConfiguration()
	if conf != nil {
		cp := *conf // pdfcpu's read may write to the configuration it is given; the caller's is not ours
		c = &cp
	}
	ctx, err := api.ReadContext(bytes.NewReader(pdf), c)
	if err == nil {
		if err := refuseUnboundedReferences(ctx); err != nil {
			return nil, err
		}
	}
	return bytes.NewReader(pdf), nil
}
