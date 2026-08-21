package rendezvous

import (
	"cmp"
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"math"
	"sync/atomic"
	"time"

	"github.com/anacrolix/dht/v2/bep44"
	"github.com/anacrolix/dht/v2/exts/getput"
	"github.com/anacrolix/torrent/bencode"
)

// Publishing and fetching a record on the DHT — as OPAQUE BYTES.
//
// This package may not import internal/ceremony or internal/sign (L1, enforced by
// l1_test.go, whose comment names this slice by coordinate). That is not an inconvenience
// to route around: a package that can read an Invitation is a package that can consult a
// pinned fingerprint, and L1 says the rendezvous must never be able to influence which
// peer is accepted. So the key, the salt and the record all arrive here as byte slices
// with no meaning attached, exactly as internal/server/discover.go does the resolving
// that internal/discovery may not.
//
// Everything cryptographic — the signature, the encryption, the expiry, the author check
// — happens in internal/ceremony, on the other side of that line.

// ErrNoRecord is "nobody is holding one", which is an ordinary outcome and not a failure:
// the peer may not have published yet. It is distinct from a transport error because the
// ladder shows the user different things for "not yet" and "we cannot reach the DHT".
var ErrNoRecord = errors.New("no record published under this key")

// publishBudget is one traversal's own budget, and it is NOT the caller's lifetime.
//
// D16's table gave the DHT candidate fetch "every 2 s"; that was measured against the
// library and amended at this slice, because a traversal cannot return early and a 2 s
// budget expires inside its first round every time. The floor is one query timeout
// (2 s, transaction.go:24 with dht.go:24-27) and convergence takes several rounds, so a
// budget under ~30 s systematically fails to reach the nodes that hold the record while
// looking exactly like "nobody published one".
// PublishBudget is exported so a diagnostic can tell the user how long a phase may take.
const PublishBudget = 45 * time.Second

const publishBudget = PublishBudget

// keyPair turns an opaque 32-byte seed into the hop's BEP-44 identity.
//
// ed25519.NewKeyFromSeed PANICS on any length but 32 (crypto/ed25519), and the seed
// arrives from a caller across a package boundary, so the length is checked here rather
// than trusted.
func keyPair(seed []byte) (ed25519.PrivateKey, [32]byte, error) {
	var pub [32]byte
	if len(seed) != ed25519.SeedSize {
		return nil, pub, fmt.Errorf("rendezvous: seed is %d bytes, want %d", len(seed), ed25519.SeedSize)
	}
	priv := ed25519.NewKeyFromSeed(seed)
	copy(pub[:], priv.Public().(ed25519.PublicKey))
	return priv, pub, nil
}

// Publish writes value under the key derived from seed, discriminated by salt.
//
// # What it can and cannot tell you, because the library will not say
//
// `getput.Put` fans out to the closest nodes in goroutines that SHADOW the error
// (`err := res.ToError()`) and only log it; the outer named return is never assigned
// after the get phase (exts/getput/getput.go:155-172). So a nil error from it means "the
// traversal that collected write tokens was not cancelled" and says **nothing whatever**
// about whether any node accepted the write. `*traversal.Stats` cannot close the gap
// either — it has exactly two fields and both describe the get traversal
// (traversal/stats.go).
//
// This is why PublishNodes counts the nodes that ANSWERED the token-gathering traversal
// rather than nodes that accepted the put: it is the strongest true statement available,
// and naming it after what it actually counts is the lesson P04.S01 already paid for when
// `SeedsAnswered` turned out to be measuring table growth.
//
// The one genuinely hard failure this does catch is the local one, and it is the one most
// likely to bite: `dht.Server.Put` runs `s.store.Put` BEFORE sending anything
// (server.go:1081), so an over-size value, an over-long salt or a stale seq fails in our
// own store and no datagram is sent. That path returns an error from every node and is
// invisible in `getput.Put`'s return — so the size is checked here, up front, where it
// can be reported.
func (s *Server) Publish(ctx context.Context, seed, salt, value []byte) error {
	priv, pub, err := keyPair(seed)
	if err != nil {
		return err
	}
	if len(salt) > 64 {
		return fmt.Errorf("rendezvous: salt is %d bytes, BEP-44 caps it at 64", len(salt))
	}
	// The cap is on the BENCODED length (bep44/item.go:132). Measure it rather than
	// reasoning about the overhead.
	if bv, err := bencode.Marshal(value); err != nil {
		return err
	} else if len(bv) > 1000 {
		return fmt.Errorf("rendezvous: record bencodes to %d bytes, BEP-44 caps it at 1000", len(bv))
	}

	ctx, leave, err := s.enter(ctx)
	if err != nil {
		return err
	}
	defer leave()
	ctx, cancel := context.WithTimeout(ctx, publishBudget)
	defer cancel()

	target := bep44.MakeMutableTarget(pub, salt)
	s.publishAttempts.Add(1)

	// The callback is handed the highest seq the traversal saw, UNINCREMENTED, and it
	// must both bump it and re-sign — seq is inside the signed preimage
	// (bep44/item.go:114-124). Returning the same seq with different bytes is refused by
	// every remote node AND by our own store, silently, which is exactly the case that
	// arises when the candidate set changes. (P04.S03 ships no republish loop — one publish
	// covers a 300 s race against a 2 h item lifetime — so this is a guard against a future
	// caller, not against a loop that exists.)
	var seqCeiling bool
	stats, err := getput.Put(ctx, target, s.dht, salt, func(seq int64) bep44.Put {
		// seq+1 must not wrap, and the attack that makes it wrap is cheap.
		//
		// The hop's ed25519 key comes from the invitation secret, so EVERY roster member
		// can write at our target. One publishing at math.MaxInt64 makes this `seq + 1`
		// overflow to math.MinInt64 — Go wraps signed overflow silently, no panic, no
		// error. Our own store accepts it (there is no prior item locally, so
		// CheckIncoming never runs), every remote refuses it with
		// ErrSequenceNumberLessThanCurrent, getput.Put shadows all of those and returns
		// nil, and Publish reports success having stored nothing anywhere.
		//
		// Preemption by an invitation holder is a known and accepted in-roster power, but
		// as a RACE the honest party re-wins by publishing higher. Overflow makes it
		// permanent. Refusing here turns a silent permanent block into an error the caller
		// can act on and a counter someone can read.
		if seq >= math.MaxInt64-1 {
			seqCeiling = true
			return bep44.Put{}
		}
		p := bep44.Put{V: value, Salt: salt, Seq: seq + 1}
		k := pub
		p.K = &k
		p.Sign(priv)
		return p
	})
	if stats != nil {
		// Read atomically: getput.Put calls op.Stop(), which returns before its query
		// goroutines drain, and those goroutines are still doing atomic adds on these
		// fields. A plain read here is a data race and -race will say so.
		s.publishNodes.Store(uint64(atomic.LoadUint32(&stats.NumResponses)))
	}
	if seqCeiling {
		s.publishSeqCeiling.Add(1)
		return errors.New("rendezvous: the sequence number at this key is at its ceiling — " +
			"somebody who holds this ceremony's invitation has taken the key")
	}
	if err != nil {
		return err
	}
	// **Cancellation has to be re-checked here, because getput.Put SHADOWS it.**
	//
	// Its put fan-out logs each node's error and discards it, and its own `err` is set only
	// when the GET traversal is cancelled — so a publish cancelled during the put phase
	// returns nil, and the doc above ("nil says nothing about whether any node accepted the
	// write") becomes a false success rather than merely a weak one. That matters now that
	// Close cancels: a caller that quit the app would be told its record was published.
	if cerr := ctx.Err(); cerr != nil {
		return fmt.Errorf("rendezvous: the publish did not finish: %w", cerr)
	}
	s.published.Add(1)
	return nil
}

// Fetch reads the record published under seed+salt.
//
// # Two traps in the return value, both inverted from the normal Go contract
//
// `getput.Get`'s ctx.Done() arm sets err and does NOT touch ret (getput.go:113-114), so a
// perfectly good record can come back beside a non-nil error — a caller writing the
// ordinary `if err != nil { return }` discards records it successfully fetched. And
// `ret.Seq` is pre-set to math.MinInt64 (getput.go:94), so testing `Seq == 0` for
// emptiness is wrong. Both are handled by treating a present, non-empty V as success
// whatever err says.
func (s *Server) Fetch(ctx context.Context, seed, salt []byte) ([]byte, int64, error) {
	_, pub, err := keyPair(seed)
	if err != nil {
		return nil, 0, err
	}
	ctx, leave, err := s.enter(ctx)
	if err != nil {
		return nil, 0, err
	}
	defer leave()
	ctx, cancel := context.WithTimeout(ctx, publishBudget)
	defer cancel()

	target := bep44.MakeMutableTarget(pub, salt)
	s.fetchAttempts.Add(1)

	res, stats, err := getput.Get(ctx, target, s.dht, nil, salt)

	// Record how many nodes answered on BOTH paths, not only the empty one.
	//
	// The first version of this stored it only where the fetch found nothing, so a
	// SUCCESSFUL fetch reported "0 nodes answered" — a false zero, in the counter whose
	// entire job is to say whether an absent record means anything. It was caught by the
	// live harness printing the number beside a record it had just retrieved.
	answered := uint32(0)
	if stats != nil {
		// Atomically: op.Stop() returns before its query goroutines drain, and they are
		// still adding to this field. A plain read is a data race and -race says so.
		answered = atomic.LoadUint32(&stats.NumResponses)
		s.fetchNodes.Store(uint64(answered))
	}

	if len(res.V) > 0 {
		var value []byte
		// V comes back as RAW BENCODE (getput.go:22) — the server fills it with
		// bencode.MustMarshal(item.V) (server.go:646) — so it is not the bytes we put in
		// until it is unmarshalled. Treating res.V as the record directly yields a length
		// prefix glued to the front, which would decrypt to nothing and read as "the peer
		// published garbage".
		if uerr := bencode.Unmarshal(res.V, &value); uerr != nil {
			s.fetchUndecodable.Add(1)
			return nil, 0, fmt.Errorf("rendezvous: fetched value did not decode: %w", uerr)
		}
		s.fetched.Add(1)
		return value, res.Seq, nil
	}
	// A cancelled or timed-out traversal is NOT an empty fetch.
	//
	// getput.Get sets err = ctx.Err() and leaves ret alone, so a caller's cancellation and
	// our own budget expiring both arrive here — and the first version of this function
	// returned ErrNoRecord for them, which the ladder reports as "your peer has not
	// published yet". It also counted them in fetchEmpty, whose own documentation says it
	// must never be summed with a transport failure. Both were wrong in the same place.
	if ctxErr := ctx.Err(); ctxErr != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		s.fetchAborted.Add(1)
		return nil, 0, fmt.Errorf("rendezvous: the lookup did not finish (%d node(s) had "+
			"answered): %w", answered, cmp.Or(ctxErr, err))
	}

	s.fetchEmpty.Add(1)

	// Nothing came back. Two very different facts wear that outcome, and the ladder shows
	// the user different things for each: **nobody is holding a record yet** (the
	// ordinary state before a peer publishes) versus **we could not reach the DHT at
	// all** (D19's diagnosis territory). The library flattens both into
	// `errors.New("value not found")` when the traversal stalls, and into ctx.Err() when
	// it does not.
	//
	// The traversal itself separates them, and it does so without matching on error text:
	// if nodes ANSWERED us and none had the record, the DHT is reachable and the peer has
	// not published. If nothing answered, the record's absence is not evidence of
	// anything. Read atomically — op.Stop() returns before its query goroutines drain and
	// they are still adding to these fields.
	if answered == 0 {
		if err != nil {
			return nil, 0, fmt.Errorf("rendezvous: no DHT node answered: %w", err)
		}
		return nil, 0, errors.New("rendezvous: no DHT node answered")
	}
	return nil, 0, ErrNoRecord
}
