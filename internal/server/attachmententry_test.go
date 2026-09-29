package server

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nib/internal/ceremony"
	"nib/internal/testpdf"
)

// TestADownloadIsTheRowTheUserClicked — /pending 745 (c). The extract route took the NAME and
// served the first entry answering to it, so with two entries whose names swap, clicking the row
// shown as b.txt downloaded a.txt's bytes. The client now sends the listing's id; each row must
// download its own entry's bytes, under the name the row showed.
func TestADownloadIsTheRowTheUserClicked(t *testing.T) {
	ts, _ := startServer(t)
	c, csrf := authedClient(t, ts)
	path := filepath.Join(t.TempDir(), "swapped.pdf")
	pdf := testpdf.WithEmbedded(
		testpdf.Embedded{Key: "a.txt", F: "b.txt", UF: "b.txt", Data: "entry keyed a.txt"},
		testpdf.Embedded{Key: "b.txt", F: "a.txt", UF: "a.txt", Data: "entry keyed b.txt"},
	)
	if err := os.WriteFile(path, pdf, 0o600); err != nil {
		t.Fatal(err)
	}
	openByPath(t, ts.URL, c, csrf, path)

	resp, err := c.Get(ts.URL + "/api/attachments")
	if err != nil {
		t.Fatal(err)
	}
	var list attachmentsResponse
	if err := json.NewDecoder(resp.Body).Decode(&list); err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if len(list.Attachments) != 2 {
		t.Fatalf("listed %+v, want two", list.Attachments)
	}
	for _, row := range list.Attachments {
		var eb bytes.Buffer
		mw := multipart.NewWriter(&eb)
		mw.WriteField("id", row.ID)
		mw.Close()
		resp := write(t, c, csrf, http.MethodPost, ts.URL+"/api/attachments/extract", mw.FormDataContentType(), &eb)
		got, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("extract of the row shown as %q (id %q): status %d: %s", row.Name, row.ID, resp.StatusCode, got)
		}
		if want := "entry keyed " + row.ID; string(got) != want {
			t.Errorf("the row shown as %q (id %q) downloaded %q, want %q", row.Name, row.ID, got, want)
		}
		if cd := resp.Header.Get("Content-Disposition"); !strings.Contains(cd, row.Name) {
			t.Errorf("the row shown as %q downloaded as %q", row.Name, cd)
		}
	}
}

// TestADocumentWithTwoCeremonyRecordsStaysFrozen — /pending 745 made Extract refuse a document
// carrying the record's key twice (ErrTwoRecords), and ceremonyFreeze treated every Extract error
// as "not under a ceremony", so that document became editable. It is refused as frozen.
func TestADocumentWithTwoCeremonyRecordsStaysFrozen(t *testing.T) {
	doc := twoRecordDocForFreeze(t)
	if _, err := ceremony.Extract(doc); !errors.Is(err, ceremony.ErrTwoRecords) {
		t.Fatalf("setup: Extract = %v, want ErrTwoRecords — this test cannot build its own stimulus", err)
	}
	err := ceremonyFreeze(doc)
	if !errors.Is(err, ErrCeremonyFrozen) {
		t.Fatalf("a document carrying two ceremony records is editable (ceremonyFreeze = %v), want ErrCeremonyFrozen", err)
	}
}

func twoRecordDocForFreeze(t *testing.T) []byte {
	t.Helper()
	r := ceremony.AttachmentName
	return testpdf.WithEmbedded(
		testpdf.Embedded{Key: r, F: r, UF: r, Data: "one"},
		testpdf.Embedded{Key: r, F: r, UF: r, Data: "two"},
	)
}
