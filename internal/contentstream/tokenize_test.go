package contentstream

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

// roundTrips asserts the package's central promise on one stream: tokenize, write, get the SAME
// bytes — and that the tokens cover the stream completely and without overlap, which is what makes
// the first true.
//
// The coverage check is not redundant with the byte comparison. A tokenizer that dropped a span and
// a writer that re-invented an equivalent one would pass a byte comparison and be exactly the
// re-serialising design this package exists to avoid.
func roundTrips(t *testing.T, name string, src []byte) []Token {
	t.Helper()
	toks := Tokenize(src)
	if len(src) > 0 && len(toks) == 0 {
		t.Fatalf("%s: %d bytes tokenized to nothing", name, len(src))
	}
	prev := 0
	for i, tk := range toks {
		if tk.Start != prev {
			t.Fatalf("%s: token %d starts at %d, previous ended at %d — the tokens do not cover "+
				"the stream, so WriteTokens is re-inventing the gap rather than copying it",
				name, i, tk.Start, prev)
		}
		if tk.End <= tk.Start {
			t.Fatalf("%s: token %d is empty (%s) — a zero-width token means the scanner did not "+
				"advance and the stream is being walked by luck", name, i, tk.Describe(src))
		}
		prev = tk.End
	}
	if prev != len(src) {
		t.Fatalf("%s: tokens cover %d of %d bytes — the tail is not in any token", name, prev, len(src))
	}
	out, err := WriteTokens(src, toks)
	if err != nil {
		t.Fatalf("%s: WriteTokens: %v", name, err)
	}
	if !bytes.Equal(out, src) {
		t.Fatalf("%s: the round trip is not byte-identical\n  in  %q\n  out %q", name, src, out)
	}
	return toks
}

// TestTheRoundTripIsByteIdentical — `PLAN-accessibility.md` P05.S01, law 1.
//
// **Byte-identical, not equivalent.** A walker that re-emits `1.0` as `1` has already lost the
// argument for every later slice, because nothing downstream can then tell its own change from the
// walker's — and a document whose bytes moved for no reason is a document whose signature stopped
// verifying for no reason.
//
// The cases are chosen to be the ones a re-serialising design gets wrong: the lexical forms nobody
// normalises the same way twice.
func TestTheRoundTripIsByteIdentical(t *testing.T) {
	for _, c := range []struct{ name, src string }{
		{"empty", ""},
		{"one operator", "q"},
		{"the corpus fixture's page", "/P <</MCID 0>> BDC\nBT /F1 24 Tf 72 700 Td (Tagged heading) Tj ET\nEMC\n"},

		// Numbers nobody normalises the same way twice.
		{"trailing-point number", "1. 0 0 1. 0 0 cm"},
		{"leading-point number", ".5 w"},
		{"explicit plus", "+3 -4 re"},
		{"redundant zeros", "1.00000 0.00000 0.00000 1.00000 72.00000 693.00000 cm"},
		{"many decimals", "0.123456789012345 w"},

		// Whitespace nobody preserves by accident.
		{"CRLF", "q\r\nQ"},
		{"CR only", "q\rQ"},
		{"tabs and runs", "q \t\t  \n\n Q"},
		{"leading whitespace", "\n\n  q"},
		{"trailing whitespace", "q\n\n  "},
		{"comment", "q % this is a comment\nQ"},
		{"comment at end with no newline", "q % trailing"},

		// Names with escapes, which a decode-and-re-encode design mangles.
		{"name with hash escape", "/A#20B Do"},
		{"name with many escapes", "/Pa#67e#20One BDC"},
		{"empty name", "/ Do"},

		// Dictionaries and arrays, whose spacing is never canonical.
		{"tight dict", "<</MCID 0>>BDC"},
		{"loose dict", "<<  /MCID   0  >> BDC"},
		{"nested dict", "<</A<</B 1>>>> BDC"},
		{"array", "[(a) -20 (b)] TJ"},
		{"tight array", "[(a)-20(b)]TJ"},

		// Strings — the case nib's own output makes load-bearing.
		{"escaped paren", `(a\)b) Tj`},
		{"escaped backslash", `(a\\b) Tj`},
		{"backslash before close", `(a\\) Tj`},
		{"nested parens", "((inner)) Tj"},
		{"escaped open only", `(a\(b) Tj`},
		{"octal escape", `(\101\102) Tj`},
		{"newline in string", "(line\nbreak) Tj"},
		{"empty string", "() Tj"},
		{"hex string", "<48656C6C6F> Tj"},
		{"empty hex string", "<> Tj"},
		{"hex with whitespace", "<48 65 6C>Tj"},

		// Unterminated constructs: the walker must still return every byte.
		{"unterminated string", "(never closed"},
		{"unterminated hex", "<4865"},
		{"lone delimiters", "} { ) >"},
	} {
		roundTrips(t, c.name, []byte(c.src))
	}
}

// TestTheRoundTripSurvivesBinaryInsideAString is its own test because it is the case MEASURED on
// nib's own output rather than imagined.
//
// At v1.129.45 a one-page Markdown conversion carries **79 NUL bytes and a `\\` escape** inside its
// literal strings, because P04 made every drawn glyph a two-byte index into an embedded font. Any
// glyph whose index happens to be 0x28 or 0x29 puts a raw paren in there too, and 0x5C a raw
// backslash. These are not hypothetical documents — they are what nib produces today.
func TestTheRoundTripSurvivesBinaryInsideAString(t *testing.T) {
	// Every byte value, inside one string, escaped the way a writer must escape them.
	var body []byte
	for b := 0; b < 256; b++ {
		switch byte(b) {
		case '(', ')', '\\':
			body = append(body, '\\', byte(b))
		default:
			body = append(body, byte(b))
		}
	}
	src := append(append([]byte("BT /F1 12 Tf ("), body...), []byte(") Tj ET")...)
	toks := roundTrips(t, "every byte value in a string", src)

	// And the string must be ONE token: if the scanner stopped early, the rest of the payload is
	// being read as operators, which is the corruption this guards.
	found := false
	for _, tk := range toks {
		if tk.Kind == LiteralString {
			if found {
				t.Fatal("the payload produced TWO literal strings — the scanner ended the first " +
					"one early, so image-like bytes are being read as operators")
			}
			found = true
			if got, want := tk.End-tk.Start, len(body)+2; got != want {
				t.Errorf("the literal string spans %d bytes, want %d — the escape or paren-balance "+
					"rule is wrong on a payload nib's own documents contain", got, want)
			}
		}
	}
	if !found {
		t.Fatal("no literal string token at all")
	}
}

// TestAnInlineImageIsOneOpaqueToken — P05.S01.T02.
//
// After `ID` the bytes are raw binary whose length the content stream never states. A tokenizer
// without a special case reads them as operators; the corpus contains no inline image at all
// (measured: `BI` = 0 in both fixtures), so the case has to be built.
//
// The payload here deliberately contains bytes that LOOK like content-stream syntax — an unbalanced
// paren, a `<<`, the letters `EI` without whitespace around them, a `%` — because that is the whole
// failure mode.
func TestAnInlineImageIsOneOpaqueToken(t *testing.T) {
	payload := []byte("\x00\xffEI(unbalanced <</A 1>> % not a comment\x01\x02EIx\xfe")
	src := append(append([]byte("q BI /W 4 /H 4 /BPC 8 /CS /G ID "), payload...), []byte("\nEI Q")...)

	toks := roundTrips(t, "inline image", src)

	var img *Token
	for i := range toks {
		if toks[i].Kind == InlineImage {
			if img != nil {
				t.Fatal("two inline-image tokens for one image")
			}
			img = &toks[i]
		}
	}
	if img == nil {
		t.Fatal("the inline image was not recognised at all, so its payload was tokenized as " +
			"operators — every byte of it is now something a splice could land inside")
	}
	got := img.Bytes(src)
	if !bytes.HasPrefix(got, []byte("BI ")) || !bytes.HasSuffix(got, []byte("EI")) {
		t.Errorf("the inline-image token is %q, which does not span BI…EI", got)
	}
	if !bytes.Contains(got, payload) {
		t.Errorf("the token does not contain the whole payload — it ended early at an `EI` inside "+
			"the binary.\n  token   %q\n  payload %q", got, payload)
	}
	// No operator token may start inside the image's bytes.
	for _, tk := range toks {
		if tk.Kind == Operator && tk.Start > img.Start && tk.Start < img.End {
			t.Errorf("operator token %s lies INSIDE the inline image", tk.Describe(src))
		}
	}
}

// TestApplyWithNoEditsReturnsTheOriginalBytes.
//
// An operation that decides it has nothing to change must cost the document nothing — not a
// re-serialisation that happens to look the same today. Asserted on identity of the SLICE, not just
// equality, because "the same bytes" and "a copy that compares equal" differ the day someone adds a
// normalisation step.
func TestApplyWithNoEditsReturnsTheOriginalBytes(t *testing.T) {
	src := []byte("q 1 0 0 1 0 0 cm Q")
	out, err := NewEdit(src).Apply()
	if err != nil {
		t.Fatal(err)
	}
	if &out[0] != &src[0] || len(out) != len(src) {
		t.Errorf("Apply with no insertions returned a copy (%q) rather than the original slice", out)
	}
}

// TestInsertionsUseOriginalCoordinates is the property that stops the off-by-one factory: a caller
// bracketing a span computes both offsets from the stream it can see, not from the stream as it
// will be after the first insertion lands.
func TestInsertionsUseOriginalCoordinates(t *testing.T) {
	src := []byte("BT (hi) Tj ET")
	// Bracket the whole thing, offsets both taken from `src`.
	out, err := NewEdit(src).
		InsertBefore(0, []byte("/P <</MCID 0>> BDC ")).
		InsertBefore(len(src), []byte(" EMC")).
		Apply()
	if err != nil {
		t.Fatal(err)
	}
	want := "/P <</MCID 0>> BDC BT (hi) Tj ET EMC"
	if string(out) != want {
		t.Errorf("got  %q\nwant %q", out, want)
	}
	// The original content must be present, contiguous and unaltered, between the brackets.
	if !bytes.Contains(out, src) {
		t.Errorf("the wrapped content is no longer contiguous in the result: %q", out)
	}
}

// TestTwoInsertionsAtTheSameOffsetKeepCallOrder — a bracket whose opener and closer land at the
// same point must not come out inside-out.
func TestTwoInsertionsAtTheSameOffsetKeepCallOrder(t *testing.T) {
	src := []byte("X")
	out, err := NewEdit(src).
		InsertBefore(0, []byte("A")).
		InsertBefore(0, []byte("B")).
		InsertBefore(0, []byte("C")).
		Apply()
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != "ABCX" {
		t.Errorf("got %q, want \"ABCX\" — insertions at one offset lost their call order", out)
	}
}

// TestAnInsertionOutsideTheStreamIsRefused, and TestWriteRefusesForeignTokens: a caller's mistake
// must not produce a plausible-looking corrupted stream.
func TestAnInsertionOutsideTheStreamIsRefused(t *testing.T) {
	src := []byte("q Q")
	for _, at := range []int{-1, len(src) + 1, 99} {
		if _, err := NewEdit(src).InsertBefore(at, []byte("x")).Apply(); err == nil {
			t.Errorf("an insertion at %d in a %d-byte stream was accepted", at, len(src))
		}
	}
	// The boundary itself is legal: inserting at len(src) appends.
	if _, err := NewEdit(src).InsertBefore(len(src), []byte("x")).Apply(); err != nil {
		t.Errorf("inserting at the end of the stream was refused: %v", err)
	}
}

func TestWriteRefusesForeignTokens(t *testing.T) {
	long := []byte("q 1 0 0 1 0 0 cm Q")
	short := []byte("q")
	if _, err := WriteTokens(short, Tokenize(long)); err == nil {
		t.Error("tokens from one stream were written against a shorter one without complaint — " +
			"the offsets address a different document")
	}
}

// TestTokenizeTerminatesOnEveryTruncation is the crash floor: a content stream truncated at any
// byte must still tokenize and round-trip. Producers truncate, transports truncate, and a walker
// that hangs or panics on one takes the whole operation with it.
func TestTokenizeTerminatesOnEveryTruncation(t *testing.T) {
	full := []byte("q BI /W 2 /H 2 ID \x00\x01\xff\nEI /P <</MCID 0>> BDC BT (a\\)b) Tj ET EMC Q")
	for n := 0; n <= len(full); n++ {
		roundTrips(t, fmt.Sprintf("truncated at %d", n), full[:n])
	}
}

// TestOperandsAndOperatorsAreTold apart well enough for a caller to find an operator by name, which
// is what S04 does to locate where page content begins.
func TestOperandsAndOperatorsAreTold(t *testing.T) {
	src := []byte("/P <</MCID 0>> BDC BT 1 0 0 1 72 700 Tm (x) Tj ET EMC")
	var ops []string
	for _, tk := range Tokenize(src) {
		if tk.Kind == Operator {
			ops = append(ops, string(tk.Bytes(src)))
		}
	}
	want := []string{"BDC", "BT", "Tm", "Tj", "ET", "EMC"}
	if strings.Join(ops, " ") != strings.Join(want, " ") {
		t.Errorf("operators = %v, want %v", ops, want)
	}
}

// TestAnInlineImageDictKeyIsNotTheDataMarker — found by reading the diff, not by a failing test.
//
// `ID` marks where an inline image's binary payload begins. The check that finds it asked only that
// the preceding byte not be a regular character — and `/` is a delimiter, so a dictionary key
// spelled **`/ID`** passed it. Everything after that key would have been swallowed as image data:
// the operators following the image, the rest of the page, silently, as one opaque token.
//
// Both spellings are driven, because the fix has to reject `/ID` WITHOUT rejecting the tight-array
// form `]ID`, which is legal and has no whitespace before the keyword either.
func TestAnInlineImageDictKeyIsNotTheDataMarker(t *testing.T) {
	for _, c := range []struct {
		name    string
		src     string
		wantEnd string // the bytes the inline-image token must end with
	}{
		{
			name:    "a key spelled /ID must not be mistaken for the marker",
			src:     "q BI /W 2 /H 2 /ID /Whatever ID \x01\x02\x03\nEI Q",
			wantEnd: "\nEI",
		},
		{
			name:    "the tight array form ]ID is still the marker",
			src:     "q BI /W 2 /D[1 0]ID \x01\x02\x03\nEI Q",
			wantEnd: "\nEI",
		},
	} {
		src := []byte(c.src)
		roundTrips(t, c.name, src)
		var img *Token
		for i, tk := range Tokenize(src) {
			if tk.Kind == InlineImage {
				img = &Tokenize(src)[i]
				break
			}
		}
		if img == nil {
			t.Errorf("%s: no inline-image token", c.name)
			continue
		}
		got := img.Bytes(src)
		if !bytes.HasSuffix(got, []byte(c.wantEnd)) {
			t.Errorf("%s: the image token is %q, which does not end at the real EI", c.name, got)
		}
		// The trailing ` Q` must survive as its own operator — if the marker was found in the wrong
		// place, the image token ate it.
		if !bytes.HasSuffix(src, []byte(" Q")) {
			t.Fatal("fixture error")
		}
		if img.End >= len(src)-1 {
			t.Errorf("%s: the image token runs to %d of %d bytes — it swallowed the rest of the "+
				"stream, which is exactly the /ID defect", c.name, img.End, len(src))
		}
	}
}

// tokenized renders a stream's tokens as "kind:text" for a structural comparison.
func tokenized(src []byte) []string {
	var out []string
	for _, tk := range Tokenize(src) {
		if tk.Kind == Whitespace {
			continue // whitespace is asserted by the round-trip law, not by shape
		}
		out = append(out, tk.Kind.String()+":"+string(tk.Bytes(src)))
	}
	return out
}

// TestTheTOKENIZATIONIsRight — and this test exists because the round-trip law is **not enough**,
// which was found by mutation rather than by reasoning.
//
// `TestTheRoundTripIsByteIdentical` asserts that the tokens cover the stream and that writing them
// reproduces it. **Every tokenization that covers the stream passes that** — including one that
// emits a token per byte, and including one whose string scanner stops at an escaped `)` and reads
// the remainder as operators. Three separate mutations proved it: removing the backslash-escape
// rule, removing paren nesting, and loosening the inline-image marker check all left the round-trip
// suite green, because every byte was still in some token and the writer still copied them in
// order.
//
// So the lexical rules have to be asserted as SHAPE. Each row below is a rule that a mutation left
// undetected until it was written down.
func TestTheTOKENIZATIONIsRight(t *testing.T) {
	for _, c := range []struct {
		name string
		src  string
		want []string
	}{
		// The backslash-escape rule. Without it the string ends at the escaped `)` and `b` becomes
		// an operator — a text run silently truncated, in a document that still round-trips.
		{"an escaped close paren does not end the string", `(a\)b) Tj`,
			[]string{`string:(a\)b)`, "operator:Tj"}},
		{"an escaped open paren does not open a nesting level", `(a\(b) Tj`,
			[]string{`string:(a\(b)`, "operator:Tj"}},
		{"a doubled backslash does not escape the paren after it", `(a\\) Tj`,
			[]string{`string:(a\\)`, "operator:Tj"}},

		// Paren nesting. Without it `((inner))` ends early and the trailing `)` becomes junk.
		{"parens nest", "((inner)) Tj",
			[]string{"string:((inner))", "operator:Tj"}},
		{"parens nest several deep", "(a(b(c)d)e) Tj",
			[]string{"string:(a(b(c)d)e)", "operator:Tj"}},

		// Operand/operator classification, which is how a caller finds anything.
		{"numbers are operands, keywords are operators", "1 0 0 1 72 700 Tm",
			[]string{"operand:1", "operand:0", "operand:0", "operand:1", "operand:72",
				"operand:700", "operator:Tm"}},
		{"a name is one operand", "/F1 24 Tf",
			[]string{"operand:/F1", "operand:24", "operator:Tf"}},
		{"a name with a hash escape is still one operand", "/A#20B Do",
			[]string{"operand:/A#20B", "operator:Do"}},
		{"negative and fractional numbers are operands", "-1.5 .5 +2 re",
			[]string{"operand:-1.5", "operand:.5", "operand:+2", "operator:re"}},

		// Dict and array structure.
		{"a dict is delimited, not parsed", "<</MCID 0>> BDC",
			[]string{"<<:<<", "operand:/MCID", "operand:0", ">>:>>", "operator:BDC"}},
		{"a nested dict keeps every delimiter", "<</A<</B 1>>>> BDC",
			[]string{"<<:<<", "operand:/A", "<<:<<", "operand:/B", "operand:1", ">>:>>", ">>:>>",
				"operator:BDC"}},
		{"an array is delimited", "[(a) -20 (b)] TJ",
			[]string{"[:[", "string:(a)", "operand:-20", "string:(b)", "]:]", "operator:TJ"}},
		{"a hex string is one token", "<48656C6C6F> Tj",
			[]string{"hexstring:<48656C6C6F>", "operator:Tj"}},

		// A comment is whitespace, not an operator.
		{"a comment is not an operator", "q % Tj Tf Do\nQ",
			[]string{"operator:q", "operator:Q"}},
	} {
		got := tokenized([]byte(c.src))
		if strings.Join(got, " | ") != strings.Join(c.want, " | ") {
			t.Errorf("%s\n  src  %q\n  got  %v\n  want %v", c.name, c.src, got, c.want)
		}
	}
}

// TestTheInlineImagePayloadIsNotTOKENIZED is the same lesson applied to the image case: the
// round-trip law cannot tell an opaque payload from a payload read as thirty operators.
func TestTheInlineImagePayloadIsNotTOKENIZED(t *testing.T) {
	payload := "\x00\xffEI(unbalanced <</A 1>> % not a comment\x01\x02EIx\xfe"
	src := []byte("q BI /W 4 /H 4 ID " + payload + "\nEI Q")
	got := tokenized(src)
	want := []string{"operator:q", "inlineimage:BI /W 4 /H 4 ID " + payload + "\nEI", "operator:Q"}
	if strings.Join(got, " | ") != strings.Join(want, " | ") {
		t.Errorf("the image payload was tokenized rather than kept opaque\n  got  %d token(s): %v\n  want %d: %v",
			len(got), got, len(want), want)
	}
}

// TestAKeywordIsNotAName drives the `/ID` rule on the only input where it is observable, and says
// plainly that the input is malformed.
//
// A well-formed inline-image dictionary holds names and numbers, so no whitespace-delimited `EI`
// can appear between a `/ID` key and the real `ID` marker — which means the looser check produces
// the same opaque span and every other test in this file stays green when it is reverted. **That is
// recorded rather than papered over**: the rule is a grammar correctness fix whose value is on
// malformed input, and a tokenizer that reads a broken stream should not turn it into a worse one.
func TestAKeywordIsNotAName(t *testing.T) {
	// NOT legal PDF: `EI` appears as a bare keyword inside the image dictionary.
	src := []byte("q BI /ID 1 /W 2 EI /H 2 ID \x01\x02\nEI Q")
	roundTrips(t, "a /ID key before a stray EI", src)

	var img *Token
	toks := Tokenize(src)
	for i := range toks {
		if toks[i].Kind == InlineImage {
			img = &toks[i]
			break
		}
	}
	if img == nil {
		t.Fatal("no inline-image token")
	}
	if !bytes.HasSuffix(img.Bytes(src), []byte("\nEI")) {
		t.Errorf("the image token ended at the stray EI inside the dictionary rather than at the "+
			"real one — `/ID` was taken for the data marker.\n  token %q", img.Bytes(src))
	}
}
