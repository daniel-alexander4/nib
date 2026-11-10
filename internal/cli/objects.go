package cli

import (
	"flag"
	"fmt"
	"os"
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
	fs := flag.NewFlagSet("nib objects", flag.ContinueOnError)
	var page int
	var asJSON bool
	fs.IntVar(&page, "page", 0, "the page to list, from 1 (required)")
	fs.BoolVar(&asJSON, "json", false, "print the listing as JSON")
	fs.Usage = usageFunc(fs, "nib objects IN --page N [--json]  |  nib objects remove ...",
		"List the pictures page N draws, in drawing order, each with its number, its digest, where it is on the page\n"+
			"and whether Nib can remove it (and why not). \"nib objects remove\" takes one off the page by that number\n"+
			"and digest. IN may be - (stdin).")
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
		fmt.Printf("picture %d  %s  %s  [%.3f %.3f %.3f %.3f]  %s\n", o.Ordinal+1, o.Kind, o.Digest,
			o.Box[0], o.Box[1], o.Box[2], o.Box[3], state)
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
			"The picture leaves the file unless another page still draws it. A signed document, one in a signing\n"+
			"ceremony and a picture inside a repeated page element are refused.")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	if page < 1 || picture < 1 || digest == "" {
		errf("--page, --picture and --digest are all required (\"nib objects IN --page N\" lists them)")
		return 2
	}
	var in string
	if inPlace {
		if out != "" {
			errf("-o/--out cannot be combined with -w/--in-place")
			return 1
		}
		if fs.NArg() != 1 || fs.Arg(0) == "-" {
			errf("-w rewrites one document in place — give exactly one file")
			return 1
		}
		in = fs.Arg(0)
	} else {
		var code int
		if in, code = singleInput(fs, out); code != 0 {
			return code
		}
	}
	pdf, err := readInput(in)
	if err != nil {
		errf("%v", err)
		return 1
	}
	result, res, err := objedit.Remove(pdf, []pdfops.ObjectEdit{{Kind: pdfops.ObjectEditDelete, Page: page, Ordinal: picture - 1, Digest: digest}})
	if err != nil {
		errf("%s: %s", inputName(in), strings.TrimPrefix(err.Error(), "pdfops: "))
		if ref, ok := objedit.Refusal(err); ok && ref.Cause == pdfops.CauseBadRequest {
			return 2
		}
		return 1
	}
	for _, r := range res.Removed {
		if r.ElementRemoved {
			fmt.Fprintf(os.Stderr, "its tag went with it%s\n", altNote(r.Alt))
		}
		if r.LinksOver > 0 {
			fmt.Fprintf(os.Stderr, "%d link(s) over the picture are still on the page\n", r.LinksOver)
		}
		if len(r.SharedWith) > 0 {
			fmt.Fprintf(os.Stderr, "the picture stays in the file: page(s) %v still draw it\n", r.SharedWith)
		}
	}
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
