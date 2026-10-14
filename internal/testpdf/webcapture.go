package testpdf

import (
	"fmt"
	"strings"
)

// WebCapture says which parts of a Web Capture record (ISO 32000-1 §14.10) WebCaptured writes.
type WebCapture struct {
	URLS, IDS, SpiderInfo, Page bool
	// Refused gives the second `/URLS` leaf's content set an `/S` pdfcpu's validator refuses, so it drops that
	// leaf and nib marks the tree unread — the shape of the one real document of /pending 889 part 2, which kept
	// 333 of its 371 names.
	Refused bool
}

// WholeWebCapture is every part of the record.
var WholeWebCapture = WebCapture{URLS: true, IDS: true, SpiderInfo: true, Page: true}

// WebCaptured is a one-page document carrying the parts of a Web Capture record w names: `/URLS` with two leaves,
// `/IDS`, the catalog's `/SpiderInfo`, and the page's `/ID` and `/PZ`.
func WebCaptured(w WebCapture) []byte {
	const id = "<0102030405060708090a0b0c0d0e0f10>"
	s := "SPS"
	if w.Refused {
		s = "Bogus"
	}
	var catalog, names, page strings.Builder
	if w.URLS {
		names.WriteString(" /URLS 10 0 R")
	}
	if w.IDS {
		names.WriteString(" /IDS 13 0 R")
	}
	catalog.WriteString("<< /Type /Catalog /Pages 2 0 R")
	if w.SpiderInfo {
		catalog.WriteString(" /SpiderInfo << /V 1.0 >>")
	}
	if names.Len() > 0 {
		catalog.WriteString(" /Names <<" + names.String() + " >>")
	}
	catalog.WriteString(" >>")
	if w.Page {
		page.WriteString(" /ID " + id + " /PZ 1")
	}
	return Assemble(map[int]string{
		1:  catalog.String(),
		2:  "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
		3:  "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 200 200]" + page.String() + " >>",
		10: "<< /Kids [11 0 R 12 0 R] >>",
		11: "<< /Limits [(http://a.example/) (http://a.example/)] /Names [(http://a.example/) 20 0 R] >>",
		12: "<< /Limits [(http://b.example/) (http://b.example/)] /Names [(http://b.example/) 21 0 R] >>",
		13: "<< /Names [" + id + " 20 0 R] >>",
		20: "<< /Type /SpiderContentSet /S /SPS /ID " + id + " /O [3 0 R] /SI << /AU (http://a.example/) >> >>",
		21: fmt.Sprintf("<< /Type /SpiderContentSet /S /%s /ID <1112131415161718191a1b1c1d1e1f20> /O [3 0 R] /SI << /AU (http://b.example/) >> >>", s),
	})
}
