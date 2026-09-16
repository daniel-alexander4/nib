package pdfops

import "testing"

// TestLangTagDeclaresOnlyWhatItCanStandBehind — `/pending 471`'s door. A user-supplied language now
// reaches SetLang, so the door refuses what a screen reader would act on wrongly.
func TestLangTagDeclaresOnlyWhatItCanStandBehind(t *testing.T) {
	for _, c := range []struct{ in, want string }{
		{"de", "de"},
		{"EN-us", "en-US"},
		{"en_US", "en-US"},
		{" fr-CA ", "fr-CA"},
		{"sr-Latn-RS", "sr-Latn-RS"},
	} {
		got, err := LangTag(c.in)
		if err != nil || got != c.want {
			t.Errorf("LangTag(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
	for _, c := range []struct{ in, why string }{
		{"", "empty"},
		{"en US", "ill-formed: a space"},
		{"de) /Lang (en", "a PDF string metacharacter"},
		{"english", "a language name, not a tag"},
		{"xx", "well-formed and in no registry — a typo"},
		{"und", "BCP 47 for undetermined"},
		{"und-Latn", "undetermined, with a script"},
	} {
		if got, err := LangTag(c.in); err == nil {
			t.Errorf("LangTag(%q) accepted %q — it is %s, and declaring it is a false statement", c.in, got, c.why)
		}
	}
}

// TestSetLangIsBehindTheDoor — the door is only one if SetLang, the thing that writes, goes through
// it. A caller that skipped LangTag must still be unable to write a string the catalog reads verbatim.
func TestSetLangIsBehindTheDoor(t *testing.T) {
	if out, err := SetLang(threePagePDF(t), "de) /Evil (x"); err == nil {
		t.Fatalf("SetLang wrote a tag carrying a PDF string delimiter: /Lang is now %q", catalogLang(t, out))
	}
	out, err := SetLang(threePagePDF(t), "EN-us")
	if err != nil {
		t.Fatalf("SetLang(EN-us): %v", err)
	}
	if got := catalogLang(t, out); got != "en-US" {
		t.Errorf("SetLang(EN-us) wrote /Lang %q, want the canonical en-US", got)
	}
}

// TestEveryOCRLanguageIsAlreadyCanonical — putting the door inside SetLang must not change what the
// OCR route, its one prior caller, has always written.
func TestEveryOCRLanguageIsAlreadyCanonical(t *testing.T) {
	if len(ocrLangBCP47) == 0 {
		t.Fatal("the OCR language table is empty, so this proves nothing")
	}
	for code, tag := range ocrLangBCP47 {
		if got, err := LangTag(tag); err != nil || got != tag {
			t.Errorf("OCR language %s declares %q; through the door it becomes %q (%v)", code, tag, got, err)
		}
	}
}
