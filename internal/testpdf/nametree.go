package testpdf

import "fmt"

// NameTree describes a hand-written catalog name tree for NameTreeDoc — `/pending 880`, ADR-164.
//
// Every shape here is one pdfcpu's validator drops without a word and no nib writer produces: a tree nested
// deeper than the validator follows, and one whose `/Kids` loop.
type NameTree struct {
	// Kind is the key under the catalog's `/Names`: "EmbeddedFiles", "Dests" or "JavaScript".
	Kind string
	// Key is the one name the deep leaf holds; "" is "deep.txt".
	Key string
	// Depth is how many nodes sit between the root and the leaf holding Key: the leaf is Depth+1 below the root.
	Depth int
	// Loop makes the node above the leaf name the first node of the chain again, beside the leaf.
	Loop bool
	// Sibling adds a second leaf directly under the root, AFTER the chain, holding the key "zz-shallow.txt".
	Sibling bool
}

// NameTreeDoc is a one-page document whose catalog `/Names` holds the tree nt describes. An embedded file's
// bytes are "HIDDEN" (the deep one) and "SHALLOW"; a destination is page 1; a script is `app.alert(1)`.
func NameTreeDoc(nt NameTree) []byte {
	key := nt.Key
	if key == "" {
		key = "deep.txt"
	}
	objs := map[int]string{
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] >>",
	}
	next := 10
	value := func(name, data string) string {
		switch nt.Kind {
		case "Dests":
			return "[3 0 R /Fit]"
		case "JavaScript":
			objs[next] = "<< /S /JavaScript /JS (app.alert\\(1\\)) >>"
			next++
			return fmt.Sprintf("%d 0 R", next-1)
		}
		objs[next] = fmt.Sprintf("<< /Type /EmbeddedFile /Length %d >>\nstream\n%s\nendstream", len(data), data)
		objs[next+1] = fmt.Sprintf("<< /Type /Filespec /F (%s) /UF (%s) /EF << /F %d 0 R /UF %d 0 R >> >>", name, name, next, next)
		next += 2
		return fmt.Sprintf("%d 0 R", next-1)
	}
	limits := fmt.Sprintf("/Limits [(%s) (%s)]", key, key)
	leaf := next
	next++
	objs[leaf] = fmt.Sprintf("<< %s /Names [(%s) %s] >>", limits, key, value(key, "HIDDEN"))
	// The chain, bottom up: chain[0] is the node above the leaf.
	below := leaf
	first := 0
	chain := make([]int, nt.Depth)
	for i := range chain {
		chain[i] = next
		next++
	}
	if nt.Depth > 0 {
		first = chain[nt.Depth-1]
	}
	for i, n := range chain {
		kids := fmt.Sprintf("%d 0 R", below)
		if i == 0 && nt.Loop {
			kids += fmt.Sprintf(" %d 0 R", first)
		}
		objs[n] = fmt.Sprintf("<< %s /Kids [%s] >>", limits, kids)
		below = n
	}
	rootKids := fmt.Sprintf("%d 0 R", below)
	if nt.Sibling {
		sib := next
		next++
		objs[sib] = fmt.Sprintf("<< /Limits [(zz-shallow.txt) (zz-shallow.txt)] /Names [(zz-shallow.txt) %s] >>", value("zz-shallow.txt", "SHALLOW"))
		rootKids += fmt.Sprintf(" %d 0 R", sib)
	}
	objs[4] = fmt.Sprintf("<< /Kids [%s] >>", rootKids)
	objs[1] = fmt.Sprintf("<< /Type /Catalog /Pages 2 0 R /Names << /%s 4 0 R >> >>", nt.Kind)
	return assemble(objs)
}
