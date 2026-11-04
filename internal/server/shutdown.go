package server

import (
	"errors"

	"nib/internal/pdfops"
	"nib/internal/tempdir"
)

// errShuttingDown is an install attempted on a Nib that is leaving. The install rechecks it under
// `s.mu` (`addDocCappedLocked`), so an import that finished after shutdown began opens no document.
var errShuttingDown = errors.New("Nib is closing, so it did not open another document")

// Shutdown is the ONE teardown of what outlives a request (ADR-185, D31): it stops installs, then
// kills every converter process group synchronously, then closes every temporary directory still open
// (`tempdir.CloseAll`) — the group first, so nothing is writing into a directory as it is removed. `cmd/nib` calls it from the single place Quit
// (ADR-169), the idle exit and SIGTERM all arrive at — never from three — so a converter cannot be
// left running with the user's source in its temp directory because the exit that ended Nib happened
// to be the wrong kind. SIGKILL and a crash cannot be handled; the next start's sweep covers them.
//
// It returns how many converter groups it ended. Idempotent.
func (s *Server) Shutdown() int {
	s.shuttingDown.Store(true)
	n := pdfops.KillConverters()
	tempdir.CloseAll()
	return n
}
