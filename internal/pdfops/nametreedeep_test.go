package pdfops

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"nib/internal/pdfread"
	"nib/internal/testpdf"
)

// TestTheTwoHundredAndFourthAttachmentDoesNotLoseTheOthers is `/pending 880` as it was found (ADR-164): files
// attached one at a time, in name order, through the door a user's attach reaches. pdfcpu's name tree grew a node
// deeper for every two; the 204th made it 101 deep, pdfcpu's validator stopped at 100 and dropped the tree, and
// `Attachments` listed NONE of 204 with no error while the bytes still held every one. The next rewrite of any
// kind then wrote the document without them, and its digest was the digest of a document with no files.
func TestTheTwoHundredAndFourthAttachmentDoesNotLoseTheOthers(t *testing.T) {
	base, err := testpdf.Text("hello")
	if err != nil {
		t.Fatal(err)
	}
	bare, err := ContentDigest(base)
	if err != nil {
		t.Fatal(err)
	}
	pdf := base
	for i := 1; i <= 210; i++ {
		pdf, err = AddAttachment(pdf, fmt.Sprintf("scan-%04d.txt", i), []byte(fmt.Sprintf("body %d", i)))
		if err != nil {
			t.Fatalf("attaching file %d: %v", i, err)
		}
		list, err := Attachments(pdf)
		if err != nil || len(list) != i {
			t.Fatalf("after attaching %d files the document lists %d (%v)", i, len(list), err)
		}
	}
	// An unrelated rewrite carries every one.
	out, err := SetLang(pdf, "en")
	if err != nil {
		t.Fatal(err)
	}
	list, err := Attachments(out)
	if err != nil || len(list) != 210 {
		t.Fatalf("after an unrelated rewrite the document lists %d of 210 files (%v)", len(list), err)
	}
	_, data, err := ReadAttachment(out, "scan-0001.txt")
	if err != nil || string(data) != "body 1" {
		t.Errorf("the first file reads %q (%v)", data, err)
	}
	// The digest covers the embedded files (rule 3 on), so it is not the digest of the document without them.
	if d, err := ContentDigest(out); err != nil || d == bare {
		t.Errorf("the digest of the document with 210 files is the digest of the document with none (%v)", err)
	}
}

// TestADeepTreeHidesNoFileFromNib is the hostile shape of the same loss: ONE file at the bottom of a tree 150
// nodes deep, which no nib writer makes. Before ADR-164 nib listed no files, found no ceremony record, digested the
// document as one with no files and wrote copies without the file — while another reader showed it.
func TestADeepTreeHidesNoFileFromNib(t *testing.T) {
	for _, sibling := range []bool{false, true} {
		doc := testpdf.NameTreeDoc(testpdf.NameTree{Kind: "EmbeddedFiles", Depth: 150, Sibling: sibling})
		want := 1
		if sibling {
			want = 2 // the deep kid comes first here: the validator alone kept the shallow file and dropped the deep one
		}
		list, err := Attachments(doc)
		if err != nil || len(list) != want {
			t.Fatalf("sibling=%v: lists %d files (%v) — want %d", sibling, len(list), err, want)
		}
		_, data, err := ReadAttachment(doc, "deep.txt")
		if err != nil || string(data) != "HIDDEN" {
			t.Errorf("sibling=%v: the deep file reads %q (%v)", sibling, data, err)
		}
		out, err := SetLang(doc, "en")
		if err != nil {
			t.Fatalf("sibling=%v: %v", sibling, err)
		}
		if list, err := Attachments(out); err != nil || len(list) != want {
			t.Errorf("sibling=%v: after a rewrite the document lists %d files (%v) — want %d", sibling, len(list), err, want)
		}
		if !bytes.Contains(out, []byte("HIDDEN")) && !hasAttachmentBytes(t, out, "deep.txt", "HIDDEN") {
			t.Errorf("sibling=%v: the rewrite lost the deep file's bytes", sibling)
		}
		// The digest moves with the deep file's own bytes.
		other := bytes.Replace(doc, []byte("HIDDEN"), []byte("hidden"), 1)
		d1, err1 := ContentDigest(doc)
		d2, err2 := ContentDigest(other)
		if err1 != nil || err2 != nil || d1 == d2 {
			t.Errorf("sibling=%v: two documents that differ only in the deep file's bytes digest alike (%v, %v)", sibling, err1, err2)
		}
		// Remove files and media removes it, and the bytes no longer hold it.
		gone, err := RemoveFilesAndMedia(doc)
		if err != nil {
			t.Fatalf("sibling=%v: %v", sibling, err)
		}
		if list, err := Attachments(gone); err != nil || len(list) != 0 {
			t.Errorf("sibling=%v: after Remove files the document lists %d (%v)", sibling, len(list), err)
		}
		if bytes.Contains(gone, []byte("HIDDEN")) || bytes.Contains(gone, []byte("EmbeddedFiles")) {
			t.Errorf("sibling=%v: after Remove files the bytes still hold the file or its tree", sibling)
		}
	}
	// The same for the two other trees nib reads: a deep destination tree keeps its name through a rewrite, and a
	// deep script tree is seen by the digest and removed by Remove active content.
	js := testpdf.NameTreeDoc(testpdf.NameTree{Kind: "JavaScript", Depth: 150})
	stripped, err := StripActive(js)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(stripped, []byte("app.alert")) {
		t.Error("Remove active content left a script that sat in a deep tree")
	}
	withScript, err1 := ContentDigest(js)
	without, err2 := ContentDigest(stripped)
	if err1 != nil || err2 != nil || withScript == without {
		t.Errorf("a document with a script in a deep tree digests as the same document without it (%v, %v)", err1, err2)
	}
}

// hasAttachmentBytes reports whether out's attachment ref decodes to want (the rewrite may compress the stream).
func hasAttachmentBytes(t *testing.T, out []byte, ref, want string) bool {
	t.Helper()
	_, data, err := ReadAttachment(out, ref)
	return err == nil && string(data) == want
}

// TestAnUnreadTreeIsNeverCalledEmpty is the rule for what nib still cannot read (ADR-164): a tree whose `/Kids`
// loop is dropped by pdfcpu's validator, and every reader that used to answer "none" answers that it could not
// read them; every write is refused, naming its cause; and removing the files — the one thing a user can still
// ask for — removes them.
func TestAnUnreadTreeIsNeverCalledEmpty(t *testing.T) {
	doc := testpdf.NameTreeDoc(testpdf.NameTree{Kind: "EmbeddedFiles", Depth: 3, Loop: true, Key: CeremonyRecordName})
	if list, err := Attachments(doc); !errors.Is(err, pdfread.ErrNameTreeUnread) {
		t.Errorf("Attachments answered %d files, %v — want ErrNameTreeUnread, never an empty list", len(list), err)
	}
	if _, _, err := ReadAttachment(doc, CeremonyRecordName); !errors.Is(err, pdfread.ErrNameTreeUnread) {
		t.Errorf("ReadAttachment answered %v — want ErrNameTreeUnread, never \"no attachment named\"", err)
	}
	if rec, err := CeremonyRecord(doc); !errors.Is(err, pdfread.ErrNameTreeUnread) {
		t.Errorf("CeremonyRecord answered %q, %v — want ErrNameTreeUnread, never \"no record\"", rec, err)
	}
	// The digest is not the digest of the same document with no tree at all.
	bare := bytes.Replace(doc, []byte("/Names << /EmbeddedFiles 4 0 R >>"), []byte("                                  "), 1)
	if list, err := Attachments(bare); err != nil || len(list) != 0 {
		t.Fatalf("the control document lists %d files (%v)", len(list), err)
	}
	d1, err1 := ContentDigest(doc)
	d2, err2 := ContentDigest(bare)
	if err1 != nil || err2 != nil || d1 == d2 {
		t.Errorf("a document whose files could not be read digests as the document with no files (%v, %v)", err1, err2)
	}
	// Every write is refused by name: a rewrite, an attach, a page operation, and a merge either way round.
	host, err := testpdf.Text("host", "second")
	if err != nil {
		t.Fatal(err)
	}
	for name, op := range map[string]func() error{
		"SetLang":       func() error { _, err := SetLang(doc, "en"); return err },
		"AddAttachment": func() error { _, err := AddAttachment(doc, "new.txt", []byte("x")); return err },
		"Rotate":        func() error { _, err := Rotate(doc, []string{"1"}, 90); return err },
		"Append to it":  func() error { _, err := Append(doc, host); return err },
		"Append it":     func() error { _, err := Append(host, doc); return err },
		"Combine":       func() error { _, err := Combine([][]byte{host, doc}); return err },
	} {
		err := op()
		if name == "AddAttachment" {
			// The attach reads the list first, and that read is the refusal.
			if !errors.Is(err, pdfread.ErrNameTreeUnread) && !errors.Is(err, pdfread.ErrNameTreeLost) {
				t.Errorf("%s answered %v — want a refusal naming the unread list", name, err)
			}
			continue
		}
		if !errors.Is(err, pdfread.ErrNameTreeLost) {
			t.Errorf("%s answered %v — want ErrNameTreeLost", name, err)
		}
	}
	if _, _, err := CarryAttachments(doc, host); !errors.Is(err, pdfread.ErrNameTreeLost) {
		t.Errorf("CarryAttachments from it answered %v — want ErrNameTreeLost, never \"nothing to carry\"", err)
	}
	// Removing the files is not refused, and removes them.
	gone, err := RemoveFilesAndMedia(doc)
	if err != nil {
		t.Fatalf("Remove files and media: %v", err)
	}
	if list, err := Attachments(gone); err != nil || len(list) != 0 {
		t.Errorf("after Remove files the document lists %d (%v)", len(list), err)
	}
	if bytes.Contains(gone, []byte("HIDDEN")) || bytes.Contains(gone, []byte("EmbeddedFiles")) {
		t.Error("after Remove files the bytes still hold the file or its tree")
	}
	// And so is removing a script tree nib could not read.
	js := testpdf.NameTreeDoc(testpdf.NameTree{Kind: "JavaScript", Depth: 3, Loop: true})
	if _, err := SetLang(js, "en"); !errors.Is(err, pdfread.ErrNameTreeLost) {
		t.Errorf("a rewrite of a document with an unread script tree answered %v — want ErrNameTreeLost", err)
	}
	stripped, err := StripActive(js)
	if err != nil {
		t.Fatalf("Remove active content: %v", err)
	}
	if bytes.Contains(stripped, []byte("app.alert")) {
		t.Error("Remove active content left the script")
	}
}
