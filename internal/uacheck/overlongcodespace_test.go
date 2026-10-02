package uacheck

import (
	"bytes"
	"compress/flate"
	"strings"
	"testing"
)

// TestAnOverlongCodespaceRangeIsRefusedNotFatal — a CMap whose one codespace range is a megabyte long, compressing
// to ~4 KB, overflowed the stack inside `CheckForUA` (fatal: the `recover` in `runOne` cannot hold it, so the whole
// process and every open document went with it). The check now returns, and the font's codes are refused by name.
func TestAnOverlongCodespaceRangeIsRefusedNotFatal(t *testing.T) {
	const n = 1 << 20
	body := "1 begincodespacerange <" + strings.Repeat("00", n) + "> <" + strings.Repeat("FF", n) + "> endcodespacerange "
	var z bytes.Buffer
	w, _ := flate.NewWriter(&z, 9)
	w.Write([]byte(body))
	w.Close()
	if z.Len() > 8<<10 {
		t.Fatalf("the hostile CMap compresses to %d bytes; the point is that it is small", z.Len())
	}
	sys := "/CIDSystemInfo << /Registry (Adobe) /Ordering (GB1) /Supplement 2 >>"
	extra := map[int]string{20: cmapStream("/WMode 1 "+sys, "Cust", "/WMode 1 def "+body)}
	extra[3] = "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200] /Contents 4 0 R /StructParents 0 /Resources << /Font << /F0 10 0 R >> >> >>"
	extra[4] = spStream("", "/P <</MCID 0>> BDC BT 10 10 Td /F0 12 Tf <2121> Tj ET EMC")
	rep, _, err := CheckForUA(buildPDF(type0Doc("20 0 R", sys, extra)))
	if err != nil {
		t.Fatal(err)
	}
	var named int
	for _, r := range rep.Results {
		if strings.Contains(r.Why, "longer than any character code") {
			if r.Verdict == Pass {
				t.Errorf("%s passes while saying it could not read the font's codes: %+v", r.Clause, r)
			}
			named++
		}
	}
	if named == 0 {
		t.Fatalf("no clause refused the overlong range by name: %+v", rep.Results)
	}
}
