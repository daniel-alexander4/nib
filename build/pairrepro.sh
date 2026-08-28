#!/usr/bin/env bash
# Tier 4 of Nib's test harness: run N real nib binaries against each other and
# complete a ceremony between them — once per transport, TCP and QUIC (D14).
#
# Usage: ./build/pairrepro.sh [-n N] [--lan] [--v6] [--keep]
#
# ── Why a fourth tier ────────────────────────────────────────────────────────
# Tiers 1-3 all run ONE Nib. A ceremony is two people on two machines, and every
# assertion about one has to be made from the other's side: that the peer's
# fingerprint is a DIFFERENT identity, that the spoken words match on both
# screens, that the document coming back carries the other party's signature.
# A single-instance harness cannot express any of that — it can only ask the one
# process what it thinks, which is asking the thing under test.
#
# So this tier boots two Nibs with two homes, two vaults and two identities, and
# drives each one's HTTP API as its own user would. D26 added it as P02's first
# slice for exactly this reason: it is what lets any later phase prove a ceremony
# at all.
#
# ── What this tier reaches that tier 3 cannot ────────────────────────────────
#   * two identities, and therefore a real pinned-peer relationship;
#   * the p2p session over a real socket between two processes;
#   * the spoken verification check answered on BOTH sides, which is the only
#     way to see that the two derive the same words;
#   * a co-signed document carrying two signatures, verified from the receiving
#     side rather than reported by the sender.
#
# ── Where it still stops ─────────────────────────────────────────────────────
# **N instances are N parties on loopback: no NAT, and no second machine.** `-n N`
# (P07.S01) boots N homes, N vaults and N identities and asserts they are genuinely
# distinct. Since P07.S03b the N>=3 path also convenes a REAL ceremony through
# `/api/ceremony/convene`, hands instance 3 its invitation, and has it take part with
# no manual pin — so what this tier says about N is now about a ceremony and not only
# about the harness.
#
# The blind spots at N, stated rather than left to be discovered. **Two of the three
# that stood here until 2026-08-25 are gone, and the third was wrong about its own
# subject:**
#   * **The relay stops at hop 2, and NOT for the reason this file used to give.** It
#     said `coSignExchange` takes one prior signer only, "until P07.S03 removes it".
#     S03 conditioned that rule and the relay still stops — measured: hop 1 leaves
#     exactly the roster prefix, `/api/session/initiate` then applies the LOCAL
#     signature before it sends (`buildCoSigned`), so the carrier signs a SECOND time
#     and L3 refuses it by name at the carrier's own machine; and party 3, handed that
#     document unchanged, IS admitted. The ceiling is a missing ROUTE — nothing hands
#     the baton on without contributing — which is **P07.S05's carry verb**. The probe
#     asserts that refusal by name and goes red the day S05 lands.
#   * **Arm expiry is observable but not drivable.** A ceremony arm bounds itself by
#     `MaxCeremonyLife` and a manual one by `sessionAcceptTimeout` (5 min). Hop 1's
#     arms are still manual, so a slow run can fail for a reason that will not exist
#     once hop 1 is convened too.
#   * ~~The run manufactures permanent pins.~~ **Closed for the N>=3 path (P07.S03b):**
#     instance 3 is pinned by ACCEPTING its invitation, which is D21's whole point, and
#     the run asserts the accept established exactly one pin. Hop 1 still hand-pins and
#     is the remaining half.
#
# **`--lan` and `--v6` are N=2-only, and the refusal is enforced above rather than
# documented here alone.** `--lan`'s zero-egress assertion holds only while arms
# carry no invitation — from P07.S02b every arm is a ceremony arm that publishes to
# the DHT after its browse window, and N−1 of them would. `--v6` generalises
# mechanically but has nothing N-shaped to observe until a hop exists.
#
# **A `fail` inside a command substitution exits the SUBSHELL, not this script.**
# Every `$( )` whose result matters is checked by its caller for that reason; the
# EXIT trap takes `$?` as an argument rather than reading a flag, because a flag set
# in a subshell never reaches the parent and the work dir would be deleted on
# exactly the runs whose logs are the diagnosis.
#
# **It cannot see two networks.** Both instances are on loopback, so everything
# NAT, routing, MTU and firewall does to a real connection is invisible here —
# and those are precisely what P03-P05's ladder exists to survive. A ceremony
# that completes here says nothing about a ceremony between two houses.
#
# What it delegates upward is the Dan-only two-machine run, which stays a
# standing VERIFY item. That distinction is the point: the single-host case is
# driven here and is no longer Dan-only; the two-machine case never was this
# harness's to make.
#
# **`--v6` is P05.S05's contribution to that line (criterion 1).** IPv6-to-IPv6
# with neither side forwarding a port is a Dan-only TWO-machine run by the phase's
# own carve-out, and this harness spawns both instances on one host, so it does NOT
# make that run — the two-machine case stays the standing VERIFY item exactly like
# the v4 one above. What is buildable is the harness, reduced to this one command:
# it runs the whole ceremony — arm, pinned handshake, spoken check both sides, two
# signatures — over `[::1]` instead of `127.0.0.1`, asserting the bind is genuinely
# v6 so a v4 fallback cannot pass for it. `NIB_PAIR_V6_ADDR=<this host's global v6,
# no port>` runs it over a global v6 address rather than loopback — still one host,
# a stronger analogue than `[::1]` and still not two machines.
#
# It also drives the HTTP API rather than the browser, so it sees nothing about
# rendering, layout or what a user can click — tier 3 owns that.
#
# **And loopback flatters QUIC specifically.** Path MTU is 65536 here, there is no
# loss, and no middlebox has an opinion about UDP. A QUIC ceremony passing here
# says the protocol and the pinning work; it says nothing about the networks that
# drop UDP outright, which is the case D14 keeps TCP for. That gap belongs to the
# same two-machine VERIFY item and is named here so a green run is not read as
# evidence for it.
#
# Requires go, curl and python3. Skips cleanly without any of them, separately,
# because a missing curl and a missing python3 are different fixes.
set -uo pipefail
cd "$(dirname "$0")/.."

# ── Flags, parsed once, in any order ─────────────────────────────────────────
#
# **This replaced three independent reads of `$1` and it was a live defect, not
# tidying.** `LAN`, `KEEP` and `V6` each tested `"${1:-}"` alone, so only the FIRST
# argument was ever seen: `--keep --v6` set `KEEP=1, V6=0` and ran the ordinary
# loopback ceremony while the operator believed they had driven tier 4c, and the
# closing banner reads the same either way. That is ADR-010's shape — a run
# configured past the very disagreement it exists to find — arriving through the
# harness's own argument parsing rather than through the protocol.
#
# The namespace re-exec below carries the whole flag set for the same reason: it
# used to hard-code `--lan`, so `--lan --keep` dropped `--keep` even with a
# correct parser in front of it.
LAN=0; KEEP=0; V6=0; N=2
while [ $# -gt 0 ]; do
  case "$1" in
    --lan)  LAN=1 ;;
    --keep) KEEP=1 ;;
    --v6)   V6=1 ;;
    -n)     shift; N="${1:-}" ;;
    -n*)    N="${1#-n}" ;;
    -h|--help)
      echo "usage: $0 [-n N] [--lan] [--v6] [--keep]"; exit 0 ;;
    *) echo "FAIL: unknown argument '$1' — usage: $0 [-n N] [--lan] [--v6] [--keep]" >&2; exit 1 ;;
  esac
  shift
done
case "$N" in
  ''|*[!0-9]*) echo "FAIL: -n wants a whole number, got '$N'" >&2; exit 1 ;;
esac
[ "$N" -ge 2 ] || { echo "FAIL: -n $N — a ceremony needs at least two parties" >&2; exit 1; }
# **The N the outer invocation asked for, carried out of band and compared (P07.S05c).**
#
# The namespace re-exec below rebuilds its argument list from `$FLAGS`, a variable maintained by
# hand — and `-n` was missing from it, so `--lan -n 4` re-executed as `--lan` alone, ran a
# TWO-PARTY ceremony, and printed its pass. Nothing failed, because a two-party run passes its own
# assertions perfectly well; the operator simply did not get the run they asked for.
#
# **A red proof for that came back "the check still PASSED", which is how this guard came to
# exist.** There was no check. `$FLAGS` is the wrong place to assert — a structural test on a
# string says nothing about what the child received — so the requested N travels separately, in the
# environment, and the child compares what it PARSED against what was ASKED. Any future flag that
# goes missing from the list fails here by name instead of silently shrinking the run.
if [ -n "${NIB_PAIR_WANT_N:-}" ] && [ "$N" != "$NIB_PAIR_WANT_N" ]; then
  echo "FAIL: this run was asked for -n $NIB_PAIR_WANT_N and parsed -n $N — the re-exec into the" >&2
  echo "      network namespace dropped the flag, so the run would have completed a ${N}-party" >&2
  echo "      ceremony and reported it as a ${NIB_PAIR_WANT_N}-party one" >&2
  exit 1
fi
# **`--lan` accepts any N since P07.S05c; `--v6` is still N=2-only.**
#
# The pair were refused together at P07.S01, when there was no N-party relay for either to drive.
# There is now, and the LAN clause's ONLY driver is `--lan -n 9`: a nine-party ceremony has eight
# hops and the armed side's announcement lasts five minutes, so from the fourth party onward a
# same-room ceremony falls through to the public DHT unless the answering listener works. A refusal
# here left that clause with no way to be exercised at all — and it was the second of two barriers,
# the first being that the `N != 2` block `exit 0`s three lines before the `LAN` block.
#
# `--v6` stays N=2-only because nothing in the relay is v6-specific yet: the session ports would
# need bracketed literals throughout and the v6-bind stimulus is gated on `port != "lan"`. Refused
# by name rather than left to produce a run that looks like a v6 pass and is not.
if [ "$N" != "2" ] && [ "$V6" = "1" ]; then
  echo "FAIL: --v6 is N=2-only — the relay's ports are not v6-shaped yet, and a run that skipped" >&2
  echo "      the v6 stimulus would print an ordinary pass while proving nothing about v6" >&2
  exit 1
fi
# **The combination is refused, and leaving it unrefused was the surviving half of the
# very defect the parser above was written for.** With both set, `V6=1` and
# `CEREMONY_HOST=[::1]` — but every consumer of them is gated on `port != "lan"`, so
# the v6-bind stimulus is skipped and the transport probe takes the LAN branch and
# never touches `$PROBE_HOST`. The run then prints the ordinary LAN pass while the
# operator believes they drove 4b and 4c at once. The LAN arm binds no address at all,
# which is P03's whole point, so there is nothing for v6 to be.
if [ "$LAN" = "1" ] && [ "$V6" = "1" ]; then
  echo "FAIL: --lan and --v6 cannot be combined — the LAN arm binds no address, so nothing is v6" >&2
  exit 1
fi
# **The WHOLE flag set, and `-n` was missing from it (found P07.S05c).**
#
# The comment on the namespace re-exec below already records why this variable exists: it used to
# hard-code `--lan`, so `--lan --keep` silently dropped `--keep`. `-n` was added afterwards and
# never joined it, so `--lan -n 9` re-executed inside the namespace as `--lan` alone and ran the
# TWO-PARTY ceremony while the operator believed they had driven a nine-party one. The same defect
# the variable was created for, one flag later — which is what a list that has to be maintained by
# hand looks like when a new member arrives.
#
# It was the THIRD of three barriers between the LAN clause and its only driver. The other two: an
# explicit `--lan` is N=2-only refusal, and the `N != 2` block exiting three lines before the LAN
# block. Any one alone made `--lan -n 9` impossible, which is why nobody had ever run it.
FLAGS=""
[ "$LAN" = "1" ] && FLAGS="$FLAGS --lan"
[ "$KEEP" = "1" ] && FLAGS="$FLAGS --keep"
[ "$V6" = "1" ] && FLAGS="$FLAGS --v6"
FLAGS="$FLAGS -n $N"

for dep in go curl python3; do
  command -v "$dep" >/dev/null 2>&1 || {
    echo "$dep not installed; skipping the two-instance ceremony tests"; exit 0; }
done

# ── LAN mode: a ceremony with no address typed anywhere ──────────────────────
# `--lan` re-execs this whole harness inside a network namespace of its own, and
# runs the ceremony with NO bind address and NO peer address — the armed side
# announces on the link, the dialing side browses for it.
#
# **Why a namespace, again.** The same reason tier 5 needs one: a multicast
# loopback copy traverses INPUT, so a default-deny host swallows discovery on
# Nib's port with no error at either end. On the development machine this run
# would fail for the firewall's reasons rather than the code's.
#
# **And why the namespace has a DEFAULT ROUTE INTO A DUMMY**, which looks wrong
# and is the whole point. P03's exit criterion says the ceremony completes with
# "no outbound internet traffic", and that has to be ASSERTED, not assumed. An
# nft output counter is the instrument — but measured, a namespace with no
# default route reads ZERO EVEN AFTER A REAL CONNECT ATTEMPT, because the kernel
# refuses at the routing stage and the packet never reaches the output hook. The
# assertion would then be true of a process trying constantly. A black-hole
# default route makes attempts into real packets the counter can see: probed at
# 0 before, 2 after a connect to 1.1.1.1.
if [ "$LAN" = "1" ] && [ "${NIB_LAN_NS:-}" != "1" ]; then
  for dep in unshare ip nft; do
    command -v "$dep" >/dev/null 2>&1 || { echo "SKIP: $dep is not installed (--lan needs it)"; exit 0; }
  done
  unshare -rn true 2>/dev/null || { echo "SKIP: unprivileged network namespaces are unavailable here"; exit 0; }
  echo "building nib outside the namespace…"
  PREBUILT="$(mktemp -d)/nib"
  go build -o "$PREBUILT" ./cmd/nib || { echo "FAIL: could not build nib" >&2; exit 1; }
  exec unshare -rn bash -c '
    set -e
    ip link set lo up
    ip link add d0 type dummy
    ip link set d0 up
    ip addr add 10.9.0.1/24 dev d0
    ip -6 addr add fd00:9::1/64 dev d0
    ip route add default dev d0          # the black hole; see above
    ip -6 route add default dev d0       # …and its IPv6 half, so the v6 counter can fire
    nft add table inet egress
    nft add chain inet egress out "{ type filter hook output priority 0 ; }"
    # TWO rules, one per family. `ip daddr` in an inet table matches IPv4 ONLY, so a
    # single rule is blind to IPv6 — and the stimulus probe below was IPv4, which means
    # it proved the counter live for exactly the family the rule could see and could not
    # detect its own blind spot. Measured: 0 -> 2 on an IPv4 connect, UNCHANGED on an
    # IPv6 one. Nib announces on an IPv6 group and dials whichever family the browse
    # resolved, so the missing half was the half most likely to carry real traffic.
    nft add rule inet egress out ip daddr != 10.9.0.0/24 ip daddr != 127.0.0.0/8 \
      ip daddr != 224.0.0.0/4 counter comment "offlink4"
    nft add rule inet egress out ip6 daddr != fd00:9::/64 ip6 daddr != ::1 \
      ip6 daddr != ff00::/8 ip6 daddr != fe80::/10 counter comment "offlink6"
    NIB_LAN_NS=1 NIB_PAIR_WANT_N="$4" NIB_PAIR_BIN="$1" exec "$2" $3
  ' _ "$PREBUILT" "$0" "$FLAGS" "$N"
fi

# link_report prints what BOTH ends of a failed hop hear on the link.
#
# **Built because /pending 300 could not be diagnosed without it.** A QUIC relay followed by a TCP
# relay over the same instances fails at hop 1 with a D19 verdict about the DHT, for a peer that is
# on the link and announcing — and the only evidence available was the failure itself. Every
# candidate cause (a lingering announcement from the previous arm, an endpoint not released at
# disarm, the D19 cause being wrong again) is a question about what the dialler could SEE, and
# nothing could answer it.
#
# Both ends, because "the convener heard nothing" and "the party announced nothing" are different
# faults with the same symptom, and a report from one end cannot tell them apart.
link_report() { # from to transport
  local fi="$1" ti="$2" transport="$3"
  echo "--- what each end hears on the link [$transport] ---" >&2
  for i in "$fi" "$ti"; do
    echo "  instance $i:" >&2
    curl -fsS "${URLS[$((i-1))]}/api/lan/heard" 2>/dev/null \
      | python3 -c 'import json,sys
d=json.load(sys.stdin)
if d.get("note"): print("    note:", d["note"])
for h in d.get("heard", []):
    print(f"    {h[\"label\"]:<10} {h[\"addr\"]:<24} {h[\"transport\"]}")
if not d.get("heard"): print("    (heard nothing)")' >&2 || echo "    (could not ask)" >&2
  done
}

# Reads the off-link packet counter the namespace installed.
offlink_packets() {
  # The SUM across both family rules. Reading only the first was the IPv6 blind spot.
  nft -j list table inet egress 2>/dev/null \
    | python3 -c 'import json,sys
d=json.load(sys.stdin); t=0
for o in d.get("nftables",[]):
    r=o.get("rule")
    if not r: continue
    for e in r.get("expr",[]):
        if "counter" in e: t += e["counter"]["packets"]
print(t)'
}


# ── v6 mode: the ceremony transport runs over IPv6 loopback ──────────────────
# `--v6` is P05.S05's hermetic analogue of criterion 1 ("IPv6-to-IPv6 completes
# with neither side forwarding a port"). The two instances' HTTP control planes
# stay on 127.0.0.1 — they are not what is under test — but the p2p ceremony
# socket binds and dials `[::1]` instead of `127.0.0.1`, so a ceremony completing
# here is a ceremony completing across a v6 socket end to end. That is the half
# T01 could not reach: T01 proved the socket answers a datagram, this proves a
# whole ceremony (arm, pinned handshake, spoken check on both sides, a document
# with two signatures) crosses it.
#
# **The bind, the dial address and the transport probe change family together, or
# the run is a v4 run wearing a v6 label.** The arm binds `[::1]:port`, initiate
# is handed `-F address=[::1]:port`, and the /dev/tcp transport probe uses the
# bare `::1` (bash's /dev/tcp wants no brackets; the HTTP/arm paths want them).
# If any one of the three stayed `127.0.0.1` the ceremony would still complete —
# over v4 — and report a v6 pass it did not earn, the same "configured past the
# thing it exists to find" hazard the LAN transport probe and ADR-010 are about.
#
# **T07's "one command" is this flag**, and the hermetic loopback run is its whole
# buildable content — criterion 1 proper is two machines and no single-host script
# reaches it. `NIB_PAIR_V6_ADDR` takes a HOST with no port (e.g. `[2001:db8::5]`,
# or `fd00:9::1` in a ULA namespace); the harness appends `:$port` itself, so a
# value carrying a port would bind `…:port:port` and fail. It moves the ceremony
# off loopback onto a real global/ULA address on the same host — closer to the
# real path, still one host.

# The ceremony transport host, in the two spellings the tools want: bracketed for
# the arm bind and the dialled address (host:port URLs), bare for bash /dev/tcp.
# NIB_PAIR_V6_ADDR overrides the loopback default for the two-machine run.
CEREMONY_HOST="127.0.0.1"   # host:port form for arm bind and -F address=
PROBE_HOST="127.0.0.1"      # bare form for /dev/tcp
if [ "$V6" = "1" ]; then
  # Accept the override bracketed or bare; normalise to bracketed for the host:port
  # URLs (arm bind, -F address) and bare for bash /dev/tcp.
  local_v6="${NIB_PAIR_V6_ADDR:-::1}"
  PROBE_HOST="${local_v6#[}"; PROBE_HOST="${PROBE_HOST%]}"   # bare, for /dev/tcp
  CEREMONY_HOST="[$PROBE_HOST]"                              # bracketed, for URLs
fi

# ── The port block, derived ONCE ─────────────────────────────────────────────
#
# One array, built here, **both probed and used**. That identity is the whole
# point: a pre-flight whose population is not the run's population proves nothing
# about the run, which is instrument.md's Population field stated as shell.
#
# N API ports, then one session port per hop per transport. Distinct rather than
# reused because consecutive ceremonies run back to back and a listener's teardown
# is not instantaneous — a reused port makes a bind race look like a transport
# failure, which is why the two-party run always had two.
PORT_BASE="${NIB_PAIR_PORT_BASE:-18541}"
API_PORTS=(); for i in $(seq 1 "$N"); do API_PORTS+=( "$((PORT_BASE + i - 1))" ); done
SESSION_BASE="${NIB_PAIR_SESSION_BASE:-$((PORT_BASE + N))}"
SESSION_PORTS=(); for i in $(seq 1 $(( (N - 1) * 2 )) ); do SESSION_PORTS+=( "$((SESSION_BASE + i - 1))" ); done
ALL_PORTS=( "${API_PORTS[@]}" "${SESSION_PORTS[@]}" )

# NIB_PAIR_PORT_A/B kept working because they are this harness's published knobs and
# an operator's muscle memory — NOT because anything in the tree passes them; a grep
# finds no caller outside this file, and no red-proof row uses one. Honoured only at
# N=2, where they mean what they meant.
if [ "$N" = "2" ]; then
  API_PORTS[0]="${NIB_PAIR_PORT_A:-${API_PORTS[0]}}"
  API_PORTS[1]="${NIB_PAIR_PORT_B:-${API_PORTS[1]}}"
  SESSION_PORTS[0]="${NIB_PAIR_SESSION_PORT:-${SESSION_PORTS[0]}}"
  SESSION_PORTS[1]="${NIB_PAIR_SESSION_PORT_QUIC:-${SESSION_PORTS[1]}}"
  ALL_PORTS=( "${API_PORTS[@]}" "${SESSION_PORTS[@]}" )
fi

URLS=(); HOMES=(); PIDS=(); WATCHERS=()
for i in $(seq 1 "$N"); do
  URLS+=( "http://127.0.0.1:${API_PORTS[$((i-1))]}" )
  HOMES+=( "" )   # filled once WORK exists
done

SESSION_PORT="${SESSION_PORTS[0]}"
SESSION_PORT_QUIC="${SESSION_PORTS[1]}"
A="${URLS[0]}"
B="${URLS[1]}"

CEREMONY_N=0
# Arrivals PER INSTANCE, keyed by 1-based index. `CEREMONY_N` counts hops in the run and is
# still printed; this is what the population assertion compares against, because in a relay
# each party receives once and the run's hop count is not any one party's document count.
declare -A ARRIVALS=()
declare -A INVITES=()   # per-instance ceremony invitation, filled by relay()
declare -A ARM_ADDR=()  # per-instance address reported at arm time; a change means a re-arm
RELAY_WORDS=(); RELAY_FINAL=""
WORDS_RELAY_QUIC=(); WORDS_RELAY_TCP=(); FINAL_QUIC=""; FINAL_TCP=""
ELAPSED_TOTAL=0
WORK="$(mktemp -d)"
for i in $(seq 1 "$N"); do HOMES[$((i-1))]="$WORK/i$i"; done

# ── Teardown, and why the trap reads $? ──────────────────────────────────────
#
# `cleanup` takes the exit status as an ARGUMENT rather than reading a flag,
# because `fail` runs inside subshells — the watchers, the ceremony pipeline — and
# a `FAILED=1` set there never reaches the parent's copy. The work dir would then
# be deleted on exactly the runs whose logs are the only diagnosis. At eight hops
# the log is what names the broken instance.
#
# And it prints on the exit STATUS, not unconditionally: v1.117.131 was committed
# on a red suite because a banner printed either way.
cleanup() { # exit-status
  local st="${1:-0}"
  if [ "$KEEP" = "1" ]; then
    echo "--keep: work dir $WORK"
    for i in $(seq 1 "$N"); do echo "  instance $i at ${URLS[$((i-1))]} (pid ${PIDS[$((i-1))]:-?})"; done
    return
  fi
  # Watchers first: they poll the instances, and killing the instances first
  # leaves them spinning against a dead port for the rest of their timeout.
  for pid in "${WATCHERS[@]:-}"; do [ -n "$pid" ] && kill "$pid" >/dev/null 2>&1; done
  for pid in "${PIDS[@]:-}"; do [ -n "$pid" ] && kill "$pid" >/dev/null 2>&1; done
  # **Wait for them to actually be GONE, and do not use `wait` to do it.** The
  # instance pids come out of a command substitution, so they are children of a
  # subshell that has already exited — this shell cannot reap them and `wait`
  # returns instantly with an error, which is the degenerate "waited" that proves
  # nothing. `kill` is asynchronous too, so checking `kill -0` immediately after it
  # reports every instance as a survivor. Poll instead, and only then assert.
  for pid in "${WATCHERS[@]:-}" "${PIDS[@]:-}"; do
    [ -n "$pid" ] || continue
    for _ in $(seq 1 40); do kill -0 "$pid" 2>/dev/null || break; sleep 0.05; done
  done
  local alive=""
  for pid in "${WATCHERS[@]:-}" "${PIDS[@]:-}"; do
    [ -n "$pid" ] && kill -0 "$pid" 2>/dev/null && alive="$alive $pid"
  done
  if [ -n "$alive" ]; then
    for pid in $alive; do kill -9 "$pid" >/dev/null 2>&1; done
    echo "WARN: processes did not exit on TERM and were killed:$alive" >&2
  fi
  if [ "$st" != "0" ]; then
    echo "the run failed — work dir PRESERVED at $WORK" >&2
    for i in $(seq 1 "$N"); do
      [ -f "${HOMES[$((i-1))]}/nib.log" ] && echo "  instance $i log: ${HOMES[$((i-1))]}/nib.log" >&2
    done
    return
  fi
  rm -rf "$WORK"
}
trap 'cleanup $?' EXIT
# **`exit 130` is load-bearing.** A trap handler that RETURNS resumes the successor
# of the interrupted command, so Ctrl-C used to tear the instances down, print
# "work dir PRESERVED", and then carry on against dead instances — and an interrupt
# landing after the last assertion exited 0, whereupon the EXIT trap ran `cleanup 0`
# and deleted the directory the first cleanup had just preserved. Measured.
trap 'cleanup 1; exit 130' INT TERM

fail() { echo "FAIL: $*" >&2; exit 1; }

# ── The pre-flight, by BIND ──────────────────────────────────────────────────
#
# Refuse ports someone else holds, BEFORE building — the lesson tier 3 learned: a
# leftover --keep run makes the failure surface four steps later and blames the
# wrong thing.
#
# **It probes by bind, and the old `curl /api/status` could not do this job.** That
# probe detected only *a nib*, so any other holder passed it; and extending its
# CONNECT shape to the session ports would have been worse than useless, because a
# UDP connect succeeds against a free port — the QUIC half could only ever have
# reported pass. A bind attempt is the one instrument that answers for both
# families, and it answers for a holder that is not a nib.
port_holder() { # port host -> prints what holds it, or nothing
  python3 - "$1" "$2" <<'PYBIND'
import socket, sys
p = int(sys.argv[1]); host = sys.argv[2]; held = []
# **The family comes from the host the run will actually bind.** Probing 127.0.0.1
# while the session socket binds `[::1]` is a probe whose population is not the
# run's — measured: a holder on `[::1]:18543` passed the pre-flight and the run
# died four steps later at "B could not arm a session", which is exactly the
# blame-the-wrong-thing this probe exists to prevent.
af = socket.AF_INET6 if ":" in host else socket.AF_INET
for kind, sk in (("tcp", socket.SOCK_STREAM), ("udp", socket.SOCK_DGRAM)):
    s = socket.socket(af, sk)
    # SO_REUSEADDR is what makes the TCP probe discriminate rather than just refuse.
    # Without it a socket left in TIME_WAIT by the PREVIOUS run reads as "held", so
    # two runs back to back fail the pre-flight for a port nothing is listening on —
    # measured, on the first N=4 run of this slice. With it, TIME_WAIT binds (a
    # server would set the same option) and a live LISTEN still refuses.
    #
    # **TCP only.** UDP has no TIME_WAIT, so the option buys nothing there and costs
    # the discrimination: a UDP holder that sets REUSEADDR itself would go undetected.
    if kind == "tcp":
        s.setsockopt(socket.SOL_SOCKET, socket.SO_REUSEADDR, 1)
    try:
        s.bind((host, p))
    except OSError:
        held.append(kind)
    finally:
        s.close()
print(",".join(held))
PYBIND
}
held=""
for p in "${ALL_PORTS[@]}"; do
  h="$(port_holder "$p" "$PROBE_HOST")"
  [ -n "$h" ] && held="$held $p($h)"
done
if [ -n "$held" ]; then
  echo "FAIL: ports already held:$held" >&2
  echo "      the run wanted this whole block: ${ALL_PORTS[*]}" >&2
  echo "      a leftover --keep run? Stop it, or set NIB_PAIR_PORT_BASE." >&2
  exit 1
fi

if [ -n "${NIB_PAIR_BIN:-}" ]; then
  # Built outside the namespace: inside it there is no network, and the black-hole
  # default route would make any fetch hang rather than fail fast.
  cp "$NIB_PAIR_BIN" "$WORK/nib"
else
  echo "building nib…"
  go build -o "$WORK/nib" ./cmd/nib || fail "could not build nib"
fi

# jq is not assumed; python3 reads the fields.
jget() { python3 -c 'import json,sys;d=json.load(sys.stdin);
ks=sys.argv[1].split(".")
for k in ks:
    d = d[int(k)] if isinstance(d, list) else d.get(k)
    if d is None: break
print("" if d is None else d)' "$1"; }

start() { # name port home
  local name="$1" port="$2" home="$3"
  mkdir -p "$home/home" "$home/config"
  HOME="$home/home" XDG_CONFIG_HOME="$home/config" \
    NIB_NO_BROWSER=1 NIB_NO_UPDATE_CHECK=1 NIB_ADDR="127.0.0.1:$port" \
    "$WORK/nib" >"$home/nib.log" 2>&1 &
  echo $!
}

wait_up() { # url
  for _ in $(seq 1 60); do
    curl -fsS -o /dev/null "$1/api/status" 2>/dev/null && return 0
    sleep 0.25
  done
  return 1
}

# ── Boot, and the two assertions that keep the roster honest ─────────────────
#
# `instances up` and `distinct identities` are asserted SEPARATELY, with their own
# messages, because "an instance never booted" and "two instances share a key" are
# two defects with one symptom. A single `sort -u | wc -l` compared against the
# length of the list that answered is green for both: four of nine booting yields
# four distinct fingerprints and the run calls itself nine.
for i in $(seq 1 "$N"); do
  PIDS+=( "$(start "i$i" "${API_PORTS[$((i-1))]}" "${HOMES[$((i-1))]}")" )
done
UP=0
for i in $(seq 1 "$N"); do
  if wait_up "${URLS[$((i-1))]}"; then
    UP=$((UP + 1))
  else
    echo "instance $i (${URLS[$((i-1))]}) did not come up; its log:" >&2
    cat "${HOMES[$((i-1))]}/nib.log" >&2 2>/dev/null || true
  fi
done
[ "$UP" = "$N" ] || fail "asked for $N instances and $UP answered — the run would proceed with a smaller roster and report the larger one"

# Each instance enrols its OWN key under its OWN home, which is what gives them
# different identities. A shared vault would make every assertion below vacuous —
# the "peers" would be one key agreeing with itself.
for i in $(seq 1 "$N"); do
  url="${URLS[$((i-1))]}"; home="${HOMES[$((i-1))]}"
  curl -fsS -X POST "$url/api/ssh/enroll" -H 'Content-Type: application/json' \
    -d "{\"mode\":\"create\",\"keyPath\":\"$home/home/.ssh/id_ed25519\"}" >/dev/null \
    || fail "could not enrol a key on instance $i ($url)"
done

csrf() { curl -fsS "$1/api/status" | jget csrf; }
CSRFS=(); FPS=()
for i in $(seq 1 "$N"); do
  url="${URLS[$((i-1))]}"
  tok="$(csrf "$url")"
  [ -n "$tok" ] || fail "instance $i ($url) returned no CSRF token"
  CSRFS+=( "$tok" )
  fp="$(curl -fsS "$url/api/peers" | jget fingerprint)"
  [ "${#fp}" = 64 ] || fail "instance $i ($url) has no identity fingerprint (got '${fp}')"
  FPS+=( "$fp" )
  # The six-word name is P01.S02's only tier-4 reader. Per instance rather than
  # for the first alone: it used to be checked on A only, and sat in the block
  # this loop replaced.
  nm="$(curl -fsS "$url/api/peers" | jget name)"
  [ "$(echo "$nm" | wc -w)" = 6 ] || fail "instance $i reports the name '$nm', which is not six words"
done

# THE assertion that makes this an N-instance harness rather than one process
# talking to itself — compared to $N, never to the length of the list that
# answered.
#
# **Its argument changed at N and the old comment would have been read as licence
# to keep it weak.** At two, sharing a vault could not even reach here: the second
# enrolment 409s first, so the check was defence in depth against a state another
# guard already prevented (probed 2026-08-19). At N the realistic failure is the
# harness's OWN indexing — instance 5's fingerprint read from instance 1's URL —
# about which the 409 says nothing. So this is load-bearing now, not a spare.
DISTINCT="$(printf '%s\n' "${FPS[@]}" | sort -u | wc -l)"
[ "$DISTINCT" = "$N" ] || fail "the $N instances reported only $DISTINCT distinct identities — either two are sharing a vault, or the driver read one instance's fingerprint from another's URL, and every assertion below would be one key agreeing with itself"

FP_A="${FPS[0]}"; FP_B="${FPS[1]}"
CSRF_A="${CSRFS[0]}"; CSRF_B="${CSRFS[1]}"

# Pin each other, by hex — the only way to pin (P01: no screen accepts a name).
curl -fsS -X POST "$A/api/peers/pin" -H 'Content-Type: application/json' -H "X-CSRF-Token: $CSRF_A" \
  -d "{\"fingerprint\":\"$FP_B\",\"label\":\"Bea\"}" >/dev/null || fail "A could not pin B"
curl -fsS -X POST "$B/api/peers/pin" -H 'Content-Type: application/json' -H "X-CSRF-Token: $CSRF_B" \
  -d "{\"fingerprint\":\"$FP_A\",\"label\":\"Ada\"}" >/dev/null || fail "B could not pin A"

# A document, made by the product itself rather than by a checked-in blob: nib
# converts Markdown natively, so the fixture is readable in review.
printf '# Lease agreement\n\nBoth parties agree to the terms above.\n' > "$WORK/doc.md"
"$WORK/nib" office "$WORK/doc.md" -o "$WORK/doc.pdf" >/dev/null 2>&1 || fail "could not build the fixture PDF"
[ -s "$WORK/doc.pdf" ] || fail "the fixture PDF is empty"

# A 1x1 PNG for the visible signature block. Bytes rather than a file in the
# repo: it is scaffolding, not content.
python3 - "$WORK/sig.png" <<'PYPNG'
import base64, sys
open(sys.argv[1], "wb").write(base64.b64decode(
    "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="))
PYPNG
[ -s "$WORK/sig.png" ] || fail "could not write the appearance PNG"

# watch_verify polls one instance for the spoken check and confirms it. Both
# parties are blocked waiting for it, so it has to run beside the request rather
# than inside it.
watch_verify() { # url csrf outfile
  local url="$1" tok="$2" out="$3"
  for _ in $(seq 1 120); do
    local w
    w="$(curl -fsS "$url/api/session/status" 2>/dev/null | jget verify.words)"
    if [ -n "$w" ]; then
      printf '%s' "$w" > "$out"
      curl -fsS -X POST "$url/api/session/verify" -H 'Content-Type: application/json' \
        -H "X-CSRF-Token: $tok" -d '{"confirmed":true}' >/dev/null 2>&1
      return 0
    fi
    sleep 0.25
  done
  return 1
}

# The ceremony, run once per transport (D14 keeps TCP beside QUIC). Everything
# from opening A's document to reading the finished one off B is inside the
# function, so both transports are driven through exactly the same steps rather
# than through a second path written to suit whichever one came second.
# **Parameterised over its two ENDS at P07.S05b.** It read the globals `A`/`B`/`CSRF_A`/`FP_B`,
# which are instances 1 and 2 and nothing else (`:304-305`), so a relay could not be expressed
# through it at all — the N>=3 block had to hand-roll each hop with raw `curl`, and its own closing
# lines named that as the gap: *"driving a relay to completion needs hop 1 to be a ceremony hop"*.
#
# The two-party callers pass `1 2` explicitly rather than relying on a default. A default would
# make a future caller that forgets the indices run silently between instances 1 and 2 — which is
# exactly the class of green this harness exists to refuse.
#
# `local` is dynamically scoped in bash, so the subshells and helpers below see these bindings
# rather than the globals. That is what lets the body stay unchanged.
ceremony() { # transport port outfile from to [indoc] [want_sigs] [want_proceeding] [invitation]
  local transport="$1" port="$2" out="$3"
  local fi="$4" ti="$5"
  local indoc="${6:-$WORK/doc.pdf}" want="${7:-2}" wantproc="${8:-0}" inv="${9:-}" armaddr="${10:-}"
  [ -n "$fi" ] && [ -n "$ti" ] \
    || fail "ceremony() was called without both endpoints — it is parameterised over (from, to) since P07.S05b"
  local A="${URLS[$((fi-1))]}" B="${URLS[$((ti-1))]}"
  local CSRF_A="${CSRFS[$((fi-1))]}" CSRF_B="${CSRFS[$((ti-1))]}"
  local FP_A="${FPS[$((fi-1))]}" FP_B="${FPS[$((ti-1))]}"
  local t0; t0="$(date +%s)"

  # The document this hop builds on. A pristine one for a two-party run — a ceremony over the
  # previous ceremony's output would carry three signatures and the count below would assert the
  # wrong thing — and the PREVIOUS HOP's output for a relay, which is the whole of what a baton is.
  #
  # **A relay hop does NOT open it, and that is the product's shape rather than a shortcut
  # (P07.S05b).** `/api/session/initiate` takes the document as a form upload, so the open is only
  # the two-party flow's "the initiator has this file on screen". On a relay the convener already
  # holds the ceremony's document and `installCeremonyResult` REPLACES it by ceremony id at every
  # hop (ADR-005/008's cap is why that door exists) — so opening each hop's output as a NEW
  # document is the harness inventing accumulation the product does not have.
  #
  # It is not cosmetic. Measured: at N=4 the convener hit `maxOpenDocs` (8) partway through the
  # second transport and `/api/open` returned 409. At N=9 it would refuse at hop 7 of the first.
  if [ -z "$inv" ]; then
    curl -fsS -X POST "$A/api/open" -H 'Content-Type: application/json' -H "X-CSRF-Token: $CSRF_A" \
      -d "{\"path\":\"$indoc\"}" >/dev/null || fail "[$transport] instance $fi could not open $indoc"
  fi

  # B arms a receive session for co-signing, on this transport.
  #
  # In LAN mode the bind is OMITTED ENTIRELY — nothing types an address anywhere,
  # which is P03's first exit criterion stated as a shell command. B binds
  # ephemerally and announces the port it got; A learns it from the link.
  #
  # **The arm carries the INVITATION for a ceremony hop, and leaving it out is not a small
  # omission (P07.S05b).** `handleSessionArm` resolves the ceremony from `req.Invitation`
  # (`session.go:1015`) and stores it on the session; the receive path reads its roster from
  # there (`:800`). Without it the receiver has an EMPTY roster, so `coSignExchange` takes its
  # non-ceremony branch and refuses hop 1 with *"a co-signature takes exactly one prior signer"* —
  # correctly, because outside a ceremony an unsigned document is not something to co-sign.
  # Measured here: that is exactly the 409 the first relay run got back.
  # **A PRE-ARMED party is not re-armed here (P07.S03b's T03).** A real ceremony's parties arm once
  # and wait; arming just before each hop is the harness making its own life easy, and it hides
  # exactly the failure the clause is about — an arm that expired while the baton was elsewhere.
  # When the caller has already armed this party it passes the address the arm reported, and this
  # function does not touch the session.
  local armbody inv_field=""
  [ -z "$inv" ] || inv_field=",\"invitation\":\"$inv\""
  if [ -n "$armaddr" ]; then
    armbody=""
  elif [ "$port" = "lan" ]; then
    armbody="{\"fingerprint\":\"$FP_A\",\"mode\":\"cosign\",\"transport\":\"$transport\"$inv_field}"
  else
    armbody="{\"fingerprint\":\"$FP_A\",\"bind\":\"$CEREMONY_HOST:$port\",\"mode\":\"cosign\",\"transport\":\"$transport\"$inv_field}"
  fi
  # **The BODY, not just the status.** `curl -f` prints "The requested URL returned error: 400"
  # and swallows the sentence the server wrote — which at N=9 left an arm failure with no
  # diagnosis at all. Every refusal on this route is a sentence about what was wrong with the
  # request, and a harness that discards it is asking a later reader to guess.
  if [ -n "$armbody" ]; then
    local armcode
    armcode="$(curl -sS -X POST "$B/api/session/arm" -H 'Content-Type: application/json' \
      -H "X-CSRF-Token: $CSRF_B" -d "$armbody" -o "$WORK/arm.$transport.json" -w '%{http_code}')"
    [ "$armcode" = "200" ] \
      || fail "[$transport] instance $ti could not arm a session (HTTP $armcode): $(head -c 300 "$WORK/arm.$transport.json")"
  fi

  # In v6 mode, the STIMULUS assertion: B actually bound a v6 socket. The ceremony
  # completing already proves the dial crossed `[::1]` (initiate is handed that address
  # and a 200 means it connected), but that is one direction. This catches the case a
  # completion alone would not: a future v4 fallback in the bind path would let the run
  # pass over v4 while claiming v6. `ln.Addr().String()` reads back the bound socket, so
  # a bracketed literal is a v6 bind and `127.0.0.1:` is not. Skipped when
  # NIB_PAIR_V6_ADDR points at a real machine, whose reported address is its own.
  if [ "$V6" = "1" ] && [ "$port" != "lan" ] && [ -z "${NIB_PAIR_V6_ADDR:-}" ]; then
    local bound
    bound="$(curl -fsS "$B/api/session/status" | sed -n 's/.*"address":"\([^"]*\)".*/\1/p')"
    case "$bound" in
      \[*\]:*) : ;;  # bracketed v6 literal, e.g. [::1]:18543
      *) fail "[$transport] --v6 armed but B reports it bound '$bound', which is not a v6 literal — the ceremony would complete over v4 and claim a v6 pass it did not earn" ;;
    esac
  fi

  # THE assertion that makes this two transports rather than one run twice.
  #
  # Everything else in this function is transport-blind: the same API calls, the
  # same words, the same signature count. With the transport field ignored, both
  # runs would use TCP and every check below would still pass — the harness would
  # report QUIC coverage it did not have. So the socket itself is observed, which
  # the app cannot self-report its way past.
  #
  # A stray TCP connect is also exactly what the accept loop is built to shrug
  # off, so the tcp branch doubles as a check that a wrong dial does not consume
  # the armed session — the ceremony below still completes after it.
  if [ "$port" = "lan" ]; then
    # No port was TYPED — but one was BOUND, and B reports it in its status. Probing
    # it is what stops the LAN runs being two TCP runs wearing different labels, the
    # same hazard the fixed-port branch below exists for. It matters more here: the
    # LAN QUIC run is the only place in the tree where a peer's transport is learned
    # from an announcement rather than configured into both sides.
    local lanaddr lanport
    lanaddr="$(curl -fsS "$B/api/session/status" | sed -n 's/.*"address":"\([^"]*\)".*/\1/p')"
    [ -n "$lanaddr" ] || fail "[$transport] B armed on the link and reports no address, so the transport cannot be probed"
    lanport="${lanaddr##*:}"
    if (exec 3<>"/dev/tcp/127.0.0.1/$lanport") 2>/dev/null; then
      exec 3<&- 3>&-
      [ "$transport" = "tcp" ] || fail "[$transport] the announced port $lanport answers TCP — the LAN QUIC run is listening on a TCP socket, so it is the TCP path wearing a different label"
    else
      [ "$transport" != "tcp" ] || fail "[$transport] the announced port $lanport does not answer TCP, so the LAN TCP run is not on the transport it asked for"
    fi
  else
    # A PRE-ARMED party bound an ephemeral port, so the port to probe is the one it reported
    # rather than one this run chose. The probe matters more there, not less: a relay hop is the
    # only place a party's transport was fixed at arm time and is being trusted several hops
    # later.
    local probeport="$port"
    [ -z "$armaddr" ] || probeport="${armaddr##*:}"
    [ -n "$probeport" ] || fail "[$transport] no port to probe — neither a typed port nor a pre-armed address reached here"
    if (exec 3<>"/dev/tcp/$PROBE_HOST/$probeport") 2>/dev/null; then
      exec 3<&- 3>&-
      [ "$transport" = "tcp" ] || fail "[$transport] port $probeport answers TCP — the QUIC run is listening on a TCP socket, so it is the TCP path wearing a different label"
    else
      [ "$transport" != "tcp" ] || fail "[$transport] port $probeport does not answer TCP, so the TCP run is not on the transport it asked for"
    fi
  fi

  # Both sides must confirm the spoken check (P01.S05) before any document byte
  # moves, and BOTH are blocked while they wait — A inside its own /initiate
  # request, B inside its session goroutine. So the confirmations arrive on
  # separate requests, from watchers started here. This is not a harness quirk: it
  # is the shape the UI has to have, and tier 3 asserts the same thing one process
  # at a time.
  rm -f "$WORK/words_a" "$WORK/words_b"
  watch_verify "$A" "$CSRF_A" "$WORK/words_a" &
  local watch_a=$!
  watch_verify "$B" "$CSRF_B" "$WORK/words_b" &
  local watch_b=$!

  # B's consent, on its own watcher: /initiate blocks until B has accepted.
  (
    for _ in $(seq 1 240); do
      if [ -n "$(curl -fsS "$B/api/session/status" 2>/dev/null | jget pending.fingerprint)" ]; then
        curl -fsS -X POST "$B/api/session/respond" -H 'Content-Type: application/json' \
          -H "X-CSRF-Token: $CSRF_B" -d '{"accept":true,"intent":"I accept"}' >/dev/null 2>&1
        exit 0
      fi
      sleep 0.25
    done
  ) &
  local watch_consent=$!

  echo "initiating the ceremony over $transport…"
  local init_out="$WORK/initiate.$transport.json"
  curl -sS -X POST "$A/api/session/initiate" -H "X-CSRF-Token: $CSRF_A" \
    -F "pdf=@$indoc" -F "appearance=@$WORK/sig.png" \
    -F "params={\"fingerprint\":\"$FP_B\",\"intent\":\"I agree to co-sign\"}" \
    $( [ -z "$inv" ] || printf %s "-F invitation=$inv" ) \
    $( [ "$port" = "lan" ] || [ -z "$armaddr" ] || printf %s "-F address=$armaddr" ) \
    $( [ "$port" = "lan" ] || [ -n "$armaddr" ] || printf %s "-F address=$CEREMONY_HOST:$port" ) \
    $( [ "$port" = "lan" ] || printf %s "-F transport=$transport" ) \
    -o "$init_out" -w '%{http_code}' > "$WORK/initiate.code" 2>"$WORK/initiate.err"
  local code
  code="$(cat "$WORK/initiate.code")"

  wait "$watch_a" 2>/dev/null; wait "$watch_b" 2>/dev/null; wait "$watch_consent" 2>/dev/null

  local words_a words_b
  words_a="$(cat "$WORK/words_a" 2>/dev/null || true)"
  words_b="$(cat "$WORK/words_b" 2>/dev/null || true)"

  # The stimulus, asserted before anything is graded: the spoken check really
  # happened on BOTH sides. Without this, a ceremony that completed because the
  # gate never fired would pass every assertion below.
  # If the initiate never got off the ground, say THAT — the words check fires first by
  # design (stimulus before grading), and without this it reports a missing spoken check
  # for a ceremony that never reached one. Probed: with the armed side's announcer
  # disabled, this used to blame L2 for what was actually "peer not found on the link".
  if [ -z "$words_a" ] && [ "$code" != "200" ]; then
    fail "[$transport] initiate returned HTTP $code before any spoken check: $(head -c 300 "$init_out" 2>/dev/null)"
  fi
  [ -n "$words_a" ] || fail "[$transport] instance A was never shown the verification words — the ceremony reached the document exchange without the spoken check (L2)"
  [ -n "$words_b" ] || fail "[$transport] instance B was never shown the verification words"
  [ "$(echo "$words_a" | wc -w)" = 4 ] || fail "[$transport] A's verification string is not four words: $words_a"
  # The property only two instances can see.
  [ "$words_a" = "$words_b" ] || fail "[$transport] the two instances derived DIFFERENT verification words — A saw '$words_a' and B saw '$words_b'. Two people comparing these would read a mismatch as an attack."

  [ "$code" = "200" ] || { cat "$init_out" >&2; cat "${HOMES[1]}/nib.log" >&2; link_report "$fi" "$ti" "$transport"; fail "[$transport] initiate returned HTTP $code"; }

  # The ceremony completed only if the document carries BOTH signatures — read
  # from B's side, because asking A whether A signed is asking the thing under test.
  # By document ID, never the active-document fallback. `/api/pdf` with no
  # `X-Nib-Doc` and no `?doc=` returns whatever that instance happens to have
  # active, so "which document did this hop produce" has no answer once an
  # instance holds more than one — which is every N>2 relay. Addressing it by id
  # makes the two-party run stricter today and gives the question an answer later.
  # **Counted PER RECEIVING INSTANCE since P07.S05b, and the global it replaced was right only
  # for a two-party run.** `CEREMONY_N` counted every hop in the run and compared it to how many
  # documents *instance 2* held — true when instance 2 is the receiver every time, and false the
  # moment a relay walks the roster, where each party receives once and the count would drift by
  # the number of hops that went elsewhere.
  ARRIVALS[$ti]=$(( ${ARRIVALS[$ti]:-0} + 1 ))
  CEREMONY_N=$((CEREMONY_N + 1))
  local docid ndocs
  docid="$(curl -fsS "$B/api/docs" | jget activeId)"
  ndocs="$(curl -fsS "$B/api/docs" | python3 -c 'import json,sys;print(len(json.load(sys.stdin).get("docs",[])))')"
  [ -n "$docid" ] || fail "[$transport] instance $ti reports no active document to fetch"
  # The population, asserted rather than assumed: after its k-th arrival, the receiving instance
  # holds exactly k. Without this, "the active document" is a guess that happens to be right at
  # N=2 and stops being right the moment an instance holds a document it did not just receive.
  [ "$ndocs" = "${ARRIVALS[$ti]}" ] \
    || fail "[$transport] after arrival ${ARRIVALS[$ti]} at instance $ti, it holds $ndocs document(s) — the active one may not be this hop's arrival"
  curl -fsS "$B/api/pdf?doc=$docid" -o "$out" || fail "[$transport] could not fetch instance $ti's document $docid"
  # **The expected count is a PARAMETER since P07.S05b, and it is an equality now.** It was
  # `n < 2`, which is right for a two-party run and useless for a relay: at hop k the document
  # must carry exactly k+1 signatures, and `>=` would pass a hop that somehow signed twice —
  # which is precisely the defect L3 exists to refuse and which this harness has watched the
  # product commit (the carrier re-signing, P07.S03b).
  python3 - "$out" "$transport" "$want" <<'PYSIG' || exit 1
import sys
b = open(sys.argv[1], "rb").read()
n = b.count(b"/ByteRange")
want = int(sys.argv[3])
if n != want:
    print(f"FAIL: [{sys.argv[2]}] the co-signed document carries {n} signature byte-ranges, "
          f"want exactly {want} — fewer means a party did not sign, more means one signed twice",
          file=sys.stderr)
    sys.exit(1)
print(f"[{sys.argv[2]}] the finished document carries {n} signatures")
PYSIG
  # ── The attestations route, on a really-signed document (P07.S04) ────────────
  #
  # It reads `document.sig` now instead of re-verifying the whole file, so what has to be
  # checked is that it still ANSWERS — with the same attestations, cross-bound, from the cached
  # status. Read from B's side, on B's arrival, by document id: asking A about A is asking the
  # thing under test, and the active-document fallback has no answer once an instance holds more
  # than one.
  #
  # `oneProceeding` is NOT asserted here and that is honest rather than an omission: no
  # production attestation carries a commitment yet (C01 is blocked on P07.S05's carry route), so
  # a document from this harness has no proceeding to be one of. What IS asserted is that the
  # route did not start claiming one anyway.
  curl -fsS "$B/api/attestations" -H "X-Nib-Doc: $docid" -o "$WORK/atts.$transport.json" \
    || fail "[$transport] the attestations route did not answer for B's document"
  # **`oneProceeding` is asserted in BOTH directions since P07.S05b, and which one is a
  # parameter.** This block used to require it FALSE unconditionally, correctly: no production
  # attestation carried a commitment, so a document from this harness had no proceeding to be one
  # of. P07.S04 populates the token and P07.S05 relays it, so a real ceremony hop must now claim
  # one — and a check that only ever requires FALSE would report a green for the exact regression
  # that matters, a relay whose signatures stopped naming their ceremony.
  #
  # **`matched` is not required on the FIRST attestation of a relay, and working out which end was
  # exempt took reading `PredecessorOf` rather than reasoning about it.** `AcceptedPeer` names the
  # previous SIGNING roster entry (`l3.go:194`), and for the first signer that is `""` — they
  # accept nobody, because nobody signed before them. So attestation 0 of a relay names no peer
  # and can never cross-bind, while every later one must. Outside a ceremony the binding is
  # pairwise and mutual, so all of them must — which is why this is keyed on `wantproc`.
  python3 - "$WORK/atts.$transport.json" "$transport" "$want" "$wantproc" <<'PYATT' || exit 1
import json, sys
d = json.load(open(sys.argv[1]))
ats = d.get("attestations") or []
t, want, wantproc = sys.argv[2], int(sys.argv[3]), sys.argv[4] == "1"
if len(ats) != want:
    print(f"FAIL: [{t}] the attestations route reports {len(ats)} attestation(s) on a document "
          f"carrying {want} signature(s) — it reads the cached status now, and the cached status "
          f"is wrong or the route is reading the wrong document", file=sys.stderr)
    sys.exit(1)
for i, a in enumerate(ats):
    if not a.get("valid"):
        print(f"FAIL: [{t}] attestation {i} is not valid", file=sys.stderr); sys.exit(1)
    if not a.get("matched") and not (wantproc and i == 0):
        print(f"FAIL: [{t}] attestation {i} is not cross-bound. In a relay only the FIRST signer "
              f"is exempt — they accept nobody because nobody signed before them; every later "
              f"party accepts their predecessor, and crossBind runs on the same data whether the "
              f"status was cached or recomputed", file=sys.stderr); sys.exit(1)
    if wantproc and i == 0 and a.get("matched"):
        print(f"FAIL: [{t}] the FIRST signer's attestation cross-binds to somebody. They accept "
              f"nobody — PredecessorOf returns \"\" for the first signing roster entry — so a "
              f"match here means the attestation names a peer it should not have", file=sys.stderr)
        sys.exit(1)
    if bool(a.get("oneProceeding")) != wantproc:
        if wantproc:
            print(f"FAIL: [{t}] attestation {i} does NOT claim one proceeding, on a hop of a real "
                  f"ceremony — the signature has stopped carrying the roster commitment, so a "
                  f"verifier can no longer tell this document was produced by an agreed "
                  f"proceeding", file=sys.stderr)
        else:
            print(f"FAIL: [{t}] attestation {i} claims to be part of one proceeding, on a document "
                  f"whose signatures carry no ceremony commitment at all. That verdict means "
                  f"agreement with the document's RECORD, and there is none.", file=sys.stderr)
        sys.exit(1)
print(f"[{t}] the attestations route answers from the cached status: {len(ats)} valid, "
      f"cross-bound, proceeding claimed={wantproc}")
PYATT

  # H25 — elapsed, PRINTED and never thresholded.
  #
  # A four-second run and a four-minute run both pass, and the second is one hop from
  # `sessionAcceptTimeout` (5 min), which every arm here is bounded by because no
  # invitation exists before P07.S02b. A threshold would be wrong for Z6's reason —
  # a loaded machine is not a defect — but a number nobody prints is a margin nobody
  # can see shrinking as N grows.
  local t1; t1="$(date +%s)"
  echo "[$transport] hop took $((t1 - t0))s (arm ceiling is $((5 * 60))s while arms are manual)"
  ELAPSED_TOTAL=$((ELAPSED_TOTAL + t1 - t0))
  WORDS="$words_a"
}

# ── N >= 3: the expected red, and why it is here ─────────────────────────────
#
# **This slice deliberately does not build the N-party relay, and this assertion is
# what stops that from becoming a silent skip.** `coSignExchange` refuses anything
# but one prior signer (`internal/p2p/session.go:407`), so hop 2 of any baton fails
# today; P07.S03 is the slice whose acceptance says it removes that refusal. Until
# then the honest options were a skip, an `|| true`, or a run behind an env var
# nobody sets — the shape `docs/red-proofs.md` already records as "a harness reports
# a pass for the tests it happens to name".
#
# So instead: drive ONE hop past the two-party case and assert the product refuses
# it, by name. The day S03 lands, this goes red and whoever is holding S03 has to
# switch the N-party path on. That is the only mechanism here that makes the
# switch-on happen rather than be remembered.
# egress_preamble proves the off-link counter can FIRE before anything asserts it is zero.
#
# **Extracted at P07.S05c so the N-party LAN run can use it too.** It was inline in the two-party
# LAN block, which is also where the `N != 2` block's `exit 0` put it out of reach — the two modes
# were mutually exclusive by ordering rather than by design, so `--lan -n 9` ran the fixed-port
# relay and never touched the link at all.
#
# The stimulus is not decoration: the first version of this instrument used a namespace with no
# default route and read zero after a real connect attempt, because the kernel refused at routing
# before the output hook. Provoked in BOTH families separately, because one rule per family means
# one blind spot per family — a single IPv4 probe once passed while the IPv6 rule did not exist,
# and Nib announces on an IPv6 group.
egress_preamble() {
  local before mid provoked
  before="$(offlink_packets)"
  timeout 2 bash -c 'exec 3<>/dev/tcp/1.1.1.1/80' 2>/dev/null || true
  mid="$(offlink_packets)"
  [ "$mid" -gt "$before" ] \
    || fail "the off-link counter did not move for an IPv4 connect to 1.1.1.1 ($before -> $mid) — it cannot see outbound IPv4, so asserting zero below would prove nothing"
  timeout 2 bash -c 'exec 3<>/dev/tcp/2606:4700:4700::1111/80' 2>/dev/null || true
  provoked="$(offlink_packets)"
  [ "$provoked" -gt "$mid" ] \
    || fail "the off-link counter did not move for an IPv6 connect ($mid -> $provoked) — it is IPv6-BLIND, which is what an \`ip daddr\` rule in an inet table is by itself, and Nib announces on an IPv6 group"
  echo "egress counter proven live in both families: $before -> $mid (v4) -> $provoked (v6)"
  nft reset counters table inet egress >/dev/null 2>&1 || true
  baseline="$(offlink_packets)"
}

if [ "$N" != "2" ]; then
  echo "N=$N: boot and identity verified; probing the model's current ceiling…"
  # Party 1 -> 2, the ordinary two-party ceremony, to produce a 2-signature document.
  for i in 1 2; do
    j=$(( i == 1 ? 2 : 1 ))
    curl -fsS -X POST "${URLS[$((i-1))]}/api/peers/pin" -H 'Content-Type: application/json' \
      -H "X-CSRF-Token: ${CSRFS[$((i-1))]}" \
      -d "{\"fingerprint\":\"${FPS[$((j-1))]}\",\"label\":\"p$j\"}" >/dev/null \
      || fail "instance $i could not pin instance $j"
  done
  ceremony tcp "${SESSION_PORTS[0]}" "$WORK/hop1.pdf" 1 2
  echo "hop 1 completed; the document carries two signatures"

  # ── Hop 2, as a REAL ceremony, and it now fails at the NEAR end ──────────────
  #
  # **Rewritten 2026-08-25 (P07.S03b T02), and the old shape was wrong in two ways.**
  #
  # It drove hop 2 as party 2 -> party 3. **Under D22 that is not a hop at all**: `hopBetween`
  # refuses any pair without the convener at one end — "under a convener hub every hop has the
  # convener at one end" — so a ceremony's second hop is convener -> party 3, and the old
  # topology was a chain the model has never had.
  #
  # And it hand-pinned, which is the residue D29 forbids and which the blind-spot list at the top
  # of this file said to stop doing when P07.S02b landed. It has. The parties are pinned by
  # ACCEPTING an invitation, which is D21's whole point, and the run asserts that no manual pin
  # was needed.
  #
  # The refusal is now raised BEFORE any network work: `/api/session/initiate` applies the local
  # signature in `buildCoSigned`, the L3 gate runs there, and the carrier signing a second time is
  # refused at its own machine. So hop 2 needs no watchers, no arm on the far side, and no
  # unreachable port — which is why all of that is gone.
  echo "convening a real $N-party ceremony…"
  roster='{"fingerprint":"'"${FPS[0]}"'","label":"p1","signs":true}'
  for i in $(seq 2 "$N"); do
    roster="$roster,"'{"fingerprint":"'"${FPS[$((i-1))]}"'","label":"p'"$i"'","signs":true}'
  done
  expires="$(python3 -c "import datetime;print((datetime.datetime.now(datetime.timezone.utc)+datetime.timedelta(hours=48)).strftime('%Y-%m-%dT%H:%M:%SZ'))")"
  curl -fsS -X POST "${URLS[0]}/api/open" -H 'Content-Type: application/json' \
    -H "X-CSRF-Token: ${CSRFS[0]}" -d "{\"path\":\"$WORK/doc.pdf\"}" >/dev/null \
    || fail "instance 1 could not open the document to convene over"
  curl -sS -X POST "${URLS[0]}/api/ceremony/convene" -H 'Content-Type: application/json' \
    -H "X-CSRF-Token: ${CSRFS[0]}" \
    -d "{\"roster\":[$roster],\"intent\":\"We agree\",\"expires\":\"$expires\",\"convenerSigns\":true}" \
    -o "$WORK/convene.json" -w '%{http_code}' > "$WORK/convene.code" 2>/dev/null
  [ "$(cat "$WORK/convene.code")" = "200" ] \
    || fail "convene failed (HTTP $(cat "$WORK/convene.code")): $(head -c 300 "$WORK/convene.json")"
  inv3="$(python3 -c "
import json
d=json.load(open('$WORK/convene.json'))
print(next(i['invitation'] for i in d['invites'] if i['fingerprint'].lower()=='${FPS[2]}'.lower()))" 2>/dev/null)"
  [ -n "$inv3" ] || fail "convene issued no invitation for instance 3"

  # Instance 3 accepts. **No /api/peers/pin anywhere on this path** — that is the assertion.
  acode="$(curl -sS -X POST "${URLS[2]}/api/ceremony/accept" -H 'Content-Type: application/json' \
    -H "X-CSRF-Token: ${CSRFS[2]}" -d "$(python3 -c "import json;print(json.dumps({'invitation':'$inv3'}))")" \
    -o "$WORK/accept.json" -w '%{http_code}')"
  [ "$acode" = "200" ] \
    || fail "instance 3 could not accept its invitation (HTTP $acode): $(head -c 300 "$WORK/accept.json")"
  [ "$(python3 -c "import json;print(json.load(open('$WORK/accept.json'))['pinned'])" 2>/dev/null)" = "1" ] \
    || fail "accepting the invitation established no pin, so D21's step was not removed"

  # Hop 2: the CONVENER offers instance 3 the document hop 1 produced.
  hop2code="$(curl -sS -X POST "${URLS[0]}/api/session/initiate" -H "X-CSRF-Token: ${CSRFS[0]}" \
    -F "pdf=@$WORK/hop1.pdf" -F "appearance=@$WORK/sig.png" \
    -F "params={\"fingerprint\":\"${FPS[2]}\",\"intent\":\"hop 2\"}" \
    -F "address=127.0.0.1:${SESSION_PORTS[2]}" -F "transport=tcp" -F "invitation=$inv3" \
    -o "$WORK/hop2.json" -w '%{http_code}')"
  if [ "$hop2code" = "200" ]; then
    fail "hop 2 SUCCEEDED (HTTP 200) — the carrier signed a second time and the far side took it.
      If that is now correct, P07.S05's carry route has landed and this whole block should go,
      along with TestTheRelayCeilingAtFourParties in internal/p2p."
  fi
  # ── What this probe MEASURES now ─────────────────────────────────────────────
  #
  # Not "the model refuses hop 2" — it does not. Measured at P07.S03b:
  #
  #   hop 1  the convener contributes, party 2 co-signs: exactly the roster prefix
  #   hop 2  /api/session/initiate applies the LOCAL signature first (buildCoSigned), so the
  #          carrier signs AGAIN, and L3 refuses it BY NAME at the carrier's own machine
  #   and    party 3, handed that document unchanged, IS admitted
  #
  # **The carry verb landed at P07.S05 and this probe is still right — for a narrower reason, and
  # the narrowing is recorded because the sentence here was stale once already.** `p2p.Carry`
  # exists and `/api/session/initiate` takes it for a `signs:false` roster member. The convener in
  # THIS run signs (the roster below is built with every party signing), so what is refused is a
  # SIGNING party taking a second turn — which is correct and is L3's whole point.
  #
  # What this run still cannot show is a relay COMPLETING, and the reason is the harness rather
  # than the product: hop 1 here is the manual two-party exchange, over a document unrelated to
  # the ceremony convened afterwards. Driving a real relay needs hop 1 to be a ceremony hop, which
  # is P07.S05a's N=4/N=9 driver.
  if grep -q "not this party's turn" "$WORK/hop2.json" 2>/dev/null; then
    echo "      L3 refused the CARRIER re-signing, by name, at its own machine (HTTP $hop2code)"
  else
    echo "--- initiate body ---" >&2; head -c 400 "$WORK/hop2.json" >&2 2>/dev/null || true
    fail "hop 2 failed with HTTP $hop2code, but not as L3's not-your-turn refusal — something
      else is broken, and without this clause it would have been credited as the relay ceiling"
  fi
  echo "  the model's ceiling holds: a SIGNING party cannot take a second turn."

  # ── The relay, walked end to end (P07.S05b) ─────────────────────────────────
  #
  # **This is the clause "the relay is expressed once, in the baton topology".** Everything above
  # this line is the ceiling probe that stood in for it while there was no carry verb; it is kept
  # because "a signing party cannot take a second turn" is still a property and it is L3's whole
  # point. What follows is the relay itself, and it is a LOOP over hops rather than a hop written
  # out N times — which is what "expressed once" has to mean in a harness whose previous shape
  # hand-rolled a single hop with raw curl.
  #
  # The convener does NOT sign. That is D22's motivating case and the one that could not work at
  # all before P07.S05: every hop is a `Carry`, chosen off the roster by the route rather than by
  # a flag here (`session.go:1408`), so this harness cannot accidentally drive the wrong verb.
  # With N parties on the roster the convener is one of them and N-1 sign, so there are N-1 hops
  # and the finished document carries N-1 signatures.
  relay() { # transport [lan]
    local transport="$1" mode="${2:-}"
    local n_hops=$(( N - 1 ))
    echo "relay over $transport: convening $N parties, non-signing convener, $n_hops hops…"

    # A ceremony of its own, so the run's two transports cannot share a document or a record.
    local roster='{"fingerprint":"'"${FPS[0]}"'","label":"p1","signs":false}'
    local i
    for i in $(seq 2 "$N"); do
      roster="$roster,"'{"fingerprint":"'"${FPS[$((i-1))]}"'","label":"p'"$i"'","signs":true}'
    done
    local expires
    expires="$(python3 -c "import datetime;print((datetime.datetime.now(datetime.timezone.utc)+datetime.timedelta(hours=48)).strftime('%Y-%m-%dT%H:%M:%SZ'))")"
    curl -fsS -X POST "${URLS[0]}/api/open" -H 'Content-Type: application/json' \
      -H "X-CSRF-Token: ${CSRFS[0]}" -d "{\"path\":\"$WORK/doc.pdf\"}" >/dev/null \
      || fail "[$transport] the convener could not open the document to convene over"
    local code
    code="$(curl -sS -X POST "${URLS[0]}/api/ceremony/convene" -H 'Content-Type: application/json' \
      -H "X-CSRF-Token: ${CSRFS[0]}" \
      -d "{\"roster\":[$roster],\"intent\":\"We agree\",\"expires\":\"$expires\",\"convenerSigns\":false}" \
      -o "$WORK/relay.convene.$transport.json" -w '%{http_code}')"
    [ "$code" = "200" ] \
      || fail "[$transport] convening $N parties failed (HTTP $code): $(head -c 300 "$WORK/relay.convene.$transport.json")"

    # The convened document is the baton's first state: it carries the record, and NO signatures.
    local docid
    docid="$(python3 -c "
import json;print(json.load(open('$WORK/relay.convene.$transport.json')).get('document',{}).get('id',''))" 2>/dev/null)"
    [ -n "$docid" ] || docid="$(curl -fsS "${URLS[0]}/api/docs" | jget activeId)"
    curl -fsS "${URLS[0]}/api/pdf?doc=$docid" -o "$WORK/relay.$transport.hop0.pdf" \
      || fail "[$transport] could not fetch the convened document"
    # STIMULUS: the baton really starts unsigned. Every count below is relative to this, so a
    # convened document that arrived already signed would shift all of them by one and the run
    # would still pass — asserting N-1 signatures on a document that started with one.
    local start_sigs
    start_sigs="$(python3 -c "print(open('$WORK/relay.$transport.hop0.pdf','rb').read().count(b'/ByteRange'))")"
    [ "$start_sigs" = "0" ] \
      || fail "[$transport] the convened document already carries $start_sigs signature(s) — every count in this relay is relative to zero"

    # Every party accepts its invitation. NO /api/peers/pin anywhere on this path (D21).
    for i in $(seq 2 "$N"); do
      local inv
      inv="$(python3 -c "
import json
d=json.load(open('$WORK/relay.convene.$transport.json'))
print(next(x['invitation'] for x in d['invites'] if x['fingerprint'].lower()=='${FPS[$((i-1))]}'.lower()))" 2>/dev/null)"
      [ -n "$inv" ] || fail "[$transport] convene issued no invitation for instance $i"
      INVITES[$i]="$inv"
      code="$(curl -sS -X POST "${URLS[$((i-1))]}/api/ceremony/accept" -H 'Content-Type: application/json' \
        -H "X-CSRF-Token: ${CSRFS[$((i-1))]}" \
        -d "$(python3 -c "import json;print(json.dumps({'invitation':'$inv'}))")" \
        -o "$WORK/relay.accept.$transport.$i.json" -w '%{http_code}')"
      [ "$code" = "200" ] \
        || fail "[$transport] instance $i could not accept its invitation (HTTP $code): $(head -c 300 "$WORK/relay.accept.$transport.$i.json")"
      # D22 is a HUB: accepting pins the CONVENER and nobody else, whatever N is.
      [ "$(python3 -c "import json;print(json.load(open('$WORK/relay.accept.$transport.$i.json'))['pinned'])" 2>/dev/null)" = "1" ] \
        || fail "[$transport] instance $i's accept established more or fewer than one pin — D22 is a hub, so a party pins the convener and nobody else"
    done

    # ── Every party arms ONCE, before hop 1, and is never re-armed (P07.S03b's T03) ───────────
    #
    # This is what a real ceremony looks like: the parties arm and wait, and the convener walks
    # the roster. Arming just before each hop is the harness making its own life easy, and it
    # cannot see the failure the clause is about — an arm that expired while the baton was
    # somewhere else. At N=9 the last party waits through seven hops before its turn.
    #
    # **The binds are EPHEMERAL, and that is what gives the no-re-arm check teeth.** With a fixed
    # port the address is the same before and after a re-arm, so comparing it proves nothing; with
    # `:0` the kernel picks, and a re-arm lands somewhere else. The clause says so in as many
    # words — *"the reported address byte-identical to what it was at arm time, because a re-arm
    # changes the ephemeral port"* — and a fixed-port version of this check would have been the
    # vacuous green it warns about.
    local i
    for i in $(seq 2 "$N"); do
      local body code
      # **In LAN mode the bind is OMITTED ENTIRELY**, which is P03's first exit criterion stated
      # as a shell command: nothing types an address anywhere, the party binds ephemerally and
      # announces the port it got, and the convener learns it from the link. Everywhere else the
      # bind is `:0` — also ephemeral, so the no-re-arm check below has teeth either way.
      if [ "$mode" = "lan" ]; then
        body="{\"fingerprint\":\"${FPS[0]}\",\"mode\":\"cosign\",\"transport\":\"$transport\",\"invitation\":\"${INVITES[$i]}\"}"
      else
        body="{\"fingerprint\":\"${FPS[0]}\",\"bind\":\"$CEREMONY_HOST:0\",\"mode\":\"cosign\",\"transport\":\"$transport\",\"invitation\":\"${INVITES[$i]}\"}"
      fi
      code="$(curl -sS -X POST "${URLS[$((i-1))]}/api/session/arm" -H 'Content-Type: application/json' \
        -H "X-CSRF-Token: ${CSRFS[$((i-1))]}" -d "$body" -o "$WORK/relay.arm.$transport.$i.json" -w '%{http_code}')"
      [ "$code" = "200" ] \
        || fail "[$transport] instance $i could not arm before hop 1 (HTTP $code): $(head -c 300 "$WORK/relay.arm.$transport.$i.json")"
      ARM_ADDR[$i]="$(python3 -c "import json;print(json.load(open('$WORK/relay.arm.$transport.$i.json')).get('address',''))" 2>/dev/null)"
      [ -n "${ARM_ADDR[$i]}" ] \
        || fail "[$transport] instance $i armed and reported no address, so nothing can be dialled at its hop"
    done
    echo "[$transport] all $(( N - 1 )) signing parties armed before hop 1"

    # **What the convener holds BEFORE the hops**, so the assertion after them is a delta rather
    # than a total. A total is contaminated by everything the run did earlier and would drift with
    # the harness; a delta answers the question the clause is about — does a relay accumulate?
    local before_docs
    before_docs="$(curl -fsS "${URLS[0]}/api/docs" | python3 -c 'import json,sys;print(len(json.load(sys.stdin).get("docs",[])))')"

    # The hops. Party k+1 receives hop k, and hop k's input is hop k-1's output — which is the
    # baton, and is why this loop is the topology rather than a description of it.
    local k prev="$WORK/relay.$transport.hop0.pdf"
    RELAY_WORDS=()
    for k in $(seq 1 "$n_hops"); do
      local to=$(( k + 1 ))
      local out="$WORK/relay.$transport.hop$k.pdf"
      # **Still armed, at the address it reported, immediately before its hop is dialled.** This
      # fails by PARTY NUMBER rather than as "hop 8 could not connect", which is the difference
      # between a diagnosis and a symptom — and a changed address is a re-arm, which is the thing
      # the clause forbids.
      local st now_addr armed_now
      st="$(curl -fsS "${URLS[$((to-1))]}/api/session/status" 2>/dev/null)"
      armed_now="$(printf '%s' "$st" | python3 -c 'import json,sys;print(json.load(sys.stdin).get("armed"))' 2>/dev/null)"
      now_addr="$(printf '%s' "$st" | python3 -c 'import json,sys;print(json.load(sys.stdin).get("address",""))' 2>/dev/null)"
      [ "$armed_now" = "True" ] \
        || fail "[$transport] instance $to is no longer armed at hop $k of $n_hops — its arm did not survive the $(( k - 1 )) hop(s) before its turn, which is exactly what a party at the far end of a relay has to do"
      [ "$now_addr" = "${ARM_ADDR[$to]}" ] \
        || fail "[$transport] instance $to reports address $now_addr, armed at ${ARM_ADDR[$to]} — the address moved, so it was RE-ARMED between hop 1 and hop $k"

      # **In LAN mode the convener is told NOTHING — not the address, not the transport.** The
      # announcement is the only thing that can carry them, which is the whole clause. The arm
      # address is still passed so this function knows the party is already armed; the `lan` port
      # is what stops it being typed into the dial.
      local dialport=""
      [ "$mode" != "lan" ] || dialport="lan"
      ceremony "$transport" "$dialport" "$out" 1 "$to" "$prev" "$k" 1 "${INVITES[$to]}" "${ARM_ADDR[$to]}"
      RELAY_WORDS+=( "$WORDS" )

      # **The byte prefix, which is what makes this a BATON rather than N ceremonies.** Asserting
      # the documents merely DIFFER is a tautology here: `/api/pdf` is addressed by id and no
      # instance is fetched twice in one relay, so two hops' documents differ whatever happened.
      # A PDF is signed incrementally, so hop k's output must literally begin with hop k-1's
      # bytes — anything else means the party started from a document of their own.
      python3 - "$prev" "$out" "$transport" "$k" <<'PYPFX' || exit 1
import sys
prev = open(sys.argv[1], "rb").read()
cur  = open(sys.argv[2], "rb").read()
t, k = sys.argv[3], sys.argv[4]
if not cur.startswith(prev):
    n = next((i for i, (a, b) in enumerate(zip(prev, cur)) if a != b), min(len(prev), len(cur)))
    print(f"FAIL: [{t}] hop {k}'s document does not begin with hop {int(k)-1}'s bytes — they "
          f"diverge at byte {n} of {len(prev)}. The party did not build on the baton they were "
          f"handed, so the relay is N ceremonies over one file rather than one proceeding.",
          file=sys.stderr)
    sys.exit(1)
print(f"[{t}] hop {k}: {len(prev)} -> {len(cur)} bytes, the previous hop's document is a prefix")
PYPFX
      prev="$out"
    done

    # **The convener holds ONE ceremony document at the end, not one per hop.** This is what
    # `installCeremonyResult` is for: a fourth commit door that REPLACES by ceremony id rather than
    # adding an arrival, because ADR-005 caps a machine at eight open documents and a nine-party
    # relay has eight hops. Without the replacement a convener would be refused its own ceremony at
    # hop 7 — and the refusal would arrive as a 409 on an unrelated-looking route, which is exactly
    # how this run first found the harness's own accumulation.
    local cdocs grew
    cdocs="$(curl -fsS "${URLS[0]}/api/docs" | python3 -c 'import json,sys;print(len(json.load(sys.stdin).get("docs",[])))')"
    grew=$(( cdocs - before_docs ))
    # **The delta must not scale with the number of hops, and ONE is the whole claim.** Every hop
    # returns a document to the convener; `installCeremonyResult` replaces the previous one by
    # ceremony id instead of adding an arrival, so eight hops leave exactly the same footprint as
    # one. A relay that accumulated would be refused its own ceremony at hop 7 by ADR-005's cap.
    [ "$grew" -le 1 ] \
      || fail "[$transport] $n_hops hops added $grew documents to the convener ($before_docs -> $cdocs) — the baton is accumulating instead of being replaced, and at N=9 ADR-005's cap of 8 refuses the ceremony partway through"
    [ "$cdocs" -le 8 ] \
      || fail "[$transport] the convener holds $cdocs documents, past ADR-005's cap of 8"
    echo "[$transport] $n_hops hops added $grew document to the convener ($before_docs -> $cdocs)"

    # ── The distinct-SIGNER set, which no count can give you (P07.S03b's T03) ──
    #
    # `/ByteRange` counts BLOCKS, so one party signing eight times satisfies any count the hops
    # assert. What has to hold is that the finished document was signed by the roster's signing
    # parties, each once, in roster order — which is L3's rule read off the result instead of off
    # the gate that was supposed to enforce it.
    #
    # Read from the LAST hop's attestations, which `ceremony()` already fetched from the receiving
    # instance by document id.
    python3 - "$WORK/atts.$transport.json" "$transport" "${FPS[@]}" <<'PYSET' || exit 1
import json, sys
path, t = sys.argv[1], sys.argv[2]
fps = [f.lower() for f in sys.argv[3:]]
signing = fps[1:]  # party 1 is the non-signing convener
ats = json.load(open(path)).get("attestations") or []
got = [a.get("fingerprint", "").lower() for a in ats]
if len(got) != len(signing):
    print(f"FAIL: [{t}] the finished document carries {len(got)} attestation(s) for "
          f"{len(signing)} obliged signers", file=sys.stderr)
    sys.exit(1)
if len(set(got)) != len(got):
    dupes = sorted({f for f in got if got.count(f) > 1})
    print(f"FAIL: [{t}] a party signed more than once — {[d[:12] for d in dupes]}. A signature "
          f"COUNT cannot see this: /ByteRange counts blocks, so one party signing "
          f"{len(got)} times satisfies every per-hop count in this run.", file=sys.stderr)
    sys.exit(1)
if got != signing:
    print(f"FAIL: [{t}] the signers are not the roster's signing parties in order.\n"
          f"  got   {[g[:12] for g in got]}\n  want  {[w[:12] for w in signing]}\n"
          f"L3's whole rule is that the signatures on a document are the roster's prefix; this "
          f"reads that off the RESULT rather than off the gate meant to enforce it.",
          file=sys.stderr)
    sys.exit(1)
print(f"[{t}] {len(got)} distinct signers, in roster order, one signature each")
PYSET

    RELAY_FINAL="$prev"
  }

  # **QUIC first, and the order is a finding rather than a preference.** A ceremony hop is dialled
  # through `connect()` (`ceremonynet.go:737`), whose dial race is fed through `filterQUIC` —
  # *"the shared endpoint speaks QUIC, and a non-QUIC candidate cannot be handshake-dialled on it"*
  # — and `dialerCeremony` opens that endpoint UNCONDITIONALLY. So a TCP candidate is filtered out
  # of every ceremony dial and the hop spins until `connectDeadline`. Measured here at N=4: hop 1
  # over TCP left the receiver armed and idle while the convener never reached its verification
  # string. QUIC is driven first so a TCP failure is read as the transport question it is.
  # ── `--lan -n N`: the relay ON THE LINK, with nothing typed (P07.S05c, T04) ────────────────
  #
  # **These two modes used to be mutually exclusive by ORDERING rather than by design.** This
  # block `exit 0`s a few lines below, three lines before the `LAN` block, so `--lan -n 9` ran the
  # fixed-port relay and never touched the link — the clause's own driver could not be invoked.
  #
  # It is the run the LAN clause exists for: a nine-party ceremony has eight hops, and the armed
  # side's announcement lasts five minutes, so from the fourth party onward a same-room ceremony
  # silently runs over the public DHT unless the answering listener works. The egress counter is
  # what makes that a measurement rather than a claim.
  if [ "$LAN" = "1" ]; then
    egress_preamble
    # **QUIC first, matching every other run in this file — and the ordering workaround that was
    # here is gone (/pending 300).** Running QUIC first once made the TCP relay that followed fail
    # with a 502 whose cause was *"Couldn't reach the rendezvous network"* — a D19 verdict about the
    # DHT, for a peer on the link and announcing — and TCP-first completed both. Reproduced twice
    # then, it does **not** reproduce on the current tree: two QUIC-first runs at v1.117.199 walked
    # all six hops and reached the egress assertion below.
    #
    # **That makes it a race rather than an ordering fault**, which is what "reproduced twice, then
    # not twice" looks like, so the order is no longer a workaround. The narrowed hypothesis is
    # recorded on the item: a browse that hears ONLY a lingering QUIC announcement from the previous
    # arm sees an all-QUIC candidate set, takes the glare path, and dials an endpoint that is gone.
    # `link_report` above is what makes that checkable the next time it fires.
    relay quic lan
    WORDS_RELAY_QUIC=( "${RELAY_WORDS[@]}" ); FINAL_QUIC="$RELAY_FINAL"
    relay tcp lan
    WORDS_RELAY_TCP=( "${RELAY_WORDS[@]}" ); FINAL_TCP="$RELAY_FINAL"
    after="$(offlink_packets)"
    # **This is RED against shipped code, and it is the clause's own criterion (P07.S05c).**
    #
    # Measured: a two-party LAN ceremony emits ZERO off-link packets; a four-party LAN ceremony
    # relay emits 120. The difference is the invitation. A ceremony hop calls `dialerCeremony`,
    # which opens a rendezvous and calls `rz.Bootstrap` unconditionally — reaching for the PUBLIC
    # DHT — and the arm side does the same. `ceremonynet.go` already suppresses the late PUBLISH
    # when the LAN answers inside the browse window; nothing suppresses the bootstrap.
    #
    # So P03's exit criterion — *"a LAN ceremony completes with NO outbound internet traffic"* — is
    # false for every ceremony that carries an invitation, which is every ceremony P07 builds. It
    # survived because the only `--lan` run was the TWO-PARTY one, which has no invitation and
    # therefore no `cer`; and `--lan -n N` was impossible until this slice removed three separate
    # barriers to it. Nothing had ever measured a ceremony on a link.
    [ "$after" = "$baseline" ] \
      || fail "a $N-party LAN relay emitted $((after - baseline)) packets destined off the link — P03's exit criterion says a LAN ceremony completes with NO outbound internet traffic, and a ceremony hop bootstraps the DHT unconditionally (dialerCeremony). A two-party LAN ceremony emits zero; the difference is the invitation."
    echo "[lan] $(( (N - 1) * 2 )) hops over two transports, and nothing left the link"
  else
    relay quic
    WORDS_RELAY_QUIC=( "${RELAY_WORDS[@]}" ); FINAL_QUIC="$RELAY_FINAL"
    relay tcp
    WORDS_RELAY_TCP=( "${RELAY_WORDS[@]}" ); FINAL_TCP="$RELAY_FINAL"
  fi

  # ── The word-strings: EQUAL within a hop, DISTINCT across hops ──────────────
  #
  # **The clause said "all 2(N-1) word-strings pairwise distinct" and that is unsatisfiable by
  # construction** — `ceremony()` asserts at every hop that the two ends derived the SAME string,
  # which is L2's entire point, so N-1 of the 2(N-1) observations are necessarily equal pairs.
  # Amended at this slice's grill into the two properties the clause was reaching for, and the
  # pair is stronger than the original: "pairwise distinct over 2(N-1)" would have been satisfied
  # by a run that never compared the two ends of anything.
  #
  # The equal-within-a-hop half is already asserted inside `ceremony()` (it fails the hop by name
  # if the two ends disagree). What is left is across hops, and it is what says the channel
  # binding is per-session rather than per-ceremony: every hop of one relay shares a ceremony, a
  # record and a convener, so a binding derived from any of those would repeat.
  python3 - "$N" "${WORDS_RELAY_QUIC[@]}" "--" "${WORDS_RELAY_TCP[@]}" <<'PYWORDS' || exit 1
import sys
n = int(sys.argv[1])
rest = sys.argv[2:]
cut = rest.index("--")
runs = {"quic": rest[:cut], "tcp": rest[cut + 1:]}
for t, words in runs.items():
    if len(words) != n - 1:
        print(f"FAIL: [{t}] the relay recorded {len(words)} word-string(s) for {n-1} hops — a hop "
              f"that produced none would make the distinctness check below vacuous",
              file=sys.stderr)
        sys.exit(1)
    if any(not w.strip() for w in words):
        print(f"FAIL: [{t}] a hop recorded an EMPTY word-string, so nothing was compared for it",
              file=sys.stderr)
        sys.exit(1)
    if len(set(words)) != len(words):
        dupes = [w for w in set(words) if words.count(w) > 1]
        print(f"FAIL: [{t}] two hops of one relay derived the SAME verification words {dupes!r}. "
              f"Every hop here shares a ceremony, a record and a convener, so a repeat means the "
              f"binding comes from one of those rather than from the channel — and two parties "
              f"reading the same four words at different hops cannot tell the hops apart.",
              file=sys.stderr)
        sys.exit(1)
    print(f"[{t}] {len(words)} hops, {len(set(words))} distinct verification strings")
# And the two relays are different ceremonies, so no string may cross between them either.
overlap = set(runs["quic"]) & set(runs["tcp"])
if overlap:
    print(f"FAIL: a verification string appears in BOTH relays ({overlap!r}) — they are separate "
          f"ceremonies over separate channels", file=sys.stderr)
    sys.exit(1)
PYWORDS

  # The two relays really produced different documents, or one of them re-read the other's.
  cmp -s "$FINAL_QUIC" "$FINAL_TCP" \
    && fail "the two relays returned BYTE-IDENTICAL final documents — one re-read the other's result"

  echo "PASS: $N instances, and a $N-party ceremony COMPLETED as a baton relay over BOTH"
  echo "      transports (${ELAPSED_TOTAL}s of hops): a non-signing convener carried it through"
  echo "      $(( N - 1 )) hops, each hop's document containing the previous one as a byte prefix,"
  echo "      every hop's verification string distinct, and the model's ceiling still refusing a"
  echo "      signing party a second turn."
  exit 0
fi

if [ "$LAN" = "1" ]; then
  egress_preamble

  # BOTH transports over the link, and A is told NEITHER the address nor the
  # transport — the announcement is the only thing that can carry them.
  #
  # This is the harness half of ADR-010. `-F transport=` used to be passed to both
  # sides in every mode, so tier 4 was configured past the very disagreement it
  # exists to find: B armed QUIC, A dialled whatever A was told, and they agreed
  # because somebody outside the protocol told them the same answer. The LAN runs
  # now pass it to B only.
  ceremony tcp lan "$WORK/final.lan.pdf" 1 2
  WORDS_LAN="$WORDS"
  ceremony quic lan "$WORK/final.lan.quic.pdf" 1 2
  WORDS_LAN_QUIC="$WORDS"

  after="$(offlink_packets)"
  [ "$after" = "$baseline" ] \
    || fail "the ceremony emitted $((after - baseline)) packets destined off the link — P03's exit criterion says a LAN ceremony completes with NO outbound internet traffic"
  echo "PASS: a ceremony completed over BOTH transports with no address and no transport (${ELAPSED_TOTAL}s of hops)"
  echo "      typed anywhere, and nothing left the link"
  echo "      words: tcp=$WORDS_LAN quic=$WORDS_LAN_QUIC"
  exit 0
fi

ceremony tcp "$SESSION_PORT" "$WORK/final.tcp.pdf" 1 2
WORDS_TCP="$WORDS"
ceremony quic "$SESSION_PORT_QUIC" "$WORK/final.quic.pdf" 1 2
WORDS_QUIC="$WORDS"

# The QUIC run really fetched ITS OWN document. /api/pdf returns B's active
# document, and after two ceremonies that is two different files — so without
# this, a second run that quietly re-read the first run's result would pass the
# signature count above and prove nothing about the second transport.
cmp -s "$WORK/final.tcp.pdf" "$WORK/final.quic.pdf" \
  && fail "the two runs returned BYTE-IDENTICAL documents — the QUIC run re-read the TCP run's result, so its signature count says nothing"

# And they are different ceremonies, so different words: identical strings would
# mean the second run replayed the first's channel binding.
[ "$WORDS_TCP" != "$WORDS_QUIC" ] \
  || fail "both ceremonies produced the same verification words ('$WORDS_TCP') — the channel binding is not per-session"

echo "PASS: a ceremony completed between two instances over BOTH transports (${ELAPSED_TOTAL}s of hops)"
echo "      tcp:  $WORDS_TCP"
echo "      quic: $WORDS_QUIC"
