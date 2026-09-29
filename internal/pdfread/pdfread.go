// Package pdfread is the one door through which nib hands a document's bytes to pdfcpu's validator, and the
// one door to pdfcpu's optimize pass (`optimize.go`). It sits below every package that reads a PDF through
// pdfcpu — `pdfops`, `uacheck`, `mdpdf`, `testpdf` — so a bound set here reaches all of them (ADR-009).
//
// # Why a door in front of the validator — `/pending 675`
//
// pdfcpu's validator follows a CMap's `/UseCMap` by recursion and remembers nothing it has visited
// (validate/font.go, v0.13.0: `validateCMapStreamDict` → `validateUseCMapEntry` → `validateCMapStreamDict`).
// Two embedded CMaps naming each other — or one naming itself — therefore recurse until Go's stack limit, and
// **a Go stack overflow is fatal, not a panic**: no `recover()` catches it, and the whole nib process dies with
// every open document's unsaved work. Measured on a 2.5 KB file: ~5 s to die, through `pdfops.FlagsJSON`
// (which the server runs when it answers an Open, so the ordinary open killed nib), `PageCount`, `Validate`,
// and `uacheck.Check` alike. pdfcpu's UNVALIDATED read (`api.ReadContext`) does not follow the reference and
// returns in well under a millisecond, so the cycle is looked for there, in the raw object graph, and the
// document is refused before the validator is reached.
//
// Two shapes, one check:
//   - `Validated` (and `ReadOptimized` over it) is `api.ReadAndValidate` with the check between its read and
//     its validation — the same context, so it costs no second parse.
//   - `Reader` is for pdfcpu's `api` functions that take a reader and do their own read: nib cannot step
//     between their read and their validation, so it reads once more, unvalidated, and hands the reader over
//     only when that read shows no cycle.
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
	"fmt"
	"sort"
	"strings"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/fault"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// ErrUseCMapCycle is the refusal of a document whose embedded CMaps name each other through `/UseCMap`.
var ErrUseCMapCycle = errors.New("its embedded CMaps name each other in a loop through /UseCMap")

// Validated is `api.ReadAndValidate` (pkg/api/api.go:171, v0.13.0) with the `/UseCMap` cycle refused between
// the read and the validation. Everything after the check is pdfcpu's own sequence, restated because the
// check must sit inside it.
func Validated(pdf []byte, conf *model.Configuration) (ctx *model.Context, err error) {
	defer fault.Catch(&err)
	if ctx, err = api.ReadContext(bytes.NewReader(pdf), conf); err != nil {
		return nil, err
	}
	if err := refuseUseCMapCycle(ctx); err != nil {
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
// unvalidated read under the same configuration shows no `/UseCMap` cycle. conf may be nil where the `api`
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
		if err := refuseUseCMapCycle(ctx); err != nil {
			return nil, err
		}
	}
	return bytes.NewReader(pdf), nil
}

// refuseUseCMapCycle refuses ctx when any stream's `/UseCMap` chain returns to a stream already on it.
//
// **Every stream is examined, not only those a font reaches**, because pdfcpu's validator reaches a CMap from
// wherever a font sits — pages, forms, annotation appearances, Type 3 glyph resources — and restating that
// reachability here would be a second walker to keep in step with pdfcpu's. The cost is a refusal of a document
// whose looping CMaps nothing uses; such a file is malformed either way. Each stream is followed once
// (`done`), so the check is linear in the object count.
func refuseUseCMapCycle(ctx *model.Context) error {
	nrs := make([]int, 0, len(ctx.Table))
	for nr, e := range ctx.Table {
		if e != nil && !e.Free {
			nrs = append(nrs, nr)
		}
	}
	sort.Ints(nrs)
	done := map[int]bool{}
	for _, start := range nrs {
		if done[start] {
			continue
		}
		var path []int
		onPath := map[int]bool{}
		for nr := start; ; {
			if onPath[nr] {
				loop := path[indexOf(path, nr):]
				return fmt.Errorf("nib will not read this document: %w (objects %s → %d), and pdfcpu's "+
					"validator would follow that loop until nib ran out of stack", ErrUseCMapCycle, joinInts(loop), nr)
			}
			if done[nr] {
				break
			}
			onPath[nr] = true
			path = append(path, nr)
			next, ok := useCMapTarget(ctx, nr)
			if !ok {
				break
			}
			nr = next
		}
		for _, nr := range path {
			done[nr] = true
		}
	}
	return nil
}

// useCMapTarget is the object number stream nr's `/UseCMap` names, when nr is a stream naming one by
// reference — the only form pdfcpu's validator recurses on (a name is a predefined CMap, and anything else
// it rejects).
func useCMapTarget(ctx *model.Context, nr int) (int, bool) {
	e, found := ctx.FindTableEntryLight(nr)
	if !found || e == nil || e.Free {
		return 0, false
	}
	sd, ok := e.Object.(types.StreamDict)
	if !ok {
		return 0, false
	}
	ir, ok := sd.Dict["UseCMap"].(types.IndirectRef)
	if !ok {
		return 0, false
	}
	return ir.ObjectNumber.Value(), true
}

func indexOf(s []int, v int) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return 0
}

func joinInts(s []int) string {
	parts := make([]string, len(s))
	for i, v := range s {
		parts[i] = fmt.Sprint(v)
	}
	return strings.Join(parts, " → ")
}
