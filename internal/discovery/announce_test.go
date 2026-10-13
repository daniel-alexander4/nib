package discovery

import (
	"bytes"
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

// tagged gives a its salt and tag the way the product does — pairing.NewTag over a real
// 32-byte fingerprint — so the tests exercise what the wire actually carries rather than
// sixteen bytes that happen to be non-zero. The salt is random, so two calls with one seed
// are two announcers of one identity.
func tagged(t *testing.T, seed byte, a Announcement) Announcement {
	t.Helper()
	fp := sha256.Sum256([]byte{seed})
	salt, tag, err := pairing.NewTag(fp[:])
	if err != nil {
		t.Fatal(err)
	}
	a.Salt, a.Tag = salt, tag
	return a
}

func good(t *testing.T) Announcement {
	t.Helper()
	return tagged(t, 1, Announcement{Port: 8443, Nonce: [nonceLen]byte{9, 8, 7, 6, 5, 4, 3, 2}})
}

func TestAnnouncementRoundTrips(t *testing.T) {
	want := good(t)
	b, err := want.Encode()
	if err != nil {
		t.Fatal(err)
	}
	// Stimulus: the thing being parsed really is a full announcement, not an empty
	// slice that would round-trip through a parser that did nothing.
	if len(b) != wireLen {
		t.Fatalf("encoded to %d bytes; an announcement is %d", len(b), wireLen)
	}
	got, err := Parse(b)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got != want {
		t.Fatalf("round trip changed the announcement:\n got %+v\nwant %+v", got, want)
	}
}

// TestTheAnnouncementCarriesATagAndNothingElse is the first acceptance clause,
// asserted against the ENCODER'S OUTPUT rather than by reading the struct.
//
// Reading the struct proves only that nobody added a field. What matters is what
// goes on the wire, and the two can differ — an encoder that appended a fingerprint
// "for convenience" would leave the struct definition untouched.
func TestTheAnnouncementCarriesATagAndNothingElse(t *testing.T) {
	fp := sha256.Sum256([]byte{1})
	name, err := pairing.Name(fp[:])
	if err != nil {
		t.Fatal(err)
	}
	a := tagged(t, 1, Announcement{Port: 8443, Nonce: [nonceLen]byte{1, 2, 3, 4, 5, 6, 7, 8}})
	b, err := a.Encode()
	if err != nil {
		t.Fatal(err)
	}

	// The fingerprint the tag was made from must not appear on the wire in any
	// form — not whole, and not truncated to the 66 bits the name encodes.
	for n := 4; n <= len(fp); n++ {
		if idx := indexOf(b, fp[:n]); idx >= 0 {
			t.Fatalf("the first %d bytes of the fingerprint appear in the announcement at offset %d "+
				"— L1 says the rendezvous path carries reachability, never identity", n, idx)
		}
	}
	// Nor the six-word name, whole or by the word (ADR-151): it is one constant for the
	// life of an identity, which is what this format stopped sending.
	for _, w := range append(strings.Fields(name), name) {
		if idx := indexOf(b, []byte(w)); idx >= 0 {
			t.Fatalf("%q, from the sender's six-word name, is in the announcement at offset %d — "+
				"the name is a stable identifier and the announcement must not carry it", w, idx)
		}
	}

	// And the whole datagram is accounted for, field by field, with nothing spare.
	want := len(magic) + 1 + 1 + 2 + 2 + nonceLen + pairing.SaltLen + pairing.TagLen
	if len(b) != want || wireLen != want {
		t.Fatalf("the announcement is %d bytes (wireLen %d); its fields add up to %d — the "+
			"difference carries something this test does not know about", len(b), wireLen, want)
	}

	// **Two announcers of ONE identity share no identifying byte.** This is the property
	// the format exists for. Everything outside the salt and the tag is equal by
	// construction here (same port, same nonce), so any other equal run would be a
	// constant derived from the identity.
	c := tagged(t, 1, Announcement{Port: 8443, Nonce: a.Nonce})
	if a.Salt == c.Salt {
		t.Fatal("two announcers drew the same salt")
	}
	if a.Tag == c.Tag {
		t.Fatal("two announcers of one identity carry the same tag — an observer links them " +
			"without knowing whose they are, which is the six-word name under another encoding")
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
		{"shorter than magic and version", ErrNotOurs, base[:prefixLen-1]},
		{"foreign traffic", ErrNotOurs, []byte("this is an mDNS packet, honestly, and quite long")},
		{"wrong magic", ErrNotOurs, mutate(func(b []byte) []byte { b[0] = 'X'; return b })},
		{"wrong version", ErrVersion, mutate(func(b []byte) []byte { b[offVersion] = version + 1; return b })},
		// Through the NAMED offsets, never `len(magic)+1` arithmetic. The port row
		// used to compute its own and went on "passing" after the transport byte
		// pushed the port along by one: it zeroed the transport and the port's high
		// byte, left a port of 251, and the parser accepted the datagram.
		{"zero port", ErrMalformed, mutate(func(b []byte) []byte {
			binary.BigEndian.PutUint16(b[offPort:], 0)
			return b
		})},
		// An undefined transport is refused rather than defaulted. Defaulting it to
		// TCP is precisely the guess ADR-010 removed — and it would be a guess made
		// on a byte an unauthenticated datagram chose.
		{"unknown transport", ErrMalformed, mutate(func(b []byte) []byte {
			b[offTransport] = 7
			return b
		})},
		{"truncated", ErrMalformed, base[:len(base)-1]},
		{"only magic and version", ErrMalformed, base[:prefixLen]},
		{"trailing bytes", ErrMalformed, append(append([]byte(nil), base...), 'x')},
		{"over the cap", ErrMalformed, make([]byte, MaxDatagram+1)},
		// The salt an announcer that never drew one would send (ADR-151).
		{"no salt", ErrMalformed, mutate(func(b []byte) []byte {
			clear(b[offSalt : offSalt+pairing.SaltLen])
			return b
		})},
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
						t.Errorf("Announcement has a field %q — the announcement carries a salted "+
							"TAG, and never the pin", nm.Name)
					}
				}
			}
			return true
		})
	}
}

// TestEncodeRefusesWhatParseWouldReject closes the loop Encode's own doc promises: "It
// refuses to encode one that would not parse".
//
// It was first written against the name's length (six 40-character words encoded to 261
// bytes and Parse refused them at the 256-byte cap). The format has one fixed length since
// ADR-151, so that disagreement cannot be built; the rule the two ends can still disagree
// on is the one `check` now holds for both — a salt must have been drawn.
func TestEncodeRefusesWhatParseWouldReject(t *testing.T) {
	// Stimulus: a real announcement encodes and parses.
	ok := good(t)
	wire, err := ok.Encode()
	if err != nil {
		t.Fatalf("a real announcement does not encode (%v); the refusal below proves nothing", err)
	}

	unsalted := ok
	unsalted.Salt = [pairing.SaltLen]byte{}
	b, err := unsalted.Encode()
	if err == nil {
		_, perr := Parse(b)
		t.Fatalf("Encode produced an announcement with no salt and Parse says %v — an announcer "+
			"that skipped pairing.NewTag would send the same tag for the life of its identity", perr)
	}
	if !errors.Is(err, ErrMalformed) {
		t.Errorf("refused as %v, want ErrMalformed", err)
	}
	// And the parser, on bytes the encoder did not launder.
	clear(wire[offSalt : offSalt+pairing.SaltLen])
	if got, err := Parse(wire); !errors.Is(err, ErrMalformed) || got != (Announcement{}) {
		t.Errorf("Parse answered %+v, %v for an announcement with no salt; want ErrMalformed", got, err)
	}
}

// TestTheTransportSurvivesTheWire is ADR-010's clause.
//
// A port without its transport is not an address: under QUIC the announced number is a
// UDP port and under TCP it is a TCP port, so a peer that reads the number and picks its
// own transport dials a socket that is not there. The format version was bumped rather
// than the field appended silently, and the round trip is what says the byte means
// anything at all — a field that encodes and parses back as the zero value is decoration.
func TestTheTransportSurvivesTheWire(t *testing.T) {
	for _, want := range []Transport{TransportTCP, TransportQUIC} {
		a := good(t)
		a.Transport = want
		b, err := a.Encode()
		if err != nil {
			t.Fatalf("%v: %v", want, err)
		}
		got, err := Parse(b)
		if err != nil {
			t.Fatalf("%v: does not survive its own parser: %v", want, err)
		}
		if got.Transport != want {
			t.Errorf("announced %v and parsed back %v — the dialing side would open the "+
				"wrong kind of socket at the right port number", want, got.Transport)
		}
		// The rest of the announcement is unchanged by the new field. An insertion
		// that shifted the port would still round-trip the transport correctly.
		if got.Port != a.Port || got.Salt != a.Salt || got.Tag != a.Tag || got.Nonce != a.Nonce {
			t.Errorf("%v: the other fields did not survive: got %+v, want %+v", want, got, a)
		}
	}
	// STIMULUS: the two transports really are different bytes on the wire. If they
	// encoded identically the loop above would pass while carrying nothing.
	tcp := good(t)
	tcp.Transport = TransportTCP
	quic := good(t)
	quic.Transport = TransportQUIC
	bt, _ := tcp.Encode()
	bq, _ := quic.Encode()
	if bytes.Equal(bt, bq) {
		t.Fatal("a TCP and a QUIC announcement encode to identical bytes, so the field is not on the wire")
	}
}

// TestAnEncoderCannotEmitATransportItsOwnParserRefuses.
//
// `check` is called by BOTH Encode and Parse for exactly this reason — the encoder and
// the parser must not drift into disagreeing about what is legal. The name-length cap had
// this defect once already (six 40-character words encoded to 261 bytes and Parse refused
// them at the 256-byte cap), so the rule is asserted rather than assumed.
func TestAnEncoderCannotEmitATransportItsOwnParserRefuses(t *testing.T) {
	a := good(t)
	a.Transport = Transport(7)
	if _, err := a.Encode(); err == nil {
		t.Fatal("encoded an announcement carrying an undefined transport; Parse refuses it, " +
			"so this Nib would emit datagrams it would itself reject")
	} else if !errors.Is(err, ErrMalformed) {
		t.Errorf("refused as %v, want ErrMalformed", err)
	}
}

// TestAnAnnouncementIsOneLengthAndNothingElseParses — /pending 711 R5-6, restated for a
// format with no text in it. The name was a length-prefixed string any on-link host chose,
// and the server logged it before a pin had matched; six `strings.Fields` was all the check
// asked, so control characters and terminal escapes reached the log. Nothing an announcement
// carries is printed any more (the hop refusal names the source address), and the property
// that keeps it so is that there is no variable part: every length but one is refused.
func TestAnAnnouncementIsOneLengthAndNothingElseParses(t *testing.T) {
	base, err := good(t).Encode()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Parse(base); err != nil {
		t.Fatalf("the unmutated announcement does not parse: %v", err)
	}
	for n := 0; n <= MaxDatagram; n++ {
		if n == wireLen {
			continue
		}
		b := make([]byte, n)
		copy(b, base)
		// Bytes past the real announcement are printable text, as a forged log line would be.
		for i := len(base); i < n; i++ {
			b[i] = 'a'
		}
		if got, err := Parse(b); err == nil || got != (Announcement{}) {
			t.Fatalf("a %d-byte datagram parsed as %+v (%v); an announcement is exactly %d bytes",
				n, got, err, wireLen)
		}
	}
}

// versionThree builds an announcement exactly as format version 3 encoded one: the header
// this version still has, then a length byte and the sender's six-word name.
func versionThree(t *testing.T, name string) []byte {
	t.Helper()
	b := []byte(magic)
	b = append(b, 3, byte(TransportTCP))
	b = binary.BigEndian.AppendUint16(b, 8443)
	b = binary.BigEndian.AppendUint16(b, hopNoneWire)
	b = append(b, 1, 2, 3, 4, 5, 6, 7, 8)
	b = append(b, byte(len(name)))
	return append(b, name...)
}

// TestAVersionThreeAnnouncementIsRefusedAsAnotherVersion — ADR-151's bump, by ADR-010's rule.
//
// A version-3 announcement names its sender by the six words. It is refused, and refused AS
// A VERSION whatever its length: a v3 datagram is 42 to 72 bytes and this format is 50, so
// a parser that judged the length first would call a healthy older Nib "malformed" — or
// "not ours" — and the one diagnostic that can tell a user "update" would never fire.
func TestAVersionThreeAnnouncementIsRefusedAsAnotherVersion(t *testing.T) {
	fp := sha256.Sum256([]byte{1})
	real, err := pairing.Name(fp[:])
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"aaa bbb ccc ddd eee fff", // 42 bytes: shorter than this format
		real,                      // what a v3 Nib really sends
		"aaaaaaaa bbbbbbbb cccccccc dddddddd eeeeeeee ffffffff", // 72 bytes: longer
		"aaaa bbbb cccc dddd eeee ffffff",                       // 50 bytes: EXACTLY this format's length
	} {
		b := versionThree(t, name)
		got, err := Parse(b)
		if err == nil {
			t.Fatalf("a version-3 announcement (%d bytes, name %q) parsed as %+v — its bytes were "+
				"read as a salt and a tag", len(b), name, got)
		}
		if !errors.Is(err, ErrVersion) || got != (Announcement{}) {
			t.Errorf("a version-3 announcement (%d bytes) was refused as %v with %+v; want ErrVersion "+
				"and the zero announcement, so a diagnostic can say a Nib of another version is here",
				len(b), err, got)
		}
	}
	if n := len(versionThree(t, "aaaa bbbb cccc dddd eeee ffffff")); n != wireLen {
		t.Fatalf("the same-length fixture is %d bytes, not %d — the row that matters most is not driven", n, wireLen)
	}
}
