package pdfread_test

import (
	"bytes"
	"errors"
	"fmt"
	"testing"
	"time"

	"nib/internal/pdfread"
	"nib/internal/testpdf"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// `/pending 675`: a `/UseCMap` loop recursed pdfcpu's validator into a fatal stack overflow — the nib process
// died, not the request. **On the code before this package these tests do not fail, they kill the test
// binary** (~5 s, "goroutine stack exceeds 1000000000-byte limit"), which is the defect itself.

// chainFrom follows `/UseCMap` references from nr in the RAW read — the stimulus, read without the door.
func chainFrom(t *testing.T, pdf []byte, nr, steps int) []int {
	t.Helper()
	ctx, err := api.ReadContext(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("setup: the unvalidated read failed: %v", err)
	}
	out := []int{nr}
	for len(out) <= steps {
		sd, _, err := ctx.DereferenceStreamDict(*types.NewIndirectRef(nr, 0))
		if err != nil || sd == nil {
			break
		}
		ir, ok := sd.Dict["UseCMap"].(types.IndirectRef)
		if !ok {
			break
		}
		nr = ir.ObjectNumber.Value()
		out = append(out, nr)
	}
	return out
}

func refusedWithin(t *testing.T, what string, f func() error) {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- f() }()
	select {
	case err := <-done:
		if !errors.Is(err, pdfread.ErrUseCMapCycle) {
			t.Errorf("%s returned %v, want the /UseCMap loop refused (pdfread.ErrUseCMapCycle)", what, err)
		}
	case <-time.After(10 * time.Second):
		t.Fatalf("%s had not returned after 10 s", what)
	}
}

func TestAUseCMapLoopIsRefusedBeforeTheValidator(t *testing.T) {
	for name, c := range testpdf.UseCMapLoops() {
		// Stimulus first: the raw object graph really loops, starting from the font's /Encoding.
		if got := chainFrom(t, c.PDF, 20, len(c.Loop)-1); fmt.Sprint(got) != fmt.Sprint(c.Loop) {
			t.Fatalf("setup: %s's /UseCMap chain from 20 is %v, want %v", name, got, c.Loop)
		}
		conf := model.NewDefaultConfiguration()
		refusedWithin(t, "Validated over "+name, func() error { _, err := pdfread.Validated(c.PDF, conf); return err })
		refusedWithin(t, "ReadOptimized over "+name, func() error { _, err := pdfread.ReadOptimized(c.PDF, conf); return err })
		refusedWithin(t, "Reader over "+name, func() error { _, err := pdfread.Reader(c.PDF, nil); return err })
	}
}

// A chain that ENDS — a CMap using another that uses a predefined one — is what /UseCMap is for, and must read.
func TestAUseCMapChainThatEndsStillReads(t *testing.T) {
	pdf := testpdf.UseCMapChain()
	if got := chainFrom(t, pdf, 20, 5); fmt.Sprint(got) != "[20 21]" {
		t.Fatalf("setup: the chain from 20 is %v, want [20 21] (21 names a predefined CMap)", got)
	}
	if _, err := pdfread.Validated(pdf, model.NewDefaultConfiguration()); err != nil {
		t.Errorf("Validated refused a /UseCMap chain with no loop: %v", err)
	}
	rs, err := pdfread.Reader(pdf, nil)
	if err != nil {
		t.Fatalf("Reader refused a /UseCMap chain with no loop: %v", err)
	}
	if n, err := api.PageCount(rs, model.NewDefaultConfiguration()); err != nil || n != 1 {
		t.Errorf("pdfcpu over Reader's reader: %d pages, %v; want 1", n, err)
	}
}
