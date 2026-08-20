package discovery

import (
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"regexp"
	"strings"
	"testing"

	"nib/internal/pairing"
)

// aName is a real six-word name, produced the way the product produces one, so the
// tests exercise the encoding the wire actually carries rather than a stand-in that
// happens to have six words in it.
func aName(t *testing.T, seed byte) string {
	t.Helper()
	fp := sha256.Sum256([]byte{seed})
	n, err := pairing.Name(fp[:])
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func good(t *testing.T) Announcement {
	t.Helper()
	return Announcement{Name: aName(t, 1), Port: 8443, Nonce: [nonceLen]byte{9, 8, 7, 6, 5, 4, 3, 2}}
}

func TestAnnouncementRoundTrips(t *testing.T) {
	want := good(t)
	b, err := want.Encode()
	if err != nil {
		t.Fatal(err)
	}
	// Stimulus: the thing being parsed really is a full announcement, not an empty
	// slice that would round-trip through a parser that did nothing.
	if len(b) <= headerLen {
		t.Fatalf("encoded to %d bytes, which is no more than the header — nothing was carried", len(b))
	}
	got, err := Parse(b)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got != want {
		t.Fatalf("round trip changed the announcement:\n got %+v\nwant %+v", got, want)
	}
}

// TestTheAnnouncementCarriesTheNameAndNothingElse is the first acceptance clause,
// asserted against the ENCODER'S OUTPUT rather than by reading the struct.
//
// Reading the struct proves only that nobody added a field. What matters is what
// goes on the wire, and the two can differ — an encoder that appended a fingerprint
// "for convenience" would leave the struct definition untouched.
func TestTheAnnouncementCarriesTheNameAndNothingElse(t *testing.T) {
	fp := sha256.Sum256([]byte{1})
	name, err := pairing.Name(fp[:])
	if err != nil {
		t.Fatal(err)
	}
	a := Announcement{Name: name, Port: 8443, Nonce: [nonceLen]byte{1, 2, 3, 4, 5, 6, 7, 8}}
	b, err := a.Encode()
	if err != nil {
		t.Fatal(err)
	}

	// The fingerprint the name was derived from must not appear on the wire in any
	// form — not whole, and not truncated to the 66 bits the name encodes.
	for n := 4; n <= len(fp); n++ {
		if idx := indexOf(b, fp[:n]); idx >= 0 {
			t.Fatalf("the first %d bytes of the fingerprint appear in the announcement at offset %d "+
				"— L1 says the rendezvous path carries reachability, never identity", n, idx)
		}
	}

	// And the whole datagram is accounted for: header + name, with nothing spare.
	if got, want := len(b), headerLen+len(name); got != want {
		t.Fatalf("the announcement is %d bytes; header plus name is %d — %d bytes carry something "+
			"this test does not know about", got, want, got-want)
	}
}

func indexOf(hay, needle []byte) int {
	return strings.Index(string(hay), string(needle))
}

// TestParseRefusesEveryMalformedShape. Each case is a real datagram built from a
// valid one, so the mutation is the only difference — a hand-written byte string
// would also fail, and would not say which check caught it.
func TestParseRefusesEveryMalformedShape(t *testing.T) {
	base, err := good(t).Encode()
	if err != nil {
		t.Fatal(err)
	}
	// Stimulus: the unmutated datagram parses. Without this every row below is
	// satisfied by a parser that refuses everything.
	if _, err := Parse(base); err != nil {
		t.Fatalf("the unmutated announcement does not parse (%v) — every refusal below "+
			"would pass for the wrong reason", err)
	}

	mutate := func(f func(b []byte) []byte) []byte {
		cp := append([]byte(nil), base...)
		return f(cp)
	}

	for _, tc := range []struct {
		name string
		want error
		in   []byte
	}{
		{"empty", ErrNotOurs, nil},
		{"shorter than the header", ErrNotOurs, base[:headerLen-1]},
		{"foreign traffic", ErrNotOurs, []byte("this is an mDNS packet, honestly, and quite long")},
		{"wrong magic", ErrNotOurs, mutate(func(b []byte) []byte { b[0] = 'X'; return b })},
		{"wrong version", ErrVersion, mutate(func(b []byte) []byte { b[len(magic)] = version + 1; return b })},
		{"zero port", ErrMalformed, mutate(func(b []byte) []byte {
			binary.BigEndian.PutUint16(b[len(magic)+1:], 0)
			return b
		})},
		{"truncated name", ErrMalformed, base[:len(base)-1]},
		{"trailing bytes", ErrMalformed, append(append([]byte(nil), base...), 'x')},
		{"name length lies short", ErrMalformed, mutate(func(b []byte) []byte {
			b[headerLen-1]--
			return b
		})},
		{"over the cap", ErrMalformed, make([]byte, MaxDatagram+1)},
		{"not six words", ErrMalformed, func() []byte {
			a := good(t)
			a.Name = "only four words here"
			b := append([]byte(nil), base...)
			b = b[:headerLen-1]
			b = append(b, byte(len(a.Name)))
			return append(b, a.Name...)
		}()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Parse(tc.in)
			if err == nil {
				t.Fatalf("accepted: %+v", got)
			}
			if !errors.Is(err, tc.want) {
				t.Errorf("refused as %v, want %v — the error class is what a diagnostic "+
					"reports, and foreign traffic must not read as a malformed peer", err, tc.want)
			}
			if got != (Announcement{}) {
				t.Errorf("a refused datagram returned a non-zero announcement %+v — a caller "+
					"that ignores the error would act on attacker-chosen values", got)
			}
		})
	}
}

// TestForeignTrafficCostsNothing. The discovery socket shares a group with whatever
// else is on the link, so the overwhelmingly common input is a datagram that is not
// ours. Rejecting one must not allocate — otherwise anyone on the link chooses how
// much garbage this process makes, and the cap alone does not prevent that.
func TestForeignTrafficCostsNothing(t *testing.T) {
	foreign := make([]byte, MaxDatagram)
	copy(foreign, "not a nib announcement at all")

	// Stimulus: it really is rejected, and as foreign rather than as malformed.
	if _, err := Parse(foreign); !errors.Is(err, ErrNotOurs) {
		t.Fatalf("the fixture is not rejected as foreign (%v); the allocation count below "+
			"would be measuring a different path", err)
	}
	if n := testing.AllocsPerRun(200, func() { _, _ = Parse(foreign) }); n != 0 {
		t.Errorf("rejecting foreign traffic allocated %v times per parse — on a shared group "+
			"that is an allocation rate chosen by whoever else is on the link", n)
	}
}

// TestNothingHereCanReachAnIdentity is L1 as a source guard.
//
// The behavioural tests above check what this package DOES. This checks what it
// could ever be asked to do: L1 says nothing learned from multicast may influence
// which peer is accepted, and the structural expression of that is that no exported
// symbol here hands back a fingerprint, and that the package cannot reach the vault
// or the signing code to consult one.
//
// A guard rather than a convention because the convention is exactly what erodes:
// the tempting change is a helper that "resolves" an announcement to a peer, and it
// would look reasonable in review.
func TestNothingHereCanReachAnIdentity(t *testing.T) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, ".", func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	pkg, ok := pkgs["discovery"]
	if !ok || len(pkg.Files) == 0 {
		t.Fatal("no non-test files parsed — every check below would pass on nothing")
	}

	// 1. No import of anything that holds or decides identity.
	for name, f := range pkg.Files {
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			switch path {
			case "nib/internal/vault", "nib/internal/sign", "nib/internal/p2p":
				t.Errorf("%s imports %s. Discovery must not be able to consult, or become, "+
					"the thing that decides which peer is accepted (L1).", name, path)
			}
		}
	}

	// 2. No exported symbol names a fingerprint. `pairing.Matches` takes one and is
	// the intended comparison, but it is the CALLER's to make, holding a pin it
	// already has — the moment this package accepts a fingerprint it has become
	// part of the acceptance decision.
	fp := regexp.MustCompile(`(?i)fingerprint|spki|pubkey|publickey`)
	for name, f := range pkg.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			fd, ok := n.(*ast.FuncDecl)
			if !ok || !fd.Name.IsExported() {
				return true
			}
			var sb strings.Builder
			if fd.Type.Params != nil {
				for _, p := range fd.Type.Params.List {
					for _, nm := range p.Names {
						sb.WriteString(nm.Name + " ")
					}
				}
			}
			if fd.Type.Results != nil {
				for _, r := range fd.Type.Results.List {
					for _, nm := range r.Names {
						sb.WriteString(nm.Name + " ")
					}
				}
			}
			if fp.MatchString(sb.String()) {
				t.Errorf("%s: exported %s names an identity in its signature (%q) — L1 forbids "+
					"this package participating in who is accepted", name, fd.Name.Name, sb.String())
			}
			return true
		})
	}

	// 3. The Announcement itself carries no identity field.
	for _, f := range pkg.Files {
		ast.Inspect(f, func(n ast.Node) bool {
			ts, ok := n.(*ast.TypeSpec)
			if !ok || ts.Name.Name != "Announcement" {
				return true
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				return true
			}
			for _, fld := range st.Fields.List {
				for _, nm := range fld.Names {
					if fp.MatchString(nm.Name) {
						t.Errorf("Announcement has a field %q — the announcement carries the "+
							"NAME, which is a public encoding, and never the pin", nm.Name)
					}
				}
			}
			return true
		})
	}
}

// TestEncodeRefusesWhatParseWouldReject closes the loop Encode's own doc promises.
//
// It says "It refuses to encode one that would not parse", and bounding the name at the
// length byte's 0xff did not keep that: six 40-character words is 245 bytes, encodes to
// 261, and Parse rejects it at the 256-byte cap. The two ends of the format disagreed
// about what is legal — the one thing check() exists to prevent — and it was unreachable
// from the product only because pairing.Name is at most 53 bytes. An invariant held by a
// caller is not held by the check that claims to hold it.
func TestEncodeRefusesWhatParseWouldReject(t *testing.T) {
	// Stimulus: a name that is legal by every OTHER rule — six words, each within the
	// per-word bounds, total under the length byte's 255 — encodes and parses.
	ok := Announcement{Name: aName(t, 3), Port: 8443}
	if _, err := ok.Encode(); err != nil {
		t.Fatalf("a real name does not encode (%v); the refusal below proves nothing", err)
	}

	long := Announcement{Name: strings.TrimSpace(strings.Repeat(strings.Repeat("a", 40)+" ", 6)), Port: 8443}
	if n := len(strings.Fields(long.Name)); n != 6 {
		t.Fatalf("the fixture is %d words, not 6 — it would be refused for the wrong reason", n)
	}
	if len(long.Name) > 0xff {
		t.Fatalf("the fixture is %d bytes, over the length byte — it would be refused for "+
			"the wrong reason", len(long.Name))
	}
	if headerLen+len(long.Name) <= MaxDatagram {
		t.Fatalf("the fixture encodes to %d bytes, inside the %d cap — it does not exercise "+
			"the disagreement", headerLen+len(long.Name), MaxDatagram)
	}

	b, err := long.Encode()
	if err == nil {
		// The defect, stated as what it costs: the encoder produced a datagram its own
		// parser refuses.
		_, perr := Parse(b)
		t.Fatalf("Encode produced %d bytes and Parse says %v — the encoder and the parser "+
			"disagree about what is legal", len(b), perr)
	}
	if !errors.Is(err, ErrMalformed) {
		t.Errorf("refused as %v, want ErrMalformed", err)
	}
}
