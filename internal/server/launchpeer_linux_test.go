//go:build linux

package server

import (
	"net/http"
	"net/netip"
	"os"
	"syscall"
	"testing"

	"nib/internal/peercred"
)

// needSockDiag skips where this machine's kernel will not open the diagnostics socket at all (a sandbox).
func needSockDiag(t *testing.T) {
	t.Helper()
	fd, err := syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_DGRAM|syscall.SOCK_CLOEXEC, 4) // NETLINK_SOCK_DIAG
	if err != nil {
		t.Skipf("no socket diagnostics on this machine: %v", err)
	}
	syscall.Close(fd)
}

// TestARealTradeIsAskedOfTheKernelAndPasses: the whole path with nothing replaced — a request over
// loopback from this process is found, owned by this user, and trades.
func TestARealTradeIsAskedOfTheKernelAndPasses(t *testing.T) {
	needSockDiag(t)
	ts, srv := startServerWith(t)
	var uid uint32
	var state peercred.State
	srv.launchPeerOwner = func(l, p netip.AddrPort) (uint32, peercred.State) {
		uid, state = peercred.Owner(l, p)
		return uid, state
	}
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/api/launch", nil)
	req.Header.Set(headerLaunch, srv.MintLaunchKey())
	res, err := (&http.Client{}).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if state != peercred.Found || uid != uint32(os.Geteuid()) {
		t.Errorf("the kernel's answer for a real trade = (uid %d, state %d), want (uid %d, found)", uid, state, os.Geteuid())
	}
	if res.StatusCode != http.StatusOK {
		t.Errorf("a real trade from this user answered %d, want 200", res.StatusCode)
	}
}
