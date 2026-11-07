package server

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nib/internal/pdfops"
)

// The status answer carries the format table, row for row: it is the only list the pickers' `accept`
// is built from (P01.S01, ADR-184), so a row the page cannot see is a format its picker never offers.
func TestStatusCarriesTheFormatTable(t *testing.T) {
	ts, _ := startServer(t)
	c := newClient(t)
	st := getStatus(t, c, ts.URL)
	want := pdfops.Formats()
	if len(st.Formats) != len(want) || len(want) == 0 {
		t.Fatalf("status lists %d formats, the table has %d", len(st.Formats), len(want))
	}
	for i, f := range want {
		v := st.Formats[i]
		if v.Key != f.Key || strings.Join(v.MIME, ",") != strings.Join(f.MIME, ",") || v.Image != f.Image {
			t.Errorf("row %d: status %+v, table %+v", i, v, f)
		}
		// The tool is named short: Open…'s tooltip sets it in parentheses beside the format, so a name carrying its own
		// parenthetical would nest them (ADR-216).
		if v.Kind != f.Kind || v.Kind == "" {
			t.Errorf("%s: kind = %q, want the table's %q, and never none (ADR-217)", f.Key, v.Kind, f.Kind)
		}
		if v.Tool != f.Tool.Short() || strings.Contains(v.Tool, "(") {
			t.Errorf("%s: tool = %q, want the short name %q", f.Key, v.Tool, f.Tool.Short())
		}
		if v.Available != pdfops.FormatAvailable(f) {
			t.Errorf("%s: available = %v, the table says %v", f.Key, v.Available, pdfops.FormatAvailable(f))
		}
	}
	raw, _ := json.Marshal(st.Formats)
	if strings.Contains(string(raw), "http") {
		t.Errorf("a format row carries a link (ADR-039): %s", raw)
	}
}

// The Open dialog's folder listing offers what every open route opens — PDFs and every row of the format
// table (ADR-215: the Door subset is retired, every row opens at every door) — and not a file it would refuse.
func TestListDirOffersWhatTheDoorsOpen(t *testing.T) {
	ts, _ := startServer(t)
	c, _ := authedClient(t, ts)
	dir := t.TempDir()
	for _, n := range []string{"a.pdf", "B.PNG", "c.jpeg", "d.tif", "e.webp", "f.docx", "g.md", "h.txt", "i.gif", "j.bmp", "k.cbz", "l.exe", "m.zip", "n"} {
		os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644)
	}
	resp, err := c.Get(ts.URL + "/api/listdir?path=" + dir)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var info listDirResponse
	json.NewDecoder(resp.Body).Decode(&info)
	var got []string
	for _, f := range info.Files {
		got = append(got, f.Name)
	}
	if strings.Join(got, ",") != "B.PNG,a.pdf,c.jpeg,d.tif,e.webp,f.docx,g.md,h.txt,i.gif,j.bmp,k.cbz" {
		t.Errorf("listed %v", got)
	}
}
