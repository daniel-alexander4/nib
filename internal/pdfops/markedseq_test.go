package pdfops

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// streamObject is a stream object's text for assembleFixture, its /Length measured.
func streamObject(dict, body string) string {
	return fmt.Sprintf("<< %s /Length %d >>\nstream\n%s\nendstream", dict, len(body), body)
}

// TestTheRunReaderRecordsEachMarkedSequence — P09.S03's input: every sequence carrying an MCID, whether
// the MCID is inline or named in `/Properties`, with the bytes that open and close it, and whether it is
// inside a form or draws one. Artifact sequences carry no MCID and are not recorded.
func TestTheRunReaderRecordsEachMarkedSequence(t *testing.T) {
	page := "/P <</MCID 0>> BDC 0 0 10 10 re f EMC\n" +
		"/Artifact BMC 1 1 2 2 re f EMC\n" +
		"/Artifact <</MCID 9>> BDC 1 1 2 2 re f EMC\n" +
		"/Span /MC1 BDC /Fm0 Do EMC\n" +
		"/P <</MCID 2>> BDC 0 0 1 1 re f"
	// The form leaves its sequence OPEN: a stream's sequences end with the stream, so the page's `EMC` after
	// the `Do` closes the page's own sequence (MCID 1) and not the form's.
	form := "/Figure <</MCID 3>> BDC 0 0 1 1 re f"
	src := assembleFixture(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Properties << /MC1 << /MCID 1 >> >> /XObject << /Fm0 5 0 R >> >> /Contents 4 0 R >>",
		4: streamObject("", page),
		5: streamObject("/Type /XObject /Subtype /Form /BBox [0 0 10 10]", form),
	})
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(src), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	d, _, _, _ := ctx.PageDict(1, false)
	content, err := ctx.PageContent(d, 1)
	if err != nil {
		t.Fatal(err)
	}
	pr, err := readPageRuns(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	want := []struct {
		mcid                      int
		opener                    string
		closed, inForm, drawsForm bool
	}{
		{0, "/P <</MCID 0>> BDC", true, false, false},
		{1, "/Span /MC1 BDC", true, false, true},
		{3, "/Figure <</MCID 3>> BDC", false, true, false},
		{2, "/P <</MCID 2>> BDC", false, false, false},
	}
	if len(pr.sequences) != len(want) {
		t.Fatalf("%d sequence(s) recorded: %+v — want MCIDs 0, 1, 3, 2; the artifacts carry none", len(pr.sequences), pr.sequences)
	}
	for i, w := range want {
		s := pr.sequences[i]
		stream := content
		if w.inForm {
			stream = []byte(form)
		}
		if s.mcid != w.mcid || s.inForm != w.inForm || s.drawsForm != w.drawsForm {
			t.Errorf("sequence %d reads MCID %d inForm %v drawsForm %v, want %d %v %v", i, s.mcid, s.inForm, s.drawsForm, w.mcid, w.inForm, w.drawsForm)
			continue
		}
		if got := string(stream[s.opener.start:s.opener.end]); got != w.opener {
			t.Errorf("sequence %d's opener covers %q, want %q", i, got, w.opener)
		}
		closed := s.close != (opSpan{})
		if closed != w.closed {
			t.Errorf("sequence %d closed %v, want %v", i, closed, w.closed)
		} else if closed {
			if got := string(stream[s.close.start:s.close.end]); got != "EMC" {
				t.Errorf("sequence %d's close covers %q", i, got)
			}
		}
	}
}
