package testpdf

import "fmt"

// AmbiguousPageTrees are the four traced page-tree shapes (/pending 755) on which pdfops'
// one-pass walk finds exactly the pages pdfcpu counts and pdfcpu's `PageDict` answers a different
// object at some position, so no one page order exists for a ceremony's DocHash to commit to.
// pdfcpu will not write any of them; each is hand-assembled.
func AmbiguousPageTrees() map[string][]byte {
	page := func(parent int, extra string) string {
		return fmt.Sprintf("<< /Type /Page /Parent %d 0 R /MediaBox [0 0 612 792] %s >>", parent, extra)
	}
	shapes := map[string]map[int]string{
		// (a) B says 2 and holds 1, A says 1 and holds 2; the root's 3 is right. pdfcpu answers
		// B1, A1, A1 and the one walk B1, A1, A2.
		"a subtree count too small, root total right": {
			1: "<< /Type /Catalog /Pages 2 0 R >>",
			2: "<< /Type /Pages /Kids [3 0 R 5 0 R] /Count 3 >>",
			3: "<< /Type /Pages /Parent 2 0 R /Kids [4 0 R] /Count 2 >>",
			4: page(3, ""),
			5: "<< /Type /Pages /Parent 2 0 R /Kids [6 0 R 7 0 R] /Count 1 >>",
			6: page(5, ""),
			7: page(5, "/Rotate 90"),
		},
		// (b) a /Page carrying a direct /Kids with one leaf kid.
		"a page with a direct kids array": {
			1: "<< /Type /Catalog /Pages 2 0 R >>",
			2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Kids [4 0 R] >>",
			4: page(3, ""),
		},
		// (c) a /Page carrying an INDIRECT /Kids: pdfcpu's ArrayEntry is direct-only.
		"a page with an indirect kids array": {
			1: "<< /Type /Catalog /Pages 2 0 R >>",
			2: "<< /Type /Pages /Kids [3 0 R] /Count 1 >>",
			3: "<< /Type /Page /Parent 2 0 R /MediaBox [0 0 612 792] /Kids 5 0 R >>",
			4: page(3, ""),
			5: "[4 0 R]",
		},
		// (d) an empty /Pages whose /Kids is an indirect empty array. Validation returns before
		// rewriting it, and a /Count pdfcpu does not skip it by (here 1) makes PageDict answer the
		// /Pages dict itself as page 1, while the one walk passes it. With /Count 0 pdfcpu skips it
		// as well and the two agree.
		"an empty pages node with an indirect empty kids": {
			1: "<< /Type /Catalog /Pages 2 0 R >>",
			2: "<< /Type /Pages /Kids [4 0 R 3 0 R 5 0 R] /Count 2 >>",
			3: page(2, ""),
			4: "<< /Type /Pages /Parent 2 0 R /Kids 6 0 R /Count 1 >>",
			5: page(2, "/Rotate 90"),
			6: "[]",
		},
	}
	out := make(map[string][]byte, len(shapes))
	for k, v := range shapes {
		out[k] = assemble(v)
	}
	return out
}
