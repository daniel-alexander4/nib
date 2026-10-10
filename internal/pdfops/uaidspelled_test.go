package pdfops

import (
	"encoding/binary"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
	"nib/internal/testpdf"
)

// The P06 phase-close review's findings on the identification door (/pending 665, R2 #4 and #5).

// withCatalogPacket reads the labelled fixture and replaces its catalog packet with edit's answer, in the
// context — the bytes a producer could have written and nib's own writers never would.
func withCatalogPacket(t *testing.T, edit func(packet string) []byte) *model.Context {
	t.Helper()
	ctx, err := pdfread.ReadOptimized(labelledFixture(t), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatal(err)
	}
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	ir, ok := root["Metadata"].(types.IndirectRef)
	if !ok {
		t.Fatal("setup: the labelled fixture has no indirect /Metadata")
	}
	sd, _, err := ctx.XRefTable.DereferenceStreamDict(ir)
	if err != nil || sd == nil {
		t.Fatalf("setup: /Metadata does not resolve: %v", err)
	}
	if err := sd.Decode(); err != nil {
		t.Fatal(err)
	}
	sd.Content = edit(string(sd.Content))
	if err := sd.Encode(); err != nil {
		t.Fatal(err)
	}
	entry, found := ctx.XRefTable.FindTableEntryForIndRef(&ir)
	if !found {
		t.Fatal("setup: no table entry for /Metadata")
	}
	entry.Object = *sd
	return ctx
}

// packetIn is the context's catalog packet, and whether the catalog still has one.
func packetIn(t *testing.T, ctx *model.Context) (string, bool) {
	t.Helper()
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	o, ok := root["Metadata"]
	if !ok {
		return "", false
	}
	sd, _, err := ctx.XRefTable.DereferenceStreamDict(o)
	if err != nil || sd == nil {
		t.Fatalf("/Metadata does not resolve: %v", err)
	}
	if err := sd.Decode(); err != nil {
		t.Fatal(err)
	}
	return string(sd.Content), true
}

// TestAnIdentificationSpelledWithACharacterReferenceIsDropped — `…/ns/id&#x2F;` is the identification's URI
// to every XML reader, and the door asked the packet's BYTES for the URI before it parsed anything.
func TestAnIdentificationSpelledWithACharacterReferenceIsDropped(t *testing.T) {
	ctx := withCatalogPacket(t, func(p string) []byte {
		if strings.Count(p, pdfuaidNS) == 0 {
			t.Fatal("setup: the labelled packet does not name the schema")
		}
		return []byte(strings.ReplaceAll(p, pdfuaidNS, strings.TrimSuffix(pdfuaidNS, "/")+"&#x2F;"))
	})
	before, _ := packetIn(t, ctx)
	if strings.Contains(before, pdfuaidNS) || !testpdf.PacketClaimsUA(before) {
		t.Fatal("setup: the packet must claim PDF/UA to a parser while its bytes never hold the URI")
	}
	had, err := dropUAIdentification(ctx)
	if err != nil {
		t.Fatal(err)
	}
	after, kept := packetIn(t, ctx)
	if !had || testpdf.PacketClaimsUA(after) {
		t.Errorf("an identification whose namespace is spelled with a character reference survived the drop "+
			"(reported a claim: %v) — the document goes on claiming PDF/UA through every change", had)
	}
	if !kept || !strings.Contains(after, "Census") {
		t.Errorf("the drop took more than the claim: /Metadata kept = %v, and the title should still be in it", kept)
	}
}

// TestAUTF16IdentificationIsDropped — XMP may be UTF-16, where the URI's bytes are not the UTF-8 ones. nib
// cannot edit such a packet, so it goes whole, as any claim nib cannot parse does.
func TestAUTF16IdentificationIsDropped(t *testing.T) {
	ctx := withCatalogPacket(t, func(p string) []byte {
		units := utf16.Encode([]rune(p))
		b := []byte{0xFF, 0xFE}
		for _, u := range units {
			b = binary.LittleEndian.AppendUint16(b, u)
		}
		return b
	})
	had, err := dropUAIdentification(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, kept := packetIn(t, ctx); !had || kept {
		t.Errorf("a UTF-16 packet naming the identification schema was left in place (claim reported: %v, "+
			"/Metadata kept: %v)", had, kept)
	}
}

// TestACharacterReferenceAloneCostsNoMetadata — the control for both: a packet that claims nothing keeps
// everything, whether it parses or not.
func TestACharacterReferenceAloneCostsNoMetadata(t *testing.T) {
	for name, edit := range map[string]func(string) []byte{
		"well-formed": func(p string) []byte { return []byte("<a>line&#xA;break</a>") },
		"not XML":     func(p string) []byte { return []byte("<a>line&#xA;break</b>") },
	} {
		ctx := withCatalogPacket(t, edit)
		want, _ := packetIn(t, ctx)
		had, err := dropUAIdentification(ctx)
		if got, kept := packetIn(t, ctx); had || err != nil || !kept || got != want {
			t.Errorf("%s: a packet with a character reference and no claim: had = %v, err = %v, kept = %v, "+
				"unchanged = %v", name, had, err, kept, got == want)
		}
	}
}

// TestAPrefixIsResolvedInItsOwnScope — the bookkeeping the rewrite does by hand: a prefix means what the
// nearest open element bound it to, and means the outer thing again once that element closes.
func TestAPrefixIsResolvedInItsOwnScope(t *testing.T) {
	const other = "http://example.org/other/"
	for _, tc := range []struct{ name, in, want string }{
		{"rebound inside, restored after",
			`<r xmlns:p="` + pdfuaidNS + `"><i xmlns:p="` + other + `"><p:keep>1</p:keep></i><p:part>1</p:part></r>`,
			`<r><i xmlns:p="` + other + `"><p:keep>1</p:keep></i></r>`},
		{"bound inside only, unbound after",
			`<r xmlns:p="` + other + `"><i xmlns:p="` + pdfuaidNS + `"><p:part>1</p:part></i><p:keep>1</p:keep></r>`,
			`<r xmlns:p="` + other + `"><i></i><p:keep>1</p:keep></r>`},
		{"the default namespace, restored to none",
			`<r><i xmlns="` + pdfuaidNS + `"><part>1</part></i><part>kept</part></r>`,
			`<r><part>kept</part></r>`},
	} {
		got, removed, err := withoutUAIdentification([]byte(tc.in))
		if err != nil || !removed || string(got) != tc.want {
			t.Errorf("%s:\n in   %s\n got  %s (removed %v, err %v)\n want %s", tc.name, tc.in, got, removed, err, tc.want)
		}
	}
}

// TestStrippingTheIdentificationIsLinearInNesting — four times the depth is about four times the work. It was
// sixteen (measured 618 ms at 20,000 deep and 10.1 s at 80,000). The best of three, as a ratio, so a busy
// machine moves both sides.
func TestStrippingTheIdentificationIsLinearInNesting(t *testing.T) {
	if testing.Short() {
		t.Skip("a timing ratio")
	}
	cost := func(n int) time.Duration {
		p := []byte(`<r xmlns:pdfuaid="` + pdfuaidNS + `"><pdfuaid:part>1</pdfuaid:part>` +
			strings.Repeat("<a>", n) + strings.Repeat("</a>", n) + `</r>`)
		best := time.Duration(0)
		for i := 0; i < 3; i++ {
			start := time.Now()
			if _, removed, err := withoutUAIdentification(p); err != nil || !removed {
				t.Fatalf("setup: removed = %v, err = %v", removed, err)
			}
			if d := time.Since(start); best == 0 || d < best {
				best = d
			}
		}
		return best
	}
	small, large := cost(20000), cost(80000)
	if ratio := float64(large) / float64(small); ratio > 9 {
		t.Errorf("80,000 nested elements took %v and 20,000 took %v — %.1f times the work for four times the "+
			"depth, so the scope lookup is walking the open elements again", large, small, ratio)
	}
}
