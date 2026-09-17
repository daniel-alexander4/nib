package pdfops

import (
	"bytes"
	"testing"

	"github.com/pdfcpu/pdfcpu/pkg/api"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/model"
)

// ownerRestricted builds the one encrypted shape nib can actually open: an EMPTY user
// password with a non-empty owner password, which is how a producer says "anyone may read
// this, nobody may edit or print it". A non-empty user password is unreachable here —
// pdfcpu cannot read the document at all without it, so every operation fails at the read.
//
// pdfops.Encrypt cannot produce this shape (it sets the same password as both user and
// owner, so RemovePassword reverses it exactly), which is why this goes to pdfcpu directly.
func ownerRestricted(t *testing.T, pdf []byte) []byte {
	t.Helper()
	var out bytes.Buffer
	if err := api.Encrypt(bytes.NewReader(pdf), &out, model.NewAESConfiguration("", "ownersecret", 256)); err != nil {
		t.Fatalf("building the restricted fixture: %v", err)
	}
	return out.Bytes()
}

// protectionOf opens an artifact the way a reader would — with the empty user password — and
// says whether it is still protected. The open is itself an assertion: for AES-256 the
// permissions are covered by the key derivation, so pdfcpu refuses a document whose `/P` was
// altered ("invalid permissions after upw ok"). That is why nothing below compares permission
// bits: a comparison there is a dead conjunct this read has already made unreachable, proven by
// rewriting the encryption dict's `/P` in `rewriteContext` and watching the failure land here
// rather than on the comparison.
func protectionOf(t *testing.T, pdf []byte) bool {
	t.Helper()
	ctx, err := api.ReadValidateAndOptimize(bytes.NewReader(pdf), model.NewAESConfiguration("", "", 256))
	if err != nil {
		t.Fatalf("reading back the artifact: %v", err)
	}
	return ctx.E != nil
}

// TestARewriteKeepsTheProtectionOfTheDocumentItRewrote grades what `/pending 527` found
// ungraded: since the page selection moved into the source context, a rewrite re-emits the
// source's `/Encrypt`, where the old fresh-context path silently dropped it and handed back an
// unprotected copy of a restricted document.
//
// **Preserving is the decision, and it is anchored on the one door.** `rewriteContext` is the
// single read-change-write both `writeMutated` and `rewriteWithConf` share (ADR-009), so this
// is a property of every operation that rewrites and not of the subset alone — which is why the
// two arms below drive one of each. Nib already has an explicit, named door for taking
// protection off (`RemovePassword`, "Remove password protection"); an unrelated page reorder
// quietly stripping a document's owner restrictions is the surprising behaviour, not the
// preserved one.
//
// Both arms are needed. "Still encrypted" alone passes for a door that encrypts everything it
// touches, and "still plain" alone passes for one that encrypts nothing.
func TestARewriteKeepsTheProtectionOfTheDocumentItRewrote(t *testing.T) {
	plain := pagesPDF(t, 3)
	restricted := ownerRestricted(t, plain)

	if !protectionOf(t, restricted) {
		t.Fatal("the fixture is not encrypted, so this test cannot see the thing it is for")
	}

	for _, op := range []struct {
		name string
		run  func([]byte) ([]byte, error)
	}{
		// A subset, through rewriteWithConf.
		{"Collect", func(b []byte) ([]byte, error) { return Collect(b, []string{"3", "1"}) }},
		{"RemovePages", func(b []byte) ([]byte, error) { return RemovePages(b, []string{"2"}) }},
		// A surgical mutation, through writeMutated — the other caller of the same door.
		{"StripMetadata", StripMetadata},
	} {
		t.Run(op.name+" keeps a restricted document restricted", func(t *testing.T) {
			out, err := op.run(restricted)
			if err != nil {
				t.Fatalf("%s on a restricted document: %v", op.name, err)
			}
			if !protectionOf(t, out) {
				t.Fatalf("%s handed back an UNPROTECTED copy of a restricted document", op.name)
			}
		})

		t.Run(op.name+" leaves a plain document plain", func(t *testing.T) {
			out, err := op.run(plain)
			if err != nil {
				t.Fatalf("%s on a plain document: %v", op.name, err)
			}
			if protectionOf(t, out) {
				t.Fatalf("%s encrypted a document that was not protected", op.name)
			}
		})
	}
}
