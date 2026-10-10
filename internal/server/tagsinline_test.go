package server

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"nib/internal/testpdf"
)

// Inline tags made editable (ADR-126) and the role map published and edited (ADR-127), at the routes.

// inlineRoleFixture is a one-page tree with both: a Document holding a `Heading 1` (role-mapped to H1), an
// INLINE Sect that owns MCID 1 and holds a paragraph, and a role map with one mapping no tag uses.
func inlineRoleFixture() []byte {
	var body strings.Builder
	for i, w := range []string{"Title", "Inside", "Body"} {
		fmt.Fprintf(&body, "/P <</MCID %d>> BDC\nBT /F1 12 Tf 72 %d Td (%s) Tj ET\nEMC\n", i, 700-20*i, w)
	}
	return testpdf.Assemble(map[int]string{
		1:  "<< /Type /Catalog /Pages 2 0 R /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /Lang (en-GB) >>",
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R /StructParents 0 >>",
		4:  fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", body.Len(), body.String()),
		5:  "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
		7:  "<< /Type /StructTreeRoot /K [8 0 R] /ParentTree 9 0 R /RoleMap << /Heading#201 /H1 /Unused /Note >> >>",
		8:  "<< /Type /StructElem /S /Document /P 7 0 R /K [20 0 R << /S /Sect /Pg 3 0 R /K [1 21 0 R] >>] >>",
		9:  "<< /Nums [0 [20 0 R null 21 0 R]] >>",
		20: "<< /Type /StructElem /S /Heading#201 /P 8 0 R /Pg 3 0 R /K [0] >>",
		21: "<< /Type /StructElem /S /P /P 8 0 R /Pg 3 0 R /K [2] >>",
	})
}

// treeOf is the tree route's answer for the open document.
func treeOf(t *testing.T, c *http.Client, base string) tagTreeResponse {
	t.Helper()
	var tree tagTreeResponse
	if err := json.Unmarshal(getBytes(t, c, base+"/api/tags/tree"), &tree); err != nil {
		t.Fatal(err)
	}
	return tree
}

// treeReading is the tree without its object numbers: what an edit that only numbers elements must keep.
func treeReading(tree tagTreeResponse) []string {
	out := make([]string, len(tree.Elements))
	for i, e := range tree.Elements {
		out[i] = fmt.Sprintf("%d under %d: %s as %s p%d %q kids %v", i, e.Parent, e.Kind, e.Standard, e.Page, e.Text, e.Kids)
	}
	return out
}

// TestTheEditRoutePromotesInlineTagsAndOneUndoTakesItBack — the promote edit through the route: the tree
// reads as it did with every id above zero, the promoted tag is then edited by its number, and each of the
// two changes is one undo, the second leaving the document's bytes as they were opened.
func TestTheEditRoutePromotesInlineTagsAndOneUndoTakesItBack(t *testing.T) {
	base, c, csrf := openTagsFixture(t, inlineRoleFixture())
	opened := getBytes(t, c, base+"/api/pdf")
	before := treeOf(t, c, base)
	if before.Unaddressable != 1 || len(before.Elements) != 4 || before.Elements[2].ID != 0 || before.Elements[2].Kind != "Sect" {
		t.Fatalf("setup: the tree reads %d inline of %+v", before.Unaddressable, before.Elements)
	}
	post := func(edits ...map[string]any) (int, string) {
		t.Helper()
		return postTags(t, c, csrf, base+"/api/tags/edit", map[string]any{"edits": edits})
	}
	// An inline tag cannot be named, and a promote that names something is refused.
	if code, body := post(map[string]any{"kind": "retype", "element": 0, "value": "Art"}); code != http.StatusBadRequest || !strings.Contains(body, "make inline tags editable") {
		t.Errorf("retyping an inline tag: edit = %d %q, want 400 pointing at the promotion", code, body)
	}
	if code, body := post(map[string]any{"kind": "promote", "element": 20}); code != http.StatusBadRequest {
		t.Errorf("a promote naming an element: edit = %d %q, want 400", code, body)
	}
	if !bytes.Equal(opened, getBytes(t, c, base+"/api/pdf")) {
		t.Fatal("a refused edit changed the document")
	}

	if code, body := post(map[string]any{"kind": "promote"}); code != http.StatusOK {
		t.Fatalf("promote = %d: %s", code, body)
	}
	after := treeOf(t, c, base)
	if after.Unaddressable != 0 || !reflect.DeepEqual(treeReading(after), treeReading(before)) {
		t.Fatalf("promoted, the tree reads %d inline and\n%v\nwant none inline and\n%v", after.Unaddressable, treeReading(after), treeReading(before))
	}
	sect := after.Elements[2].ID
	if sect <= 0 || after.Elements[0].ID != before.Elements[0].ID || after.Elements[1].ID != 20 || after.Elements[3].ID != 21 {
		t.Fatalf("promoted, the ids read %d %d %d %d — the inline tag needs a number and the others keep theirs",
			after.Elements[0].ID, after.Elements[1].ID, sect, after.Elements[3].ID)
	}
	if code, body := post(map[string]any{"kind": "promote"}); code != http.StatusBadRequest || !strings.Contains(body, "no tag in this document is written inline") {
		t.Errorf("a second promote: edit = %d %q, want 400 saying nothing is inline", code, body)
	}

	if code, body := post(map[string]any{"kind": "retype", "element": sect, "value": "Art"}); code != http.StatusOK {
		t.Fatalf("retyping the promoted tag = %d: %s", code, body)
	}
	if got := treeOf(t, c, base).Elements[2]; got.Standard != "Art" || got.ID != sect {
		t.Errorf("the promoted tag reads %+v, want an Art with the number it was given", got)
	}
	if code, body := postTags(t, c, csrf, base+"/api/undo", map[string]any{}); code != http.StatusOK {
		t.Fatalf("undo = %d: %s", code, body)
	}
	if got := treeOf(t, c, base); got.Unaddressable != 0 || got.Elements[2].Standard != "Sect" || got.Elements[2].ID != sect {
		t.Errorf("one undo left %d inline and %+v — want the retype alone taken back", got.Unaddressable, got.Elements[2])
	}
	if code, body := postTags(t, c, csrf, base+"/api/undo", map[string]any{}); code != http.StatusOK {
		t.Fatalf("second undo = %d: %s", code, body)
	}
	if !bytes.Equal(opened, getBytes(t, c, base+"/api/pdf")) {
		t.Error("two undos did not leave the document as it was opened — the promotion is not one step")
	}
}

// TestTheTreeRoutePublishesTheRoleMapAndTheEditRouteChangesIt — the role map on the wire, a mapping
// replaced (the tag typed with it resolves differently, and one undo restores it), one removed and one
// added; each refusal a 400 with its sentence; and an untagged document's role map an empty list.
func TestTheTreeRoutePublishesTheRoleMapAndTheEditRouteChangesIt(t *testing.T) {
	base, c, csrf := openTagsFixture(t, inlineRoleFixture())
	want := []tagRoleView{{Name: "Heading 1", To: "H1", Standard: "H1", Elements: 1}, {Name: "Unused", To: "Note", Standard: "Note", Elements: 0}}
	start := treeOf(t, c, base)
	if !reflect.DeepEqual(start.RoleMap, want) {
		t.Fatalf("the tree route's role map reads %+v, want %+v", start.RoleMap, want)
	}
	post := func(edits ...map[string]any) (int, string) {
		t.Helper()
		return postTags(t, c, csrf, base+"/api/tags/edit", map[string]any{"edits": edits})
	}
	opened := getBytes(t, c, base+"/api/pdf")
	for _, bad := range []struct {
		edit map[string]any
		says string
	}{
		{map[string]any{"kind": "rolemap", "value": "P"}, "needs the name"},
		{map[string]any{"kind": "rolemap", "role": "H1", "value": "H2"}, "H1 is a standard structure type"},
		{map[string]any{"kind": "rolemap", "role": "Heading 1", "value": "Unused"}, "is not a standard structure type"},
		{map[string]any{"kind": "rolemap", "role": "Heading 1", "value": ""}, "1 tag(s) are still of type Heading 1"},
		{map[string]any{"kind": "rolemap", "role": "Nowhere", "value": ""}, "no mapping for Nowhere"},
		{map[string]any{"kind": "rolemap", "role": "Heading 1", "value": "H1"}, "already mapped to H1"},
		{map[string]any{"kind": "rolemap", "role": "Heading 1", "value": "H2", "element": 20}, "names a type, not an element"},
	} {
		if code, body := post(bad.edit); code != http.StatusBadRequest || !strings.Contains(body, bad.says) {
			t.Errorf("%v: edit = %d %q, want 400 saying %q", bad.edit, code, body, bad.says)
		}
	}
	if !bytes.Equal(opened, getBytes(t, c, base+"/api/pdf")) {
		t.Fatal("a refused role map edit changed the document")
	}

	if code, body := post(map[string]any{"kind": "rolemap", "role": "Heading 1", "value": "H2"}); code != http.StatusOK {
		t.Fatalf("replacing a mapping = %d: %s", code, body)
	}
	got := treeOf(t, c, base)
	if e := got.Elements[1]; e.Kind != "Heading 1" || e.Standard != "H2" || e.Text != "Title" {
		t.Errorf("the heading reads %+v, want the same tag resolving to H2", e)
	}
	if got.RoleMap[0] != (tagRoleView{Name: "Heading 1", To: "H2", Standard: "H2", Elements: 1}) {
		t.Errorf("the role map's first row reads %+v", got.RoleMap[0])
	}
	if code, body := postTags(t, c, csrf, base+"/api/undo", map[string]any{}); code != http.StatusOK {
		t.Fatalf("undo = %d: %s", code, body)
	}
	if back := treeOf(t, c, base); !reflect.DeepEqual(back, start) {
		t.Errorf("one undo left the tree route answering %+v, want what it answered at the start", back)
	}

	// One batch: a mapping removed and one added for a name with a space in it.
	if code, body := post(map[string]any{"kind": "rolemap", "role": "Unused", "value": ""}, map[string]any{"kind": "rolemap", "role": "Side Bar", "value": "Sect"}); code != http.StatusOK {
		t.Fatalf("removing and adding = %d: %s", code, body)
	}
	if got := treeOf(t, c, base).RoleMap; !reflect.DeepEqual(got, []tagRoleView{{Name: "Heading 1", To: "H1", Standard: "H1", Elements: 1}, {Name: "Side Bar", To: "Sect", Standard: "Sect", Elements: 0}}) {
		t.Errorf("after the batch the role map reads %+v", got)
	}

	// An untagged document: an empty list on the wire, never null.
	ubase, uc, _ := openTagsFixture(t, untaggedTagsFixture(t))
	if raw := string(getBytes(t, uc, ubase+"/api/tags/tree")); !strings.Contains(raw, `"roleMap":[]`) {
		t.Errorf("an untagged document's tree answers %s — want \"roleMap\":[]", raw)
	}
}
