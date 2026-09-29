package pdfops

import (
	"errors"
	"testing"
)

func firstParagraphBox(t *testing.T, pdf []byte) [4]float64 {
	t.Helper()
	l, _ := layoutOf(t, pdf)
	return paragraphBox(l.paragraphs[0])
}

// TestTheReflowDoor — P06.S05's server-facing door: it acts only on the paragraph it was shown (ADR-001), writes nothing for the paragraph's own text (law 1), and keeps everything it draws inside the
// paragraph's own box — the NibFlags decision for this phase (/pending 457).
func TestTheReflowDoor(t *testing.T) {
	pdf := helveticaPage(reflowPara + reflowTail)
	paras, err := Paragraphs(pdf, 1)
	if err != nil || len(paras) != 2 || paras[0].Refusal != "" {
		t.Fatalf("Paragraphs: %+v, %v", paras, err)
	}
	orig := paras[0].Text
	edit := "The quick brown fox jumps over the sleepy old dog and runs away from here."

	if _, _, err := ReflowParagraph(pdf, 1, 0, "some other paragraph's text", edit); !errors.Is(err, ErrReflowStale) {
		t.Errorf("a paragraph named by text it does not read as was reflowed anyway: %v", err)
	}
	if _, _, err := ReflowParagraph(pdf, 1, 7, orig, edit); !errors.Is(err, ErrReflowStale) {
		t.Errorf("an index past the page's paragraphs: %v", err)
	}
	if out, cause, err := ReflowParagraph(pdf, 1, 0, orig, orig); out != nil || cause != "" || err != nil {
		t.Errorf("the paragraph's own text wrote %d bytes (cause %q, %v)", len(out), cause, err)
	}
	out, cause, err := ReflowParagraph(pdf, 1, 0, orig, edit)
	if err != nil || cause != "" || out == nil {
		t.Fatalf("the edit: cause %q, %v", cause, err)
	}
	after, err := Paragraphs(out, 1)
	if err != nil || after[0].Text != edit {
		t.Fatalf("the rewritten page reads %+v (%v)", after, err)
	}
	b, a := firstParagraphBox(t, pdf), firstParagraphBox(t, out)
	const eps = 1e-6
	if a[0] < b[0]-eps || a[1] < b[1]-eps || a[2] > b[2]+eps || a[3] > b[3]+eps {
		t.Errorf("the rewritten paragraph occupies %v, outside its own box %v — a flag beside it could now sit on it", a, b)
	}

}
