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
	c.mu.Lock()
	self := c.self
	in := d19Inputs{
		dhtResponded:     c.rz.Stats().Responses > 0, // rz.Stats is mutex-guarded; safe under c.mu
		peerSeen:         c.peerSeen.Load(),
		mappingDependent: self.V4.Mapping == rendezvous.MappingEndpointDependent || self.V6.Mapping == rendezvous.MappingEndpointDependent,
		havePortMap:      c.portMap != nil,
		mapUnroutable:    c.mapUnroutable,
		sharedSpace:      self.SharedAddressSpace,
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
	mappingDependent bool // this side is endpoint-dependent on either family
	havePortMap      bool // a routable port-map was obtained and published
	mapUnroutable    bool // the router answered but the address was unpublishable (double-NAT)
	sharedSpace      bool // the mapped address is carrier space (100.64/10)
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
		// D9: offer port-forward ONLY when the user controls a NAT — the router gave NO answer AND the
		// address is not carrier space. Within cause 3 an "answer" is always the unroutable double-NAT
		// one (a routable answer would be havePortMap), which cannot be port-forwarded either.
		if in.mapUnroutable || in.sharedSpace {
			d.detail = "This machine is behind a network that hands out a different address per " +
				"destination — a carrier-grade or double NAT you can't open a port on. A VPN you " +
				"both already run is the way through."
		} else {
			d.detail = "This machine is behind a network that hands out a different address per " +
				"destination. Enabling UPnP or a port-forward on your router, or using a VPN you " +
				"both run, would let it connect."
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
