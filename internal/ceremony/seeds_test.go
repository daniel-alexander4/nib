package ceremony

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/netip"
	"strings"
	"testing"
)

var b64 = base64.RawURLEncoding

func seedsOf(t *testing.T, ss ...string) []netip.AddrPort {
	t.Helper()
	var out []netip.AddrPort
	for _, s := range ss {
		out = append(out, netip.MustParseAddrPort(s))
	}
	return out
}

// TestSeedsSurviveThePasteRoundTrip.
func TestSeedsSurviveThePasteRoundTrip(t *testing.T) {
	_, inv := invited(t)
	inv.Seeds = seedsOf(t, "212.129.33.59:6881", "87.98.162.88:6881")
	text, err := inv.Encode()
	if err != nil {
		t.Fatal(err)
	}
	back, err := ParseInvitation(text)
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Seeds) != 2 || back.Seeds[0] != inv.Seeds[0] {
		t.Fatalf("seeds did not round-trip: %v", back.Seeds)
	}
	// The pasteable claim, measured rather than asserted in prose.
	t.Logf("invitation with 2 seeds: %d characters", len(text))

	// An invitation WITHOUT seeds must encode exactly as it did before the field existed —
	// that is what `omitempty` buys, and it is why no version bump is needed.
	plain := inv
	plain.Seeds = nil
	ptext, err := plain.Encode()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(ptext, "seeds") {
		t.Error("an invitation with no seeds still carries the field")
	}
	raw, err := json.Marshal(plain)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "seeds") {
		t.Errorf("omitempty is not in effect: %s", raw)
	}
}

// TestAHostnameCannotBeASeed — the clause says ParseAddrPort "structurally cannot resolve a
// hostname", which is a stronger statement than the resolver blacklist guarding the DHT
// package. Driven, because a blacklist is a list of names and this is a property of a type.
func TestAHostnameCannotBeASeed(t *testing.T) {
	if _, err := netip.ParseAddrPort("router.bittorrent.com:6881"); err == nil {
		t.Fatal("ParseAddrPort accepted a hostname — the whole no-resolver argument rests " +
			"on it refusing")
	}
	// And through the wire form: a hostname in the JSON must refuse the invitation, not
	// silently become something else.
	_, inv := invited(t)
	text, err := inv.Encode()
	if err != nil {
		t.Fatal(err)
	}
	bad := strings.Replace(rawJSONOf(t, text), `"convener"`, `"seeds":["router.bittorrent.com:6881"],"convener"`, 1)
	if _, err := ParseInvitation(reencode(t, bad)); err == nil {
		t.Fatal("an invitation naming a hostname as a seed parsed cleanly")
	}
}

// TestAnUnroutableSeedIsDroppedAndTheListIsCappedWholesale.
//
// A bad ENTRY is a transcription error and is dropped; an oversized LIST cannot be, so it is
// loud. Refusing the whole invitation for one mangled seed would hand an attacker a denial
// of honest invitations.
func TestAnUnroutableSeedIsDroppedAndTheListIsCappedWholesale(t *testing.T) {
	_, inv := invited(t)
	inv.Seeds = seedsOf(t,
		"212.129.33.59:6881", // good
		"127.0.0.1:6881",     // loopback
		"192.168.1.1:6881",   // private
		"224.0.0.1:6881",     // multicast
		"93.184.216.34:53",   // routable host, reflection port
		"93.184.216.34:443",  // routable host, TCP-fallback port — Target allows, Seed must not
		"87.98.162.88:6881",  // good
		"212.129.33.59:6881", // duplicate of the first
	)
	kept, dropped, err := validateSeeds(inv.Seeds)
	if err != nil {
		t.Fatalf("a within-cap list was refused wholesale: %v", err)
	}
	if len(kept) != 2 {
		t.Fatalf("kept %v, want the two routable high-port addresses", kept)
	}
	if dropped != 6 {
		t.Errorf("dropped = %d, want 6", dropped)
	}

	// Over the cap: refused WHOLE, and the invitation returned is the zero value.
	over := make([]netip.AddrPort, 0, MaxSeeds+1)
	for i := 0; i <= MaxSeeds; i++ {
		over = append(over, netip.MustParseAddrPort("212.129.33.59:6881"))
	}
	if _, _, err := validateSeeds(over); !errors.Is(err, ErrInvitationSeeds) {
		t.Fatalf("an over-cap list was not refused: %v", err)
	}
	inv.Seeds = over
	if _, err := inv.Encode(); err == nil {
		t.Error("an over-cap seed list encoded cleanly — the producing door does not validate")
	}
}

// TestBothDoorsValidateSeeds.
//
// P04.S04 shipped MaxRoster in ParseInvitation alone, and its review found that this bound
// the RECIPIENTS and left the convener — the party that dials every hop and emits every
// packet — unbounded, because a convener never parses its own invitation. Doing seeds one
// door only would repeat it verbatim.
func TestBothDoorsValidateSeeds(t *testing.T) {
	_, inv := invited(t)
	over := make([]netip.AddrPort, 0, MaxSeeds+1)
	for i := 0; i <= MaxSeeds; i++ {
		over = append(over, netip.MustParseAddrPort("212.129.33.59:6881"))
	}

	// Door 1: producing.
	inv.Seeds = over
	if _, err := inv.Encode(); err == nil {
		t.Error("Encode accepted an over-cap seed list — the convener door is unguarded")
	}

	// Door 2: consuming. Build the text by hand, since Encode now refuses to make one.
	good := inv
	good.Seeds = nil
	text, err := good.Encode()
	if err != nil {
		t.Fatal(err)
	}
	list := `"seeds":[` + strings.TrimSuffix(strings.Repeat(`"212.129.33.59:6881",`, MaxSeeds+1), ",") + `],`
	bad := strings.Replace(rawJSONOf(t, text), `"convener"`, list+`"convener"`, 1)
	got, err := ParseInvitation(reencode(t, bad))
	if err == nil {
		t.Error("ParseInvitation accepted an over-cap seed list — the recipient door is unguarded")
	}
	if len(got.Seeds) != 0 || len(got.Roster) != 0 {
		t.Errorf("a refused invitation leaked fields: %+v", got)
	}
}

// rawJSONOf recovers the JSON inside an encoded invitation, so a test can inject a field no
// encoder would produce — which is exactly what an attacker does.
func rawJSONOf(t *testing.T, text string) string {
	t.Helper()
	body := strings.TrimPrefix(text, invitationPrefix)
	dot := strings.LastIndex(body, ".")
	b, err := b64.DecodeString(body[:dot])
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// reencode rebuilds the text form around hand-edited JSON, checksum and all.
func reencode(t *testing.T, raw string) string {
	t.Helper()
	sum := sha256.Sum256([]byte(raw))
	return invitationPrefix + b64.EncodeToString([]byte(raw)) + "." + hex.EncodeToString(sum[:4])
}

// TestEachDoorAPPLIESTheFilteredList — not merely computes it.
//
// The first version of this file asserted the CAP at both doors and the PREDICATE on its
// own, and never that either door applied the result. A mutation proved the gap: replacing
// `inv.Seeds = kept` and `i.Seeds = kept` with `_ = kept` left the whole package green.
//
// The failure that hides there is not subtle: ParseInvitation would return the attacker's
// raw list, and the CLI hands it straight to rz.Seed() — so the bootstrap retry fires UDP
// at the recipient's own LAN and at a multicast group, from their host, with nothing red.
func TestEachDoorAppliesTheFilteredList(t *testing.T) {
	_, inv := invited(t)
	mixed := seedsOf(t,
		"212.129.33.59:6881", // keep
		"127.0.0.1:6881",     // loopback
		"192.168.1.9:6881",   // private
		"224.0.0.1:1900",     // multicast
		"87.98.162.88:6881",  // keep
	)

	// The CONSUMING door: hand it the raw list through the wire form.
	base := inv
	base.Seeds = nil
	text, err := base.Encode()
	if err != nil {
		t.Fatal(err)
	}
	var quoted []string
	for _, ap := range mixed {
		quoted = append(quoted, `"`+ap.String()+`"`)
	}
	raw := strings.Replace(rawJSONOf(t, text), `"convener"`,
		`"seeds":[`+strings.Join(quoted, ",")+`],"convener"`, 1)
	got, err := ParseInvitation(reencode(t, raw))
	if err != nil {
		t.Fatalf("a list with droppable entries was refused wholesale: %v", err)
	}
	// STIMULUS: the raw list really did contain five, so "two survived" is a filter and not
	// an empty input.
	if len(mixed) != 5 {
		t.Fatal("setup: the mixed list is not what this test thinks")
	}
	if len(got.Seeds) != 2 {
		t.Fatalf("ParseInvitation returned %v — the filtered list was computed and not "+
			"applied, so an attacker's loopback and multicast addresses reach the bootstrap",
			got.Seeds)
	}
	for _, ap := range got.Seeds {
		if !ap.Addr().IsGlobalUnicast() || ap.Addr().IsPrivate() {
			t.Errorf("%v survived the parse", ap)
		}
	}
	if got.SeedsDropped != 3 {
		t.Errorf("SeedsDropped = %d, want 3 — the acceptance says dropped AND counted, and "+
			"a count nobody can read is not counted", got.SeedsDropped)
	}

	// The PRODUCING door refuses rather than drops: the convener's own list must be clean,
	// and silently shipping fewer than they chose is a producer that lies about what it sent.
	prod := inv
	prod.Seeds = mixed
	if _, err := prod.Encode(); err == nil {
		t.Error("Encode silently dropped three of the convener's own addresses")
	}
}
