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
	if len(args) > 0 && args[0] == "replace" {
		return cmdObjectsReplace(args[1:])
	}
	fs := flag.NewFlagSet("nib objects", flag.ContinueOnError)
	var page int
	var asJSON bool
	fs.IntVar(&page, "page", 0, "the page to list, from 1 (required)")
	fs.BoolVar(&asJSON, "json", false, "print the listing as JSON")
	fs.Usage = usageFunc(fs, "nib objects IN --page N [--json]  |  nib objects remove ...  |  nib objects move ...  |  nib objects replace ...",
		"List the pictures page N draws, in drawing order, each with its number, its digest, where it is on the page\n"+
			"and whether Nib can remove it and move it (and why not); then its drawings — lines and shapes whose outlines\n"+
			"touch, each one object — numbered on their own. \"nib objects remove\" takes one off the page by that number\n"+
			"and digest; \"nib objects move\" places one in a new box; \"nib objects replace\" puts another picture in\n"+
			"a picture's place. IN may be - (stdin).")
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
	if len(objs.Objects) == 0 && len(objs.Drawings) == 0 {
		fmt.Printf("page %d draws no pictures and no drawings\n", page)
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
	for _, o := range objs.Drawings {
		state := "removable"
		if !o.Editable {
			state = "not removable: " + pdfops.ObjectCauseWords(o.Why)
		}
		move := "movable"
		if !o.Movable {
			move = "not movable: " + pdfops.ObjectCauseWords(o.MoveWhy)
		}
		fmt.Printf("drawing %d  %s  %s  [%.4f %.4f %.4f %.4f]  %s; %s\n", o.Ordinal+1, o.Kind, o.Digest,
			o.Box[0], o.Box[1], o.Box[2], o.Box[3], state, move)
	}
	if objs.DrawingsUngrouped {
		fmt.Println("the page draws too many overlapping lines and shapes for Nib to group them; no drawing is listed")
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
	var page, picture, drawing int
	var digest, out string
	var inPlace bool
	fs.IntVar(&page, "page", 0, "the page, from 1 (required)")
	objectFlags(fs, &picture, &drawing)
	fs.StringVar(&digest, "digest", "", "its digest as \"nib objects\" lists it (required)")
	outFlag(fs, &out)
	inPlaceFlag(fs, &inPlace)
	fs.Usage = usageFunc(fs, "nib objects remove IN --page N (--picture K | --drawing K) --digest D -o OUT  |  ... -w IN",
		"Remove one picture or drawing from a page: the operators that draw it, and a picture's tag when the tag holds\n"+
			"nothing else. A picture leaves the file unless another page or the form still uses it. A signed document, one in\n"+
			"a signing ceremony and anything inside a repeated page element are refused, and so is a tagged drawing.")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	object, ordinal, ok := objectChosen(picture, drawing)
	if page < 1 || !ok || digest == "" {
		errf("--page, one of --picture or --drawing, and --digest are all required (\"nib objects IN --page N\" lists them)")
		return 2
	}
	in, result, res, code := editObjects(fs, out, inPlace, pdfops.ObjectEdit{Kind: pdfops.ObjectEditDelete, Object: object, Page: page, Ordinal: ordinal, Digest: digest})
	if code != 0 {
		return code
	}
	for _, r := range res.Removed {
		if r.ElementRemoved {
			fmt.Fprintf(os.Stderr, "its tag went with it%s\n", altNote(r.Alt))
		}
		if r.LinksOver > 0 {
			fmt.Fprintf(os.Stderr, "%d link(s) over it are still on the page\n", r.LinksOver)
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

// objectFlags declares --picture and --drawing: an object is named by one of them, never both (PLAN-image-edit P03.S03).
func objectFlags(fs *flag.FlagSet, picture, drawing *int) {
	fs.IntVar(picture, "picture", 0, "the picture's number as \"nib objects\" lists it, from 1")
	fs.IntVar(drawing, "drawing", 0, "the drawing's number as \"nib objects\" lists it, from 1")
}

// objectChosen is the object and ordinal --picture or --drawing named, and false unless exactly one did.
func objectChosen(picture, drawing int) (string, int, bool) {
	switch {
	case picture >= 1 && drawing == 0:
		return pdfops.ObjectPicture, picture - 1, true
	case drawing >= 1 && picture == 0:
		return pdfops.ObjectDrawing, drawing - 1, true
	}
	return "", 0, false
}

// cmdObjectsMove is `nib objects move IN --page N --picture K --digest D --box L,T,R,B -o OUT` (or -w IN): the box is
// in the listing's space, fractions of the displayed page from its top-left corner (PLAN-image-edit P02.S01, D8).
func cmdObjectsMove(args []string) int {
	fs := flag.NewFlagSet("nib objects move", flag.ContinueOnError)
	var page, picture, drawing int
	var digest, box, out string
	var inPlace bool
	fs.IntVar(&page, "page", 0, "the page, from 1 (required)")
	objectFlags(fs, &picture, &drawing)
	fs.StringVar(&digest, "digest", "", "its digest as \"nib objects\" lists it (required)")
	fs.StringVar(&box, "box", "", "the new box: left,top,right,bottom as fractions of the page, as the listing prints them (required)")
	outFlag(fs, &out)
	inPlaceFlag(fs, &inPlace)
	fs.Usage = usageFunc(fs, "nib objects move IN --page N (--picture K | --drawing K) --digest D --box L,T,R,B -o OUT  |  ... -w IN",
		"Place one picture in a new box, which moves and resizes it, or move one drawing to a box of its own size.\n"+
			"Only a picture drawn straight and on its own (one placement, nothing else drawn with it) is moved; its tag and\n"+
			"description stay as they are. A drawing is moved, never resized. A signed document, one in a signing ceremony\n"+
			"and anything inside a repeated page element are refused.")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	object, ordinal, ok := objectChosen(picture, drawing)
	if page < 1 || !ok || digest == "" || box == "" {
		errf("--page, one of --picture or --drawing, --digest and --box are all required (\"nib objects IN --page N\" lists them)")
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
	in, result, res, code := editObjects(fs, out, inPlace, pdfops.ObjectEdit{Kind: pdfops.ObjectEditMove, Object: object, Page: page, Ordinal: ordinal, Digest: digest, Box: &r})
	if code != 0 {
		return code
	}
	for _, m := range res.Moved {
		if m.LinksOver > 0 {
			fmt.Fprintf(os.Stderr, "%d link(s) over where it was are still there\n", m.LinksOver)
		}
	}
	return writeEdited(in, out, inPlace, result)
}

// cmdObjectsReplace is `nib objects replace IN --page N --picture K --digest D --with FILE [--alt TEXT | --keep-alt]
// -o OUT` (or -w IN): FILE is imported as the window's upload is (`pdfops.ReplacementImport`), after the document's
// own refusal, and put in the picture's place (PLAN-image-edit P03.S01).
func cmdObjectsReplace(args []string) int {
	fs := flag.NewFlagSet("nib objects replace", flag.ContinueOnError)
	var page, picture int
	var digest, with, alt, out string
	var keepAlt, inPlace bool
	fs.IntVar(&page, "page", 0, "the page, from 1 (required)")
	fs.IntVar(&picture, "picture", 0, "the picture's number as \"nib objects\" lists it, from 1 (required)")
	fs.StringVar(&digest, "digest", "", "the picture's digest as \"nib objects\" lists it (required)")
	fs.StringVar(&with, "with", "", "the picture to put in its place: PNG, JPEG, TIFF, WebP, GIF or BMP (required)")
	fs.StringVar(&alt, "alt", "", "a new description for a described picture")
	fs.BoolVar(&keepAlt, "keep-alt", false, "keep a described picture's description for the new picture")
	outFlag(fs, &out)
	inPlaceFlag(fs, &inPlace)
	fs.Usage = usageFunc(fs, "nib objects replace IN --page N --picture K --digest D --with FILE [--alt TEXT | --keep-alt] -o OUT  |  ... -w IN",
		"Put another picture in one picture's place. Where the picture can be moved the new one is fitted inside its\n"+
			"box keeping its proportion; otherwise it fills the box. A described picture needs --alt or --keep-alt. A\n"+
			"signed document, one in a signing ceremony and a picture inside a repeated page element are refused.")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	if page < 1 || picture < 1 || digest == "" || with == "" {
		errf("--page, --picture, --digest and --with are all required (\"nib objects IN --page N\" lists them)")
		return 2
	}
	if keepAlt && alt != "" {
		// Two flags that contradict each other are the command line's to refuse, before any file is read; the door
		// refuses the same request from the route (P03.S01 review W3).
		errf("--alt and --keep-alt cannot be combined")
		return 2
	}
	e := pdfops.ObjectEdit{Kind: pdfops.ObjectEditReplace, Page: page, Ordinal: picture - 1, Digest: digest, Alt: alt, KeepAlt: keepAlt}
	in, result, res, code := editObjectsWith(fs, out, inPlace, e, func(pdf []byte, e *pdfops.ObjectEdit) error {
		if err := objedit.Check(pdf); err != nil {
			return err
		}
		data, err := os.ReadFile(with)
		if err != nil {
			return err
		}
		src, size := pdfops.BytesSource(data)
		ctx, stop := interruptContext()
		defer stop()
		got, err := pdfops.Import(ctx, src, size, with, pdfops.ReplacementImport("cli"))
		if err != nil {
			return err
		}
		e.With = got.PDF
		return nil
	})
	if code != 0 {
		return code
	}
	for _, r := range res.Replaced {
		switch {
		case r.Stretched:
			fmt.Fprintln(os.Stderr, "the new picture is stretched to the old one's box: its proportions differ, and this picture cannot be moved")
		case r.Turned != 0:
			fmt.Fprintf(os.Stderr, "the new picture was turned upright (%d degrees)\n", r.Turned)
		}
		switch {
		case len(r.SharedWith) > 0:
			fmt.Fprintf(os.Stderr, "the old picture stays in the file: page(s) %v still draw it\n", r.SharedWith)
		case r.StaysInFile:
			fmt.Fprintln(os.Stderr, "the old picture stays in the file: the form still uses it")
		}
	}
	return writeEdited(in, out, inPlace, result)
}

// editObjects reads the one input a picture edit names (IN, or the file -w rewrites) and applies e through the door.
// A non-zero code is the command's exit: 2 for a request the door could not read, 1 for every other refusal.
func editObjects(fs *flag.FlagSet, out string, inPlace bool, e pdfops.ObjectEdit) (string, []byte, pdfops.ObjectEditResult, int) {
	return editObjectsWith(fs, out, inPlace, e, nil)
}

// editObjectsWith is editObjects with prep run on the document before the edit — a replace asks the document's own
// refusal and imports its picture there, so a signed document's picture is never read.
func editObjectsWith(fs *flag.FlagSet, out string, inPlace bool, e pdfops.ObjectEdit, prep func(pdf []byte, e *pdfops.ObjectEdit) error) (string, []byte, pdfops.ObjectEditResult, int) {
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
	if prep != nil {
		if err := prep(pdf, &e); err != nil {
			errf("%s: %s", inputName(in), strings.TrimPrefix(err.Error(), "pdfops: "))
			return "", nil, pdfops.ObjectEditResult{}, 1
		}
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
