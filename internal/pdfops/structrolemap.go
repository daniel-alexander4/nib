package pdfops

import (
	"fmt"
	"sort"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"
)

// The role map, published and edited — ADR-127 (`/pending 855` item 4).
//
// A document's `/RoleMap` says what each of its own structure type names means: `/Heading#201 → /H1`. A
// reader that does not know a custom name follows the map until it reaches a standard type
// (`standardRole`), and an element whose name leads nowhere is an element nothing can announce — ua1 7.1 t5.
//
// # What is published
//
// One row per mapping the reader holds (`structTree.roleMap`): the custom name, the type it is mapped to
// as written, the standard type that resolves to, and how many elements are typed with the name.
//
// # What one edit does
//
// `rolemap` sets, replaces or removes ONE entry. It names no element.
//
//   - The name mapped is never a standard type: *"standard structure types … shall not be remapped"*
//     (ISO 14289-1 §7.1; veraPDF's ua1 7.1 t7, `uacheck`'s `checkStandardTypeNotRemapped`).
//   - What it is mapped TO is always a standard type. A mapping onto another custom name is how a chain is
//     made, and a chain is how a loop is made (ua1 7.1 t6); an edit that can only write an arrow onto a
//     standard type can write neither. Chains a producer wrote are read as they always were and left alone.
//   - A mapping is not removed while an element is still typed with the name: those elements would be left
//     with a type nothing explains, which is the failure the map exists to prevent.
//
// # Names
//
// pdfcpu decodes a name's `#xx` escapes when it parses one — a dictionary key and a name value alike
// (`model.parseName`) — and encodes them when it writes (`types.EncodeName`, in `Dict.PDFString` and
// `Name.PDFString`). So `readStructTree` holds `Heading 1`, an element's `/S` reads `Heading 1`, and the
// inverse of that reading is to write the name as it is read: the key `Heading 1`, which leaves the file as
// `/Heading#201`. Nothing here escapes by hand; a second escaping would write `#2320`.

// viewRole is one entry of the document's role map, as a reviewer sees it.
type viewRole struct {
	// name is the custom type; to is what the map sends it to, as written; standard is where that leads
	// (`standardRole`) — the name itself for one on a loop, as an element typed with it reads.
	name, to, standard string
	// elements is how many elements carry the name as their `/S`.
	elements int
}

// roleMapView is tree's role map, sorted by name.
func roleMapView(tree *structTree) []viewRole {
	used := map[string]int{}
	for _, e := range tree.elems {
		used[e.kind]++
	}
	out := make([]viewRole, 0, len(tree.roleMap))
	for name, to := range tree.roleMap {
		out = append(out, viewRole{name: name, to: to, standard: standardRole(tree, name), elements: used[name]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

// setRoleMapping sets, replaces or removes the role map's entry for ed.role (ADR-127).
func setRoleMapping(ctx *model.Context, tree *structTree, ed structEdit) error {
	if ed.elem != 0 || ed.parent != 0 || len(ed.headers) != 0 {
		return fmt.Errorf("%w: a role map edit names a type, not an element — leave the element, parent and header cells out", ErrTagsReview)
	}
	role := ed.role
	if role == "" {
		return fmt.Errorf("%w: a role map edit needs the name of the custom type it maps", ErrTagsReview)
	}
	if standardStructTypes[role] {
		return fmt.Errorf("%w: %s is a standard structure type, and a standard type cannot be mapped to another (PDF/UA-1 §7.1: the standard types shall not be remapped)", ErrTagsReview, role)
	}
	var rm types.Dict
	if o, has := tree.root["RoleMap"]; has && o != nil {
		d, err := ctx.DereferenceDict(o)
		// A second line: pdfcpu's validation refuses a `/RoleMap` that is not a dictionary when the document
		// is read (measured: an array, a string and a number each fail there), so this is not reached today.
		if err != nil || d == nil {
			return fmt.Errorf("pdfops: the document's /RoleMap is not a dictionary, so no mapping can be read from it or written to it")
		}
		rm = d
	}
	_, mapped := rm[role]
	if ed.value == "" {
		if !mapped {
			return fmt.Errorf("%w: the role map has no mapping for %s, so there is none to remove", ErrTagsReview, role)
		}
		used := 0
		for _, e := range tree.elems {
			if e.kind == role {
				used++
			}
		}
		if used > 0 {
			return fmt.Errorf("%w: %d tag(s) are still of type %s, and without its mapping nothing would say what that type means — change their type first, then remove the mapping", ErrTagsReview, used, role)
		}
		delete(rm, role)
		if len(rm) == 0 {
			// An empty map explains nothing; the key goes with its last entry.
			delete(tree.root, "RoleMap")
		}
		return nil
	}
	if !standardStructTypes[ed.value] {
		return fmt.Errorf("%w: %q is not a standard structure type, and a custom type is mapped only to a standard one", ErrTagsReview, ed.value)
	}
	if mapped && tree.roleMap[role] == ed.value {
		return fmt.Errorf("%w: %s is already mapped to %s", ErrTagsReview, role, ed.value)
	}
	if rm == nil {
		rm = types.Dict{}
		tree.root["RoleMap"] = rm
	}
	rm[role] = types.Name(ed.value)
	return nil
}
