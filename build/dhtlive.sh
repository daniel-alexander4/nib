#!/usr/bin/env bash
# Nib's ONE non-hermetic harness: the self-address probe against the real BitTorrent DHT.
#
# Usage: ./build/dhtlive.sh
#        NIB_LIVE_SEEDS=ip:port[,ip:port...] ./build/dhtlive.sh
#
# NIB_LIVE_SEEDS is P04.S06's rescue path: seeds the OPERATOR resolved, standing in for
# ones an invitation would carry. Nib must never resolve a name on the bootstrap path and
# a live seed test is the one place tempted to, so the addresses are an input rather than
# a lookup. Without it that test skips and everything else here runs unchanged.
#
# ── Why this is out of the routine loop ──────────────────────────────────────
# Every numbered tier is hermetic, and tier 3 was deliberately MADE hermetic (v1.109.3)
# after an outbound update check turned an unrelated github.com outage into a red on a
# test about console errors. A tier that reaches the public internet imports every
# stranger's outage into your build.
#
# So this is shaped like ./build/winrepro.sh instead: named in the contract, run
# deliberately, never part of `go build && go test && ...`. Run it when touching
# internal/rendezvous, the seed list, or anything about NAT classification.
#
# ── Why it has to exist ──────────────────────────────────────────────────────
# P04.S02's acceptance says this side learns its own public IP:port "from DHT responses
# alone, and the port is observed ON THE WIRE FROM A REAL NODE, not inferred from the
# type carrying a port field". The phase-open note had already settled the
# representation by reading `krpc.Msg.IP`, and said in as many words that reading a type
# proves a library can represent a port, not that one comes back.
#
# No hermetic tier can close that. Two anacrolix servers on loopback DO set the `ip`
# field — the library sets it on every non-error reply — so a loopback test proves the
# plumbing and nothing whatever about the network. Measured on the public DHT the day
# this was written: 144 of 181 responders carry the field and 37 do not, and the
# bootstrap ROUTERS never do, so a router-only table reads zero.
#
# ── What this harness CANNOT discharge ───────────────────────────────────────
# **One machine, one network, one sample.** It reports the NAT class of whatever is
# between this host and the internet. It cannot tell you the probe is right about a
# carrier-grade NAT, a symmetric one, or a dual-stack host, because it has only ever
# seen this one — and the classification is a snapshot of one allocation rather than a
# property of the NAT (Linux MASQUERADE preserves a free source port, so a
# conditionally-symmetric NAT reads as endpoint-independent until that port is
# contended). It also measures nothing about FILTERING behaviour, which is the other
# half of whether a punch succeeds.
#
# And it depends on strangers. A run that fails here has three causes it does report
# apart — no network, a dead seed list, and a probe that is wrong — and telling them
# apart is the whole reason the test's messages are written the way they are.
#
# What it delegates upward: a second machine on a different network. That stays a
# Dan-only VERIFY item, exactly as the two-machine ceremony does.
set -euo pipefail
cd "$(dirname "$0")/.."

fail() { echo "FAIL: $*" >&2; exit 1; }

command -v go >/dev/null 2>&1 || { echo "SKIP: go is not installed"; exit 0; }

OUT="$(mktemp)"
trap 'rm -f "$OUT"' EXIT

echo "probing the public DHT (this leaves the machine)…"
NIB_LIVE_DHT=1 go test ./internal/rendezvous/ -run 'TestLive' -v -count=1 \
  >"$OUT" 2>&1 || { cat "$OUT"; fail "the live probe did not pass — read the message above; it names which of no-network, dead-seeds and a broken probe it was"; }

# THREE outcomes, not two, and collapsing the third into either of the others is the
# failure this block exists to prevent.
#
# The DHT is a third party. It can decline to talk to us — measured: after heavy
# crawling the public routers still answer `ping` and return no nodes to `find_node`,
# so a cold bootstrap yields an empty table. That is not a defect in the probe and
# must not be a red; it is also emphatically not a pass, because nothing was
# exercised. So it exits 0 saying SKIP, loudly, and says what was not verified.
#
# **Scoped PER TEST, and it was not always.** When this harness ran one named test, a
# whole-file grep for UNREACHABLE was the same thing as asking about that test. Once it
# began running every TestLive*, the same grep started reporting "the self-address probe is
# UNVERIFIED" whenever the *publish/fetch* test skipped — a false sentence about a test
# that had just passed — while suppressing the probe's own evidence lines and exiting
# before the per-test block below could run. A gate that widens with its subject has to be
# re-scoped with it.
if grep -q -- "--- SKIP: TestLiveSelfAddressProbe" "$OUT"; then
  # No trailing space in the pattern: the test emits "UNREACHABLE: ...", and requiring
  # a space after the word dropped the one line that says WHY the run skipped.
  sed -n 's/^[[:space:]]*[a-z_]*\.go:[0-9]*: //; /^\(LOCAL\|BOOTSTRAP\|UNREACHABLE\)/p' "$OUT"
  echo "SKIP: the public DHT did not answer — the self-address probe is UNVERIFIED by this run."
  echo "      Nothing about internal/rendezvous was checked here; try again later."
  exit 0
fi

# The stimulus, before anything is graded: the test must have RUN. Its opt-in skip is how
# it stays out of the routine loop, and a harness that read a skip as a pass would report
# exactly the coverage this file exists to provide.
grep -q -- "--- PASS: TestLiveSelfAddressProbe" "$OUT" \
  || { cat "$OUT"; fail "TestLiveSelfAddressProbe did not PASS — if it SKIPPED, NIB_LIVE_DHT did not reach it and this harness verified nothing"; }

# The publish/fetch clause is reported SEPARATELY, and a skip of it is not a failure.
#
# P04.S03's acceptance — "a published endpoint is retrievable by the peer" — is a claim
# about strangers, and strangers are entitled to be unreachable. But a skip must never be
# summed with a pass: this harness's whole discipline is that "it would not talk to us" is
# not evidence the code works. So the two outcomes print different sentences.
if grep -q -- "--- PASS: TestLivePublishAndFetch" "$OUT"; then
  PUBFETCH="verified"
elif grep -q -- "--- SKIP: TestLivePublishAndFetch" "$OUT"; then
  PUBFETCH="SKIPPED — the DHT did not answer; the publish/fetch round trip is UNVERIFIED by this run"
else
  cat "$OUT"; fail "TestLivePublishAndFetch neither passed nor skipped — it failed"
fi

# The invitation-seed rescue, reported separately for the same reason publish/fetch is —
# with a THIRD outcome of its own, because this one has two ways of not running. Absent
# NIB_LIVE_SEEDS the mechanism was never offered any seeds; present but with a working
# shipped list, the mechanism is correctly never entered. Neither is evidence, and neither
# is a defect, and calling both "skipped" would hide which of them happened.
SEEDRESCUE="not attempted — set NIB_LIVE_SEEDS to drive it"
if [ -n "${NIB_LIVE_SEEDS:-}" ]; then
  if grep -q -- "--- PASS: TestLiveInvitationSeedsRescue" "$OUT"; then
    SEEDRESCUE="verified — invitation seeds bootstrapped a machine the shipped list could not"
  elif grep -q -- "--- SKIP: TestLiveInvitationSeedsRescue" "$OUT"; then
    SEEDRESCUE="SKIPPED — either the shipped list worked (path never entered) or the supplied seeds answered nothing; UNVERIFIED by this run"
  else
    cat "$OUT"; fail "TestLiveInvitationSeedsRescue neither passed nor skipped — it failed"
  fi
fi

# Evidence, not just a green. Each of these is a separate claim the pass does not make.
grep -q "BOOTSTRAP" "$OUT" || fail "no bootstrap line — the seed list was never exercised"
grep -q "OBSERVED"  "$OUT" || fail "no node reported our address; the pass above is not about the wire"
grep -q "CLASSIFIED" "$OUT" || fail "nothing was classified; D19's test did not run"

# Strip go test's "    live_test.go:53: " prefix before matching, or this prints nothing
# at all and the harness reports a green with no evidence under it.
sed -n 's/^[[:space:]]*[a-z_]*\.go:[0-9]*: //; /^\(LOCAL\|BOOTSTRAP\|OBSERVED\|PROBE\|CLASSIFIED\|TRANSLATED\|NOTE\|PUBLISHER\|PUBLISHED\|HARVESTED\|FETCHER\|UNREACHABLE\) /p' "$OUT"

echo "PASS: a real DHT node reported this host's public endpoint, port included, and the"
echo "      mapping classified — from a cold start with no cached nodes"
echo "      publish/fetch round trip: $PUBFETCH"
echo "      invitation-seed rescue:   $SEEDRESCUE"
