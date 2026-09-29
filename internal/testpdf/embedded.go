package testpdf

import (
	"bytes"
	"fmt"
	"strings"
)

// Embedded is one entry of the catalog's /Names /EmbeddedFiles tree as WithEmbedded writes it: the
// tree KEY, the filespec's /F and /UF (an empty one is left out), and the file's bytes.
type Embedded struct {
	Key, F, UF string
	Data       string
}

// WithEmbedded is a one-page document whose embedded-files tree holds exactly the given entries, in
// the given order, each with its own filespec and its own uncompressed stream.
//
// It exists because every interesting shape here is one pdfcpu refuses to write (/pending 745): a
// key and a /UF that disagree, two entries sharing a /UF, the same key twice. Each is an ordinary
// PDF a counterparty can hand over, so it is written by hand, the way WithContent is.
func WithEmbedded(entries ...Embedded) []byte {
	lit := func(s string) string {
		r := strings.NewReplacer(`\`, `\\`, `(`, `\(`, `)`, `\)`)
		return "(" + r.Replace(s) + ")"
	}
	objs := []string{
		"", // 1: catalog, written last when the entry objects are numbered
		"<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		"<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Contents 4 0 R >>",
		"<< /Length 0 >>\nstream\n\nendstream",
	}
	var names strings.Builder
	for _, e := range entries {
		stream := len(objs) + 1
		objs = append(objs, fmt.Sprintf("<< /Type /EmbeddedFile /Length %d >>\nstream\n%s\nendstream", len(e.Data), e.Data))
		fs := "<< /Type /Filespec"
		if e.F != "" {
			fs += " /F " + lit(e.F)
		}
		if e.UF != "" {
			fs += " /UF " + lit(e.UF)
		}
		fs += fmt.Sprintf(" /EF << /F %d 0 R /UF %d 0 R >> >>", stream, stream)
		objs = append(objs, fs)
		fmt.Fprintf(&names, " %s %d 0 R", lit(e.Key), len(objs))
	}
	objs[0] = "<< /Type /Catalog /Pages 2 0 R /Names << /EmbeddedFiles << /Names [" + names.String() + " ] >> >> >>"
	var b bytes.Buffer
	b.WriteString("%PDF-1.7\n")
	offs := make([]int, len(objs))
	for i, o := range objs {
		offs[i] = b.Len()
		fmt.Fprintf(&b, "%d 0 obj\n%s\nendobj\n", i+1, o)
	}
	xref := b.Len()
	fmt.Fprintf(&b, "xref\n0 %d\n0000000000 65535 f \n", len(objs)+1)
	for _, o := range offs {
		fmt.Fprintf(&b, "%010d 00000 n \n", o)
	}
	fmt.Fprintf(&b, "trailer\n<< /Size %d /Root 1 0 R >>\nstartxref\n%d\n%%%%EOF\n", len(objs)+1, xref)
	return b.Bytes()
}
