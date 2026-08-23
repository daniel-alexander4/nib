package server

import (
	"errors"
	"net/http"

	"nib/internal/p2p"
	"nib/internal/rendezvous"
)

// D19 diagnosis (P05.S11): a FAILED ceremony connect is classified on the mapping/filtering axis
// into one of four causes (cause 5, clock skew, is a typed p2p.ClockSkewError the caller lifts
// first). Each carries a plain-language SUMMARY and a technical DETAIL behind a disclosure (D19's
// presentation pin). The classification is DIAGNOSTIC ONLY (L1) — it changes the message, never the
// pin check.
type d19Cause int

const (
	// causeUndiagnosed — not a diagnosable ceremony (the manual/LAN dial or a TCP ceremony with no
	// rendezvous). The caller falls back to the generic message.
	causeUndiagnosed d19Cause = iota
	// causeDHTUnreachable (cause 2) — no DHT response reached us at all.
	causeDHTUnreachable
	// causePeerNotStarted (cause 1) — the DHT answered, but the peer published nothing.
	causePeerNotStarted
	// causeMappingDependent (cause 3) — the peer published; this side's mapping is endpoint-dependent
	// with no routable port-map, so a direct link between these networks is not possible.
	causeMappingDependent
	// causeOther (cause 4) — the peer published, the mapping classes do not explain the failure.
	causeOther
	// causePeerRecordUnusable — the DHT answered and held a record for this peer, but the gate could
	// not turn it into a candidate: refused as stale/wrong-ceremony/forged, or a valid record with no
	// address yet. Distinct from causePeerNotStarted, which is "nothing for this peer at all" — telling
	// a user whose peer HAS published to "open the ceremony" is the confident-false statement this
	// separates out (the gate's own Refused() counters exist because that conflation is otherwise
	// invisible). Appended after causeOther so the existing cause values do not shift.
	causePeerRecordUnusable
)

// diagnosis is the diagnostic message for a failed connect: a plain SUMMARY and a technical DETAIL.
type diagnosis struct {
	cause   d19Cause
	summary string
	detail  string
}

// diagnose classifies a failed connect from the signals gathered on the ceremony (P05.S11). It is
// called AFTER connect has returned a failure — connect joins the feed goroutines first, so reading
// the gate here is safe. The predicates are ORDERED (cause 2 before cause 1, grill P1): both leave
// `gate.Accepted == 0`, and "no DHT at all" must be distinguished from "DHT answered, peer silent"
// by a CUMULATIVE field (`Responses`), never the last-fetch-only `FetchNodes`.
func (c *ceremonyID) diagnose() diagnosis {
	if c == nil || c.rz == nil || c.gate == nil {
		return diagnosis{cause: causeUndiagnosed} // manual/LAN or TCP ceremony — out of D19's scope
	}
	// The gate itself is NOT read here: diagnose() also runs on the live-status path, concurrently
	// with the feed that writes the gate, so the record-refused/empty signals are snapshotted into
	// atomics in feedCandidates (the gate's only writer) and read here. This is the same discipline
	// peerSeen follows, and it is why diagnose() reads only atomic/mutex-guarded signals.
	c.mu.Lock()
	self := c.self
	in := d19Inputs{
		dhtResponded:     c.rz.Stats().Responses > 0, // rz.Stats is mutex-guarded; safe under c.mu
		peerSeen:         c.peerSeen.Load(),
		recordRefused:    c.recordRefused.Load(),
		recordEmpty:      c.recordEmpty.Load(),
		mappingDependent: self.V4.Mapping == rendezvous.MappingEndpointDependent || self.V6.Mapping == rendezvous.MappingEndpointDependent,
		havePortMap:      c.portMap != nil,
		mapUnroutable:    c.mapUnroutable,
		mapRefused:       c.mapRefused,
		sharedSpace:      self.SharedAddressSpace,
		v6Independent:    self.V6.Mapping == rendezvous.MappingEndpointIndependent && self.V6.Addr.IsValid(),
	}
	c.mu.Unlock()
	return classifyD19(in)
}

// d19Inputs are the four (plus advice) signals a diagnosis is classified from. Extracting them makes
// classifyD19 a PURE function of the network signals — no peer identity in sight (the L1 pin: the
// classification is diagnostic only) and every cause driveable in a unit test without a live DHT.
type d19Inputs struct {
	dhtResponded     bool // any DHT reply reached us (cumulative Responses>0)
	peerSeen         bool // a candidate for the peer was admitted
	recordRefused    bool // the gate saw a record for this peer and rejected it (stale/wrong/forged)
	recordEmpty      bool // a valid, verified record for this peer that carried no address
	mappingDependent bool // this side is endpoint-dependent on either family
	havePortMap      bool // a routable port-map was obtained and published
	mapUnroutable    bool // the router answered but the address was unpublishable (double-NAT)
	mapRefused       bool // a gateway answered the port-map tier and REFUSED (a result code)
	sharedSpace      bool // the mapped address is carrier space (100.64/10), and V4-ONLY — see below
	v6Independent    bool // this side has a directly reachable IPv6 endpoint, whatever V4 is doing
}

// classifyD19 turns the signals into D19's cause + message. Predicates are ORDERED: cause 2 (no DHT)
// before cause 1 (DHT up, peer silent) — both leave peerSeen false — then cause 3, then cause 4.
func classifyD19(in d19Inputs) diagnosis {
	// Cause 2 FIRST — no reply from any DHT node reached us, ever (cumulative).
	if !in.dhtResponded {
		return diagnosis{
			cause:   causeDHTUnreachable,
			summary: "Couldn't reach the rendezvous network.",
			detail: "Nib got no reply from the peer-to-peer directory it uses to find the other " +
				"side — most often a network that blocks outbound UDP. You can still connect over " +
				"your local network, or by typing the other machine's address, or over a VPN you " +
				"both already run.",
		}
	}
	// The directory answered AND held a record for this peer, but the gate could not use it — BEFORE
	// cause 1, because both leave peerSeen false and "we saw their record" is the more specific (and
	// more truthful) thing to say. Refused (stale/wrong-ceremony/forged) and empty (up but no address
	// yet) get different advice; neither is "they haven't started".
	if !in.peerSeen && (in.recordRefused || in.recordEmpty) {
		if in.recordRefused {
			return diagnosis{
				cause:   causePeerRecordUnusable,
				summary: "Found the other side, but couldn't use what they published.",
				detail: "Nib reached the directory and found a record for this peer but rejected it — " +
					"it may be stale, from a different ceremony, or its clock may be off. Check that " +
					"you are both in the same ceremony and your clocks are set, then try again.",
			}
		}
		return diagnosis{
			cause:   causePeerRecordUnusable,
			summary: "The other side is connecting but hasn't published an address yet.",
			detail: "Nib found this peer in the directory, but they have not learned a reachable " +
				"address yet. Give it a moment and try again.",
		}
	}
	// Cause 1 — the directory answered but held nothing for this peer.
	if !in.peerSeen {
		return diagnosis{
			cause:   causePeerNotStarted,
			summary: "The other side hasn't started their ceremony yet.",
			detail: "Nib reached the directory but found no address published for this peer. Ask " +
				"them to open the ceremony on their machine, then try again.",
		}
	}
	// Cause 3 — the peer published; this side is endpoint-dependent with no routable mapping. One-sided
	// (D17): only THIS side's class is known. Either family being dependent is enough.
	if in.mappingDependent && !in.havePortMap {
		d := diagnosis{cause: causeMappingDependent, summary: "A direct connection between these two networks isn't possible."}
		// FIRST: do not deny reachability this side HAS. `mappingDependent` is either family, so a
		// DS-Lite / 464XLAT host — V4 endpoint-dependent through the carrier, V6 endpoint-independent
		// and global — reached this branch and was told a direct connection "isn't possible" while
		// holding a directly reachable IPv6 endpoint that P05.S05 built the tier for. That is a false
		// statement about reachability, which is worse than bad advice.
		if in.v6Independent {
			d.summary = "The IPv4 path between these two networks is blocked."
			d.detail = "This machine's IPv4 address changes depending on where it connects, so an " +
				"IPv4 connection can't be made — but it does have a directly reachable IPv6 " +
				"address. If the other side has IPv6 too, try again. If they don't, a VPN you both " +
				"run is the way through."
			return d
		}
		// D9: offer a port-forward ONLY when the user controls a NAT. The two branches below are
		// "the router told us something" and "we learned nothing", and only the first is evidence.
		// A gateway that ANSWERED and refused is the one case where a manual port-forward is
		// exactly the right advice: the router is the user's, it is reachable, and UPnP is not
		// merely on but talking — which is why they got a refusal instead of silence. Before the
		// two branches below, because both of those are about a router that told us nothing
		// useful, and this one told us something (/pending 263).
		if in.mapRefused {
			d.detail = "This machine is behind a network that hands out a different address per " +
				"destination, and your router refused Nib's request to open a port automatically. " +
				"It is reachable and it is yours — so adding the port-forward by hand in the " +
				"router's own settings is the thing that will work here. A VPN you both run also " +
				"does it."
			return d
		}
		if in.mapUnroutable || in.sharedSpace {
			d.detail = "This machine is behind a network that hands out a different address per " +
				"destination — a carrier-grade or double NAT you can't open a port on. A VPN you " +
				"both already run is the way through."
		} else {
			// **This branch is reached by learning NOTHING, and it used to read that as evidence
			// the user controls a NAT.** `mapUnroutable` is set only where a gateway ANSWERED with
			// a screened-out address (ceremonynet.go); a gateway that never answers at all —
			// PCP absent, UPnP switched off, carrier NAT, an IPv6-transition network — leaves both
			// it and `sharedSpace` false and fell to here. `sharedSpace` cannot rescue it either:
			// it is `addrscope.SharedSpace`, 100.64/10, IPv4-only by construction, and DS-Lite,
			// NAT64 and 464XLAT are all invisible to a reflexive probe (a NAT64 prefix is a
			// DESTINATION, never a source — addrscope says so at the line). So the honest move is
			// not a better detector, which would only make the false statement more specific: it is
			// to stop promising. The advice is conditioned on a fact only the user has.
			d.detail = "This machine is behind a network that hands out a different address per " +
				"destination, and Nib couldn't get an answer from your router — so it can't tell " +
				"whether there is one you control. If there is, enabling UPnP or a port-forward on " +
				"it may help. If your connection goes through your provider's network instead, a " +
				"VPN you both run is the way through."
		}
		return d
	}
	// Cause 4 — the peer published, the classes do not explain it: filtering, a firewall, asymmetry.
	return diagnosis{
		cause:   causeOther,
		summary: "Couldn't establish a connection.",
		detail: "The other side published an address but the connection didn't complete — most " +
			"likely a firewall or filtering on one of the two networks. A VPN you both run is the " +
			"reliable fallback.",
	}
}

// causeName is the machine-readable tag P06 keys rendering on (D19's disclosure UI).
func causeName(c d19Cause) string {
	switch c {
	case causeDHTUnreachable:
		return "rendezvous-unreachable"
	case causePeerNotStarted:
		return "peer-not-started"
	case causeMappingDependent:
		return "mapping-dependent"
	case causeOther:
		return "connection-failed"
	case causePeerRecordUnusable:
		return "peer-record-unusable"
	default:
		return "unknown"
	}
}

// diagnosisResponse is the structured failure body a client (and P06's ceremony panel) reads: a
// plain-language `summary` first and a technical `detail` behind a disclosure (D19's presentation
// pin), plus a machine tag. `error` mirrors `summary` so a client that only reads the flat error
// field still shows the plain sentence.
type diagnosisResponse struct {
	Error   string `json:"error"`
	Cause   string `json:"cause"`
	Summary string `json:"summary"`
	Detail  string `json:"detail"`
}

// writeConnectDiagnosis renders a failed ceremony connect as D19's diagnosis (P05.S11). Cause 5
// (clock skew) is a typed p2p.ClockSkewError carrying its own message and is lifted FIRST (D35),
// mirroring connectFailure. A non-ceremony/TCP failure (causeUndiagnosed) falls back to the flat
// message. The classification is DIAGNOSTIC ONLY — it never touched the pin check (L1).
func (s *Server) writeConnectDiagnosis(w http.ResponseWriter, cer *ceremonyID, err error) {
	var skew *p2p.ClockSkewError
	if errors.As(err, &skew) {
		writeJSONStatus(w, http.StatusBadGateway, diagnosisResponse{
			Error:   skew.Error(),
			Cause:   "clock-skew",
			Summary: skew.Error(),
			Detail: "The two machines' clocks disagree by more than the connection tolerates, so " +
				"each rejected the other's short-lived certificate. Set this machine's clock and try again.",
		})
		return
	}
	d := cer.diagnose()
	if d.cause == causeUndiagnosed {
		httpError(w, http.StatusBadGateway, connectFailure(err))
		return
	}
	writeJSONStatus(w, http.StatusBadGateway, diagnosisResponse{
		Error:   d.summary,
		Cause:   causeName(d.cause),
		Summary: d.summary,
		Detail:  d.detail,
	})
}
