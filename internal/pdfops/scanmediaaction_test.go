package pdfops

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/testpdf"
)

// mediaActionDoc is one page carrying a URI link (object 5, which the gentle tier must keep) and whatever the case
// adds: `page` is extra page-dict keys, `catalog` extra catalog keys, `annots` extra /Annots entries.
func mediaActionDoc(catalog, page, annots string, extra map[int]string) []byte {
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R " + catalog + " >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Annots [5 0 R " + annots + "] " + page + " >>",
		5: "<< /Type /Annot /Subtype /Link /Rect [20 20 30 30] /A << /S /URI /URI (https://example.com/) >> >>",
	}
	for n, v := range extra {
		objs[n] = v
	}
	return testpdf.Assemble(objs)
}

// The objects a media action names. Each carries a marker the output is searched for: an object nothing names is
// not written, so a marker still present is something the removal left reachable.
var (
	screenAnnot = "<< /Type /Annot /Subtype /Screen /Rect [0 0 10 10] /P 3 0 R /T (SCREENMARK) >>"
	rendition   = "<< /Type /Rendition /S /MR /N (RENDITIONMARK) >>"
	soundStream = "<< /Type /Sound /R 8000 /Mark (SOUNDMARK) /Length 2 >>\nstream\n\x00\x00\nendstream"
)

// mediaActionCases is every place an action can be held, each holding one that plays or drives media. `marks` are
// the strings that must be gone from the output.
var mediaActionCases = map[string]struct {
	pdf   func() []byte
	marks []string
}{
	"a link's Rendition action names a Screen annotation and a rendition": {func() []byte {
		return mediaActionDoc("", "", "10 0 R 20 0 R", map[int]string{
			10: "<< /Type /Annot /Subtype /Link /Rect [40 40 50 50] /A 11 0 R >>",
			11: "<< /S /Rendition /OP 0 /AN 20 0 R /R 12 0 R >>", 12: rendition, 20: screenAnnot,
		})
	}, []string{"SCREENMARK", "RENDITIONMARK"}},
	"a Sound action behind a GoTo's /Next": {func() []byte {
		return mediaActionDoc("", "", "10 0 R", map[int]string{
			10: "<< /Type /Annot /Subtype /Link /Rect [40 40 50 50] /A << /S /GoTo /D [3 0 R /Fit] /Next 11 0 R >> >>",
			11: "<< /S /Sound /Sound 12 0 R >>", 12: soundStream,
		})
	}, []string{"SOUNDMARK"}},
	"a bookmark's Movie action": {func() []byte {
		return mediaActionDoc("/Outlines 30 0 R", "", "20 0 R", map[int]string{
			30: "<< /Type /Outlines /First 31 0 R /Last 31 0 R /Count 1 >>",
			31: "<< /Title (play) /Parent 30 0 R /A << /S /Movie /Annotation 20 0 R /Operation /Play >> >>",
			20: "<< /Type /Annot /Subtype /Movie /Rect [0 0 10 10] /P 3 0 R /T (MOVIEMARK) /Movie << /F (clip.avi) >> >>",
		})
	}, []string{"MOVIEMARK"}},
	"a GoTo3DView action on a link": {func() []byte {
		return mediaActionDoc("", "", "10 0 R 20 0 R", map[int]string{
			10: "<< /Type /Annot /Subtype /Link /Rect [40 40 50 50] /A << /S /GoTo3DView /TA 20 0 R /V /D >> >>",
			20: "<< /Type /Annot /Subtype /3D /Rect [0 0 10 10] /P 3 0 R /T (THREEDMARK) /3DD 21 0 R >>",
			21: "<< /Type /3D /Subtype /U3D /Length 3 >>\nstream\nU3D\nendstream",
		})
	}, []string{"THREEDMARK"}},
	"a RichMediaExecute action on a link": {func() []byte {
		return mediaActionDoc("", "", "10 0 R 20 0 R", map[int]string{
			10: "<< /Type /Annot /Subtype /Link /Rect [40 40 50 50] /A << /S /RichMediaExecute /TA 20 0 R /CMD << /C (play) >> >> >>",
			20: "<< /Type /Annot /Subtype /RichMedia /Rect [0 0 10 10] /P 3 0 R /T (RICHMARK)" +
				" /RichMediaContent << /Type /RichMediaContent >> >>",
		})
	}, []string{"RICHMARK"}},
	"a page's /AA entry": {func() []byte {
		return mediaActionDoc("", "/AA << /O << /S /Rendition /OP 0 /AN 20 0 R /R 12 0 R >> >>", "20 0 R", map[int]string{
			12: rendition, 20: screenAnnot,
		})
	}, []string{"SCREENMARK", "RENDITIONMARK"}},
	"an annotation's /AA entry": {func() []byte {
		return mediaActionDoc("", "", "10 0 R", map[int]string{
			10: "<< /Type /Annot /Subtype /Link /Rect [40 40 50 50] /AA << /E << /S /Sound /Sound 12 0 R >> >> >>",
			12: soundStream,
		})
	}, []string{"SOUNDMARK"}},
	"the catalog's /OpenAction": {func() []byte {
		return mediaActionDoc("/OpenAction << /S /Sound /Sound 12 0 R >>", "", "", map[int]string{12: soundStream})
	}, []string{"SOUNDMARK"}},
	"a form field's /A, on the parent of its widget": {func() []byte {
		return mediaActionDoc("/AcroForm << /Fields [40 0 R] >>", "", "41 0 R", map[int]string{
			40: "<< /FT /Btn /Ff 65536 /T (b) /Kids [41 0 R] /A << /S /Sound /Sound 12 0 R >> >>",
			41: "<< /Type /Annot /Subtype /Widget /Rect [60 60 80 80] /Parent 40 0 R >>",
			12: soundStream,
		})
	}, []string{"SOUNDMARK"}},
}

// eachObjectIn calls fn for every dictionary, array and string the file holds, wherever it sits — read object by
// object, not along the roads the removal walks, so a holder the removal forgot is still seen.
func eachObjectIn(t *testing.T, pdf []byte, fn func(types.Object)) {
	t.Helper()
	ctx, err := api.ReadContext(context.Background(), bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("re-reading the PDF: %v", err)
	}
	var look func(o types.Object, depth int)
	look = func(o types.Object, depth int) {
		if depth > 8 {
			return
		}
		fn(o)
		switch v := o.(type) {
		case types.Dict:
			for _, kid := range v {
				look(kid, depth+1)
			}
		case types.StreamDict:
			look(v.Dict, depth+1)
		case types.Array:
			for _, kid := range v {
				look(kid, depth+1)
			}
		}
	}
	for nr, e := range ctx.XRefTable.Table {
		if e == nil || e.Free {
			continue
		}
		if o, err := ctx.XRefTable.Dereference(*types.NewIndirectRef(nr, 0)); err == nil {
			look(o, 0)
		}
	}
}

// playingActions returns the type of every media action dictionary the file holds.
func playingActions(t *testing.T, pdf []byte) []string {
	t.Helper()
	var found []string
	eachObjectIn(t, pdf, func(o types.Object) {
		if d, ok := o.(types.Dict); ok && mediaActions[nameVal(d, "S")] {
			found = append(found, nameVal(d, "S"))
		}
	})
	return found
}

// writes reports whether the file holds the string mark in any object.
func writes(t *testing.T, pdf []byte, mark string) bool {
	t.Helper()
	found := false
	eachObjectIn(t, pdf, func(o types.Object) {
		switch v := o.(type) {
		case types.StringLiteral:
			found = found || strings.Contains(v.Value(), mark)
		case types.HexLiteral:
			found = found || strings.Contains(v.Value(), mark)
		}
	})
	return found
}

// TestRemovingMediaDropsTheActionsThatPlayIt — `/pending 865`: the gentle tier unlinked a media annotation from its
// page and left the action that plays it, and the action names the annotation (`/AN`, `/TA`, `/Annotation`) and the
// clip (`/R`, `/Sound`) — so both were still written out. Every holder of an action, in both tiers: no media action
// left, nothing it named left, the page count and the plain link as they were.
func TestRemovingMediaDropsTheActionsThatPlayIt(t *testing.T) {
	tiers := map[string]func([]byte) ([]byte, error){"RemoveFilesAndMedia": RemoveFilesAndMedia, "StripActive": StripActive}
	for name, c := range mediaActionCases {
		pdf := c.pdf()
		if len(playingActions(t, pdf)) == 0 {
			t.Fatalf("setup: %s carries no media action", name)
		}
		for _, mark := range c.marks {
			if !writes(t, pdf, mark) {
				t.Fatalf("setup: %s does not carry %s", name, mark)
			}
		}
		for tier, run := range tiers {
			out, err := run(pdf)
			if err != nil {
				t.Errorf("%s on %s: %v", tier, name, err)
				continue
			}
			if err := Validate(out); err != nil {
				t.Errorf("%s on %s: the result does not validate: %v", tier, name, err)
				continue
			}
			if left := playingActions(t, out); len(left) > 0 {
				t.Errorf("%s on %s left media-playing actions in the file: %v", tier, name, left)
			}
			for _, mark := range c.marks {
				if writes(t, out, mark) {
					t.Errorf("%s on %s: %s is still written — what the action named was not removed", tier, name, mark)
				}
			}
			if n, err := PageCount(out); err != nil || n != 1 {
				t.Errorf("%s on %s: %d pages (%v), want 1", tier, name, n, err)
			}
			_, actions := annotFacts(t, out)
			if kept := has(actions, "URI"); kept != (tier == "RemoveFilesAndMedia") {
				t.Errorf("%s on %s: the plain URI link's action kept=%v", tier, name, kept)
			}
		}
	}
}

// TestRemovingMediaKeepsTheRestOfAnAdditionalActions — the gentle tier leaves active code: an /AA loses the entry
// that plays and keeps the script beside it.
func TestRemovingMediaKeepsTheRestOfAnAdditionalActions(t *testing.T) {
	pdf := mediaActionDoc("", "/AA << /O << /S /Sound /Sound 12 0 R >> /C << /S /JavaScript /JS (KEPTSCRIPT) >> >>", "",
		map[int]string{12: soundStream})
	out, err := RemoveFilesAndMedia(pdf)
	if err != nil {
		t.Fatal(err)
	}
	if writes(t, out, "SOUNDMARK") {
		t.Error("the page's /AA /O sound is still written")
	}
	if !writes(t, out, "KEPTSCRIPT") {
		t.Error("the page's /AA /C script went with the sound — this tier leaves active code")
	}
}

// TestRemoveFilesAndMediaRefusesWhenAMediaActionRemains — the verifier sees a media action as residue of THIS tier,
// and no other action as its business.
func TestRemoveFilesAndMediaRefusesWhenAMediaActionRemains(t *testing.T) {
	rep := must(t, mediaActionCases["a Sound action behind a GoTo's /Next"].pdf())
	err := residue(rep, ErrFilesOrMediaRemain, filesOrMedia)
	if err == nil || !strings.Contains(err.Error(), "Plays embedded media") {
		t.Errorf("a report carrying a Sound action is not residue of the files-and-media removal: %v", err)
	}
	if err := residue(must(t, mediaActionDoc("", "", "", nil)), ErrFilesOrMediaRemain, filesOrMedia); err != nil {
		t.Errorf("a plain URI link is this tier's to keep, and it was called residue: %v", err)
	}
}

// TestEveryMediaActionIsARiskyAction — what makes StripActive at least as strong as the gentle tier, and what
// makes Scan report (and so the gentle tier's verifier see) every type that tier removes.
func TestEveryMediaActionIsARiskyAction(t *testing.T) {
	for s := range mediaActions {
		if _, ok := riskyActions[s]; !ok {
			t.Errorf("%s is removed by the files-and-media tier and is not a riskyActions type: Scan does not report it and StripActive keeps it", s)
		}
	}
}
