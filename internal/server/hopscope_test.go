package server

import "testing"

// TestARaceNeverDialsAnotherHopsCandidate — the PREDICATE of criterion 19, and only that.
//
// The clause is "the race and the glare tie-break are scoped to the current hop: a convener
// holding candidates for a later party never dials them during this hop". A stray candidate would
// have failed the PIN — and been DIALLED first, which is exactly what the criterion forbids. The
// convener is the party this bites: under D22's hub it holds a hop with every party, so it is the
// one machine that genuinely has a later party's addresses in hand while running this hop.
//
// **This test drives `hopScoped` with a hand-built candidate, and no production candidate can be
// the one it refuses (`/pending 517`).** The only producer of a `sourceDHT` candidate stamps
// `Hop: c.hop` from the same `*ceremonyID` whose `cer.hop` the only call site compares against, and
// the gate has already bound the hop as AEAD context before either runs — see `hopScoped`'s own
// doc, which used to claim this file made the criterion "a property rather than a discipline" and
// now says where the enforcement actually is. What is asserted below is that the predicate is the
// right predicate, which is worth keeping and is not the same statement.
func TestARaceNeverDialsAnotherHopsCandidate(t *testing.T) {
	const thisHop = 0

	// SETUP: a candidate FOR this hop passes, or "the other one is refused" is equally true
	// of a filter that refuses everything and the DHT tier would simply never work.
	mine := candidate{Addr: "203.0.113.10:34154", Transport: "quic", Source: sourceDHT, Hop: thisHop}
	if !hopScoped(mine, thisHop) {
		t.Fatal("setup: a candidate for this very hop was refused, so the refusal below " +
			"cannot distinguish hop scoping from a filter that drops everything")
	}

	later := candidate{Addr: "203.0.113.11:34154", Transport: "quic", Source: sourceDHT, Hop: thisHop + 1}
	if hopScoped(later, thisHop) {
		t.Error("a candidate belonging to a LATER hop was admitted to this race. It would " +
			"fail the pin — and it would be dialled first, which is the thing criterion 19 " +
			"forbids: a convener reaching out to a party three positions away, from the " +
			"user's own machine, during somebody else's hop.")
	}

	// The tiers that belong to NO hop are unaffected. Scoping that dropped them would take
	// the LAN and manual paths out with it — and those are the two tiers D8 says survive
	// when the DHT does not, so this is not a hypothetical regression.
	for _, c := range []candidate{
		{Addr: "192.168.1.9:8443", Transport: "tcp", Source: sourceLAN},
		{Addr: "203.0.113.12:8443", Transport: "tcp", Source: sourceTyped},
	} {
		if !hopScoped(c, thisHop) {
			t.Errorf("a %s candidate was dropped by hop scoping; it belongs to no hop, and "+
				"dropping it removes the tier that survives when the DHT does not", c.Source)
		}
	}
}
