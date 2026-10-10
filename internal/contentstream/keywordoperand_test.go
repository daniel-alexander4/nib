package contentstream

import "testing"

// TestABooleanOrNullIsAnOperand — /pending 632. `true`, `false` and `null` are objects (ISO 32000-1 §7.3.2,
// §7.3.9) and were emitted as Operator tokens, so a walker taking an operator as the end of an operand run cut
// a property list in two at a boolean. Every other keyword is still an operator.
func TestABooleanOrNullIsAnOperand(t *testing.T) {
	src := []byte("/P <</MCID 3 /X true /Y false /Z null>> BDC truer q EMC")
	want := map[string]Kind{"true": Operand, "false": Operand, "null": Operand,
		"BDC": Operator, "truer": Operator, "q": Operator, "EMC": Operator}
	seen := map[string]bool{}
	for _, tk := range Tokenize(src) {
		word := string(tk.Bytes(src))
		if k, ok := want[word]; ok {
			seen[word] = true
			if tk.Kind != k {
				t.Errorf("%q is tokenized as %v, want %v", word, tk.Kind, k)
			}
		}
	}
	if len(seen) != len(want) {
		t.Fatalf("only %d of %d words were found as tokens of their own: %v", len(seen), len(want), seen)
	}
}
