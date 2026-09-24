package uacheck

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Every legal encoding of what the language and marked rules read — `/pending 489`.
//
// veraPDF's corpus stores a `/Lang` as a hex string, as an indirect object, and on marked content, and a
// `/Marked` indirectly; this checker read only the direct forms nib and LibreOffice write. Each case below
// is one of those encodings on a document built for it, beside a control that differs only in declaring
// nothing, so a pass cannot come from somewhere else in the document.

// hexEN is UTF-16BE "en" with its byte-order mark, as a producer writes a hex /Lang.
const hexEN = "<FEFF0065006E>"

// langDoc builds a one-page document whose catalog carries catalogExtra, whose page resources carry
// resExtra, whose page dictionary carries pageExtra, and whose content stream is content — plus any
// further objects numbered 6 and up.
func langDoc(catalogExtra, resExtra, pageExtra, content string, extra map[int]string) []byte {
	objs := map[int]string{
		1: "<< /Type /Catalog /Pages 2 0 R " + catalogExtra + " >>",
		2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Resources << /Font << /F1 5 0 R >> " + resExtra + " >> /Contents 4 0 R " + pageExtra + " >>",
		4: fmt.Sprintf("<< /Length %d >>\nstream\n%s\nendstream", len(content), content),
		5: "<< /Type /Font /Subtype /Type1 /BaseFont /Helvetica >>",
	}
	for n, o := range extra {
		objs[n] = o
	}
	return buildPDF(objs)
}

const shown = "BT /F1 24 Tf 72 700 Td (Hello) Tj ET"

func TestTheContentLanguageRuleReadsEveryLegalEncoding(t *testing.T) {
	tagged := func(elemLang string) []byte {
		return langDoc("/MarkInfo << /Marked true >> /StructTreeRoot 7 0 R", "", "/StructParents 0",
			"/P <</MCID 0>> BDC "+shown+" EMC", map[int]string{
				7: "<< /Type /StructTreeRoot /K [8 0 R] /ParentTree 9 0 R >>",
				8: "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K 0 " + elemLang + " >>",
				9: "<< /Nums [0 [8 0 R]] >>",
			})
	}
	for _, c := range []struct {
		name string
		pdf  []byte
		want Verdict
	}{
		{"control: untagged text and no language anywhere", langDoc("", "", "", shown, nil), Fail},
		{"a hex catalog /Lang", langDoc("/Lang "+hexEN, "", "", shown, nil), Pass},
		{"an indirect catalog /Lang", langDoc("/Lang 6 0 R", "", "", shown, map[int]string{6: "(en-US)"}), Pass},
		{"control: an empty marked-content property list", langDoc("", "", "", "/Span <<>> BDC "+shown+" EMC", nil), Fail},
		{"a literal /Lang on the marked content", langDoc("", "", "", "/Span <</Lang (en-US)>> BDC "+shown+" EMC", nil), Pass},
		{"a hex /Lang on the marked content", langDoc("", "", "", "/Span <</Lang "+hexEN+" >> BDC "+shown+" EMC", nil), Pass},
		{"a /Lang in a named property list", langDoc("", "/Properties << /P0 << /Lang (en-US) >> >>", "", "/Span /P0 BDC "+shown+" EMC", nil), Pass},
		{"control: a tagged element declaring no language", tagged(""), Fail},
		{"a hex /Lang on the structure element", tagged("/Lang " + hexEN), Pass},
		{"an indirect /Lang on the structure element", tagged("/Lang 6 0 R"), Pass},
		// Present counts, even empty: veraPDF passes 7.2 t34 on its corpus files 7.2-t29-fail-n/o/p,
		// which declare an empty /Lang in each of these three places and fail only 7.2 t29.
		{"an empty catalog /Lang", langDoc("/Lang ()", "", "", shown, nil), Pass},
		{"an empty /Lang on the marked content", langDoc("", "", "", "/Span <</Lang ()>> BDC "+shown+" EMC", nil), Pass},
		{"an empty /Lang on the structure element", tagged("/Lang ()"), Pass},
	} {
		pdf := c.pdf
		if strings.Contains(c.name, "indirect /Lang on the structure") {
			pdf = langDoc("/MarkInfo << /Marked true >> /StructTreeRoot 7 0 R", "", "/StructParents 0",
				"/P <</MCID 0>> BDC "+shown+" EMC", map[int]string{
					6: "(en-US)",
					7: "<< /Type /StructTreeRoot /K [8 0 R] /ParentTree 9 0 R >>",
					8: "<< /Type /StructElem /S /P /P 7 0 R /Pg 3 0 R /K 0 /Lang 6 0 R >>",
					9: "<< /Nums [0 [8 0 R]] >>",
				})
		}
		if got := verdictOf(t, pdf, "7.2 t34"); got.Verdict != c.want {
			t.Errorf("%s: 7.2 t34 reports %v (%s at %s), want %v", c.name, got.Verdict, got.Why, got.Where, c.want)
		}
	}
}

func TestTheMarkedRuleReadsAnIndirectBoolean(t *testing.T) {
	doc := func(value string) []byte {
		return langDoc("/MarkInfo << /Marked 10 0 R >> /StructTreeRoot 7 0 R", "", "", shown, map[int]string{
			7:  "<< /Type /StructTreeRoot /K [] >>",
			10: value,
		})
	}
	if got := verdictOf(t, doc("true"), "6.2 t1"); got.Verdict != Pass {
		t.Errorf("/Marked stored indirectly as true reports %v (%s), want Pass — veraPDF reads it as true", got.Verdict, got.Why)
	}
	if got := verdictOf(t, doc("false"), "6.2 t1"); got.Verdict != Fail {
		t.Errorf("control: /Marked stored indirectly as false reports %v (%s), want Fail", got.Verdict, got.Why)
	}
}

func TestAnAltWithARealLanguageIsDeterminedAndOneWithoutIsNot(t *testing.T) {
	titled := plainDoc(t)
	item := func(lang string) string { return `<rdf:li xml:lang="` + lang + `">text</rdf:li>` }
	for _, c := range []struct {
		name string
		body string
		want Verdict
	}{
		{"one Alt with x-default and en-US", `<dc:title><rdf:Alt>` + item("x-default") + item("en-US") + `</rdf:Alt></dc:title>`, Pass},
		{"a determined Alt beside one offering only x-default", `<dc:title><rdf:Alt>` + item("x-default") + item("en-US") + `</rdf:Alt></dc:title>` +
			`<dc:description><rdf:Alt>` + item("x-default") + `</rdf:Alt></dc:description>`, Fail},
	} {
		got := verdictOf(t, withPacketBody(t, titled, c.body), "7.2 t33")
		if got.Verdict != c.want {
			t.Errorf("%s: 7.2 t33 reports %v (%s), want %v", c.name, got.Verdict, got.Why, c.want)
		}
	}
}

// TestTheIdentificationClauseIsPresenceNotThePartValue — 5 t1 is whether the identification is there;
// veraPDF files the part's value under 5 t2, which nib does not implement (`/pending 489`, corpus file
// `5-t02-fail-a.pdf`: veraPDF passes 5 t1 and fails 5 t2 on pdfuaid:part "2").
func TestTheIdentificationClauseIsPresenceNotThePartValue(t *testing.T) {
	titled := plainDoc(t)
	part := func(v string) string {
		return `<pdfuaid:part xmlns:pdfuaid="http://www.aiim.org/pdfua/ns/id/">` + v + `</pdfuaid:part>`
	}
	if got := verdictOf(t, withPacketBody(t, titled, part("2")), "5 t1"); got.Verdict != Pass {
		t.Errorf("pdfuaid:part 2 reports %v for 5 t1 (%s), want Pass — veraPDF passes 5 t1 and fails 5 t2 there", got.Verdict, got.Why)
	}
	if got := verdictOf(t, withPacketBody(t, titled, `<dc:format>application/pdf</dc:format>`), "5 t1"); got.Verdict != Fail {
		t.Errorf("control: a packet with no identification reports %v for 5 t1 (%s), want Fail", got.Verdict, got.Why)
	}
	// And the value is 5 t2's, where it is still judged and named.
	if got := verdictOf(t, withPacketBody(t, titled, part("2")), "5 t2"); got.Verdict != Fail || !strings.Contains(got.Why, `"2"`) {
		t.Errorf("pdfuaid:part 2 reports %v for 5 t2 (%s), want Fail naming the value", got.Verdict, got.Why)
	}
	if got := verdictOf(t, withPacketBody(t, titled, part("1")), "5 t2"); got.Verdict != Pass {
		t.Errorf("pdfuaid:part 1 reports %v for 5 t2 (%s), want Pass", got.Verdict, got.Why)
	}
}

// TestTheCheckerReadsLangAndMarkedThroughItsOwnDoors — routing. Every site reads a `/Lang` through
// `declaresLang` and a `/Marked` through `boolValue`; a bare cast reintroduced anywhere in the package is
// the defect this item fixed, back.
func TestTheCheckerReadsLangAndMarkedThroughItsOwnDoors(t *testing.T) {
	bare := regexp.MustCompile(`\["(Lang|Marked)"\]\.\(types\.|(Boolean|String|Name)Entry\("(Lang|Marked)"\)`)
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	langCalls, boolCalls, scanned := 0, 0, 0
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		scanned++
		src := string(b)
		for i, line := range strings.Split(src, "\n") {
			if bare.MatchString(line) {
				t.Errorf("%s:%d reads /Lang or /Marked with a bare cast — use declaresLang / boolValue: %s", f, i+1, strings.TrimSpace(line))
			}
		}
		langCalls += strings.Count(src, "declaresLang(") + strings.Count(src, "catalogLang(") + strings.Count(src, `.text("Lang")`)
		boolCalls += strings.Count(src, "boolValue(")
	}
	if scanned < 10 {
		t.Fatalf("the scan read %d file(s) — not the package", scanned)
	}
	// The /Lang doors, and every site that goes through one — SIX since P04.S04, listed so a drop names
	// which reader left rather than being absorbed by a slack floor:
	//
	//	declaresLang            document.go (its definition), content.go (inheritedLangOf's element check)
	//	catalogLang             rules_language.go (its definition, 7.2 t29), xmp.go (catalogDeclaresLang)
	//	propertyList.text(Lang) content.go (openSequence — the marked-content property list)
	//
	// It was seven until this slice deleted `declaresLangFor`, the SECOND climb for "is a language
	// determined" that 7.2 t34 read and that disagreed with `parentLang` on three shapes (`/pending 635`).
	// One door fewer here is one question with one answer, which is the opposite of a reader leaving.
	if langCalls < 6 {
		t.Errorf("the /Lang doors are called %d time(s) — a reader has stopped going through them", langCalls)
	}
	if boolCalls < 2 {
		t.Errorf("boolValue is called %d time(s) — checkMarkInfo has stopped going through it", boolCalls)
	}
}
