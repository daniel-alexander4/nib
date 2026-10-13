package sign

import (
	"bytes"
	"encoding/hex"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
	"unicode/utf16"

	dpdf "github.com/digitorus/pdf"
	psign "github.com/digitorus/pdfsign/sign"
	"github.com/pdfcpu/pdfcpu/pkg/pdfcpu/types"

	"nib/internal/testpdf"
)

// literalBreakers are the bytes a PDF literal string gives a meaning: `(` `)` nest and close it, `\`
// starts an escape, and a carriage return is read as a line feed (ISO 32000-1 7.3.4.2).
var literalBreakers = [4]byte{'(', ')', '\\', '\r'}

func breaksALiteral(u uint16) bool {
	for _, b := range literalBreakers {
		if byte(u>>8) == b || byte(u) == b {
			return true
		}
	}
	return false
}

// everyAffectedCharacter is every BMP character one of whose two UTF-16 bytes is a literalBreaker,
// and one supplementary character for each surrogate half that is.
func everyAffectedCharacter() string {
	var b strings.Builder
	for r := rune(0x80); r < 0x10000; r++ {
		if r >= 0xD800 && r <= 0xDFFF {
			continue
		}
		if breaksALiteral(uint16(r)) {
			b.WriteRune(r)
		}
	}
	for r := rune(0x10000); r <= 0x10FFFF; r += 0x101 {
		hi, lo := utf16.EncodeRune(r)
		if breaksALiteral(uint16(hi)) || breaksALiteral(uint16(lo)) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// textPdfcpuReads is the new signature's `/Name` and `/Reason` as PDFCPU reads them — a second
// parser, sharing nothing with the `digitorus/pdf` reader `Verify` reports from.
func textPdfcpuReads(t *testing.T, signed []byte) (name, reason string) {
	t.Helper()
	revs, err := sweepRevisions(signed)
	if err != nil || len(revs) == 0 {
		t.Fatalf("sweep: %v, %d records", err, len(revs))
	}
	ctx, err := pdfcpuRead(signed)
	if err != nil {
		t.Fatal(err)
	}
	d, err := ctx.DereferenceDict(*types.NewIndirectRef(int(revs[len(revs)-1].Obj), 0))
	if err != nil || d == nil {
		t.Fatalf("pdfcpu does not read the signature dictionary: %v", err)
	}
	read := func(key string) string {
		o, _ := ctx.Dereference(d[key])
		s, err := types.StringOrHexLiteral(o)
		if err != nil || s == nil {
			t.Fatalf("pdfcpu does not read /%s (%T): %v", key, o, err)
		}
		return *s
	}
	return read("Name"), read("Reason")
}

// TestTextInAnyScriptIsSignedAsTyped is /pending 879: a name or a reason is signed whatever it is
// written in, and reads back as typed through nib's verifier AND through pdfcpu. Before ADR-163 the
// first two rows were refused by nib's own read-back (`invalid escape sequence \q`, `EOF`), and the
// third was SIGNED with a `/Reason` nobody typed (U+5C31 is `\1`, an octal escape).
func TestTextInAnyScriptIsSignedAsTyped(t *testing.T) {
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	id := newIdentity(t, "Solo")
	p12 := makeP12(t, "山田 太郎", "p12pass")
	texts := map[string]string{
		"Japanese, U+5C71 is a backslash and q":       "山田 太郎",
		"Arabic, U+0628 opens a parenthesis":          "محمد العربي",
		"Chinese, U+5C31 is an octal escape":          "就这样签署",
		"Arabic, U+0629 closes the string":            "مدينة",
		"accented Latin with an unmatched bracket":    "Müller :-)",
		"accented Latin with a backslash":             `naïve C:\dir`,
		"Malayalam, every unit holds a return":        "മലയാളം",
		"a surrogate pair":                            "😀 signed",
		"untouched: accented Latin":                   "Ünïcode",
		"untouched: ASCII with every escaped byte":    "a(b)c\\d\re",
		"every affected character there is, together": everyAffectedCharacter(),
	}
	signers := map[string]func(name, reason string) ([]byte, error){
		"Sign": func(n, r string) ([]byte, error) {
			return Sign(base, id.certPEM, id.keyPEM, Options{Name: n, Reason: r, When: time.Now()})
		},
		"SignApproval": func(n, r string) ([]byte, error) {
			return SignApproval(base, id.certPEM, id.keyPEM, Options{Name: n, Reason: r, When: time.Now()})
		},
		"SignExternal": func(n, r string) ([]byte, error) {
			return SignExternal(base, p12, "p12pass", Options{Name: n, Reason: r, When: time.Now()})
		},
	}
	for what, text := range texts {
		for door, signIt := range signers {
			for _, field := range []string{"name", "reason"} {
				name, reason := "Solo", "agreed"
				if field == "name" {
					name = text
				} else {
					reason = text
				}
				signed, err := signIt(name, reason)
				if err != nil {
					t.Errorf("%s, %s as the %s: %v", door, what, field, err)
					continue
				}
				st := Verify(signed)
				if st.State != Valid || len(st.Signers) != 1 {
					t.Errorf("%s, %s as the %s: state %s with %d signers, want valid/1", door, what, field, st.State, len(st.Signers))
					continue
				}
				if s := st.Signers[0]; s.Name != name || s.Reason != reason {
					t.Errorf("%s, %s as the %s: nib reads name %.40q reason %.40q", door, what, field, s.Name, s.Reason)
				}
				if n, r := textPdfcpuReads(t, signed); n != name || r != reason {
					t.Errorf("%s, %s as the %s: pdfcpu reads name %.40q reason %.40q", door, what, field, n, r)
				}
			}
		}
	}

	// The imported certificate's own name is the signature's when none is given, and it is text too.
	signed, err := SignExternal(base, p12, "p12pass", Options{Reason: "agreed", When: time.Now()})
	if err != nil {
		t.Fatalf("SignExternal under the certificate's own name: %v", err)
	}
	if st := Verify(signed); len(st.Signers) != 1 || st.Signers[0].Name != "山田 太郎" {
		t.Errorf("SignExternal under the certificate's own name: signers %+v", st.Signers)
	}
}

// TestWhatTheLibraryIsHandedIsAConformingLiteral reads the BYTES: between the library's own BOM and
// the closing parenthesis there is nothing but a backslash and the digits 0-7, so no reader has
// anything to interpret differently — and no affected character is on the count by luck.
func TestWhatTheLibraryIsHandedIsAConformingLiteral(t *testing.T) {
	// STIMULUS: the set is the one the ADR counts — 1,503 with a `(`, `)` or `\\` byte, 497 more with
	// only a carriage return.
	bmp, noCR := 0, 0
	for r := rune(0); r < 0x10000; r++ {
		if r >= 0xD800 && r <= 0xDFFF || !breaksALiteral(uint16(r)) {
			continue
		}
		bmp++
		if hi, lo := byte(r>>8), byte(r); hi != '\r' && lo != '\r' ||
			strings.IndexByte(`()\`, hi) >= 0 || strings.IndexByte(`()\`, lo) >= 0 {
			noCR++
		}
	}
	if bmp != 2000 || noCR != 1503 {
		t.Fatalf("STIMULUS: %d BMP characters hold a byte a literal string interprets, %d of them other than a "+
			"carriage return; ADR-163 says 2,000 and 1,503", bmp, noCR)
	}
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	id := newIdentity(t, "Solo")
	signed, err := SignApproval(base, id.certPEM, id.keyPEM, Options{Name: "Solo", Reason: everyAffectedCharacter(), When: time.Now()})
	if err != nil {
		t.Fatal(err)
	}
	open := []byte("/Reason (\xFE\xFF")
	at := bytes.LastIndex(signed, open)
	if at < 0 {
		t.Fatal("no /Reason opening with the library's BOM in the signed bytes")
	}
	body := signed[at+len(open):]
	end := bytes.IndexByte(body, ')')
	if end < 0 || end%4 != 0 || end == 0 {
		t.Fatalf("the reason's literal is %d bytes to its first ')', want a positive multiple of 4", end)
	}
	for i, c := range body[:end] {
		if ok := c == '\\' && i%4 == 0 || c >= '0' && c <= '7' && i%4 != 0; !ok {
			t.Fatalf("byte %d of the reason's literal is %#x — want `\\ddd` and nothing else", i, c)
		}
	}
	// An ASCII text is the library's to escape, and is handed over untouched.
	if got := libraryText("a(b)c\\d"); got != "a(b)c\\d" {
		t.Errorf("libraryText changed an ASCII text to %q", got)
	}
}

// TestASignatureWhoseTextIsNotTheTextAskedForIsRefused is the post-condition (ADR-163): a library
// that writes a `/Reason` or a `/Name` other than the one asked for — which is what the unescaped
// string did for U+5C31, and what an upstream fix would do to `libraryText`'s escapes — produces a
// refusal, never a signature over text nobody typed.
func TestASignatureWhoseTextIsNotTheTextAskedForIsRefused(t *testing.T) {
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	id := newIdentity(t, "Solo")
	for field, alter := range map[string]func(*psign.SignData){
		"reason": func(d *psign.SignData) { d.Signature.Info.Reason = "something else" },
		"name":   func(d *psign.SignData) { d.Signature.Info.Name = "someone else" },
	} {
		withLibrary(t, func(i io.ReadSeeker, w io.Writer, r *dpdf.Reader, n int64, d psign.SignData) error {
			alter(&d)
			return psign.Sign(i, w, r, n, d)
		}, func() {
			out, err := SignApproval(base, id.certPEM, id.keyPEM, Options{Name: "Solo", Reason: "agreed", When: time.Now()})
			if !errors.Is(err, ErrSignedOutputUnreadable) || out != nil {
				t.Errorf("a library that wrote another %s: err=%v, %d bytes returned — want ErrSignedOutputUnreadable and nothing", field, err, len(out))
			} else if !strings.Contains(err.Error(), field) {
				t.Errorf("the refusal does not name the %s: %v", field, err)
			}
		})
	}
	// CONTROL: the same wrapper changing nothing signs.
	withLibrary(t, psign.Sign, func() {
		if _, err := SignApproval(base, id.certPEM, id.keyPEM, Options{Name: "Solo", Reason: "agreed", When: time.Now()}); err != nil {
			t.Errorf("CONTROL: %v", err)
		}
	})
}

// TestAnotherToolsTextIsReadWhicheverWayItIsWritten is the READ side of the same class: a signature
// made elsewhere writes its `/Reason` as a hex string, or as a literal with its bytes escaped, and
// nib's verifier reports the text either way. The swap is inside the signature's coverage, so the
// signature no longer verifies — what is read here is the reason, and that it is still reported.
func TestAnotherToolsTextIsReadWhicheverWayItIsWritten(t *testing.T) {
	const text = "山田 太郎 (就) محمد"
	var be []byte
	for _, u := range utf16.Encode([]rune(text)) {
		be = append(be, byte(u>>8), byte(u))
	}
	escaped := "(\xFE\xFF"
	for _, c := range be {
		switch c {
		case '(', ')', '\\':
			escaped += "\\" + string(rune(c))
		case '\r':
			escaped += `\r`
		default:
			escaped += string([]byte{c})
		}
	}
	escaped += ")"
	forms := map[string]string{
		"a hex string":                 "<FEFF" + strings.ToUpper(hex.EncodeToString(be)) + ">",
		"a hex string in lower case":   "<feff" + hex.EncodeToString(be) + ">",
		"a literal with escaped bytes": escaped,
		"a literal of octal escapes":   "(" + rawOctal(append([]byte{0xFE, 0xFF}, be...)) + ")",
	}
	base, err := testpdf.Form()
	if err != nil {
		t.Fatal(err)
	}
	id := newIdentity(t, "Solo")
	for what, form := range forms {
		// A placeholder of the form's own length, so the swap moves no offset.
		placeholder := strings.Repeat("x", len(form)-2)
		signed, err := SignApproval(base, id.certPEM, id.keyPEM, Options{Name: "Solo", Reason: placeholder, When: time.Now()})
		if err != nil {
			t.Fatal(err)
		}
		was := []byte("/Reason (" + placeholder + ")")
		if bytes.Count(signed, was) != 1 {
			t.Fatalf("STIMULUS %s: the placeholder reason is in the signed bytes %d times, want 1", what, bytes.Count(signed, was))
		}
		other := bytes.Replace(signed, was, []byte("/Reason "+form), 1)
		st := Verify(other)
		if len(st.Signers) != 1 {
			t.Errorf("%s: %d signers reported (state %s), want the one whose reason is read", what, len(st.Signers), st.State)
			continue
		}
		if got := st.Signers[0].Reason; got != text {
			t.Errorf("%s: nib reads the reason as %q, want %q", what, got, text)
		}
	}
}

func rawOctal(b []byte) string {
	var s strings.Builder
	for _, c := range b {
		s.WriteByte('\\')
		s.WriteByte('0' + c>>6)
		s.WriteByte('0' + c>>3&7)
		s.WriteByte('0' + c&7)
	}
	return s.String()
}
