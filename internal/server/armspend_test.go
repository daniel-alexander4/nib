package server

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/hex"
	"errors"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nib/internal/p2p"
	"nib/internal/sign"
)

// TestTheSpendRuleOverEveryOutcome — /pending 766, inventory rows A3, A4, A6.
//
// The rule as a pure function, over every outcome a served connection can end with: a decision made
// by a party who got through ends the arm, a failure of the channel keeps it, a clean completion
// ends it unless a re-delivery is owed, and anything with no row ends it.
func TestTheSpendRuleOverEveryOutcome(t *testing.T) {
	// The commitment failure as p2p raises it. Its sentinels are unexported, so the row is driven
	// through a real exchange in the listener test below; here the predicate is asked directly.
	type row struct {
		name      string
		err       error
		wantClass string
		wantSpent bool
	}
	rows := []row{
		{"a lost channel (EOF)", io.EOF, spendLostChannel, false},
		{"a lost channel (wrapped, as verify.go wraps a frame error)", fmt.Errorf("receive commitment: %w", io.ErrUnexpectedEOF), spendLostChannel, false},
		{"a lost channel (a deadline)", os.ErrDeadlineExceeded, spendLostChannel, false},
		{"a lost channel (a reset)", &net.OpError{Op: "read", Err: errors.New("connection reset by peer")}, spendLostChannel, false},
		{"a role this build cannot read", fmt.Errorf("%w: role(238)", p2p.ErrRoleUnreadable), "role-unreadable", false},
		{"a role this arm does not serve", p2p.ErrRoleRefused, "wrong-role", false},
		{"the consent slot is taken", errConsentBusy, "busy", false},
		{"the spoken-check slot is taken", errVerifyBusy, "busy", false},
		{"a declined co-signature", p2p.ErrCoSignDeclined, "declined", true},
		{"a declined transfer", p2p.ErrDeclined, "declined", true},
		{"a consent nobody answered", p2p.ErrConsentTimedOut, spendUnanswered, true},
		{"a spoken check nobody answered", p2p.ErrVerificationTimedOut, spendUnanswered, true},
		{"the words rejected here", p2p.ErrVerificationDeclined, "words-rejected", true},
		{"the words rejected there", p2p.ErrPeerDidNotConfirm, "words-rejected-by-peer", true},
		{"a refused arrival", fmt.Errorf("%w: detail", p2p.ErrRosterMismatch), "arrival-refused", true},
		{"a proceeding that has ended", p2p.ErrCeremonyEnded, "arrival-refused", true},
		{"not this party's turn", p2p.ErrNotYourTurn, "signing-refused", true},
		{"accepted and not saved", fmt.Errorf("%w: disk full", p2p.ErrNotStored), "not-saved", true},
		{"the arm is already gone", errConsentNotArmed, "arm-gone", true},
		{"an error nobody classified", errors.New("something new"), spendUnclassified, true},
	}
	for _, r := range rows {
		for _, redelivers := range []bool{false, true} {
			class, spent := spendClass(r.err, redelivers)
			if class != r.wantClass || spent != r.wantSpent {
				t.Errorf("%s (redelivers=%t): class %q spent=%t, want %q spent=%t",
					r.name, redelivers, class, spent, r.wantClass, r.wantSpent)
			}
		}
	}
	// A clean completion is the one outcome the re-delivery debt changes.
	if class, spent := spendClass(nil, false); class != spendCompleted || !spent {
		t.Errorf("a clean completion with nothing owed: class %q spent=%t, want %q spent", class, spent, spendCompleted)
	}
	if class, spent := spendClass(nil, true); class != spendRedelivers || spent {
		t.Errorf("a clean completion that owes a re-delivery: class %q spent=%t, want %q kept — the "+
			"re-delivery window would close the moment the hop completed", class, spent, spendRedelivers)
	}
}

// TestEveryP2PSentinelHasASpendRow — /pending 766, inventory row A6.
//
// "Zero unclassified" is a fact only if the table was checked against the sentinels that exist. They
// are read from p2p's source: every exported package-level `Err…` declared with `errors.New` must
// have a row by name, and every named row must still be a sentinel p2p declares.
func TestEveryP2PSentinelHasASpendRow(t *testing.T) {
	files, err := filepath.Glob("../p2p/*.go")
	if err != nil {
		t.Fatal(err)
	}
	declared := map[string]bool{}
	fset := token.NewFileSet()
	for _, f := range files {
		if strings.HasSuffix(f, "_test.go") {
			continue
		}
		af, perr := parser.ParseFile(fset, f, nil, 0)
		if perr != nil {
			t.Fatal(perr)
		}
		for _, d := range af.Decls {
			gd, ok := d.(*ast.GenDecl)
			if !ok || gd.Tok != token.VAR {
				continue
			}
			for _, sp := range gd.Specs {
				vs := sp.(*ast.ValueSpec)
				for i, n := range vs.Names {
					if !strings.HasPrefix(n.Name, "Err") || i >= len(vs.Values) {
						continue
					}
					call, ok := vs.Values[i].(*ast.CallExpr)
					if !ok {
						continue
					}
					if sel, ok := call.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "New" {
						declared[n.Name] = true
					}
				}
			}
		}
	}
	// STIMULUS: the scan read p2p's sentinels at all, and found the ones this item is about.
	for _, must := range []string{"ErrCoSignDeclined", "ErrConsentTimedOut", "ErrRoleRefused", "ErrPeerNotPinned"} {
		if !declared[must] {
			t.Fatalf("the scan of ../p2p did not find %s — it is not reading the sentinels, so a clean "+
				"result below would say nothing (found %d)", must, len(declared))
		}
	}
	rows := map[string]bool{}
	for _, o := range armOutcomes {
		if o.name == "" {
			continue
		}
		if rows[o.name] {
			t.Errorf("%s has two rows in armOutcomes; the first wins and the second is dead", o.name)
		}
		rows[o.name] = true
	}
	for name := range declared {
		if !rows[name] {
			t.Errorf("p2p.%s has no row in armOutcomes. An arm that ends a connection with it falls to "+
				"the default — spent, with a log line and no reason — instead of an outcome somebody "+
				"decided. Give it a row: does a party who got through decide it, or did the channel fail?", name)
		}
	}
	for name := range rows {
		if !declared[name] {
			t.Errorf("armOutcomes has a row named %s and p2p declares no such sentinel", name)
		}
	}
}

// TestAnUnclassifiedOutcomeEndsTheArmAndSaysSoOnce — inventory row A6's other half: the default is
// "spent", and it is not silent.
func TestAnUnclassifiedOutcomeEndsTheArmAndSaysSoOnce(t *testing.T) {
	var buf bytes.Buffer
	old := log.Writer()
	log.SetOutput(&buf)
	defer log.SetOutput(old)
	s := &Server{}
	strange := errors.New("an outcome no row answers for")
	if !s.armSpentBy(consentAnchor{kind: armInteractive}, nil, strange) {
		t.Fatal("an outcome nobody classified kept the arm — the default must be the safe direction")
	}
	if n := strings.Count(buf.String(), "no row for"); n != 1 {
		t.Errorf("the unclassified outcome was logged %d time(s), want 1:\n%s", n, buf.String())
	}
	if !strings.Contains(buf.String(), strange.Error()) {
		t.Errorf("the log line does not name the outcome:\n%s", buf.String())
	}
	buf.Reset()
	// And a classified one is not logged as unclassified.
	s.armSpentBy(consentAnchor{kind: armInteractive}, nil, p2p.ErrCoSignDeclined)
	if strings.Contains(buf.String(), "no row for") {
		t.Errorf("a decline was logged as unclassified:\n%s", buf.String())
	}
}

// TestAnEndedArmsNoticeIsOnlyForAnArmStillArmed — the notice is the arm's, so a gate torn down by
// Cancel (which leaves through the timeout's sentinel) or a goroutine outliving its arm writes none.
func TestAnEndedArmsNoticeIsOnlyForAnArmStillArmed(t *testing.T) {
	s := &Server{}
	cer := &ceremonyID{}
	gone := consentAnchor{cer: cer, kind: armInteractive}
	if !s.armSpentBy(gone, nil, p2p.ErrConsentTimedOut) {
		t.Fatal("a consent timeout kept the arm")
	}
	if n := s.sess.status().Notice; n != nil {
		t.Fatalf("an arm that is no longer armed left a notice: %+v", n)
	}
	s.sess.arms[armInteractive] = &arm{kind: armInteractive, cer: cer}
	s.armSpentBy(gone, nil, p2p.ErrConsentTimedOut)
	n := s.sess.status().Notice
	if n == nil || n.What != "arm-ended-unanswered" {
		t.Fatalf("a live arm ended by an unanswered consent left notice %+v", n)
	}
}

// ── The rule on real listeners (/pending 766, inventory rows A1–A4) ──────────────────────────────

// armOutcomeCase is one way a connection to an armed party can end, driven from the dialling side.
type armOutcomeCase struct {
	name string
	// consentWait shortens the gates for the two rows that wait one out; zero leaves five minutes.
	consentWait time.Duration
	// drive plays the dialling side on conn and the user at the armed machine through h.
	drive func(t *testing.T, h *armHarness, conn *p2p.Conn)
	// kept is what the rule says. notice is the sticky notice an ended arm must leave, or "".
	kept   bool
	notice string
}

type armHarness struct {
	c         *http.Client
	csrf      string
	url       string
	aCert     []byte
	aKey      []byte
	aFP       []byte
	aFPHex    string
	bFP       []byte
	aSigned   []byte
	address   string
	transport string
	initErr   chan error
}

// dial completes a pinned handshake to the armed listener, as the transport's production dialler does.
func (h *armHarness) dial(t *testing.T) *p2p.Conn {
	t.Helper()
	if h.transport == "tcp" {
		conn, err := p2p.Dial(context.Background(), h.address, h.aCert, h.aKey, h.bFP, 10*time.Second)
		if err != nil {
			t.Fatalf("setup: dial: %v", err)
		}
		t.Cleanup(func() { conn.Close() })
		return conn
	}
	end, err := p2p.NewSharedEndpoint("127.0.0.1:0")
	if err != nil {
		t.Fatalf("setup: endpoint: %v", err)
	}
	t.Cleanup(func() { end.Close() })
	hc, err := p2p.QUICDialHandshakeOn(context.Background(), end, h.address, h.aCert, h.aKey, h.bFP, 10*time.Second)
	if err != nil {
		t.Fatalf("setup: dial: %v", err)
	}
	conn, err := hc.Promote(context.Background(), true)
	if err != nil {
		t.Fatalf("setup: promote: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return conn
}

func (h *armHarness) status(t *testing.T) sessionStatus {
	t.Helper()
	var st sessionStatus
	sessGet(t, h.c, h.url+"/api/session/status", &st)
	return st
}

// initiate runs a real co-sign initiate on conn in the background; its error lands on h.initErr.
func (h *armHarness) initiate(conn *p2p.Conn) {
	go func() {
		_ = p2p.WriteRole(conn.Channel, p2p.RoleCoSign)
		_, err := p2p.Initiate(conn.Channel, h.aSigned, h.aFP, okVerifier{}, p2p.Roster{})
		conn.Close() // as every production dialler does once its exchange returns
		h.initErr <- err
	}()
}

// waitVerify waits for the spoken check to be on the armed machine's screen. It does not fail on the
// initiator's error: several rows expect one.
func (h *armHarness) waitVerify(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for h.status(t).Verify == nil {
		if time.Now().After(deadline) {
			t.Fatal("setup: the spoken check never reached the screen, so this row drove nothing")
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func (h *armHarness) waitPending(t *testing.T) string {
	t.Helper()
	deadline := time.Now().Add(15 * time.Second)
	for {
		if st := h.status(t); st.Pending != nil {
			return st.Pending.ID
		}
		if time.Now().After(deadline) {
			t.Fatal("setup: the consent request never reached the screen, so this row drove nothing")
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func rawFrame(t *testing.T, w io.Writer, b []byte) {
	t.Helper()
	var hdr [4]byte
	binary.BigEndian.PutUint32(hdr[:], uint32(len(b)))
	if _, err := w.Write(append(hdr[:], b...)); err != nil {
		t.Fatalf("setup: write frame: %v", err)
	}
}

func readRawFrame(t *testing.T, r io.Reader) []byte {
	t.Helper()
	var hdr [4]byte
	if _, err := io.ReadFull(r, hdr[:]); err != nil {
		t.Fatalf("setup: read frame header: %v", err)
	}
	b := make([]byte, binary.BigEndian.Uint32(hdr[:]))
	if _, err := io.ReadFull(r, b); err != nil {
		t.Fatalf("setup: read frame: %v", err)
	}
	return b
}

func armOutcomeCases() []armOutcomeCase {
	return []armOutcomeCase{
		{
			name: "the channel is lost before anything is said",
			drive: func(t *testing.T, h *armHarness, conn *p2p.Conn) {
				conn.Close()
			},
			kept: true,
		},
		{
			name: "the dial asks for a role this build cannot read",
			drive: func(t *testing.T, h *armHarness, conn *p2p.Conn) {
				if !conn.Channel.SpeaksRoleFrame() {
					t.Fatal("setup: the channel did not negotiate the role frame, so no role is read")
				}
				_ = conn.Channel.Stream.SetDeadline(time.Now().Add(5 * time.Second))
				rawFrame(t, conn.Channel.Stream, []byte{0xEE})
				// Hang up once the arm has had time to read it, as a dialler that got no answer does. The
				// arm's own close of a QUIC channel waits on this end, so a row that held the channel
				// open would be asking about an arm that had not yet been told the outcome.
				time.Sleep(500 * time.Millisecond)
				conn.Close()
			},
			kept: true,
		},
		{
			name: "the dial asks for a role this arm does not serve",
			drive: func(t *testing.T, h *armHarness, conn *p2p.Conn) {
				if err := p2p.WriteRole(conn.Channel, p2p.RoleTransfer); !errors.Is(err, p2p.ErrRoleRefused) {
					t.Fatalf("setup: a transfer dialled into a co-sign arm got %v, want the named refusal", err)
				}
				conn.Close() // refused: the dialler hangs up, and the arm's close stops waiting on it
			},
			kept: true,
		},
		{
			name: "the commitment check fails",
			drive: func(t *testing.T, h *armHarness, conn *p2p.Conn) {
				if err := p2p.WriteRole(conn.Channel, p2p.RoleCoSign); err != nil {
					t.Fatalf("setup: role: %v", err)
				}
				// The initiator's half of the commit-reveal exchange, with a reveal that is NOT what
				// was committed to — choosing after seeing, as it looks on the wire.
				s := conn.Channel.Stream
				_ = s.SetDeadline(time.Now().Add(10 * time.Second))
				commit := make([]byte, 32)
				reveal := make([]byte, 32)
				_, _ = rand.Read(commit)
				_, _ = rand.Read(reveal)
				rawFrame(t, s, commit)
				if got := readRawFrame(t, s); len(got) != 32 {
					t.Fatalf("setup: the arm's commitment is %d bytes", len(got))
				}
				rawFrame(t, s, reveal)
				_ = readRawFrame(t, s) // the arm reveals, then checks ours
				conn.Close()
			},
			notice: "arm-ended-tamper-check",
		},
		{
			name: "the channel is lost after the words are on screen",
			drive: func(t *testing.T, h *armHarness, conn *p2p.Conn) {
				h.initiate(conn)
				h.waitVerify(t)
				conn.Close()
				postJSON(t, h.c, h.csrf, h.url+"/api/session/verify", map[string]any{"confirmed": true})
			},
			kept: true,
		},
		{
			name: "the user says the words do not match",
			drive: func(t *testing.T, h *armHarness, conn *p2p.Conn) {
				h.initiate(conn)
				h.waitVerify(t)
				postJSON(t, h.c, h.csrf, h.url+"/api/session/verify", map[string]any{"confirmed": false})
			},
		},
		{
			name:        "nobody answers the spoken check",
			consentWait: 700 * time.Millisecond,
			drive: func(t *testing.T, h *armHarness, conn *p2p.Conn) {
				h.initiate(conn)
				h.waitVerify(t)
			},
			notice: "arm-ended-unanswered",
		},
		{
			name: "the user declines the document",
			drive: func(t *testing.T, h *armHarness, conn *p2p.Conn) {
				h.initiate(conn)
				h.waitVerify(t)
				postJSON(t, h.c, h.csrf, h.url+"/api/session/verify", map[string]any{"confirmed": true})
				id := h.waitPending(t)
				postJSON(t, h.c, h.csrf, h.url+"/api/session/respond", map[string]any{"id": id, "accept": false})
			},
		},
		{
			name:        "nobody answers the consent request",
			consentWait: 3 * time.Second,
			drive: func(t *testing.T, h *armHarness, conn *p2p.Conn) {
				h.initiate(conn)
				h.waitVerify(t)
				postJSON(t, h.c, h.csrf, h.url+"/api/session/verify", map[string]any{"confirmed": true})
				h.waitPending(t)
			},
			notice: "arm-ended-unanswered",
		},
	}
}

// TestADecisionEndsTheArmAndALostChannelKeepsItOnBothTransports — /pending 766, rows A1–A4.
//
// Every outcome, on a real TCP listener and a real QUIC one, each a ceremony arm made through the
// arm route and dialled by a genuinely pinned peer. What is read is what the user's page reads:
// `armed` on `/api/session/status`, and the sticky notice.
//
// The two arm loops used to answer this from two rules that disagreed. Over TCP a failed commitment
// check left the arm listening and a channel lost after the words were up ended it; over QUIC a
// refused role ended it. The notice an ended arm leaves must be the same sentence on both.
func TestADecisionEndsTheArmAndALostChannelKeepsItOnBothTransports(t *testing.T) {
	summaries := map[string]map[string]string{} // outcome → transport → the notice's summary
	for _, oc := range armOutcomeCases() {
		for _, transport := range []string{"tcp", "quic"} {
			oc, transport := oc, transport
			t.Run(transport+"/"+oc.name, func(t *testing.T) {
				consentWaitForTest.Store(int64(oc.consentWait))
				defer consentWaitForTest.Store(0)

				ts, _ := startServer(t)
				c, csrf := authedClient(t, ts)
				var me struct {
					Fingerprint string `json:"fingerprint"`
				}
				sessGet(t, c, ts.URL+"/api/peers", &me)
				bFP, _ := hex.DecodeString(me.Fingerprint)
				aCert, aKey, err := sign.GenerateIdentity("Alice")
				if err != nil {
					t.Fatal(err)
				}
				aFP, _ := sign.Fingerprint(aCert)
				aFPHex := hex.EncodeToString(aFP)
				pinPeer(t, c, csrf, ts.URL, aFPHex)
				prepared, bobInv := convenedTwoParty(t, aCert, aKey, aFPHex, me.Fingerprint)
				place, _ := p2p.NextPlacement(prepared)
				att := p2p.Attestation{Signer: "Alice", AcceptedPeer: me.Fingerprint, AcceptedPeerLabel: "Bob", Intent: "I agree", When: time.Now()}
				aSigned, err := p2p.Contribute(prepared, aCert, aKey, att, place)
				if err != nil {
					t.Fatal(err)
				}
				var armed sessionStatus
				sessDecode(t, write(t, c, csrf, http.MethodPost, ts.URL+"/api/session/arm", "application/json",
					jsonBody(armRequest{Fingerprint: aFPHex, Bind: "127.0.0.1:0", Transport: transport, Invitation: bobInv})), &armed)
				if !armed.Armed || armed.Address == "" {
					t.Fatalf("setup: arm failed: %+v", armed)
				}
				h := &armHarness{c: c, csrf: csrf, url: ts.URL, aCert: aCert, aKey: aKey, aFP: aFP, aFPHex: aFPHex,
					bFP: bFP, aSigned: aSigned, address: armed.Address, transport: transport, initErr: make(chan error, 1)}
				defer func() {
					write(t, c, csrf, http.MethodPost, ts.URL+"/api/session/disarm", "application/json", nil).Body.Close()
				}()

				oc.drive(t, h, h.dial(t))

				if oc.kept {
					// Kept, and LIVE: still armed after the loop has had time to act on the outcome
					// (the QUIC arm paces its way back, so this spans that), and a pinned peer can
					// still complete a handshake against it.
					until := time.Now().Add(1200 * time.Millisecond)
					for time.Now().Before(until) {
						if !h.status(t).Armed {
							t.Fatalf("the arm ENDED on %q — nobody who got through decided anything, "+
								"so the party is no longer reachable for the session it armed for", oc.name)
						}
						time.Sleep(50 * time.Millisecond)
					}
					again := h.dial(t)
					if len(again.Channel.PeerFP) == 0 {
						t.Fatal("the arm reports armed and a second pinned dial was not verified")
					}
					again.Close()
					if n := h.status(t).Notice; n != nil && strings.HasPrefix(n.What, "arm-ended") {
						t.Errorf("a kept arm left an arm-ended notice: %+v", n)
					}
					return
				}
				ended := time.Now().Add(oc.consentWait + 8*time.Second)
				for h.status(t).Armed {
					if time.Now().After(ended) {
						t.Fatalf("the arm is STILL ARMED after %q — a party who got through decided, or "+
							"the check that guards the words failed, and the listener went on to take "+
							"another connection as though nothing had happened", oc.name)
					}
					time.Sleep(50 * time.Millisecond)
				}
				st := h.status(t)
				if oc.notice == "" {
					if st.Notice != nil && strings.HasPrefix(st.Notice.What, "arm-ended") {
						t.Errorf("%q left notice %+v; the user did this themselves and needs no telling", oc.name, st.Notice)
					}
					return
				}
				if st.Notice == nil || st.Notice.What != oc.notice {
					t.Fatalf("the arm ended on %q and the status notice is %+v, want %q — the arm is "+
						"gone and nothing on the page says why", oc.name, st.Notice, oc.notice)
				}
				if st.Notice.Summary == "" || st.Notice.Detail == "" {
					t.Errorf("the notice has no sentence: %+v", st.Notice)
				}
				if summaries[oc.name] == nil {
					summaries[oc.name] = map[string]string{}
				}
				summaries[oc.name][transport] = st.Notice.Summary + "\n" + st.Notice.Detail
			})
		}
	}
	for name, by := range summaries {
		if len(by) == 2 && by["tcp"] != by["quic"] {
			t.Errorf("%q is told differently by transport:\n tcp:  %s\n quic: %s", name, by["tcp"], by["quic"])
		}
	}
}

// TestEveryWayRoundTheCeremonyArmLoopIsPaced — /pending 766, inventory row A7.
//
// `runCeremonyReceive` loops for the ceremony's whole life, and each way back to the top after a
// FAILURE is driven by the remote peer: how fast it comes round is how fast the peer fails. Three of
// them were paced through `reraceWait`; the unreadable-role path `continue`d bare, so a peer that
// completed the handshake and sent one bad byte was taken again as fast as it could send it.
//
// The property is routing: every `continue` in the loop sits directly after the wait, and the
// kept-after-a-failure branch at the bottom — where an unreadable role now lands — waits too. The
// one bare way round is a CLEAN completion, which is bounded by the re-delivery window and by a peer
// having to complete a whole exchange each time.
func TestEveryWayRoundTheCeremonyArmLoopIsPaced(t *testing.T) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, "session.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var fn *ast.FuncDecl
	for _, d := range f.Decls {
		if fd, ok := d.(*ast.FuncDecl); ok && fd.Name.Name == "runCeremonyReceive" {
			fn = fd
		}
	}
	if fn == nil {
		t.Fatal("setup: runCeremonyReceive not found in session.go — this guard walked nothing")
	}
	// isWait: `if !sleepOrDone(ctx, d) { return … }`.
	isWait := func(st ast.Stmt) bool {
		is, ok := st.(*ast.IfStmt)
		if !ok {
			return false
		}
		un, ok := is.Cond.(*ast.UnaryExpr)
		if !ok || un.Op != token.NOT {
			return false
		}
		call, ok := un.X.(*ast.CallExpr)
		if !ok {
			return false
		}
		id, ok := call.Fun.(*ast.Ident)
		return ok && id.Name == "sleepOrDone"
	}
	continues, paced := 0, 0
	ast.Inspect(fn, func(n ast.Node) bool {
		blk, ok := n.(*ast.BlockStmt)
		if !ok {
			return true
		}
		for i, st := range blk.List {
			br, ok := st.(*ast.BranchStmt)
			if !ok || br.Tok != token.CONTINUE {
				continue
			}
			continues++
			if i > 0 && isWait(blk.List[i-1]) {
				paced++
				continue
			}
			t.Errorf("runCeremonyReceive `continue`s at %s without waiting first. A peer that fails "+
				"there is taken again as fast as it fails, for the whole of the arm's window",
				fset.Position(br.Pos()))
		}
		return true
	})
	// STIMULUS: the walk found the loop's retries, so "none unpaced" is a fact about them.
	if continues < 2 {
		t.Fatalf("found %d `continue` in runCeremonyReceive, want the two retry arms at least — the "+
			"walk is not reading the loop", continues)
	}
	// And the bottom of the loop: the kept-after-a-failure branch, which has no `continue` to carry
	// the check above — it falls through to the top. It must hold the wait.
	var loop *ast.ForStmt
	ast.Inspect(fn, func(n ast.Node) bool {
		if fs, ok := n.(*ast.ForStmt); ok && loop == nil {
			loop = fs
		}
		return true
	})
	if loop == nil {
		t.Fatal("setup: runCeremonyReceive has no loop")
	}
	last, ok := loop.Body.List[len(loop.Body.List)-1].(*ast.IfStmt)
	if !ok {
		t.Fatalf("the ceremony arm's loop does not end in the kept-after-a-failure branch (%s)",
			fset.Position(loop.Body.List[len(loop.Body.List)-1].Pos()))
	}
	var cond bytes.Buffer
	if be, ok := last.Cond.(*ast.BinaryExpr); ok {
		if x, ok := be.X.(*ast.Ident); ok {
			cond.WriteString(x.Name + " " + be.Op.String())
		}
	}
	if cond.String() != "xerr !=" {
		t.Fatalf("the loop's last statement tests %q, want the kept outcome's `xerr != nil`", cond.String())
	}
	waits := false
	for _, st := range last.Body.List {
		if isWait(st) {
			waits = true
		}
	}
	if !waits {
		t.Error("an outcome that keeps the ceremony arm after a failure — a lost channel, an unreadable " +
			"or refused role — goes straight back round the loop with no wait. The peer sets the pace")
	}
}
