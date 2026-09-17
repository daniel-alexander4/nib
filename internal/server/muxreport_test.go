package server

import (
	"strings"
	"testing"

	"nib/internal/udpmux"
)

// TestMuxReportSpeaksOnlyForADatagramNibItselfDropped — /pending 512.
//
// `udpmux`'s counters had no production reader on the socket a CEREMONY runs on:
// `SharedEndpoint.Stats()` was called by tests alone, and the only human-facing print of mux
// counters is `nib rendezvous`, which builds its own mux. So a read loop that dropped a datagram
// through a bug in Nib's own router could only ever look, to the user, like a quiet network.
// `muxReport` is that reader, and this pins the two things it must get right.
func TestMuxReportSpeaksOnlyForADatagramNibItselfDropped(t *testing.T) {
	// A working socket says NOTHING. Every other udpmux counter describes routing that worked,
	// and a line printed on every failed connect is a line a reader learns to skip — punchReport's
	// argument, applied to the same detail string.
	busy := udpmux.Stats{
		RoutedLongHeader: 40, RoutedByCID: 900, RoutedToDHT: 300,
		Learned: 3, Expired: 1, Peers: 2, DroppedQUIC: 7, DroppedDHT: 11,
	}
	if got := muxReport(busy); got != "" {
		t.Errorf("a socket that routed 1240 datagrams and dropped 18 to full queues reported %q; "+
			"a full queue is a slow consumer, not a defect, and this sentence blames Nib", got)
	}

	// A datagram lost to a panic is said, and the COUNT is said with it — "some datagrams" is
	// not a bug report.
	if got := muxReport(udpmux.Stats{Panicked: 3}); !strings.Contains(got, "3 datagram") {
		t.Errorf("muxReport with Panicked=3 = %q; it must name the count", got)
	}
	// And it must say whose fault it is, because the whole D19 surface around it is otherwise
	// telling the user about THEIR network.
	if got := muxReport(udpmux.Stats{Panicked: 1}); !strings.Contains(got, "defect in Nib") {
		t.Errorf("muxReport = %q; a user reading this alongside D19's network causes has no way "+
			"to tell it is not another one", got)
	}
}
