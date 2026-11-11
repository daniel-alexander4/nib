package cli

import (
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"nib/internal/objedit"
	"nib/internal/pdfops"
)

// cmdObjects is `nib objects`: list the pictures a page draws, each with the address an edit names it by, and
// `nib objects remove` to take one off the page (PLAN-image-edit P01.S02). Both go through `internal/objedit`, the
// door the window's route uses, so a signed or ceremony document is refused here as there.
func cmdObjects(args []string) int {
	if len(args) > 0 && args[0] == "remove" {
		return cmdObjectsRemove(args[1:])
	}
	if len(args) > 0 && args[0] == "move" {
		return cmdObjectsMove(args[1:])
	}
	fs := flag.NewFlagSet("nib objects", flag.ContinueOnError)
	var page int
	var asJSON bool
	fs.IntVar(&page, "page", 0, "the page to list, from 1 (required)")
	fs.BoolVar(&asJSON, "json", false, "print the listing as JSON")
	fs.Usage = usageFunc(fs, "nib objects IN --page N [--json]  |  nib objects remove ...  |  nib objects move ...",
		"List the pictures page N draws, in drawing order, each with its number, its digest, where it is on the page\n"+
			"and whether Nib can remove it and move it (and why not). \"nib objects remove\" takes one off the page by\n"+
			"that number and digest; \"nib objects move\" places one in a new box. IN may be - (stdin).")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	if page < 1 {
		errf("missing --page (the page to list, from 1)")
		return 2
	}
	if fs.NArg() != 1 {
		errf("expected one input PDF, got %d", fs.NArg())
		return 1
	}
	pdf, err := readInput(fs.Arg(0))
	if err != nil {
		errf("%v", err)
		return 1
	}
	objs, err := objedit.List(pdf, page)
	if err != nil {
		errf("%s: %s", inputName(fs.Arg(0)), strings.TrimPrefix(err.Error(), "pdfops: "))
		return 1
	}
	if asJSON {
		return printTagJSON(objs)
	}
	if len(objs.Objects) == 0 {
		fmt.Printf("page %d draws no pictures\n", page)
	}
	for _, o := range objs.Objects {
		state := "removable"
		if !o.Editable {
			state = "not removable: " + pdfops.ObjectCauseWords(o.Why)
		}
		move := "movable"
		if !o.Movable {
			move = "not movable: " + pdfops.ObjectCauseWords(o.MoveWhy)
		}
		fmt.Printf("picture %d  %s  %s  [%.4f %.4f %.4f %.4f]  %s; %s\n", o.Ordinal+1, o.Kind, o.Digest,
			o.Box[0], o.Box[1], o.Box[2], o.Box[3], state, move)
	}
	if objs.FormsNotRead > 0 {
		fmt.Printf("%d repeated page element(s) could not be read; what they draw is not listed\n", objs.FormsNotRead)
	}
	return 0
}

// cmdObjectsRemove is `nib objects remove IN --page N --picture K --digest D -o OUT` (or -w IN). The digest is the
// precondition: a picture the file no longer draws at that number is refused, never another one removed.
func cmdObjectsRemove(args []string) int {
	fs := flag.NewFlagSet("nib objects remove", flag.ContinueOnError)
	var page, picture int
	var digest, out string
	var inPlace bool
	fs.IntVar(&page, "page", 0, "the page, from 1 (required)")
	fs.IntVar(&picture, "picture", 0, "the picture's number as \"nib objects\" lists it, from 1 (required)")
	fs.StringVar(&digest, "digest", "", "the picture's digest as \"nib objects\" lists it (required)")
	outFlag(fs, &out)
	inPlaceFlag(fs, &inPlace)
	fs.Usage = usageFunc(fs, "nib objects remove IN --page N --picture K --digest D -o OUT  |  ... -w IN",
		"Remove one picture from a page: the operator that draws it, and its tag when the tag holds nothing else.\n"+
			"The picture leaves the file unless another page or the form still uses it. A signed document, one in a signing\n"+
			"ceremony and a picture inside a repeated page element are refused.")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	if page < 1 || picture < 1 || digest == "" {
		errf("--page, --picture and --digest are all required (\"nib objects IN --page N\" lists them)")
		return 2
	}
	in, result, res, code := editObjects(fs, out, inPlace, pdfops.ObjectEdit{Kind: pdfops.ObjectEditDelete, Page: page, Ordinal: picture - 1, Digest: digest})
	if code != 0 {
		return code
	}
	for _, r := range res.Removed {
		if r.ElementRemoved {
			fmt.Fprintf(os.Stderr, "its tag went with it%s\n", altNote(r.Alt))
		}
		if r.LinksOver > 0 {
			fmt.Fprintf(os.Stderr, "%d link(s) over the picture are still on the page\n", r.LinksOver)
		}
		switch {
		case len(r.SharedWith) > 0:
			fmt.Fprintf(os.Stderr, "the picture stays in the file: page(s) %v still draw it\n", r.SharedWith)
		case r.StaysInFile:
			fmt.Fprintln(os.Stderr, "the picture stays in the file: the form still uses it")
		}
	}
	return writeEdited(in, out, inPlace, result)
}

// cmdObjectsMove is `nib objects move IN --page N --picture K --digest D --box L,T,R,B -o OUT` (or -w IN): the box is
// in the listing's space, fractions of the displayed page from its top-left corner (PLAN-image-edit P02.S01, D8).
func cmdObjectsMove(args []string) int {
	fs := flag.NewFlagSet("nib objects move", flag.ContinueOnError)
	var page, picture int
	var digest, box, out string
	var inPlace bool
	fs.IntVar(&page, "page", 0, "the page, from 1 (required)")
	fs.IntVar(&picture, "picture", 0, "the picture's number as \"nib objects\" lists it, from 1 (required)")
	fs.StringVar(&digest, "digest", "", "the picture's digest as \"nib objects\" lists it (required)")
	fs.StringVar(&box, "box", "", "the new box: left,top,right,bottom as fractions of the page, as the listing prints them (required)")
	outFlag(fs, &out)
	inPlaceFlag(fs, &inPlace)
	fs.Usage = usageFunc(fs, "nib objects move IN --page N --picture K --digest D --box L,T,R,B -o OUT  |  ... -w IN",
		"Place one picture in a new box, which moves and resizes it. Only a picture drawn straight and on its own\n"+
			"(one placement, nothing else drawn with it) is moved; its tag and description stay as they are. A signed\n"+
			"document, one in a signing ceremony and a picture inside a repeated page element are refused.")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	if page < 1 || picture < 1 || digest == "" || box == "" {
		errf("--page, --picture, --digest and --box are all required (\"nib objects IN --page N\" lists them)")
		return 2
	}
	var r pdfops.MapRect
	parts := strings.Split(box, ",")
	if len(parts) != 4 {
		errf("--box wants four numbers, left,top,right,bottom")
		return 2
	}
	for i, p := range parts {
		v, err := strconv.ParseFloat(strings.TrimSpace(p), 64)
		if err != nil {
			errf("--box: %q is not a number", p)
			return 2
		}
		r[i] = v
	}
	in, result, res, code := editObjects(fs, out, inPlace, pdfops.ObjectEdit{Kind: pdfops.ObjectEditMove, Page: page, Ordinal: picture - 1, Digest: digest, Box: &r})
	if code != 0 {
		return code
	}
	for _, m := range res.Moved {
		if m.LinksOver > 0 {
			fmt.Fprintf(os.Stderr, "%d link(s) over where the picture was are still there\n", m.LinksOver)
		}
	}
	return writeEdited(in, out, inPlace, result)
}

// editObjects reads the one input a picture edit names (IN, or the file -w rewrites) and applies e through the door.
// A non-zero code is the command's exit: 2 for a request the door could not read, 1 for every other refusal.
func editObjects(fs *flag.FlagSet, out string, inPlace bool, e pdfops.ObjectEdit) (string, []byte, pdfops.ObjectEditResult, int) {
	var in string
	if inPlace {
		if out != "" {
			errf("-o/--out cannot be combined with -w/--in-place")
			return "", nil, pdfops.ObjectEditResult{}, 1
		}
		if fs.NArg() != 1 || fs.Arg(0) == "-" {
			errf("-w rewrites one document in place — give exactly one file")
			return "", nil, pdfops.ObjectEditResult{}, 1
		}
		in = fs.Arg(0)
	} else {
		var code int
		if in, code = singleInput(fs, out); code != 0 {
			return "", nil, pdfops.ObjectEditResult{}, code
		}
	}
	pdf, err := readInput(in)
	if err != nil {
		errf("%v", err)
		return "", nil, pdfops.ObjectEditResult{}, 1
	}
	result, res, err := objedit.Edit(pdf, []pdfops.ObjectEdit{e})
	if err != nil {
		errf("%s: %s", inputName(in), strings.TrimPrefix(err.Error(), "pdfops: "))
		if ref, ok := objedit.Refusal(err); ok && ref.Cause == pdfops.CauseBadRequest {
			return "", nil, res, 2
		}
		return "", nil, res, 1
	}
	return in, result, res, 0
}

// writeEdited writes an edit's bytes back over in (-w) or to out.
func writeEdited(in, out string, inPlace bool, result []byte) int {
	if inPlace {
		if err := writeAtomic(in, result); err != nil {
			errf("%s: %v", in, err)
			return 1
		}
		fmt.Printf("%s: rewritten\n", in)
		return 0
	}
	return writeOut(out, result)
}

// altNote is the clause that names a removed tag's description, or nothing.
func altNote(alt string) string {
	if alt == "" {
		return ""
	}
	return fmt.Sprintf(", and its description (%q) too", alt)
}
