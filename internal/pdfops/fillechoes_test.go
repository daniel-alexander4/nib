package pdfops

import (
	"fmt"
	"strings"
	"testing"

	"nib/internal/testpdf"
)

// echoMarker is a value no document or message holds on its own.
const echoMarker = "MARKER-9137-SSN"

// TestARefusedFillNeverEchoesTheValueItWasGiven — ADR-183. A return's SSN is a value the caller
// types into a field, and an error reaches stderr, a log and a terminal's scrollback. The field's
// name and the options the form itself offers may appear; the value may not. Every choice field
// kind refuses an unknown value here, so each is a stimulus that reaches the message. The check
// judges a value against the `options` the data itself carries (a fill laid over an export), so
// each datum names them; without them nothing is judged and nothing could be echoed.
func TestARefusedFillNeverEchoesTheValueItWasGiven(t *testing.T) {
	base, err := testpdf.Text("x")
	if err != nil {
		t.Fatal(err)
	}
	form, err := AuthorForm(base, []FormField{
		{Page: 1, Rect: [4]float64{20, 700, 120, 720}, Kind: "dropdown", Name: "filing", Options: []string{"single", "joint"}},
		{Page: 1, Rect: [4]float64{20, 650, 120, 670}, Kind: "radio", Name: "status", Options: []string{"a", "b"}},
		{Page: 1, Rect: [4]float64{20, 600, 40, 620}, Kind: "check", Name: "agree"},
		{Page: 1, Rect: [4]float64{20, 550, 120, 570}, Kind: "text", Name: "ssn"},
	})
	if err != nil {
		t.Fatal(err)
	}
	list := testpdf.Assemble(map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R /AcroForm << /Fields [4 0 R] /DA (/Helv 10 Tf 0 g) /DR << /Font << /Helv 3 0 R >> >> >> >>",
		2: "<< /Type /Pages /Kids [5 0 R] /Count 1 >>",
		3: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		4: "<< /Type /Annot /Subtype /Widget /FT /Ch /T (picks) /Opt [(red) (blue)] /Rect [20 20 120 80] /P 5 0 R >>",
		5: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Annots [4 0 R] >>",
	})
	cases := []struct {
		kind, name string
		pdf        []byte
		data       string
	}{
		{"combobox", "filing", form, `{"forms":[{"combobox":[{"name":"filing","options":["single","joint"],"value":"%s"}]}]}`},
		{"radiobuttongroup", "status", form, `{"forms":[{"radiobuttongroup":[{"name":"status","options":["a","b"],"value":"%s"}]}]}`},
		// A value of the wrong JSON type is refused by the decoder, whose messages name a type, not a value.
		{"checkbox", "agree", form, `{"forms":[{"checkbox":[{"name":"agree","value":"%s"}]}]}`},
		{"textfield", "ssn", form, `{"forms":[{"textfield":[{"name":"ssn","value":["%s"]}]}]}`},
		{"listbox", "picks", list, `{"forms":[{"listbox":[{"name":"picks","options":["red","blue"],"values":["%s"]}]}]}`},
	}
	for _, c := range cases {
		t.Run(c.kind, func(t *testing.T) {
			_, err := FillFormJSON(c.pdf, []byte(fmt.Sprintf(c.data, echoMarker)))
			if err == nil {
				t.Fatalf("a %s filled with a value it does not offer was accepted: the test reaches nothing", c.kind)
			}
			if c.kind == "listbox" || c.kind == "combobox" || c.kind == "radiobuttongroup" {
				if !strings.Contains(err.Error(), "unknown value") || !strings.Contains(err.Error(), c.name) {
					t.Fatalf("the refusal is %q, want it to name the field %q and say the value is unknown", err, c.name)
				}
			}
			if strings.Contains(err.Error(), echoMarker) {
				t.Errorf("the error repeats the value it was given: %q", err)
			}
		})
	}
}
