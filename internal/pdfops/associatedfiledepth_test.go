package pdfops

import (
	"errors"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// TestAnAssociatedFileTooDeepToFindIsRefusedNotSkipped — P02 close F01. dropAssociatedFileKeys used to stop at a depth
// and return quietly, and its caller then cleared the read mark that makes the write refuse an `/AF`: a file the user
// removed stayed in the document. The stimulus is asserted first (the deep `/AF` really is still there, and the
// shallow one really is stripped), so a green here is not about a document the walk never reached.
func TestAnAssociatedFileTooDeepToFindIsRefusedNotSkipped(t *testing.T) {
	inner := types.Dict{"AF": types.Array{}}
	var deep types.Object = inner
	for i := 0; i < maxAssociatedFileDepth+5; i++ {
		deep = types.Dict{"K": deep}
	}
	shallow := types.Dict{"K": types.Dict{"AF": types.Array{}}}
	xt := &model.XRefTable{Table: map[int]*model.XRefTableEntry{
		1: {Object: deep},
		2: {Object: shallow},
	}}

	err := dropAssociatedFileKeys(xt)

	if _, still := inner["AF"]; !still {
		t.Fatal("setup: the deep /AF was stripped, so the walk reached it and this test is about nothing")
	}
	if _, still := shallow["K"].(types.Dict)["AF"]; still {
		t.Fatal("setup: the shallow /AF was not stripped — the walk is not running")
	}
	if !errors.Is(err, errAssociatedFileTooDeep) {
		t.Errorf("a /AF nested past %d levels was skipped without a refusal (err = %v): the removal would report success with the file still in the document",
			maxAssociatedFileDepth, err)
	}
}
