// Package peercred asks the kernel who owns the far end of a loopback connection (ADR-157). It is a
// leaf — standard library only — because two doors need the answer: the launch-key trade in
// internal/server and the render origin in internal/browserrender (ADR-201), and pdfops reaches the
// second, so the lookup cannot live in server.
package peercred

// State is what the kernel said about the far end of a connection.
type State int

const (
	Unknown State = iota // the kernel would not answer, or this platform cannot ask
	Gone                 // no live socket of this machine is the far end
	Found                // the far end is a socket of this machine, owned by the returned user
)
