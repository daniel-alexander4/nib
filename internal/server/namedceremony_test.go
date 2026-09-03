package server

import (
	"errors"
	"testing"

	"nib/internal/sign"
)

// TestOnlyAnUNNAMEDCeremonyFallsThroughToTheManualPath is /pending 356's own discriminator, and
// the guard that lets that item close.
//
// **The item proposed a fix the code already had.** It read: *"a request that NAMES a ceremony and
// cannot resolve it should be refused at the route with that sentence, rather than falling through
// to a path whose behaviour depends on the absence … the discriminator is named-and-unresolvable
// versus not-named."* That is exactly what `handleSessionInitiate` does — it refuses on any
// `ceremonyFor` error except `errNoCeremony` — and nothing asserted it.
//
// **What actually happened was a harness naming a field no route reads.** It passed
// `ceremony=<id>` to `/api/session/initiate`, which reads `invitation` and nothing else, so the
// product correctly saw "nothing named" and took the manual path. Claim of absence with the
// search: `grep -n 'FormValue("ceremony")' internal/server/*.go` outside tests returns nothing.
//
// **Both arms, because either alone is worthless.** Refusing everything would break the manual and
// LAN paths, which name no ceremony and must fall through; refusing nothing is the defect the item
// describes. The discriminator is the whole property.
func TestOnlyAnUNNAMEDCeremonyFallsThroughToTheManualPath(t *testing.T) {
	cert, _, err := sign.GenerateIdentity("me")
	if err != nil {
		t.Fatal(err)
	}
	peer := make([]byte, 32)

	// The FALL-THROUGH arm: nothing named. `handleSessionInitiate` treats this one error as
	// "carry on without a ceremony", which is what the manual and LAN paths need.
	if _, e := ceremonyFor("", cert, nil, peer); !errors.Is(e, errNoCeremony) {
		t.Errorf("an empty invitation gave %v, want errNoCeremony — the manual and LAN paths name "+
			"no ceremony and this is the error the route lets through", e)
	}

	// The REFUSAL arm: named and unresolvable, in each shape a caller can produce. Every one must
	// be an error the route does NOT let through, or a request that named a ceremony proceeds on a
	// path whose behaviour depends on the name being absent — and a `signs:false` convener signs.
	for _, tc := range []struct{ name, text string }{
		{"unparseable payload", "nib-invite-v1:not-base64!!"},
		{"not an invitation at all", "hello"},
		{"a ceremony id, which is not an invitation", "aa11bb22cc33dd44ee55ff6677889900"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, e := ceremonyFor(tc.text, cert, nil, peer)
			if e == nil {
				t.Fatalf("%q resolved to a ceremony", tc.text)
			}
			if errors.Is(e, errNoCeremony) {
				t.Errorf("%q gave errNoCeremony, which `handleSessionInitiate` LETS THROUGH — so a "+
					"request that named a ceremony would take the manual co-sign path, and a "+
					"non-signing convener would apply its own signature to a document it is a "+
					"signs:false party on. /pending 356", tc.text)
			}
		})
	}
}
