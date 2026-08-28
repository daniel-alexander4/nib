package server

import (
	"context"
	"net"
	"testing"
	"time"

	"nib/internal/discovery"
	"nib/internal/pairing"
	"nib/internal/sign"
	"nib/internal/vault"
)

// scriptedBrowser yields sightings in order, then blocks until the read deadline and times out —
// which is what a real socket on a quiet link does, and is what makes "and then nothing happened"
// an assertion rather than a script that ran out.
type scriptedBrowser struct {
	seen []discovery.Seen
	i    int
	done chan struct{}
}

func (b *scriptedBrowser) Read(deadline time.Time) (discovery.Seen, error) {
	if b.i >= len(b.seen) {
		select {
		case <-b.done:
		default:
			close(b.done)
		}
		time.Sleep(2 * time.Millisecond)
		return discovery.Seen{}, context.DeadlineExceeded
	}
	s := b.seen[b.i]
	b.i++
	return s, nil
}

func seenFrom(t *testing.T, fp []byte, port uint16) discovery.Seen {
	t.Helper()
	name, err := pairing.Name(fp)
	if err != nil {
		t.Fatal(err)
	}
	return discovery.Seen{
		Announcement: discovery.Announcement{Name: name, Port: port, Transport: discovery.TransportTCP},
		From:         &net.UDPAddr{IP: net.ParseIP("10.9.0.7"), Port: int(port)},
	}
}

// TestTheArmAnswersItsOwnPeerAndNobodyElse — P07.S05c's policy, including the case the whole
// mechanism exists for.
//
// **Every hop of every run so far falls INSIDE the five-minute announce window**, so `answerHopSeekers`
// shipped wired, correct and unproven: the state it was built for — a peer arriving after that
// window has expired — is not reached by any run in the tree. Driving it through a real socket
// costs five minutes of wall clock or a knob in the shipped binary; a fake clock and a fake browser
// cost microseconds, which is why the policy is separated from the socket.
//
// Four states, and the third and fourth are the ones with teeth:
//
//   - a stranger announces → no answer (L1: an arm answers only a name it already holds);
//   - the arm's own peer announces → answer;
//   - that peer announces AGAIN inside the window → no answer (a re-dial must not stack a second
//     announcer, and two armed peers must not answer each other forever);
//   - and again AFTER the window → answer, which is a later hop of the same ceremony.
func TestTheArmAnswersItsOwnPeerAndNobodyElse(t *testing.T) {
	mine, _, err := sign.GenerateIdentity("Convener")
	if err != nil {
		t.Fatal(err)
	}
	myFP, _ := sign.Fingerprint(mine)
	other, _, err := sign.GenerateIdentity("Stranger")
	if err != nil {
		t.Fatal(err)
	}
	otherFP, _ := sign.Fingerprint(other)

	pins := []vault.PinnedPeer{{Fingerprint: myFP, Label: "Convener"}}

	// STIMULUS: the two identities really differ, or "answered only mine" is trivially true.
	if string(myFP) == string(otherFP) {
		t.Fatal("setup: the two identities are the same, so this cannot discriminate")
	}

	br := &scriptedBrowser{done: make(chan struct{}), seen: []discovery.Seen{
		seenFrom(t, otherFP, 5001), // a stranger
		seenFrom(t, myFP, 5002),    // the peer this arm is for
		seenFrom(t, myFP, 5002),    // …again, immediately
		seenFrom(t, myFP, 5002),    // …and again, after the window (the clock jumps below)
	}}

	// A clock the test drives. It jumps past hopAnnounceWindow only before the FOURTH sighting,
	// so the third is inside the window and the fourth is outside it.
	base := time.Now()
	var reads int
	now := func() time.Time {
		// Read() is called once per sighting; the jump lands between the third and the fourth.
		if reads >= 3 {
			return base.Add(hopAnnounceWindow * 3)
		}
		return base
	}

	var answers []time.Time
	var answeredFor [][]byte
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-br.done
		cancel()
	}()
	answerLoop(ctx, browserFunc(func(d time.Time) (discovery.Seen, error) {
		s, e := br.Read(d)
		reads++
		return s, e
	}), pins, now, func(c candidate) bool {
		answers = append(answers, now())
		answeredFor = append(answeredFor, c.Fingerprint)
		return true
	})

	if len(answers) != 2 {
		t.Fatalf("the arm answered %d time(s), want 2 — one for its peer's first announcement and "+
			"one for the announcement after the window; a third means the rate limit is not "+
			"holding, and a first-only means a later hop of the same ceremony cannot find this "+
			"party at all, which is the whole reason this mechanism exists", len(answers))
	}
	if !answers[0].Equal(base) {
		t.Errorf("the first answer is stamped %v, want the base time — the clock is not being "+
			"read where the rate limit reads it", answers[0])
	}
	if answers[1].Sub(answers[0]) < hopAnnounceWindow {
		t.Errorf("the two answers are %v apart, less than the %v window — the second was not the "+
			"post-expiry one", answers[1].Sub(answers[0]), hopAnnounceWindow)
	}
	// **WHICH peer, not how many.** A count cannot tell "answered my peer, then answered it again
	// after the window" from "answered a STRANGER, then answered my peer" — both are two. Measured:
	// deleting the resolve gate left the count assertions above green.
	for i, fp := range answeredFor {
		if string(fp) != string(myFP) {
			t.Errorf("answer %d was for %x, want this arm's own peer %x — an arm that answers a "+
				"name it does not hold is L1's whole prohibition, and it makes two armed parties "+
				"answer each other forever", i, fp, myFP)
		}
	}
}

// TestAFailedAnswerDoesNotSpendTheWindow — the arm must not be silenced by an answer it never made.
//
// `answer` returns false when there is nothing truthful to announce: a loopback bind, or no usable
// interface. Counting that as an answer would start the rate limit for a party that never spoke,
// so the next sighting — possibly the one hop that mattered — would be ignored too.
func TestAFailedAnswerDoesNotSpendTheWindow(t *testing.T) {
	cert, _, err := sign.GenerateIdentity("Convener")
	if err != nil {
		t.Fatal(err)
	}
	fp, _ := sign.Fingerprint(cert)
	pins := []vault.PinnedPeer{{Fingerprint: fp, Label: "Convener"}}

	br := &scriptedBrowser{done: make(chan struct{}), seen: []discovery.Seen{
		seenFrom(t, fp, 5002),
		seenFrom(t, fp, 5002),
	}}
	base := time.Now()
	tries := 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-br.done
		cancel()
	}()
	answerLoop(ctx, br, pins, func() time.Time { return base }, func(candidate) bool {
		tries++
		return false // never managed to announce
	})
	if tries != 2 {
		t.Errorf("the arm tried to answer %d time(s), want 2 — a failed answer spent the rate-limit "+
			"window, so a party that never actually announced was then silenced", tries)
	}
}

// browserFunc adapts a function to the browser interface.
type browserFunc func(time.Time) (discovery.Seen, error)

func (f browserFunc) Read(d time.Time) (discovery.Seen, error) { return f(d) }
