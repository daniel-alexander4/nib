package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"

	"nib/internal/pdfops"
	"nib/internal/sign"
)

// cmdFlatten burns a form's widgets into the page and removes the form (ADR-183, RelaxTax Group B1). The page's own
// text is untouched and a filled value stays text: this is the vector alternative to the window's Flatten, which
// rasterises each page.
func cmdFlatten(args []string) int {
	fs := flag.NewFlagSet("nib flatten", flag.ContinueOnError)
	var out string
	var dropSig, asJSON bool
	outFlag(fs, &out)
	fs.BoolVar(&dropSig, "drop-signature", false, "flatten a signed document anyway, taking its signature(s) away (refused without it: the form's content is what a signature covers)")
	fs.BoolVar(&asJSON, "json", false, "print the counts as JSON on stderr: {\"flattened\",\"blank\",\"hidden\"} (stdout carries the PDF with -o -)")
	fs.Usage = usageFunc(fs, "nib flatten IN -o OUT [--drop-signature] [--json]",
		"Draw every form widget's appearance into its page as vector content and remove the widgets and the form. The page's text stays text and a filled value is found by nib text.\n"+
			"A widget with a value but no appearance refuses the whole call (fill it with nib first); a widget with neither is blank; a hidden one is removed undrawn.\n"+
			"IN may be - and -o - writes the PDF to stdout. The drawn content is decoration: a screen reader does not announce a flattened value.")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	in, code := singleInput(fs, out)
	if code != 0 {
		return code
	}
	var res pdfops.FlattenResult
	rc := transform(in, out, func(b []byte) ([]byte, error) {
		if !dropSig && sign.HasSignatureBlob(b) {
			return nil, errors.New("this document is signed, and flattening its form would change the bytes its signatures cover — flatten before it is signed, or pass --drop-signature to take the signature away")
		}
		flat, r, err := pdfops.FlattenForm(b, dropSig)
		res = r
		return flat, err
	})
	if rc != 0 {
		return rc
	}
	if asJSON {
		j, _ := json.Marshal(res)
		fmt.Fprintln(os.Stderr, string(j))
	} else {
		errf("flattened %d widget(s); %d blank, %d hidden", res.Flattened, res.Blank, res.Hidden)
	}
	return 0
}
