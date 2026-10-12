package sign

import (
	"errors"
	"testing"

	"github.com/digitorus/pdfsign/verify"
)

// TestASignerWhoseBagCannotBeKeyedDoesNotJoin — /pending 653 part 10.
//
// `bagKeyOfSigner` answered "" for a bag holding a nil certificate, and `bagKey` answers "" for a
// record whose blob carries no certificates, so the join's cross-check compared "" with "" and
// passed. It failed closed only on the belief that the library never reports a nil certificate.
// The record below is the one shape where the two coincide.
func TestASignerWhoseBagCannotBeKeyedDoesNotJoin(t *testing.T) {
	revs := []Revision{{Obj: 7, libPos: 0, bag: bagKey(nil)}}
	// STIMULUS: the record's bag really is the empty key, or the case below proves nothing.
	if revs[0].bag != "" {
		t.Fatalf("setup: a record with no certificates keys as %q, want the empty key", revs[0].bag)
	}
	signers := []verify.Signer{{Certificates: []verify.Certificate{{Certificate: nil}}}}
	if _, err := joinLibrary(revs, signers); !errors.Is(err, errJoin) {
		t.Fatalf("a signer whose bag cannot be keyed joined a record with no certificates (%v), "+
			"want %v — two different \"cannot key this\" answers compared equal", err, errJoin)
	}
}
