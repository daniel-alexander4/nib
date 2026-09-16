package server

import (
	"bytes"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"regexp"
	"strings"
	"testing"

	"nib/internal/pdfops"
	"nib/internal/testpdf"
)

// TestFinalizingLogsAClaimItCouldNotCheck — `/pending 504`, found beside `nib sign`'s identical drop.
// ADR-032 says a document pdfcpu cannot read is signed as it was read, "logged"; `handleFinalize` dropped
// the error without the log, so a claim the signature then sealed left no trace.
func TestFinalizingLogsAClaimItCouldNotCheck(t *testing.T) {
	base, err := testpdf.Text("to be finalized")
	if err != nil {
		t.Fatal(err)
	}
	// A catalog whose /Type pdfcpu's validation refuses and the signer does not read.
	broken := regexp.MustCompile(`/Type\s*/Catalog`).ReplaceAll(base, []byte("/Type /Katalog"))
	// STIMULUS: the check really fails on these bytes.
	if bytes.Equal(broken, base) {
		t.Fatal("setup: the fixture's catalog was not found to break")
	}
	if _, derr := pdfops.DropUAIdentificationUnlessSigned(broken, false); derr == nil {
		t.Fatal("setup: the PDF/UA check succeeds on the broken catalog, so no log is owed")
	}
	var logged bytes.Buffer
	log.SetOutput(&logged)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })

	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	fw, _ := mw.CreateFormFile("pdf", "doc.pdf")
	fw.Write(broken)
	mw.WriteField("params", `{"reason":"Finalized in Nib"}`)
	mw.Close()
	resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/finalize", mw.FormDataContentType(), &buf)
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("finalize status %d: %s — ADR-032: a failed check never costs the signature", resp.StatusCode, body)
	}
	if !strings.Contains(logged.String(), "PDF/UA identification could not be checked") {
		t.Errorf("the check failed and the document was signed with nothing logged (log %q)", logged.String())
	}
}
