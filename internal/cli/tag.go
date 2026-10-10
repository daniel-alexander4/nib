package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"nib/internal/pdfops"
	"nib/internal/tagwrite"
)

// cmdTag is the structure editor on the command line — `PLAN-accessibility.md` P10.
//
// `tree` and `propose` read (P10.S01). Each reaches the door the matching route reaches — the tree route
// and `nib tag tree` both call `pdfops.ReadStructure`, the propose route and `nib tag propose` both call
// `pdfops.ProposeTags` — so the command line and the Tags panel cannot hold two readings of one document.
// `--json` prints the doors' own values, whose field names a server test holds equal to the routes'
// responses: a script reads exactly what the panel reads.
func cmdTag(args []string) int {
	usage := func(w *os.File) {
		fmt.Fprint(w, "usage: nib tag tree|propose IN [--json]\n"+
			"       nib tag untagged IN [--page N] [--json]\n"+
			"       nib tag commit IN -o OUT --review REVIEW.json\n"+
			"       nib tag edit IN -o OUT --edits EDITS.json\n"+
			"       nib tag remove IN -o OUT\n\n"+
			"Read and write a document's structure. tree prints the tags it already has, in reading order, and\n"+
			"propose prints the structure nib would propose; untagged prints what the pages draw that no tag\n"+
			"owns — what \"nib tag edit\" can tag as a region; none of the three writes anything. commit writes a reviewed\n"+
			"proposal, edit corrects the existing tree and remove takes every tag away so the document can be\n"+
			"tagged again (-w rewrites the one input); all three refuse a signed document.\n"+
			"Run \"nib tag SUBCOMMAND -h\" for a subcommand's flags.\n")
	}
	if len(args) == 0 {
		usage(os.Stderr)
		return 1
	}
	switch args[0] {
	case "-h", "--help", "help":
		usage(os.Stderr)
		return 0
	case "tree":
		return tagTree(args[1:])
	case "propose":
		return tagPropose(args[1:])
	case "untagged":
		return tagUntagged(args[1:])
	case "commit":
		return tagWrite(args[1:], "commit")
	case "edit":
		return tagWrite(args[1:], "edit")
	case "remove":
		return tagWrite(args[1:], "remove")
	}
	errf("unknown tag subcommand %q — tree, propose, untagged, commit, edit or remove (run \"nib tag -h\")", args[0])
	return 1
}

// tagWrite is `nib tag commit`, `nib tag edit` (P10.S02) and `nib tag remove` (ADR-120, which takes no
// request): read the request file, write through the
// tagwrite door the Tags panel's routes reach — which refuses a signed document — and write the result
// to -o or, with -w, over the one input. A stale request exits 1 with the door's sentence; a request
// that is malformed on its own terms exits 2.
func tagWrite(args []string, mode string) int {
	fs := flag.NewFlagSet("nib tag "+mode, flag.ContinueOnError)
	var out, request string
	var inPlace bool
	outFlag(fs, &out)
	inPlaceFlag(fs, &inPlace)
	requestFlag, usage := "review", "nib tag commit IN -o OUT --review REVIEW.json  |  nib tag commit -w IN --review REVIEW.json"
	about := "Write a reviewed proposal as the document's structure. REVIEW.json is {\"elements\": [{\"id\", \"role\", \"ignore\", \"text\",\n" +
		"\"alt\"}]}; \"nib tag propose --json\" prints one that keeps every proposed role. A proposed Figure is the one\n" +
		"element it does not settle: give each its \"alt\" — what the picture shows — or \"ignore\": true, or the\n" +
		"commit is refused. A signed document is refused."
	if mode == "edit" {
		requestFlag, usage = "edits", "nib tag edit IN -o OUT --edits EDITS.json  |  nib tag edit -w IN --edits EDITS.json"
		about = "Correct the existing structure tree as one batch. EDITS.json is {\"edits\": [{\"kind\", \"element\", \"value\",\n" +
			"\"parent\", \"index\", \"headers\", \"page\", \"rect\", \"pieces\", \"alt\"}]} — kind retype, move, alt, scope,\n" +
			"colspan, rowspan, headers, artifact, create, delete or region; element an id from \"nib tag tree\"; headers\n" +
			"the ids of the header cells that head a table cell. create adds an empty tag: \"value\" its type,\n" +
			"\"parent\" the id it goes under (0 or -1: the top of the tree), \"index\" its place among that tag's tags\n" +
			"(left out: last), and no \"element\". delete takes a tag away and keeps what it held, which moves up\n" +
			"to the tag above. move takes \"parent\" and \"index\" the same way (0: the parent it has; -1: the top of\n" +
			"the tree). region tags what a page draws that no\n" +
			"tag owns — text, pictures, drawn graphics — as ONE new tag: \"value\", \"parent\" and \"index\" as create's,\n" +
			"\"page\", and \"rect\" [left, top, right, bottom] as fractions of the page as it is shown (0 0 is its top\n" +
			"left) — or \"pieces\", the \"rect\" of each piece \"nib tag untagged --json\" lists — and \"alt\", which a\n" +
			"Figure needs. A rect takes what is centred in it; a piece takes what lies inside it. A signed document is refused."
	}
	if mode == "remove" {
		usage = "nib tag remove IN -o OUT  |  nib tag remove -w IN"
		about = "Take the document's structure tree and every marked-content id away, leaving it untagged — the way to tag\n" +
			"again a document that arrived tagged (\"nib tag propose\", then \"nib tag commit\"). What was marked as an\n" +
			"artifact stays marked. A signed document is refused."
	} else {
		fs.StringVar(&request, requestFlag, "", "the request file")
	}
	fs.Usage = usageFunc(fs, usage, about)
	if code, ok := parse(fs, args); !ok {
		return code
	}
	if mode != "remove" && request == "" {
		errf("missing --%s (the request file)", requestFlag)
		return 2
	}
	var in string
	if inPlace {
		if out != "" {
			errf("-o/--out cannot be combined with -w/--in-place")
			return 1
		}
		// A request describes one document, so -w rewrites exactly one.
		if fs.NArg() != 1 || fs.Arg(0) == "-" {
			errf("-w rewrites the one document the request describes — give exactly one file")
			return 1
		}
		in = fs.Arg(0)
	} else {
		var code int
		if in, code = singleInput(fs, out); code != 0 {
			return code
		}
	}
	var f *os.File
	var err error
	if mode != "remove" {
		if f, err = os.Open(request); err != nil {
			errf("%v", err)
			return 1
		}
		defer f.Close()
	}
	var result []byte
	pdf, rerr := readInput(in)
	if rerr != nil {
		errf("%v", rerr)
		return 1
	}
	switch mode {
	case "commit":
		var reviews []pdfops.TagReview
		if reviews, err = tagwrite.DecodeReview(f); err == nil {
			result, err = tagwrite.Commit(pdf, reviews)
		}
	case "edit":
		var edits []pdfops.StructureEdit
		if edits, err = tagwrite.DecodeEdits(f); err == nil {
			result, err = tagwrite.Edit(pdf, edits)
		}
	default:
		result, err = tagwrite.Remove(pdf)
	}
	if err != nil {
		errf("%s: %s", inputName(in), strings.TrimPrefix(strings.TrimPrefix(err.Error(), "pdfops: "), "tagwrite: "))
		if errors.Is(err, tagwrite.ErrMalformed) || errors.Is(err, pdfops.ErrTagsReview) {
			return 2
		}
		return 1
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

// tagTree prints the document's existing structure tree.
func tagTree(args []string) int {
	fs := flag.NewFlagSet("nib tag tree", flag.ContinueOnError)
	var asJSON bool
	fs.BoolVar(&asJSON, "json", false, "print the tree as JSON, in the shape the Tags panel reads")
	fs.Usage = usageFunc(fs, "nib tag tree IN [--json]",
		"Print the document's existing structure tree in reading order: each element's id (what an edit names),\n"+
			"type, page, alt text, header scope, a table cell's spans and header cells, and text. Writes nothing.")
	if code, ok := parse(fs, args); !ok {
		return code
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
	tree, err := pdfops.ReadStructure(pdf)
	if err != nil {
		errf("%v", err)
		return 1
	}
	if asJSON {
		return printTagJSON(tree)
	}
	if !tree.Tagged {
		errf("%s has no structure tree — \"nib tag propose\" shows the structure nib would propose for it", inputName(fs.Arg(0)))
		return 0
	}
	level := func(i int) int {
		d := 0
		for p := tree.Elements[i].Parent; p >= 0 && d < 64; p = tree.Elements[p].Parent {
			d++
		}
		return d
	}
	for i, e := range tree.Elements {
		name := e.Standard
		if e.Kind != e.Standard {
			name = fmt.Sprintf("%s (%s)", e.Standard, e.Kind)
		}
		var notes []string
		if e.Standard == "Figure" {
			if e.HasAlt {
				notes = append(notes, "alt: "+e.Alt)
			} else {
				notes = append(notes, "no alt text")
			}
		}
		if e.Standard == "TH" {
			if e.Scope != "" {
				notes = append(notes, "scope: "+e.Scope)
			} else {
				notes = append(notes, "no scope")
			}
		}
		if e.ColSpan > 1 {
			notes = append(notes, fmt.Sprintf("spans %d columns", e.ColSpan))
		}
		if e.RowSpan > 1 {
			notes = append(notes, fmt.Sprintf("spans %d rows", e.RowSpan))
		}
		if len(e.Headers) > 0 {
			heads := make([]string, len(e.Headers))
			for k, j := range e.Headers {
				heads[k] = strconv.Itoa(tree.Elements[j].ID)
			}
			notes = append(notes, "headed by: "+strings.Join(heads, ", "))
		}
		line := fmt.Sprintf("%-6d %s%s", e.ID, strings.Repeat("  ", level(i)), name)
		if e.Page > 0 {
			line += fmt.Sprintf("  p%d", e.Page)
		}
		if len(notes) > 0 {
			line += "  [" + strings.Join(notes, "; ") + "]"
		}
		if t := strings.TrimSpace(e.Text); t != "" {
			line += "  " + clipRunes(t, 60)
		}
		// The role, alt text, scope and text are all the document's own (/pending 727); nib's
		// parts of the line carry no control character, so the whole line goes through the door.
		fmt.Println(termText(line))
	}
	if tree.Unaddressable > 0 {
		errf("%d element(s) are written inline (id 0) and cannot be named by an edit", tree.Unaddressable)
	}
	return 0
}

// tagUntagged prints what the document's pages draw that no structure element owns (ADR-125) — the reader
// the untagged route reaches (`pdfops.ReadUntagged`), and the keyboard's way to what a region edit can take.
func tagUntagged(args []string) int {
	fs := flag.NewFlagSet("nib tag untagged", flag.ContinueOnError)
	var asJSON bool
	var page int
	fs.BoolVar(&asJSON, "json", false, "print the pieces as JSON, in the shape the Tags panel reads")
	fs.IntVar(&page, "page", 0, "the one page to read (left out: every page)")
	fs.Usage = usageFunc(fs, "nib tag untagged IN [--page N] [--json]",
		"Print what the pages draw that no tag owns: text grouped into paragraphs, pictures, drawings (painted\n"+
			"paths whose boxes touch), and rules and plain boxes last. Each line gives the page, the kind, whether the\n"+
			"page marks the piece as decoration (an artifact), and where it is — left, top, right, bottom as fractions\n"+
			"of the page as it is shown, which is what a region edit's \"pieces\" takes (\"nib tag edit\"). A piece a\n"+
			"form XObject draws is listed and cannot be tagged from the page. Writes nothing.")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	if fs.NArg() != 1 {
		errf("expected one input PDF, got %d", fs.NArg())
		return 1
	}
	if page < 0 {
		errf("--page is a page number, 1 or more")
		return 2
	}
	pdf, err := readInput(fs.Arg(0))
	if err != nil {
		errf("%v", err)
		return 1
	}
	got, err := pdfops.ReadUntagged(pdf, page)
	if err != nil {
		errf("%s: %s", inputName(fs.Arg(0)), strings.TrimPrefix(err.Error(), "pdfops: "))
		return 1
	}
	if asJSON {
		return printTagJSON(got)
	}
	for _, p := range got.Pieces {
		var notes []string
		if p.Decoration {
			notes = append(notes, "decoration")
		}
		if p.InForm {
			notes = append(notes, "drawn inside a form")
		}
		line := fmt.Sprintf("p%-3d %-7s %.4f %.4f %.4f %.4f", p.Page, p.Kind, p.Rect[0], p.Rect[1], p.Rect[2], p.Rect[3])
		if len(notes) > 0 {
			line += "  [" + strings.Join(notes, "; ") + "]"
		}
		if t := strings.TrimSpace(p.Text); t != "" {
			line += "  " + clipRunes(t, 60)
		}
		// The text is the document's own (/pending 727): the whole line goes through the terminal door.
		fmt.Println(termText(line))
	}
	if len(got.Pieces) == 0 {
		where := "this document"
		if page > 0 {
			where = fmt.Sprintf("page %d", page)
		}
		errf("nothing on %s is untagged", where)
	}
	return 0
}

// tagPropose prints the structure nib would propose for the document.
func tagPropose(args []string) int {
	fs := flag.NewFlagSet("nib tag propose", flag.ContinueOnError)
	var asJSON bool
	fs.BoolVar(&asJSON, "json", false, "print the proposal as JSON, in the shape the Tags card reads")
	fs.Usage = usageFunc(fs, "nib tag propose IN [--json]",
		"Print the headings, paragraphs, list items, ruled tables and figures nib would propose for the\n"+
			"document, in reading order; a table's rows and cells are indented under it. A Figure is proposed for\n"+
			"each picture the page draws and is printed with the box it fills (left, bottom, right, top, in points):\n"+
			"to commit it, give it an \"alt\" in the review — what the picture shows — or \"ignore\": true.\n"+
			"Writes nothing: a proposal is reviewed before it is committed.")
	if code, ok := parse(fs, args); !ok {
		return code
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
	prop, err := pdfops.ProposeTags(pdf)
	if err != nil {
		errf("%v", err)
		return 1
	}
	if asJSON {
		return printTagJSON(prop)
	}
	// A table's rows and cells are indented under it (ADR-121): an element's depth is its parent's and one.
	depth := make([]int, len(prop.Elements))
	for i, e := range prop.Elements {
		if e.Parent >= 0 && e.Parent < i {
			depth[i] = depth[e.Parent] + 1
		}
		// A Figure has no text: what says which picture it is, is where it is drawn (ADR-122).
		words := clipRunes(strings.TrimSpace(e.Text), 70)
		if e.Role == "Figure" {
			words = fmt.Sprintf("a picture at %.0f %.0f %.0f %.0f — needs a description (\"alt\"), or \"ignore\"", e.Rect[0], e.Rect[1], e.Rect[2], e.Rect[3])
		}
		fmt.Println(termText(fmt.Sprintf("%-4d %s%-5s p%-3d %s", e.ID, strings.Repeat("  ", depth[i]), e.Role, e.Page, words)))
	}
	for _, u := range prop.Unsupported {
		errf("page %d: %s — check its order carefully", u.Page, u.Reason)
	}
	if len(prop.NoText) > 0 {
		pages := make([]string, len(prop.NoText))
		for i, p := range prop.NoText {
			pages[i] = strconv.Itoa(p)
		}
		errf("no text on page(s) %s — make a scan searchable (OCR) before tagging it", strings.Join(pages, ", "))
	}
	if len(prop.Elements) == 0 {
		errf("nothing to propose: the document draws no text nib can read")
	}
	return 0
}

// printTagJSON writes v to stdout as indented JSON.
func printTagJSON(v any) int {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		errf("%v", err)
		return 1
	}
	return 0
}

// clipRunes shortens s to at most n runes, marking a cut with an ellipsis.
func clipRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}
