package cli

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"

	"nib/internal/pdfops"
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
		fmt.Fprint(w, "usage: nib tag tree IN [--json]  |  nib tag propose IN [--json]\n\n"+
			"Read a document's structure. tree prints the tags it already has, in reading order;\n"+
			"propose prints the structure nib would propose for it. Neither writes anything.\n"+
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
	}
	errf("unknown tag subcommand %q — tree or propose (run \"nib tag -h\")", args[0])
	return 1
}

// tagTree prints the document's existing structure tree.
func tagTree(args []string) int {
	fs := flag.NewFlagSet("nib tag tree", flag.ContinueOnError)
	var asJSON bool
	fs.BoolVar(&asJSON, "json", false, "print the tree as JSON, in the shape the Tags panel reads")
	fs.Usage = usageFunc(fs, "nib tag tree IN [--json]",
		"Print the document's existing structure tree in reading order: each element's id (what an edit names),\n"+
			"type, page, alt text, header scope and text. Writes nothing.")
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
		fmt.Println(line)
	}
	if tree.Unaddressable > 0 {
		errf("%d element(s) are written inline (id 0) and cannot be named by an edit", tree.Unaddressable)
	}
	return 0
}

// tagPropose prints the structure nib would propose for the document.
func tagPropose(args []string) int {
	fs := flag.NewFlagSet("nib tag propose", flag.ContinueOnError)
	var asJSON bool
	fs.BoolVar(&asJSON, "json", false, "print the proposal as JSON, in the shape the Tags card reads")
	fs.Usage = usageFunc(fs, "nib tag propose IN [--json]",
		"Print the headings, paragraphs and list items nib would propose for the document, in reading order.\n"+
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
	for _, e := range prop.Elements {
		fmt.Printf("%-4d %-3s p%-3d %s\n", e.ID, e.Role, e.Page, clipRunes(strings.TrimSpace(e.Text), 70))
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
