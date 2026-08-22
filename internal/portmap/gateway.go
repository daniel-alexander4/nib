package portmap

import (
	"errors"
	"net/netip"
)

// ErrNoGateway reports that no default gateway could be found — no route, or a platform whose
// discovery is not implemented. It is an ordinary tier miss, never a ceremony failure: the
// port-mapping tier simply contributes no candidate, exactly as D15 says all three protocols
// failing is a miss and not an error.
var ErrNoGateway = errors.New("portmap: no default gateway found")

// DefaultGateway returns the host's default-route gateway address, from which a NAT-PMP/PCP
// server is reachable on port 5351. Its implementation is per-platform (see the build-tagged
// files); a platform without one returns ErrNoGateway rather than a zero value, because a
// zero Addr that read as "0.0.0.0" would send mapping requests into the void and look like a
// gateway that never answered.
func DefaultGateway() (netip.Addr, error) {
	return defaultGateway()
}
