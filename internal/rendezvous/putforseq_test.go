package rendezvous

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"math"
	"testing"

	"github.com/anacrolix/dht/v2/bep44"
)

// emptyItemTarget is sha1 of the empty string, MEASURED rather than asserted: it is what
// `bep44.Put{}.Target()` computes, because an empty Put has no key, so it is IMMUTABLE and
// its target is the hash of its (nil) value. Written as a literal here so the test names the
// thing that used to be published rather than describing it.
const emptyItemTarget = "da39a3ee5e6b4b0d3255bfef95601890afd80709"

// TestARefusedPublishEmitsNothingThatAnybodyStores.
//
// # The defect
//
// At the sequence ceiling `putForSeq`'s predecessor returned `bep44.Put{}` to mean "refuse",
// and `Publish` then reported the refusal to its caller. But `getput.SeqToPut` has no error
// return and v2.24.0 uses whatever comes back UNCONDITIONALLY: `put := seqToPut(autoSeq)`
// (`exts/getput/getput.go:154`), fanned out to every node in `op.Closest()` (`:155-168`), each
// of which calls `Server.Put`, whose FIRST line writes to the local store (`server.go:1081`)
// before the context is ever looked at.
//
// So the refusal published. An empty item — immutable, unsigned, at a target belonging to
// nobody — written to strangers by the branch whose entire job is to refuse.
//
// # Why this test and not the round-trip one
//
// `roundtrip_test.go` asserts the REFUSAL and never the silence, which is why nothing saw
// this: getting there over a live DHT needs an in-roster holder to have published at
// math.MaxInt64. The decision is now a function, so the question can be asked directly of
// what it returns — and asked as the protocol asks it, with bep44's own Check and
// CheckIncoming, rather than by re-stating their rules here.
func TestARefusedPublishEmitsNothingThatAnybodyStores(t *testing.T) {
	pubKey, privKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	var pub [32]byte
	copy(pub[:], pubKey)
	salt := []byte("hop-1")
	value := "a candidate record"

	// STIMULUS: the ordinary path really does increment, or "the ceiling path does not
	// increment" below is equally true of a function that never increments at all.
	ordinary, ceiling := putForSeq(41, value, salt, pub, privKey)
	if ceiling {
		t.Fatal("seq 41 reported as the ceiling — the predicate is broken, so every " +
			"assertion below is about the wrong branch")
	}
	if ordinary.Seq != 42 {
		t.Fatalf("the ordinary path put seq %d; want 42. It must bump, because seq is inside "+
			"the signed preimage and a re-sign at the same seq is refused everywhere.",
			ordinary.Seq)
	}

	got, ceiling := putForSeq(math.MaxInt64, value, salt, pub, privKey)
	if !ceiling {
		t.Fatal("math.MaxInt64 was not reported as the ceiling; seq+1 wraps to math.MinInt64 " +
			"and the block becomes permanent")
	}

	// 1. It is not the empty item. This is the defect, named by its target.
	gotTarget := got.Target()
	if h := hex.EncodeToString(gotTarget[:]); h == emptyItemTarget {
		t.Errorf("the refusal returned the EMPTY item (target %s = sha1 of the empty "+
			"string). getput.Put uses the callback's return unconditionally and fans it out "+
			"to every closest node, storing locally first — so the branch that refuses to "+
			"publish published an empty item, at a target belonging to nobody, to strangers.",
			h)
	}

	// 2. It is addressed to OUR mutable target, so nothing lands anywhere we do not own.
	if !got.IsMutable() {
		t.Error("the refusal returned an IMMUTABLE put; its target is then the hash of its " +
			"value rather than our key, which is how it reached a stranger's keyspace")
	}
	want := bep44.MakeMutableTarget(pub, salt)
	if got.Target() != want {
		t.Errorf("the refusal targets %x; want our own mutable target %x", got.Target(), want)
	}

	// 3. It is well-formed by bep44's own rule — an unsigned or malformed item is refused
	//    for the wrong reason, and would be accepted by a node that does not check.
	if cerr := bep44.Check(got.ToItem()); cerr != nil {
		t.Errorf("the refusal is malformed: %v", cerr)
	}

	// 4. And nobody stores it. Asked with bep44's own CheckIncoming against what is
	//    necessarily present in this case — a record at the ceiling — rather than by
	//    restating the rule.
	// The stored value is the ATTACKER'S, not ours: reaching the ceiling at all requires an
	// in-roster holder to have taken the key, and taking it means writing their own record.
	// This distinction is not decoration — CheckIncoming has a branch for equal seq with an
	// EQUAL value ("the node SHOULD reset its timeout counter") and returns nil there, so a
	// fixture that reuses our own value tests the one case the attack cannot produce, and
	// reports the fix as broken. It did, on the first run.
	stored := bep44.Put{V: "the preempting record", Salt: salt, Seq: math.MaxInt64}
	sk := pub
	stored.K = &sk
	stored.Sign(privKey)
	if ierr := bep44.CheckIncoming(stored.ToItem(), got.ToItem()); !errors.Is(ierr, bep44.ErrSequenceNumberLessThanCurrent) {
		t.Errorf("a node holding the ceiling record would accept the refusal (CheckIncoming "+
			"= %v); it must be refused, or the refusal is a publish", ierr)
	}

	// 5. And the identical-value branch, stated so the limit is written down rather than
	//    discovered: if the stored record happens to carry OUR value at the same seq, the
	//    refusal is accepted as a timeout refresh. It stores nothing new — same target, same
	//    key, same bytes — so it is not a publish, and it is unreachable in the attack this
	//    guards, where the holder wrote their own record to take the key.
	same := bep44.Put{V: value, Salt: salt, Seq: math.MaxInt64}
	same.K = &sk
	same.Sign(privKey)
	if ierr := bep44.CheckIncoming(same.ToItem(), got.ToItem()); ierr != nil {
		t.Logf("identical-value branch now refuses too (%v) — stricter than when this was "+
			"written, and fine; the note above is what needs updating", ierr)
	}
}
