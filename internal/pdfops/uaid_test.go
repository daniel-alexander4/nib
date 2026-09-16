package pdfops

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nib/internal/testpdf"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// catalogPacket returns the decoded catalog XMP packet, "" when there is none.
func catalogPacket(t *testing.T, pdf []byte) string {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewDefaultConfiguration())
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	root, err := ctx.XRefTable.Catalog()
	if err != nil {
		t.Fatal(err)
	}
	o, ok := root["Metadata"]
	if !ok {
		return ""
	}
	sd, _, err := ctx.XRefTable.DereferenceStreamDict(o)
	if err != nil || sd == nil {
		t.Fatalf("metadata stream: %v", err)
	}
	if err := sd.Decode(); err != nil {
		t.Fatal(err)
	}
	return string(sd.Content)
}

const uaPacketHead = "<?xpacket begin=\"\ufeff\" id=\"W5M0MpCehiHzreSzNTczkc9d\"?>\n<x:xmpmeta xmlns:x=\"adobe:ns:meta/\">\n  <rdf:RDF xmlns:rdf=\"http://www.w3.org/1999/02/22-rdf-syntax-ns#\">\n"
const uaPacketTail = "  </rdf:RDF>\n</x:xmpmeta>\n" + "                                        \n<?xpacket end=\"w\"?>"

func TestTheIdentificationIsRemovedInEveryEncoding(t *testing.T) {
	title := `    <rdf:Description rdf:about="" xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:title><rdf:Alt><rdf:li xml:lang="x-default">A &amp; B</rdf:li></rdf:Alt></dc:title></rdf:Description>` + "\n"
	for _, c := range []struct{ name, body string }{
		{"element form", `    <rdf:Description rdf:about="" xmlns:pdfuaid="http://www.aiim.org/pdfua/ns/id/"><pdfuaid:part>1</pdfuaid:part></rdf:Description>` + "\n"},
		{"attribute form", `    <rdf:Description rdf:about="" xmlns:pdfuaid="http://www.aiim.org/pdfua/ns/id/" pdfuaid:part="1"/>` + "\n"},
		{"a prefix the schema is not usually bound to", `    <rdf:Description rdf:about="" xmlns:ua="http://www.aiim.org/pdfua/ns/id/"><ua:part>1</ua:part><ua:rev>2024</ua:rev></rdf:Description>` + "\n"},
		{"the schema beside the title in one Description", `    <rdf:Description rdf:about="" xmlns:pdfuaid="http://www.aiim.org/pdfua/ns/id/" xmlns:dc2="http://purl.org/dc/elements/1.1/" pdfuaid:part="1"><dc2:creator>kept</dc2:creator></rdf:Description>` + "\n"},
	} {
		in := uaPacketHead + title + c.body + uaPacketTail
		out, removed, err := withoutUAIdentification([]byte(in))
		if err != nil || !removed {
			t.Errorf("%s: removed=%v err=%v — the identification was not taken out", c.name, removed, err)
			continue
		}
		s := string(out)
		if strings.Contains(s, "pdfua/ns/id") || strings.Contains(s, ">1<") || strings.Contains(s, `part="1"`) {
			t.Errorf("%s: the identification survives:\n%s", c.name, s)
		}
		for _, keep := range []string{"A &amp; B", `xml:lang="x-default"`, "<?xpacket begin=\"\ufeff\"", "<?xpacket end=\"w\"?>", "                                        \n"} {
			if !strings.Contains(s, keep) {
				t.Errorf("%s: removing the claim disturbed the rest of the packet — %q is gone:\n%s", c.name, keep, s)
			}
		}
		if c.name == "the schema beside the title in one Description" && !strings.Contains(s, "<dc2:creator>kept</dc2:creator>") {
			t.Errorf("%s: a property sharing the Description with the claim was removed with it:\n%s", c.name, s)
		}
	}
}

func TestAPacketWithNoIdentificationIsLeftAsItsOwnBytes(t *testing.T) {
	in := uaPacketHead + `    <rdf:Description rdf:about="" xmlns:dc="http://purl.org/dc/elements/1.1/"><dc:format>application/pdf</dc:format></rdf:Description>` + "\n" + uaPacketTail
	out, removed, err := withoutUAIdentification([]byte(in))
	if err != nil || removed || string(out) != in {
		t.Errorf("a packet without the schema came back changed (removed=%v err=%v)", removed, err)
	}
}

func TestAClaimThatCannotBeParsedTakesTheMetadataWithIt(t *testing.T) {
	_, _, err := withoutUAIdentification([]byte(`<x:xmpmeta xmlns:pdfuaid="http://www.aiim.org/pdfua/ns/id/"><pdfuaid:part>1</x:xmpmeta>`))
	if err == nil {
		t.Fatal("a malformed packet naming the schema parsed without error, so the delete-the-stream branch cannot be reached")
	}
}

// claimsUA is the shared reader (`testpdf.PacketClaimsUA`): the element or attribute in the namespace,
// never the string, which a PDF/A extension-schema block carries while claiming nothing.
func claimsUA(t *testing.T, packet string) bool {
	t.Helper()
	return testpdf.PacketClaimsUA(packet)
}

// TestDropUAIdentificationOnVeraPDFsOwnLabelledFiles — third-party producers, both encodings.
func TestDropUAIdentificationOnVeraPDFsOwnLabelledFiles(t *testing.T) {
	dir := os.Getenv("NIB_UA_CORPUS")
	if dir == "" {
		home, _ := os.UserHomeDir()
		dir = filepath.Join(home, "nib", "verapdfs", "PDF_UA-1")
	}
	files := []string{
		"7.4 Headings/7.4.2 Numbered headings/7.4.2-t01-pass-d.pdf",
		"7.3 Graphics/7.3-t01-pass-c.pdf",
	}
	for _, f := range files {
		src, err := os.ReadFile(filepath.Join(dir, f))
		if err != nil {
			t.Skipf("SKIP (not a pass): veraPDF's PDF/UA-1 corpus is not at %s, so third-party packets are UNMEASURED here", dir)
		}
		if !claimsUA(t, catalogPacket(t, src)) {
			t.Fatalf("setup: %s carries no identification, so dropping one proves nothing", f)
		}
		out, had, err := dropUAIdentificationBytes(src)
		if err != nil || !had {
			t.Errorf("%s: had=%v err=%v", f, had, err)
			continue
		}
		p := catalogPacket(t, out)
		if p == "" {
			t.Errorf("%s: the drop removed the whole metadata stream from a well-formed packet — the title and every other property went with the claim", f)
			continue
		}
		if claimsUA(t, p) {
			var mentions []string
			for _, line := range strings.Split(p, "\n") {
				if strings.Contains(line, "pdfua") {
					mentions = append(mentions, strings.TrimSpace(line))
				}
			}
			t.Errorf("%s: after the drop the packet still claims PDF/UA; lines naming it:\n\t%s", f, strings.Join(mentions, "\n\t"))
		}
	}
}

// TestASignedDocumentIsNeverRewrittenToDropAClaim — the declared gap, asserted so it stays declared: a
// rewrite of a signed document destroys its signature, so the unless-signed door hands it back untouched.
func TestASignedDocumentIsNeverRewrittenToDropAClaim(t *testing.T) {
	labelled := labelledFixture(t)
	same, err := DropUAIdentificationUnlessSigned(labelled, true)
	if err != nil || !bytes.Equal(same, labelled) {
		t.Errorf("a document reported as signed came back rewritten (err %v) — its signature would not survive", err)
	}
	// Control: the same bytes reported unsigned DO lose the claim, or "untouched" above proves nothing.
	dropped, err := DropUAIdentificationUnlessSigned(labelled, false)
	if err != nil {
		t.Fatal(err)
	}
	if testpdf.PacketClaimsUA(catalogPacket(t, dropped)) {
		t.Error("control: an unsigned labelled document kept its claim through the door")
	}
}

func TestADocumentWithNoClaimIsNotRewritten(t *testing.T) {
	src := threePagePDF(t)
	out, had, err := dropUAIdentificationBytes(src)
	if err != nil || had || !bytes.Equal(out, src) {
		t.Errorf("a document with no identification came back rewritten (had=%v err=%v, same bytes=%v)", had, err, bytes.Equal(out, src))
	}
}
