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
TESTBIN="$(mktemp -d)/discovery.test"
go test -c -o "$TESTBIN" ./internal/discovery/ >/dev/null

run() {
  unshare -rn bash -c '
    set -e
    ip link set lo up
    ip link add d0 type dummy
    ip link set d0 up
    ip addr add 10.9.0.1/24 dev d0
    # Note what this does NOT reach: a dummy interface reports up|broadcast|RUNNING
    # (measured — an earlier version of this comment claimed the opposite, and a probe
    # that made FlagRunning a filter stayed green here, which is how the false claim
    # was caught). So the idle-no-carrier case that FlagRunning would wrongly exclude
    # is covered by the tier-1 table test, not by this namespace. A dummy also lacks
    # the MULTICAST flag and joins anyway, which is why the selection does not require
    # it: the kernel does not enforce it.
    NIB_MCAST_NETNS=1 "$0" -test.run "TestTwoProcessesDiscoverEachOther|TestTheSocketJoinsTheInterfacesItChose|TestOwnAnnouncementsAreFilteredByNonceNotAddress|TestTwoSocketsCanShareThePort" -test.v
  ' "$TESTBIN"
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
grep -q -- "--- SKIP" "$OUT" \
  && fail "a test skipped inside the namespace — in here nothing should be unable to run"

echo "PASS: two processes discovered each other over real multicast in a private namespace"
