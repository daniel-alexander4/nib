package pdfops

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

// canaryDocx is a DOCX that asks LibreOffice to fetch two things from url while converting: a linked
// picture and an attached template.
func canaryDocx(t *testing.T, base string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	add := func(name, body string) {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		w.Write([]byte(body))
	}
	add("[Content_Types].xml", `<?xml version="1.0"?><Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types"><Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/><Default Extension="xml" ContentType="application/xml"/><Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/></Types>`)
	add("_rels/.rels", `<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/></Relationships>`)
	add("word/_rels/document.xml.rels", fmt.Sprintf(`<?xml version="1.0"?><Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships"><Relationship Id="rId9" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="%s/canary-img.png" TargetMode="External"/><Relationship Id="rId8" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/attachedTemplate" Target="%s/canary-tpl.dotm" TargetMode="External"/></Relationships>`, base, base))
	add("word/document.xml", `<?xml version="1.0"?><w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships" xmlns:wp="http://schemas.openxmlformats.org/drawingml/2006/wordprocessingDrawing" xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" xmlns:pic="http://schemas.openxmlformats.org/drawingml/2006/picture"><w:body><w:p><w:r><w:t>hello</w:t></w:r></w:p><w:p><w:r><w:drawing><wp:inline><wp:extent cx="914400" cy="914400"/><wp:docPr id="1" name="p"/><a:graphic><a:graphicData uri="http://schemas.openxmlformats.org/drawingml/2006/picture"><pic:pic><pic:nvPicPr><pic:cNvPr id="1" name="p"/><pic:cNvPicPr/></pic:nvPicPr><pic:blipFill><a:blip r:link="rId9"/></pic:blipFill><pic:spPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="914400" cy="914400"/></a:xfrm><a:prstGeom prst="rect"><a:avLst/></a:prstGeom></pic:spPr></pic:pic></a:graphicData></a:graphic></wp:inline></w:drawing></w:r></w:p></w:body></w:document>`)
	add("word/settings.xml", `<?xml version="1.0"?><w:settings xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main" xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships"><w:attachedTemplate r:id="rId8"/></w:settings>`)
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// TestLibreOfficeFetchesNothingWhileConverting — D33's loopback canary. The positive control runs the
// SAME document under an empty profile and requires the listener to be hit: a zero is evidence only
// where the stimulus is shown to work. Skipped, loudly and by name, where LibreOffice is absent — a
// present-path test that did not run (W16).
func TestLibreOfficeFetchesNothingWhileConverting(t *testing.T) {
	if !LibreOfficeAvailable() {
		t.Skip("NOT RUN (not a pass): LibreOffice is absent, so the egress canary was not driven")
	}
	var hits atomic.Int64
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		http.NotFound(w, r)
	})}
	go srv.Serve(ln)
	defer srv.Close()
	doc := canaryDocx(t, "http://"+ln.Addr().String())

	// CONTROL: an empty profile — LibreOffice's defaults — reaches the listener.
	dir := t.TempDir()
	in := filepath.Join(dir, "in.docx")
	if err := os.WriteFile(in, doc, 0o600); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	out := exec.CommandContext(ctx, libreOfficePath(), "--headless", "--nologo", "--nofirststartwizard",
		"-env:UserInstallation="+fileURL(filepath.ToSlash(filepath.Join(dir, "profile"))),
		"--convert-to", "pdf", "--outdir", dir, in)
	if b, err := out.CombinedOutput(); err != nil {
		t.Fatalf("the control conversion failed: %v\n%s", err, b)
	}
	time.Sleep(300 * time.Millisecond)
	if hits.Load() == 0 {
		t.Skip("NOT RUN (not a pass): this LibreOffice does not fetch the linked picture under defaults, so the canary proves nothing here")
	}
	controlHits := hits.Load()

	// THE CLAIM: nib's conversion of the same document reaches nothing.
	pdf, err := ConvertOfficeToPDF(context.Background(), doc, ".docx")
	if err != nil {
		t.Fatalf("the conversion failed: %v", err)
	}
	time.Sleep(300 * time.Millisecond)
	if !LooksLikePDF(pdf) {
		t.Fatal("no PDF came back")
	}
	if got := hits.Load() - controlHits; got != 0 {
		t.Fatalf("LibreOffice made %d request(s) to the listener while converting under nib's profile (the control made %d)", got, controlHits)
	}
}

func TestTheOfficeProfileIsSeededBeforeLibreOfficeReadsIt(t *testing.T) {
	p := filepath.Join(t.TempDir(), "profile")
	if err := seedOfficeProfile(p); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(p, "user", "registrymodifications.xcu"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"MacroSecurityLevel", "ooInetProxyType", "ooInetHTTPSProxyPort", "ooInetNoProxy"} {
		if !bytes.Contains(b, []byte(want)) {
			t.Errorf("the seeded profile does not set %s", want)
		}
	}
}
