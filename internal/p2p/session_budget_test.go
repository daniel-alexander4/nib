package p2p

import (
	"os"
	"regexp"
	"strings"
	"testing"
	"time"
)

// TestSessionBudgetIsTheSumOfWhatInitiateArms — the arithmetic, stated once.
func TestSessionBudgetIsTheSumOfWhatInitiateArms(t *testing.T) {
	if want := 2*exchangeDeadline + remoteDecisionDeadline; SessionBudget() != want {
		t.Errorf("SessionBudget() = %s, want %s", SessionBudget(), want)
	}
	// And it is materially larger than one phase — the specific mistake it exists to stop a
	// caller making. Asserted as a RELATION, not a literal, so a constant change does not
	// force a fixture edit while still catching a caller that reaches for the phase budget.
	if SessionBudget() <= ExchangeBudget() {
		t.Errorf("SessionBudget() %s is not larger than one ExchangeBudget() %s — either the "+
			"deadlines collapsed or this guard has stopped meaning anything",
			SessionBudget(), ExchangeBudget())
	}
	if got, want := SessionBudget(), 24*time.Minute; got != want {
		t.Errorf("SessionBudget() = %s, want %s — if the constants moved deliberately, this "+
			"literal moves with them AND internal/server's ceremonyHopBudget doc is re-read, "+
			"because the plan's per-hop figure is quoted in P07's C20", got, want)
	}
}

// TestSessionBudgetCountsEveryDeadlineInitiateArms is the guard that keeps the sum honest.
//
// SessionBudget adds up three arms. Nothing in the type system says Initiate arms three — so
// a fourth `SetDeadline` added later would silently make the budget an under-estimate, which
// is the exact direction that hurts: a caller reserves too little and asks somebody to consent
// to a signature on a proceeding that has already ended.
//
// A source scan rather than a runtime observation, because the arms are wall-clock deadlines
// on a real connection and there is nothing to count at run time without a session.
func TestSessionBudgetCountsEveryDeadlineInitiateArms(t *testing.T) {
	src, err := os.ReadFile("session.go")
	if err != nil {
		t.Fatal(err)
	}
	body := funcBody(t, string(src), "func Initiate(")
	arms := regexp.MustCompile(`SetDeadline\(`).FindAllString(body, -1)
	// Stimulus first: if the scan found NOTHING, it is reading the wrong function and every
	// count below is a comparison between two zeros.
	if len(arms) == 0 {
		t.Fatal("no SetDeadline call found in Initiate — this scan is not reading the function " +
			"it names, so its count means nothing")
	}
	const want = 3
	if len(arms) != want {
		t.Errorf("Initiate arms %d deadlines and SessionBudget adds up %d. A budget smaller than "+
			"what the session can actually spend is the defect this exists to prevent — update "+
			"SessionBudget and this count together.", len(arms), want)
	}
}

// TestDeliveryLegBudgetCountsEveryDeadlineSendDocumentArms is the same guard for the other
// function, and it is separate ON PURPOSE (P08.S05b).
//
// `DeliveryLegBudget` equals `SessionBudget` today only because `SendDocument` and `Initiate`
// happen to arm the same three deadlines. Sharing one guard would tie a claim about the transfer
// path to a count taken over the co-sign path, so a fourth arm added to either would be graded
// against the other's number — which is the duplicate derivation `SessionBudget`'s own doc grades
// critical, one level up. Two functions, two scans, and they are expected to diverge at P08.S05d.
func TestDeliveryLegBudgetCountsEveryDeadlineSendDocumentArms(t *testing.T) {
	src, err := os.ReadFile("session.go")
	if err != nil {
		t.Fatal(err)
	}
	body := funcBody(t, string(src), "func SendDocument(")
	arms := regexp.MustCompile(`SetDeadline\(`).FindAllString(body, -1)
	if len(arms) == 0 {
		t.Fatal("no SetDeadline call found in SendDocument — this scan is not reading the " +
			"function it names, so its count means nothing")
	}
	const want = 3
	if len(arms) != want {
		t.Errorf("SendDocument arms %d deadlines and DeliveryLegBudget adds up %d. A delivery term "+
			"smaller than what the leg can actually spend means Convene admits a ceremony whose "+
			"round cannot finish inside the deadline the user set.", len(arms), want)
	}
}

// TestDeliveryLegBudgetIsNotSmallerThanTheLegCanSpend — the arithmetic half.
//
// The figure this replaced was `bootstrapBudget + connectDeadline + postConsentDeadline`, on the
// reasoning that a delivery leg runs no local human gate. That confused expected latency with
// armed budget: `SendDocument` arms `remoteDecisionDeadline` regardless of the verifier, and
// `ReceiveDocument`'s Accepter is a live human gate on the production path. It was short by 22
// minutes per leg.
func TestDeliveryLegBudgetIsNotSmallerThanTheLegCanSpend(t *testing.T) {
	if got, want := DeliveryLegBudget(), 2*exchangeDeadline+remoteDecisionDeadline; got != want {
		t.Errorf("DeliveryLegBudget() = %s, want %s (the three arms SendDocument takes)", got, want)
	}
	// **The refuted figure, named by its own arithmetic so a shrink has to argue past it.** The
	// slice was firmed at `bootstrapBudget + connectDeadline + postConsentDeadline`; the p2p half
	// of that is `postConsentDeadline` alone, and the first cut of this assertion compared against
	// it — `24m > 2m`, true of almost any value, which is no assertion at all. What actually
	// distinguishes the refuted figure from the real one is the two `exchangeDeadline` arms and
	// `remoteDecisionDeadline`, so those are what it names.
	if refuted := postConsentDeadline; DeliveryLegBudget() == refuted {
		t.Errorf("DeliveryLegBudget() = %s — the refuted figure's p2p half. SendDocument arms two "+
			"exchangeDeadlines and a remoteDecisionDeadline whatever the verifier does.", refuted)
	}
	// A literal pin, as SessionBudget has. Without one, the first assertion is a restatement of the
	// implementation and a change to exchangeDeadline moves function and test together.
	if got, want := DeliveryLegBudget(), 24*time.Minute; got != want {
		t.Errorf("DeliveryLegBudget() = %s, want %s. Change this literal deliberately, with the "+
			"reservation it feeds — internal/server's ceremonyDeliveryLegBudget and Convene's "+
			"DeliveryBudget — and not as a consequence of moving a constant.", got, want)
	}
	// And it must dominate the post-consent write, which is the term the refuted figure kept.
	if DeliveryLegBudget() <= remoteDecisionDeadline {
		t.Errorf("DeliveryLegBudget() = %s does not exceed remoteDecisionDeadline (%s); the leg's "+
			"cost is that arm PLUS two exchange windows", DeliveryLegBudget(), remoteDecisionDeadline)
	}
	// It must cover what the RECEIVER can spend, or the sender gives up while the peer is still
	// within its own budget — `ReceiveDocument` arms exchangeDeadline twice then postConsentDeadline.
	if recv := 2*exchangeDeadline + postConsentDeadline; DeliveryLegBudget() < recv {
		t.Errorf("DeliveryLegBudget() = %s but the receiver can spend %s; the sender would give up "+
			"first and report a transport failure for a peer that was still working",
			DeliveryLegBudget(), recv)
	}
}

// funcBody returns the source of the function whose signature starts with prefix, from its
// opening brace to the matching close.
func funcBody(t *testing.T, src, prefix string) string {
	t.Helper()
	i := strings.Index(src, prefix)
	if i < 0 {
		t.Fatalf("cannot find %q in the source", prefix)
	}
	rest := src[i:]
	open := strings.Index(rest, "{")
	if open < 0 {
		t.Fatalf("no opening brace after %q", prefix)
	}
	depth := 0
	for j := open; j < len(rest); j++ {
		switch rest[j] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return rest[open : j+1]
			}
		}
	}
	t.Fatalf("unbalanced braces after %q", prefix)
	return ""
}
