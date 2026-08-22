//go:build !linux

package portmap

import "net/netip"

// defaultGateway is not implemented off Linux. It returns ErrNoGateway — the REAL error, not a
// zero value — so the port-mapping tier is simply absent on those platforms rather than
// silently sending requests to 0.0.0.0. This is the declared gap the Go-SME lesson insists on:
// a //go:build variant that no-ops must say so, never return success. macOS/BSD would read the
// routing socket (PF_ROUTE); Windows would call GetBestRoute. Both are follow-on work, and
// until then the tier misses on those platforms, which is an ordinary tier miss.
func defaultGateway() (netip.Addr, error) { return netip.Addr{}, ErrNoGateway }
