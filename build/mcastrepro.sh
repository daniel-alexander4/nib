#!/usr/bin/env bash
# Tier 5 of Nib's test harness: link-local discovery between two PROCESSES, over real
# multicast, inside a network namespace of its own.
#
# Usage: ./build/mcastrepro.sh
#
# ── Why a namespace ──────────────────────────────────────────────────────────
# Because otherwise the host decides the result. A multicast loopback copy traverses
# INPUT, so a default-deny firewall swallows discovery on Nib's port with NO ERROR at
# either end — measured on the development machine, where 224.0.0.251:5353 delivers and
# the same group on another port simply times out. A harness that ran on the host would
# be green on a permissive machine, red on a locked-down one, and in neither case
# testing this code.
#
# So this creates its own namespace with one dummy interface and runs the discovery
# tests inside it. Nothing about the host's rules, its other daemons, or whatever else
# is on its LAN reaches in.
#
# `unshare -r` maps the caller to root INSIDE the new user namespace, so no privilege
# is needed outside it and this is safe to run unattended.
#
# ── What this tier reaches that tiers 1-4 cannot ─────────────────────────────
# Tier 1's discovery tests SKIP on any host that swallows the group, which is honest
# and is why they skip rather than fail — but a skip is not a verification. This tier
# is where "two processes discover each other over real multicast" is actually driven:
# two separate OS processes, two sockets sharing a port, two group memberships, and a
# datagram that really crosses a kernel.
#
# ── Where it still stops ─────────────────────────────────────────────────────
# **A dummy interface is not a network.** There is no other host, no switch, no IGMP
# snooping, no MTU below 1500 that matters, and no second machine to disagree with.
# What this proves is that the code joins, sends, receives, and filters correctly; it
# says nothing about a real LAN with real switches, which is P03's exit criterion and
# stays the two-machine VERIFY item.
#
# **It cannot see Windows at all**, and that is the one this tier most conspicuously
# does not cover: x/net's SetControlMessage is unimplemented there, IPv4 group joins
# resolve the interface to an address rather than an index, and FlagRunning carries no
# information. Those are P03.S05's, and they close on a real-Windows run that is Dan's.
set -euo pipefail
cd "$(dirname "$0")/.."

fail() { printf 'FAIL: %s\n' "$*" >&2; exit 1; }

# ── Teardown, which this tier ran without (/pending 518) ─────────────────────
# `BINDIR` holds two COMPILED test binaries and `$OUT` the whole namespace log, and
# neither was ever removed — on the green path as much as the red one. **Measured: 50 MB
# per run** (`discovery.test` 6 MB, `server.test` 45 MB), and two such directories were
# already sitting in /tmp when this was found. Every other harness that calls `mktemp`
# sets a trap — uirepro, pairrepro, ceremonyrepro, winrepro, dhtlive, gen-notices,
# redproof — and this was the one that did not; `verify_test.go` now asserts that, so a
# harness added without one fails rather than being noticed by somebody's `du`.
#
# The LOG is kept when the run failed, because on a red run it is the whole diagnosis —
# the same split `pairrepro.sh`'s `cleanup` makes, and for the same reason. The binaries
# go either way: they are rebuildable from the tree and they are the 50 MB.
#
# Written as `if` blocks, not `&&` chains: this script sets `-e`, and a trailing `[ -n
# "$X" ] && …` that goes false makes the handler exit early, skipping the cleanup below it.
BINDIR=""
OUT=""
teardown() {
  local st=$?
  if [ -n "$BINDIR" ]; then rm -rf "$BINDIR"; fi
  if [ -n "$OUT" ]; then
    if [ "$st" = 0 ]; then
      rm -f "$OUT"
    else
      printf 'the run failed — the namespace log is kept at %s\n' "$OUT" >&2
    fi
  fi
}
trap teardown EXIT

# Skip cleanly and separately, because a missing unshare and a missing ip are
# different fixes — the same rule tiers 2 and 3 follow for their own dependencies.
for dep in go unshare ip; do
  command -v "$dep" >/dev/null 2>&1 || { echo "SKIP: $dep is not installed"; exit 0; }
done
if ! unshare -rn true 2>/dev/null; then
  echo "SKIP: unprivileged network namespaces are unavailable here (kernel or seccomp policy)"
  exit 0
fi

echo "building the discovery tests…"
# Compiled OUTSIDE the namespace: inside it there is no network, and a build that
# needed to fetch anything would fail for a reason that has nothing to do with
# multicast. Everything the test needs is in the module cache by now.
BINDIR="$(mktemp -d)"
TESTBIN="$BINDIR/discovery.test"
go test -c -o "$TESTBIN" ./internal/discovery/ >/dev/null
# The server's tests too: resolving an announcement to a dialable candidate is the
# other half of discovery, and it lives in internal/server because internal/discovery
# is guarded against importing the vault (L1). One namespace, both halves.
SRVBIN="$BINDIR/server.test"
go test -c -o "$SRVBIN" ./internal/server/ >/dev/null

# The tests this tier runs, named ONCE (/pending 505). The `-test.run` pattern and the PASS
# assertions below are both built from these lists: the run line used to name six discovery tests
# and the greps checked three, so `TestTheSocketJoinsTheInterfacesItChose`,
# `TestOwnAnnouncementsAreFilteredByNonceNotAddress` and `TestTwoSocketsCanShareThePort` could be
# renamed — matching nothing, running nothing, printing nothing — with this tier still green.
DISC_TESTS=(
  TestTwoProcessesDiscoverEachOther
  TestTheSocketJoinsTheInterfacesItChose
  TestOwnAnnouncementsAreFilteredByNonceNotAddress
  TestTwoSocketsCanShareThePort
  TestAnIPv6OnlyInterfaceIsSkippedForTheIPv4Group
  TestAnOffLinkUnicastIsDroppedByTheReadLoop
)
SRV_TESTS=(TestARealAnnouncementResolvesToACandidate)
join_re() { local IFS='|'; printf '^(%s)$' "$*"; }
DISC_RE="$(join_re "${DISC_TESTS[@]}")"
SRV_RE="$(join_re "${SRV_TESTS[@]}")"

run() {
  unshare -rn bash -c '
    set -e
    ip link set lo up
    ip link add d0 type dummy
    ip link set d0 up
    ip addr add 10.9.0.1/24 dev d0
    # A SECOND interface with only an IPv6 address. This is the Windows divergence made
    # visible on Linux: an IPv4 group join on Windows resolves the interface to an
    # ADDRESS (setIPv4MreqToInterface walks ifi.Addrs()), so an interface whose IPv4
    # lease has not arrived is joinable here and refused there. Requiring the address on
    # both platforms keeps the chosen set identical, and this is the only interface in
    # the tree that can exercise it without a Windows machine.
    ip link add d2 type dummy
    ip link set d2 up
    ip addr add fd00:9::1/64 dev d2
    # A THIRD interface on a subnet of its own. The off-link test builds a socket that
    # joins d0 only and then sends from the d3 address, which is a real source address on
    # a link the socket did not join — the one shape that exercises the ingress scope
    # check without spoofing.
    ip link add d3 type dummy
    ip link set d3 up
    ip addr add 10.99.0.1/24 dev d3
    # Note what this does NOT reach: a dummy interface reports up|broadcast|RUNNING
    # (measured — an earlier version of this comment claimed the opposite, and a probe
    # that made FlagRunning a filter stayed green here, which is how the false claim
    # was caught). So the idle-no-carrier case that FlagRunning would wrongly exclude
    # is covered by the tier-1 table test, not by this namespace. A dummy also lacks
    # the MULTICAST flag and joins anyway, which is why the selection does not require
    # it: the kernel does not enforce it.
    NIB_MCAST_NETNS=1 "$0" -test.run "$2" -test.v
    NIB_MCAST_NETNS=1 "$1" -test.run "$3" -test.v
  ' "$TESTBIN" "$SRVBIN" "$DISC_RE" "$SRV_RE"
}

OUT="$(mktemp)"
if ! run >"$OUT" 2>&1; then
  cat "$OUT" >&2
  fail "the discovery tests did not pass inside the namespace"
fi
cat "$OUT"

# The stimulus, asserted before anything is graded: the driven test must have RUN, not
# skipped. Its skip path is deliberate on a hostile host, and a harness that treated a
# skip as a pass would report coverage this tier exists to provide.
grep -q -- "--- PASS: TestTwoProcessesDiscoverEachOther" "$OUT" \
  || fail "TestTwoProcessesDiscoverEachOther did not PASS inside the namespace — if it SKIPPED, the namespace was not detected and this harness verified nothing"
grep -q "DISCOVERED" "$OUT" \
  || fail "no process reported discovering another; the pass above is not about discovery"
# Every named test PASSED — each one, from the same list the run line was built from. The
# trailing ` (` pins the exact name, so `TestX` cannot be satisfied by a `TestXAndMore`.
for t in "${DISC_TESTS[@]}" "${SRV_TESTS[@]}"; do
  grep -qF -- "--- PASS: $t (" "$OUT" \
    || fail "$t did not PASS inside the namespace — renamed, skipped, or not run; this tier names it and must see it pass"
done
grep -q "RESOLVED" "$OUT" \
  || fail "no announcement was resolved to a candidate; the pass above is not about resolution"
grep -q -- "--- SKIP" "$OUT" \
  && fail "a test skipped inside the namespace — in here nothing should be unable to run"

echo "PASS: two processes discovered each other over real multicast, and an announcement"
echo "      resolved to a dialable candidate — in a private namespace"
