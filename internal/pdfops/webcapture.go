package pdfops

import (
	"errors"

	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/pdfread"
)

// Web Capture (ISO 32000-1 §14.10) is a record of the web pages a document was converted from: the catalog's
// `/URLS` and `/IDS` name trees, its `/SpiderInfo`, and each page's `/ID` and `/PZ`. Nothing a reader shows comes
// from it. pdfcpu's validator drops a leaf of `/URLS` holding a value it refuses, so ADR-164 marks the tree unread
// and every write of such a document is refused — fill, rotate, anything (/pending 889 part 2: one real document
// in 3,785, 333 of its 371 names kept). The record is never carried across the validator; it is removed when the
// user asks, and only then.

// ErrWebCaptureRemains is RemoveWebCapture's refusal: its own output still scans as carrying the record.
var ErrWebCaptureRemains = errors.New("a web capture record remains after removing it")

// RemoveWebCapture removes the whole Web Capture record — both trees, `/SpiderInfo`, every page's `/ID` and `/PZ`
// — through the one removal door for a tree Nib could not read whole (`pdfread.ForgetNameTree`, its third named
// caller), and verifies the result as the other removals do. Like them it rewrites the file, so a signature is
// lost; the window asks first.
func RemoveWebCapture(pdf []byte) ([]byte, error) {
	out, err := writeMutated(pdf, func(ctx *model.Context) error {
		xt := ctx.XRefTable
		root, err := xt.Catalog()
		if err != nil {
			return err
		}
		if names := derefDict(xt, root["Names"]); names != nil {
			dropKey(xt, names, "URLS")
			dropKey(xt, names, "IDS")
		}
		// pdfcpu's parsed copies, which its writer binds back into /Names (StripActive's reason, /pending 729).
		delete(xt.Names, "URLS")
		delete(xt.Names, "IDS")
		pdfread.ForgetNameTree(xt, "URLS")
		pdfread.ForgetNameTree(xt, "IDS")
		dropKey(xt, root, "SpiderInfo")
		return eachPage(xt, root, func(page types.Dict, _ int) {
			dropKey(xt, page, "ID")
			dropKey(xt, page, "PZ")
		})
	})
	if err != nil {
		return nil, err
	}
	if err := verifyRemoved(out, ErrWebCaptureRemains, webCapture); err != nil {
		return nil, err
	}
	return out, nil
}

// webCapture is RemoveWebCapture's remit as Scan reports it.
func webCapture(f Finding) bool { return f.Kind == "webCapture" }

// hasWebCapture is Scan's test for the record: present if any part of it is.
func hasWebCapture(xt *model.XRefTable, root types.Dict) bool {
	if names := derefDict(xt, root["Names"]); names != nil {
		if _, ok := names.Find("URLS"); ok {
			return true
		}
		if _, ok := names.Find("IDS"); ok {
			return true
		}
	}
	if _, ok := root.Find("SpiderInfo"); ok {
		return true
	}
	found := false
	_ = eachPage(xt, root, func(page types.Dict, _ int) {
		_, id := page.Find("ID")
		_, pz := page.Find("PZ")
		found = found || id || pz
	})
	return found
}
