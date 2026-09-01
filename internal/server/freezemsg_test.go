package server

import (
	"strings"
	"testing"

	"nib/internal/ceremony"
)

// /pending 341 — the freeze refusal was false about the act at one of its three callers, and
// left the user with nowhere to go.
//
// `ceremonyFreeze` is reached from the two mutation doors AND from `handleSave`. Its message
// said *"editing it now would change the document every other party was invited to sign. Their
// copies would stop matching this one"* — accurate at the first two, and false at the third,
// where the user pressed Save and edited nothing. Worse, the second clause is backwards for a
// Save of the convened bytes: writing them is what would make the copies MATCH. A refusal that
// misnames what the user just did reads as a bug in Nib rather than a rule about the ceremony,
// which is this repo's recurring worst-defect shape — a confident false statement rather than a
// wrong computation.
//
// The second half is what the user is left holding. The commit doors mutate memory only, so a
// convened document's bytes reach disk at exactly ONE place — the mirror — while the file in
// their own folder stays the pre-ceremony draft, unsigned and carrying no record. `nib verify`
// on that file reports "unsigned" about a document under a live ceremony. Refusing without
// naming the mirror leaves them with a document they cannot save and no account of where the
// real one went.
//
// **What this deliberately does NOT decide**: whether a byte-identical write should be exempt
// from the freeze, and whether Nib should offer to write the ceremony copy into the user's own
// folder. Both change what the freeze PERMITS and are design questions; the wording and the
// signpost are defects and are what this covers.

func TestTheFreezeRefusalIsTrueOfEveryCallerAndSaysWhereTheDocumentIs(t *testing.T) {
	doc := convenedBytes(t)
	rec, err := ceremony.Extract(doc)
	if err != nil {
		t.Fatalf("setup: the fixture carries no ceremony record (%v) — the refusal below would not fire", err)
	}

	ferr := ceremonyFreeze(doc)
	if ferr == nil {
		t.Fatal("a convened document was not frozen at all — nothing below is about a refusal")
	}
	msg := ferr.Error()

	// The false clause, and the reason this item exists. "editing" named ONE caller's act and
	// was wrong at another.
	if strings.Contains(msg, "editing it now") {
		t.Errorf("the refusal still says \"editing it now\": it is reached from handleSave too, where the user pressed Save and edited nothing, so the sentence is false at one of its three callers.\n  got: %s", msg)
	}
	// And this one is not merely imprecise, it is backwards for the Save case.
	if strings.Contains(msg, "stop matching") {
		t.Errorf("the refusal still claims the parties' copies \"would stop matching this one\" — for a Save of the convened bytes that is inverted: writing them is what would make the copies match.\n  got: %s", msg)
	}

	// It must say where the document actually is, or a refused user has nowhere to go. Built
	// from the record's OWN id rather than a literal, so a refusal that names some other
	// ceremony's directory fails here.
	want := "~/nib/ceremonies/" + rec.ID + "/document.pdf"
	if !strings.Contains(msg, want) {
		t.Errorf("the refusal does not name the ceremony's own copy (%s), so a user whose Save is refused cannot find the document that was actually signed.\n  got: %s", want, msg)
	}
	// The ceremony id still leads the sentence: it is what makes both halves about the same
	// proceeding rather than a generic complaint.
	if !strings.Contains(msg, "ceremony "+rec.ID) {
		t.Errorf("the refusal no longer names the ceremony it is about.\n  got: %s", msg)
	}
}
