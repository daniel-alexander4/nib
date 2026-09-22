package uacheck

import (
	"fmt"
	"strings"
)

// treeDoc builds a one-page tagged document from a nested spec like "Document(Table(TR(TD),Span))" —
// P03.S02's fixture shape, every one of which was run through veraPDF 1.30.2 before the rule it grades
// was written (the slice's grill note in the plan has the table).
// Every leaf element carries one MCID of its own; "#mcid" as a kid puts a bare MCID in its parent's /K and
// "#objr" an object reference to a link annotation.
func treeDoc(roleMap, spec string) []byte {
	objs := map[int]string{}
	next := 20
	mcid := 0
	var content strings.Builder
	var nums []string
	annots := ""
	var parse func(s string, parentRef string) (string, string)
	// parse returns (the kid entry for the parent's /K, the remaining string)
	parse = func(s string, parentRef string) (string, string) {
		i := strings.IndexAny(s, "(),")
		name := s
		rest := ""
		if i >= 0 {
			name, rest = s[:i], s[i:]
		}
		switch name {
		case "#mcid":
			m := mcid
			mcid++
			fmt.Fprintf(&content, "/P << /MCID %d >> BDC BT /F1 12 Tf 72 %d Td (m%d) Tj ET EMC\n", m, 700-12*m, m)
			nums = append(nums, parentRef)
			return fmt.Sprint(m), rest
		case "#objr":
			if annots != "" {
				panic("treeDoc: one #objr per document — a second would overwrite the first annotation")
			}
			annots = "/Annots [9 0 R]"
			objs[9] = "<< /Type /Annot /Subtype /Link /Rect [0 0 10 10] /StructParent 1000 /Contents (x) /P 3 0 R >>"
			objs[8] = fmt.Sprintf("<< /Nums [0 [%s] 1000 %s] >>", "NUMS0", parentRef)
			return "<< /Type /OBJR /Obj 9 0 R >>", rest
		}
		// "TD!r2!c3!scope=Row!id=h1!headers=h1+h2" — a cell's Table attributes and its /ID, after the type.
		attrs, extra := "", ""
		if parts := strings.Split(name, "!"); len(parts) > 1 {
			name = parts[0]
			for _, a := range parts[1:] {
				switch {
				case strings.HasPrefix(a, "r"):
					attrs += " /RowSpan " + a[1:]
				case strings.HasPrefix(a, "c"):
					attrs += " /ColSpan " + a[1:]
				case strings.HasPrefix(a, "scope="):
					attrs += " /Scope /" + a[len("scope="):]
				case strings.HasPrefix(a, "headers="):
					var hs []string
					for _, h := range strings.Split(a[len("headers="):], "+") {
						hs = append(hs, "("+h+")")
					}
					attrs += " /Headers [" + strings.Join(hs, " ") + "]"
				case strings.HasPrefix(a, "id="):
					extra += " /ID (" + a[len("id="):] + ")"
				default:
					panic("treeDoc: unknown cell attribute " + a)
				}
			}
			if attrs != "" {
				extra += " /A << /O /Table" + attrs + " >>"
			}
		}
		n := next
		next++
		ref := fmt.Sprintf("%d 0 R", n)
		var kids []string
		if strings.HasPrefix(rest, "(") {
			rest = rest[1:]
			for {
				k, r := parse(rest, ref)
				kids = append(kids, k)
				rest = r
				if strings.HasPrefix(rest, ",") {
					rest = rest[1:]
					continue
				}
				rest = strings.TrimPrefix(rest, ")")
				break
			}
		} else {
			m := mcid
			mcid++
			fmt.Fprintf(&content, "/%s << /MCID %d >> BDC BT /F1 12 Tf 72 %d Td (m%d) Tj ET EMC\n", "Span", m, 700-12*m, m)
			nums = append(nums, ref)
			kids = []string{fmt.Sprint(m)}
		}
		objs[n] = fmt.Sprintf("<< /Type /StructElem /S /%s /P %s /Pg 3 0 R /K [%s]%s >>", name, parentRef, strings.Join(kids, " "), extra)
		return ref, rest
	}
	top, rest := parse(spec, "7 0 R")
	if rest != "" {
		panic(fmt.Sprintf("treeDoc: %q is not one balanced element spec — %q is left over", spec, rest))
	}
	c := content.String()
	objs[1] = "<< /Type /Catalog /Pages 2 0 R /Lang (en-US) /MarkInfo << /Marked true >> /StructTreeRoot 7 0 R /ViewerPreferences << /DisplayDocTitle true >> >>"
	objs[2] = "<< /Type /Pages /Kids [3 0 R] /Count 1 >>"
	objs[3] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /StructParents 0 /Tabs /S " + annots + " /Resources << /Font << /F1 5 0 R >> >> /Contents 4 0 R >>"
	objs[4] = fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(c), c)
	objs[5] = "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>"
	rm := ""
	if roleMap != "" {
		rm = "/RoleMap << " + roleMap + " >>"
	}
	objs[7] = fmt.Sprintf("<< /Type /StructTreeRoot /K [%s] /ParentTree 8 0 R %s >>", top, rm)
	if objs[8] == "" {
		objs[8] = fmt.Sprintf("<< /Nums [0 [%s]] >>", strings.Join(nums, " "))
	} else {
		objs[8] = strings.Replace(objs[8], "NUMS0", strings.Join(nums, " "), 1)
	}
	return buildPDF(objs)
}
