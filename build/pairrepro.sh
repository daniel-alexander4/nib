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
# The window is printed even on a non-empty answer: "heard nothing" and "listened for 200ms"
# are different faults with the same line, which is this function own reason for existing.
print("    listened %sms" % d.get("windowMs"))
for h in d.get("heard", []):
    # The six-word NAME as well as the label: the label is this harness own bookkeeping and the
    # name is what the peer actually announced, so a mismatch between them is visible here and
    # nowhere else.
    print("    %-10s %-24s %-6s %s" % (h["label"], h["addr"], h["transport"], h["name"]))
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

restart() { # index (1-based) — kill an instance and bring it back on the same home and port
  # **P08.S01's verb, and the reason it did not exist is the reason it is needed.** Nothing in this
  # harness ever stopped an instance mid-run, so nothing had ever read state written by a previous
  # process — which is precisely the capability D24 requires and which no criterion could observe.
  #
  # Same $HOME and same port, because the point is continuity: a restart onto a fresh home would
  # test a new machine joining, which is a different (and already covered) thing.
  local i="$1" idx=$(( $1 - 1 )) pid
  pid="${PIDS[$idx]}"
  kill "$pid" 2>/dev/null || true
  # Waited on by kill -0 rather than by `wait`, for the reason the cleanup trap gives: these come
  # out of a command substitution and are this shell's grandchildren, so `wait` returns instantly
  # with an error — the degenerate "waited" that proves nothing.
  for _ in $(seq 1 40); do kill -0 "$pid" 2>/dev/null || break; sleep 0.1; done
  kill -0 "$pid" 2>/dev/null && { echo "restart: instance $i would not die" >&2; return 1; }
  PIDS[$idx]="$(start "i$i" "${API_PORTS[$idx]}" "${HOMES[$idx]}")"
  wait_up "${URLS[$idx]}" || { echo "restart: instance $i did not come back" >&2; return 1; }
  # **The CSRF token is per PROCESS, so a restart invalidates the cached one.** Found by running
  # this clause: the first attempt used the pre-restart token and got a 403, which the assertion
  # below would have read as "the lookup does not discriminate" — a true failure with the wrong
  # diagnosis. Refreshing it here keeps the verb honest: what a restart must NOT lose is the
  # ceremony, and a session token is not part of that.
  local tok
  tok="$(csrf "${URLS[$idx]}")"
  [ -n "$tok" ] || { echo "restart: instance $i returned no CSRF token after coming back" >&2; return 1; }
  CSRFS[$idx]="$tok"
  return 0
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

# assert_spoken_check grades L2's stimulus for ONE hop, and it is the one door for that rule.
#
# **It exists because the rule had two doors and they drifted the day the second was written.**
# `decline_round` copied this sequence and dropped two of its clauses: the "initiate never got off
# the ground" guard — whose absence reports a 502 connect failure as *"nobody was shown the
# verification words"*, blaming L2 for something that never reached it, the exact misdiagnosis
# `ceremony()` records having measured and fixed — and the four-word shape check, without which two
# ends both returning one malformed word are equal and pass.
#
# `want` is the HTTP code that means the hop got as far as a person: 200 for a hop that completes,
# 409 for one a person refuses. Everything else is the same question either way.
assert_spoken_check() { # label words_a words_b code want body
  local label="$1" wa="$2" wb="$3" code="$4" want="$5" body="$6"
  # This fires FIRST by design — stimulus before grading — so without this arm a hop that never
  # reached the spoken check is reported as a spoken-check failure.
  if [ -z "$wa" ] && [ "$code" != "$want" ]; then
    fail "[$label] initiate returned HTTP $code before any spoken check: $body"
  fi
  [ -n "$wa" ] || fail "[$label] the initiating instance was never shown the verification words — the hop reached the document exchange without the spoken check (L2). HTTP $code: $body"
  [ -n "$wb" ] || fail "[$label] the receiving instance was never shown the verification words. HTTP $code: $body"
  [ "$(echo "$wa" | wc -w)" = 4 ] || fail "[$label] the verification string is not four words: $wa"
  # The property only two instances can see.
  [ "$wa" = "$wb" ] || fail "[$label] the two instances derived DIFFERENT verification words — one saw '$wa' and the other '$wb'. Two people comparing these would read a mismatch as an attack."
}

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
          -H "X-CSRF-Token: $CSRF_B" -d "$CONSENT_ANSWER" >/dev/null 2>&1
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

  # The stimulus, asserted before anything is graded: the spoken check really happened on BOTH
  # sides. Without this, a ceremony that completed because the gate never fired would pass every
  # assertion below. Through `assert_spoken_check`, which `decline_round` also calls — the clauses
  # here were copied once and two of them were lost in the copy.
  assert_spoken_check "$transport" "$words_a" "$words_b" "$code" 200 "$(head -c 300 "$init_out" 2>/dev/null)"

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

  # ── The receiver's own durable copy, on disk (/pending 343) ──────────────────
  #
  # **No harness on any machine had ever inspected `~/nib/ceremonies/`.** Named search, before this
  # clause existed: `grep -rn 'nib/ceremonies\|ceremonies/' build/*.sh` returned nothing. So P08.S02's
  # property — a hop's contribution reaches DISK, on the machine that signed it, before it reaches
  # the wire — was observed at tier 1 and by nothing above it.
  #
  # It is asserted from the RECEIVER's filesystem rather than from any route, because a route is the
  # in-memory document answering a question about the file. `$out` is what that instance reports it
  # produced; the mirror must be those bytes.
  #
  # **Matched by CONTENT, not by counting directories.** The two transports run against the same
  # homes, so instance $ti holds one ceremony directory per transport run by design — a count would
  # be right on the first run and wrong on the second, which is the shape this file keeps refusing.
  if [ "$wantproc" = "1" ]; then
    python3 - "${HOMES[$((ti-1))]}/home/nib/ceremonies" "$out" "$transport" "$ti" <<'PYMIRROR' || exit 1
import hashlib, os, sys
root, out, t, ti = sys.argv[1], sys.argv[2], sys.argv[3], sys.argv[4]
want = open(out, "rb").read()
if not os.path.isdir(root):
    print(f"FAIL: [{t}] instance {ti} signed a ceremony hop and has no {root} at all. The only "
          f"durable copy of the document carrying its own signature does not exist, so a crash "
          f"or a restart loses it — which is what D24 and P08.S02 exist to prevent.",
          file=sys.stderr)
    sys.exit(1)
found = []
for d in sorted(os.listdir(root)):
    p = os.path.join(root, d, "document.pdf")
    if os.path.isfile(p):
        found.append((d, open(p, "rb").read()))
if not found:
    print(f"FAIL: [{t}] {root} exists and holds no document.pdf under any ceremony id "
          f"(subdirectories: {sorted(os.listdir(root))})", file=sys.stderr)
    sys.exit(1)
for d, b in found:
    if b == want:
        print(f"[{t}] instance {ti} kept its own copy: {d}/document.pdf, "
              f"{len(b)} bytes, identical to the document it produced")
        sys.exit(0)
print(f"FAIL: [{t}] instance {ti} holds {len(found)} mirrored ceremony document(s) and NONE is "
      f"the {len(want)}-byte document this hop produced "
      f"(sha256 {hashlib.sha256(want).hexdigest()[:16]}); on disk: "
      + ", ".join(f"{d}={len(b)}B/{hashlib.sha256(b).hexdigest()[:16]}" for d, b in found)
      + ". The mirror is stale or belongs to another hop, so the durable record of this "
        "signature is not this signature.", file=sys.stderr)
sys.exit(1)
PYMIRROR
  fi

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

# ── P08.S05e: a ceremony DECLINED at hop 3, and the parties who signed are told ──────────────
#
# **Placed BELOW `ceremony()` rather than above it.** It used to sit between `ceremony()`'s doc
# block and `ceremony()` itself, so ~350 lines separated that prose from the function it
# describes and a reader arriving at it met this one instead — the misattached-doc shape P08.S05f
# found twice in Go and `/pending 352` counts eighteen of. Shell has no compiler to bind a
# comment, which makes placement the only thing that does.
#
# It CALLS `ceremony()` for the two hops that sign and hand-rolls only the hop that refuses.
# The hand-roll is not duplication left standing: `ceremony()` grades a COMPLETED hop — the
# signature count, the fetched document, the attestations route — and a declined hop produces
# none of those, so driving one through it would mean gating half its body on a flag. What the
# two genuinely share is the spoken check, and that lives in `assert_spoken_check` below,
# called by both.
# CONSENT_ANSWER is what `ceremony()`'s watcher posts to /api/session/respond.
#
# **It is acceptance and nothing overrides it, which is a narrower thing than it was written to be.**
# The first cut of P08.S05e added it as the hook by which `decline_round` would drive its refusal
# through `ceremony()` — and then hand-rolled the refusal anyway, leaving a parameterisation with
# one value and no second caller. It stays a variable because the sentence above is where the
# reason lives: `ceremony()` grades a hop that COMPLETES — the signature count, the fetched
# document, the attestations route — so answering "no" through it would gate half its body on a
# flag rather than reuse it.
CONSENT_ANSWER='{"accept":true,"intent":"I accept"}'


# ── P08.S05e: a ceremony DECLINED at hop 3, and the parties who signed are told ──────────────
#
# C06's telling half. Everything above drives ceremonies that COMPLETE; this is the only sequence
# in the tree that produces an end state, because an end state comes from a person refusing and no
# other input to this system makes one.
#
# **A fresh ceremony, not the relay's.** The relay's is complete, and a completed proceeding cannot
# be declined — so this convenes its own, signs two hops, and refuses the third. The parties who
# signed at hops 1 and 2 are then owed the four things C06 names, and the assertion is that they
# were TOLD: the telling lands on their sticky notice, which is the only channel a delivery arm has.
decline_round() {
  local transport="quic"
  # Per call, not global: nothing calls this twice today, and a second call inheriting the first's
  # invitations and addresses would arm against a ceremony that had ended.
  local -A DINV=() DADDR=()
  # **It drives FOUR parties and says so, rather than indexing off the end of the roster.** Its
  # shape is fixed by the criterion — hops 1 and 2 sign, hop 3 refuses, and the two who signed are
  # the ones owed the telling — so it reads `URLS[3]`/`FPS[3]` directly. At N=3 that index is
  # unbound and `set -u` aborts the whole run with a bash error instead of a sentence; at N=9 the
  # five parties who never armed are still roster parties, and the round dials each for the full
  # 300 s connect deadline before failing on a count. Both are refusals this states rather than
  # discovers.
  if [ "$N" -lt 4 ]; then
    echo "decline: skipped — it drives exactly four parties and this run has $N (C06's telling half needs two signers, a refuser and a convener)"
    return 0
  fi
  echo "decline: convening a 4-party decline inside this $N-instance run…"
  local roster expires code dcid inv i
  # The first FOUR instances only. At N>4 the rest are not on this ceremony's roster at all, so
  # the round has nobody extra to walk and no 300 s leg to spend on a party that never armed.
  roster="$(python3 -c "
import json,sys
fps=sys.argv[1:5]
print(','.join(json.dumps({'fingerprint':f,'label':'p%d'%(n+1),'signs':n>0}) for n,f in enumerate(fps)))" "${FPS[@]}")"
  expires="$(python3 -c "
import datetime;print((datetime.datetime.now(datetime.timezone.utc)+datetime.timedelta(hours=6)).strftime('%Y-%m-%dT%H:%M:%SZ'))")"
  # **Close what the relays left open first.** The convener accumulates a document per hop per
  # transport plus the delivery rounds', and ADR-005's cap is 8 — the harness's own relay comment
  # records hitting it at N=4. This runs after two full relays, so it is over the cap by
  # construction rather than by accident.
  # **`/api/close` is a CLOSE ALL**, and this loop used to close per document and swallow what
  # came back. `handleClose` calls `setDoc(nil)`, which clears the whole registry — its own doc
  # says so and says P06 owns per-document close — so the second call in a loop legitimately 409s
  # with "that document is no longer open". A per-call check therefore fails on correct behaviour;
  # what is worth grading is the OUTCOME, which is what the cap cares about.
  python3 - "${URLS[0]}" "${CSRFS[0]}" <<'PYCLOSE' || exit 1
import json, sys, urllib.request
base, csrf = sys.argv[1], sys.argv[2]

def docs():
    return json.load(urllib.request.urlopen(base + "/api/docs")).get("docs") or []

open_docs = docs()
if open_docs:
    d = open_docs[0]
    req = urllib.request.Request(base + "/api/close", method="POST",
                                 data=json.dumps({"id": d["id"]}).encode(),
                                 headers={"Content-Type": "application/json", "X-CSRF-Token": csrf,
                                          "X-Nib-Doc": d["id"]})
    try:
        urllib.request.urlopen(req).read()
    except Exception as e:
        print("FAIL: decline: /api/close was refused (%s), so the convener is still holding "
              "documents against ADR-005's cap of 8 and the open below will 409 about "
              "something else" % e, file=sys.stderr)
        sys.exit(1)
left = docs()
if left:
    print("FAIL: decline: %d document(s) are still open after a close that clears the whole "
          "registry — the convener accumulated one per hop per transport plus the delivery "
          "rounds', and ADR-005 refuses the ninth" % len(left), file=sys.stderr)
    sys.exit(1)
print("decline: closed %d document(s) the relays left open" % len(open_docs))
PYCLOSE
  code="$(curl -sS -X POST "${URLS[0]}/api/open" -H 'Content-Type: application/json' \
    -H "X-CSRF-Token: ${CSRFS[0]}" -d "{\"path\":\"$WORK/doc.pdf\"}" \
    -o "$WORK/decline.open.json" -w '%{http_code}')"
  [ "$code" = "200" ] \
    || fail "decline: the convener could not open a document to convene over (HTTP $code): $(head -c 300 "$WORK/decline.open.json")"
  code="$(curl -sS -X POST "${URLS[0]}/api/ceremony/convene" -H 'Content-Type: application/json' \
    -H "X-CSRF-Token: ${CSRFS[0]}" \
    -d "{\"roster\":[$roster],\"intent\":\"We agree\",\"expires\":\"$expires\",\"convenerSigns\":false}" \
    -o "$WORK/decline.convene.json" -w '%{http_code}')"
  [ "$code" = "200" ] || fail "decline: convene failed (HTTP $code): $(head -c 300 "$WORK/decline.convene.json")"
  dcid="$(python3 -c "import json;print(json.load(open('$WORK/decline.convene.json'))['ceremony'])")"
  [ -n "$dcid" ] || fail "decline: convene returned no ceremony id"

  for i in 2 3 4; do
    inv="$(python3 -c "
import json
d=json.load(open('$WORK/decline.convene.json'))
print(next(x['invitation'] for x in d['invites'] if x['fingerprint'].lower()=='${FPS[$((i-1))]}'.lower()))")"
    [ -n "$inv" ] || fail "decline: no invitation for instance $i"
    DINV[$i]="$inv"
    code="$(curl -sS -X POST "${URLS[$((i-1))]}/api/ceremony/accept" -H 'Content-Type: application/json' \
      -H "X-CSRF-Token: ${CSRFS[$((i-1))]}" -d "$(python3 -c "import json;print(json.dumps({'invitation':'$inv'}))")" \
      -o /dev/null -w '%{http_code}')"
    [ "$code" = "200" ] || fail "decline: instance $i could not accept (HTTP $code)"
  done

  # Hops 1 and 2 SIGN. Their parties are the ones the telling is owed to.
  local docid prev
  docid="$(python3 -c "import json;print(json.load(open('$WORK/decline.convene.json')).get('document',{}).get('id',''))")"
  curl -fsS "${URLS[0]}/api/pdf?doc=$docid" -o "$WORK/decline.hop0.pdf" \
    || fail "decline: could not fetch the convened document"
  prev="$WORK/decline.hop0.pdf"
  for i in 2 3; do
    code="$(curl -sS -X POST "${URLS[$((i-1))]}/api/session/arm" -H 'Content-Type: application/json' \
      -H "X-CSRF-Token: ${CSRFS[$((i-1))]}" \
      -d "{\"fingerprint\":\"${FPS[0]}\",\"bind\":\"$CEREMONY_HOST:0\",\"mode\":\"cosign\",\"transport\":\"$transport\",\"invitation\":\"${DINV[$i]}\"}" \
      -o "$WORK/decline.arm.$i.json" -w '%{http_code}')"
    [ "$code" = "200" ] || fail "decline: instance $i could not arm (HTTP $code)"
    DADDR[$i]="$(python3 -c "import json;print(json.load(open('$WORK/decline.arm.$i.json')).get('address',''))")"
    # want = the HOP number: the convener does not sign, so hop k carries exactly k signatures.
    ceremony "$transport" "${DADDR[$i]##*:}" "$WORK/decline.hop$((i-1)).pdf" 1 "$i" "$prev" "$((i-1))" 1 "${DINV[$i]}" "${DADDR[$i]}"
    prev="$WORK/decline.hop$((i-1)).pdf"
  done
  echo "decline: hops 1 and 2 signed; party 4 will now refuse"

  # Hop 3 REFUSES. Driven with the same watcher the successful hops use, answering the other way —
  # so the refusal is a person saying no through the real consent route, not an injected error.
  code="$(curl -sS -X POST "${URLS[3]}/api/session/arm" -H 'Content-Type: application/json' \
    -H "X-CSRF-Token: ${CSRFS[3]}" \
    -d "{\"fingerprint\":\"${FPS[0]}\",\"bind\":\"$CEREMONY_HOST:0\",\"mode\":\"cosign\",\"transport\":\"$transport\",\"invitation\":\"${DINV[4]}\"}" \
    -o "$WORK/decline.arm.4.json" -w '%{http_code}')"
  [ "$code" = "200" ] || fail "decline: instance 4 could not arm (HTTP $code)"
  DADDR[4]="$(python3 -c "import json;print(json.load(open('$WORK/decline.arm.4.json')).get('address',''))")"

  # **The spoken check comes BEFORE the consent gate, so a decline that is never asked for is
  # never given.** Both ends block on the words — the convener inside its own `/initiate` request
  # and party 4 inside its session goroutine — exactly as `ceremony()` describes, so the
  # confirmations have to arrive on separate requests from watchers started here. Measured
  # 2026-09-02: without them the hop sat until `ErrVerificationTimedOut` fired and `/initiate`
  # answered 409 for THAT, which the code-only assertion below used to accept as the refusal.
  rm -f "$WORK/decline.words_a" "$WORK/decline.words_4"
  watch_verify "${URLS[0]}" "${CSRFS[0]}" "$WORK/decline.words_a" &
  local dva=$!
  watch_verify "${URLS[3]}" "${CSRFS[3]}" "$WORK/decline.words_4" &
  local dv4=$!
  (
    for _ in $(seq 1 240); do
      if [ -n "$(curl -fsS "${URLS[3]}/api/session/status" 2>/dev/null | jget pending.fingerprint)" ]; then
        curl -fsS -X POST "${URLS[3]}/api/session/respond" -H 'Content-Type: application/json' \
          -H "X-CSRF-Token: ${CSRFS[3]}" -d '{"accept":false}' >/dev/null 2>&1
        exit 0
      fi
      sleep 0.25
    done
  ) &
  local dw=$!
  code="$(curl -sS -X POST "${URLS[0]}/api/session/initiate" -H "X-CSRF-Token: ${CSRFS[0]}" \
    -F "pdf=@$prev" -F "appearance=@$WORK/sig.png" \
    -F "params={\"fingerprint\":\"${FPS[3]}\",\"intent\":\"hop 3\"}" \
    -F "address=$CEREMONY_HOST:${DADDR[4]##*:}" -F "transport=$transport" -F "invitation=${DINV[4]}" \
    -o "$WORK/decline.hop3.json" -w '%{http_code}')"
  wait "$dva" 2>/dev/null; wait "$dv4" 2>/dev/null; wait "$dw" 2>/dev/null || true

  # The stimulus, asserted before the refusal is graded — the house rule this harness applies to
  # every hop. A decline is a person answering the consent gate, and nothing reaches that gate
  # until both ends have confirmed the words; so a run where the words never appeared did not
  # drive a refusal, whatever it returned.
  local dwa dw4 dbody
  dwa="$(cat "$WORK/decline.words_a" 2>/dev/null || true)"
  dw4="$(cat "$WORK/decline.words_4" 2>/dev/null || true)"
  dbody="$(head -c 300 "$WORK/decline.hop3.json" 2>/dev/null || true)"
  # The same door `ceremony()` uses. A decline reaches the consent gate only THROUGH the spoken
  # check, so a hop where the words never appeared did not drive a refusal, whatever it returned —
  # and 409 is the code that means this hop got as far as a person.
  assert_spoken_check "decline hop 3" "$dwa" "$dw4" "$code" 409 "$dbody"

  [ "$code" = "409" ] \
    || fail "decline: hop 3 returned HTTP $code, want 409 — the party refused, and a refusal that
      does not reach the convener as a refusal cannot end a proceeding: $dbody"
  # **And 409 for the RIGHT reason.** `/api/session/initiate` answers 409 for five different
  # things — a decline, an unanswered consent request, a words MISMATCH, a words TIMEOUT, and a
  # contribution refusal (`internal/server/session.go:2369-2418`). Four of them are failures in
  # which nobody ever refused, so the bare code is satisfied by a hop that never got a person's
  # answer at all. Measured: this clause passed on the words-timeout arm before the watchers
  # above existed, and the run then failed one line down for a defect that was not there.
  case "$dbody" in
    *"declined to co-sign"*) : ;;
    *) fail "decline: hop 3 returned 409, but not for a decline — the body reads $dbody. A person
      answering 'no' is the only input that produces an end state, so a 409 from any other arm of
      this route means the sequence below is grading a proceeding nobody ended." ;;
  esac

  # **The convener ATTESTED it.** Before P08.S05e nothing did: `SignTermination` and
  # `WriteTermination` had zero production callers, so a declined proceeding was a sentence shown to
  # one person and no record anywhere.
  # **The file's CONTENT, not its existence.** `[ -f ]` passes on an empty file, on one carrying
  # `StateCompleted`, and on one with no signature — and the comment above says "attested", which
  # is a claim about all three. It also checks the marker the round reads to decide whom NOT to
  # walk, since the two are written together and a termination without it restores the 300 s leg.
  python3 - "${HOMES[0]}/home/nib/ceremonies/$dcid" "${FPS[3]}" <<'PYTERM' || exit 1
import json, os, sys
d = sys.argv[1]
tp = os.path.join(d, "termination.json")
if not os.path.exists(tp):
    print("FAIL: decline: the convener wrote no termination for a proceeding it just saw declined "
          "— the parties who signed at hops 1 and 2 have no way to learn it is over",
          file=sys.stderr)
    sys.exit(1)
t = json.load(open(tp))
if t.get("state") != "declined":
    print("FAIL: decline: the attestation records state=%r, want 'declined' — a proceeding a party "
          "refused is being attested as something else" % t.get("state"), file=sys.stderr)
    sys.exit(1)
for k in ("ceremony", "sig"):
    if not t.get(k):
        print("FAIL: decline: the attestation carries no %s, so it is a file rather than something "
              "a recipient can check" % k, file=sys.stderr)
        sys.exit(1)
ep = os.path.join(d, "ended-by")
if not os.path.exists(ep):
    print("FAIL: decline: the convener attested the end state but recorded nobody as having ended "
          "it — the round then walks the party that refused and spends its whole connect deadline "
          "reaching a machine that pruned this ceremony when it declined", file=sys.stderr)
    sys.exit(1)
ender = open(ep).read().strip().lower()
if ender != sys.argv[2].lower():
    print("FAIL: decline: the recorded ender is %r; the party that refused was %r. The round would "
          "skip the wrong party — silencing a telling that is owed and walking a leg that cannot "
          "succeed." % (ender[:12], sys.argv[2][:12]), file=sys.stderr)
    sys.exit(1)
print("decline: the convener attested state=declined and recorded who ended it (D28)")
PYTERM

  # And the round TELLS the parties who signed. Their notice is the only channel a delivery arm has.
  local daddrs="{}"
  for i in 2 3; do
    # **The disarm's status is checked.** It drops the INTERACTIVE arm so that `status()` falls
    # through to the delivery slot, and the address read below is what that fall-through returns.
    # Swallowed, a failed disarm hands back the interactive arm's port and the round dials a
    # listener that is not the delivery rendezvous — under a red that says "no delivery arm",
    # which is not what happened.
    local dcode
    dcode="$(curl -sS -X POST "${URLS[$((i-1))]}/api/session/disarm" -H "X-CSRF-Token: ${CSRFS[$((i-1))]}" -o /dev/null -w '%{http_code}')"
    [ "$dcode" = "200" ] \
      || fail "decline: instance $i would not drop its interactive arm (HTTP $dcode), so the address read below would be that arm's and not the delivery rendezvous"
    local a
    a="$(curl -fsS "${URLS[$((i-1))]}/api/session/status" | jget address 2>/dev/null || true)"
    [ -n "$a" ] || fail "decline: instance $i has no delivery arm, so it cannot be told"
    # Observed for the assertion, sent only off the link — the relay's rounds make the same
    # distinction and for the same reason (S05h). On the link the convener must FIND them.
    [ "$LAN" != "1" ] || continue
    daddrs="$(python3 -c "
import json,sys
d=json.loads(sys.argv[1]); d[sys.argv[2]]='$CEREMONY_HOST:'+sys.argv[3].rsplit(':',1)[1]; print(json.dumps(d))" "$daddrs" "${FPS[$((i-1))]}" "$a")"
  done
  code="$(curl -sS -X POST "${URLS[0]}/api/ceremony/deliver" -H 'Content-Type: application/json' \
    -H "X-CSRF-Token: ${CSRFS[0]}" \
    -d "$(python3 -c "
import json,sys;print(json.dumps({'ceremony':sys.argv[1],'addresses':json.loads(sys.argv[2])}))" "$dcid" "$daddrs")" \
    -o "$WORK/decline.deliver.json" -w '%{http_code}')"
  [ "$code" = "200" ] || fail "decline: the end-state round returned HTTP $code: $(head -c 300 "$WORK/decline.deliver.json")"

  # **The round's own per-party report, graded.** It is the only thing outside the product that
  # reads the round's outcome (/pending 353), and it is where the shape of a DECLINED round is
  # visible: the party that refused is still a roster party, so the walk reaches it — and its own
  # machine pruned the ceremony's stored invitation when it declined, so it can never arm a
  # delivery rendezvous for this proceeding again.
  python3 - "$WORK/decline.deliver.json" "${FPS[1]}" "${FPS[2]}" "${FPS[3]}" <<'PYDEL' || exit 1
import json, sys
report = json.load(open(sys.argv[1]))
want_delivered = [f.lower() for f in sys.argv[2:4]]
decliner = sys.argv[4].lower()
rows = report.get("parties", [])
parties = {p["fingerprint"].lower(): p for p in rows}
# Counted on the ROWS, not on the dict they key into: two outcomes for one party collapse in a
# dict and read as a correct count, which is the shape that hides a party walked twice.
if len(rows) != len(parties):
    print("FAIL: the round reported %d rows for %d distinct parties — a party appears twice, so "
          "one leg ran more than once and the report cannot say which outcome is that party's"
          % (len(rows), len(parties)), file=sys.stderr)
    sys.exit(1)
if len(rows) != 3:
    print("FAIL: the round reported %d parties, want 3 — the convener is skipped as holding it "
          "already, and every other roster party is walked" % len(rows), file=sys.stderr)
    sys.exit(1)
for fp in want_delivered:
    p = parties.get(fp)
    if p is None:
        print("FAIL: the round's report names no outcome for %s, a party that SIGNED" % fp[:12],
              file=sys.stderr)
        sys.exit(1)
    if not p.get("delivered"):
        print("FAIL: the round reports %s (%s) as not delivered: %r. This party signed, so the "
              "telling is what C06 owes it." % (p.get("label"), fp[:12], p.get("reason")),
              file=sys.stderr)
        sys.exit(1)
d = parties.get(decliner)
if d is None:
    print("FAIL: the round's report names no outcome for the party that DECLINED — it is still a "
          "roster party and a walk that silently omits one cannot be reconciled", file=sys.stderr)
    sys.exit(1)
if d.get("delivered"):
    print("FAIL: the round reports the party that declined as delivered. That machine pruned this "
          "ceremony's stored invitation when it refused and holds no mirror to verify an "
          "attestation against, so a leg that reports success there is reporting it about nothing.",
          file=sys.stderr)
    sys.exit(1)
# **`skipped` specifically, and the reason matched — not "some verdict was reported".**
# A leg that was WALKED and merely failed also carries a reason (`res.Reason = err.Error()`), so a
# predicate of "reason or skipped" is satisfied identically by the 300 s dial this rule exists to
# remove. Delete the skip from production and the loose form stays green.
if not d.get("skipped"):
    print("FAIL: the party that declined is reported skipped=%r reason=%r — the round WALKED a leg "
          "that cannot succeed, which costs the whole connect deadline (300 s) on this round and "
          "on every re-run of it" % (d.get("skipped"), d.get("reason")), file=sys.stderr)
    sys.exit(1)
if "ended the proceeding" not in (d.get("reason") or ""):
    print("FAIL: the skipped party's reason is %r, which does not say they ended the proceeding. "
          "A skip that reads like a failure is one a convener will re-run forever."
          % d.get("reason"), file=sys.stderr)
    sys.exit(1)
print("decline: the round delivered 2 tellings and SKIPPED the party that refused (%s)"
      % d.get("reason"))
PYDEL

  # **C06 names FOUR things, and a tag is none of them.** The header above says the parties "are
  # owed the four things C06 names, and the assertion is that they were TOLD" — while the check
  # graded only the opaque `what`. A telling that covers three of four reads as complete from a
  # tag, which is the shape the tier-1 clause for this was written against and the tier-4 one had
  # dropped. The wording is the product's; what is asserted is that each of the four IS said.
  for i in 2 3; do
    curl -fsS "${URLS[$((i-1))]}/api/session/status" -o "$WORK/decline.notice.$i.json" \
      || fail "decline: instance $i's status could not be read, so nothing can be said about what it was told"
    python3 - "$WORK/decline.notice.$i.json" "$i" <<'PYTELL' || exit 1
import json, sys
st = json.load(open(sys.argv[1]))
who = sys.argv[2]
n = st.get("notice") or {}
if n.get("what") != "ceremony-declined":
    print("FAIL: decline: instance %s's notice is %r, want 'ceremony-declined'. This party SIGNED "
          "and holds a signed document; without the telling it believes the proceeding is still "
          "travelling and has no way to learn otherwise." % (who, n.get("what")), file=sys.stderr)
    sys.exit(1)
text = ((n.get("summary") or "") + " " + (n.get("detail") or "")).lower()
# One clause per thing C06 requires a party to be told, asserted separately: a telling that
# covers three of four is not a telling, and a single joined match cannot tell which is missing.
owed = [
    ("that it is over",                    ["over", "declined"]),
    ("who ended it",                       ["convener"]),
    ("that their signature stands",        ["signature stands", "your signature"]),
    ("that a re-run starts from the original unsigned file", ["original unsigned", "new proceeding"]),
]
missing = [name for name, alts in owed if not any(a in text for a in alts)]
if missing:
    print("FAIL: decline: instance %s was told the proceeding ended but not %s. C06 names four "
          "things a party who has already signed is owed, and the telling said: %r"
          % (who, " or ".join(missing), text[:300]), file=sys.stderr)
    sys.exit(1)
print("decline: instance %s was told all four things C06 names" % who)
PYTELL
  done
  echo "decline: both parties who signed were told the proceeding ended (C06)"
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

  # ── P08.S01: instance 3 restarts and rejoins from its own disk ──────────────
  #
  # The criterion is D24's — a ceremony survives quitting Nib — and before this slice it was false
  # on every machine but the convener's: `/api/ceremony/accept` pinned the convener and threw the
  # invitation away, so a restarted party had a pin, an identity, and no way back in.
  #
  # **The assertion is that the arm request carries NO invitation.** That distinction is the whole
  # clause: this harness holds `inv3` in a shell variable, so a re-arm that pasted it again would
  # pass while the product could not do it — ADR-010's "configured past the disagreement" shape,
  # which this file has already been burned by once.
  cid="$(python3 -c "import json;print(json.load(open('$WORK/convene.json'))['ceremony'])" 2>/dev/null)"
  [ -n "$cid" ] || fail "convene returned no ceremony id"
  restart 3 || fail "instance 3 did not survive a restart"
  # Stimulus: a DIFFERENT ceremony id is refused after the restart, so the lookup is live and
  # discriminating. Without it the pass below is satisfied by a build that ignores the field.
  rcode="$(curl -sS -X POST "${URLS[2]}/api/session/arm" -H 'Content-Type: application/json' \
    -H "X-CSRF-Token: ${CSRFS[2]}" -o "$WORK/rearm-bad.json" -w '%{http_code}' \
    -d "$(python3 -c "import json;print(json.dumps({'fingerprint':'${FPS[0]}','bind':'127.0.0.1:0','transport':'tcp','ceremony':'cc'+'0'*30}))")")"
  [ "$rcode" = "400" ] \
    || fail "arming for a ceremony this machine never accepted returned HTTP $rcode, not 400 — the stored-invitation lookup does not discriminate"
  rcode="$(curl -sS -X POST "${URLS[2]}/api/session/arm" -H 'Content-Type: application/json' \
    -H "X-CSRF-Token: ${CSRFS[2]}" -o "$WORK/rearm.json" -w '%{http_code}' \
    -d "$(python3 -c "import json;print(json.dumps({'fingerprint':'${FPS[0]}','bind':'127.0.0.1:0','transport':'tcp','ceremony':'$cid'}))")")"
  [ "$rcode" = "200" ] \
    || fail "instance 3 could not re-arm from its own disk after a restart (HTTP $rcode): $(head -c 300 "$WORK/rearm.json")"
  echo "instance 3 restarted and re-armed from its own disk, with no invitation in the request (D24, P08.S01)"
  curl -sS -X POST "${URLS[2]}/api/session/disarm" -H "X-CSRF-Token: ${CSRFS[2]}" -o /dev/null || true

  # **Hop 2 is driven over the CONVENED document, and it used to be driven over an unrelated one
  # (fixed 2026-08-28, P07's phase close).**
  #
  # This offered `hop1.pdf` — the output of the MANUAL two-party exchange above — together with
  # `inv3`, an invitation for the ceremony convened afterwards. The comment below said so in as
  # many words: "over a document unrelated to the ceremony convened afterwards". So the request
  # carried TWO faults at once: a party out of turn, AND an invitation that does not describe this
  # document at all. L3's not-your-turn happened to be the refusal that fired, and the clause read
  # as driving L3.
  #
  # P07.S07b added the arrival check to the dial door — C17 at the door that never had it — and
  # its refusal is both earlier and more accurate: *this invitation does not describe the ceremony
  # in this document*. The clause then went red, correctly, and the fix is the fixture rather than
  # the check. This is the second harness to hit it today; `build/ceremonyrepro.sh`'s L3 clause had
  # the same shape and the same cause.
  #
  # So: the convened document, offered by a party who is genuinely out of turn. Instance 3 is
  # THIRD in the signing order and the document carries no signature yet, so the prefix rule says
  # instance 1 signs first — one fault, named by L3, with the arrival check satisfied.
  curl -fsS "${URLS[0]}/api/pdf" -o "$WORK/convened.pdf" 2>/dev/null \
    || fail "could not fetch the convened document for hop 2"
  [ -s "$WORK/convened.pdf" ] || fail "the convened document is empty"
  hop2code="$(curl -sS -X POST "${URLS[2]}/api/session/initiate" -H "X-CSRF-Token: ${CSRFS[2]}" \
    -F "pdf=@$WORK/convened.pdf" -F "appearance=@$WORK/sig.png" \
    -F "params={\"fingerprint\":\"${FPS[0]}\",\"intent\":\"hop 2\"}" \
    -F "address=127.0.0.1:${SESSION_PORTS[0]}" -F "transport=tcp" -F "invitation=$inv3" \
    -o "$WORK/hop2.json" -w '%{http_code}')"
  if [ "$hop2code" = "200" ]; then
    fail "hop 2 SUCCEEDED (HTTP 200) — a party THIRD in the signing order contributed to a
      document carrying no signature at all, so the roster prefix rule is not being enforced
      at the initiating door."
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
    local docid rel_cid
    docid="$(python3 -c "
import json;print(json.load(open('$WORK/relay.convene.$transport.json')).get('document',{}).get('id',''))" 2>/dev/null)"
    # The ceremony id, for the delivery round below (P08.S05g).
    rel_cid="$(python3 -c "
import json;print(json.load(open('$WORK/relay.convene.$transport.json')).get('ceremony',''))" 2>/dev/null)"
    [ -n "$rel_cid" ] || fail "[$transport] convene returned no ceremony id, so the delivery round has nothing to name"
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

    # ── The DELIVERY ROUND (P08.S05g; C08, C10) ──────────────────────────────────
    #
    # Everything above proves the document was BUILT. This proves it was DELIVERED — which is a
    # different fact and, until this slice, one no harness had ever asked. C08 is "the finished
    # document reaches every party, including those whose hop completed hours earlier": in a baton
    # relay only the LAST party ever sees the finished file, and every earlier one holds a document
    # missing every signature added after theirs.
    #
    # **Driven at the convener's route, because the round is a route** (S05g's rung-2 decision):
    # `POST /api/ceremony/deliver`. That also makes C10's re-run literally a second POST rather
    # than something this harness has to simulate.
    #
    # **The evidence is on each RECIPIENT's disk, never the convener's report.** Asking the sender
    # whether it delivered is asking the thing under test — the same rule this file already applies
    # to signature counts ("read from B's side, because asking A whether A signed is asking the
    # thing under test").
    #
    # **Each party's delivery address is OBSERVED, not configured.** These instances have no DHT
    # and no multicast — every hop above is driven by a typed `address=` — so nothing here can
    # resolve a rendezvous, and the round's off-LAN discovery is a live-network property this tier
    # cannot reach (said again in the acceptance ledger rather than implied).
    #
    # **All three clauses are true of the PLAIN mode and false of `--lan` (P08.S05h).** That mode
    # re-execs into a namespace built so multicast works, hops are driven with no address at all,
    # and since S05h the round there resolves each party over the link and nothing else. The
    # address below is read in both modes and sent only in this one; see the block that sends it.
    #
    # What IS drivable in the plain mode is the round itself, and the address for it comes from
    # each party's own `/api/session/status`:
    # the product bound it, and the harness reads what it bound. A harness handed the same constant
    # on both sides proves nothing — ADR-010's lesson, which this file has been burned by once.
    local addrs="{}" fpx adx
    for i in $(seq 2 "$N"); do
      # **Cancel the co-signing session first, and that is what a person does too.** A party that
      # has signed stays interactively armed for the re-delivery window (`connectDeadline`), and
      # `status.address` reports the INTERACTIVE arm when both slots are full — so without this the
      # convener dialled the co-sign listener with a delivery frame and the round hung. Cancel
      # tears down that slot ALONE since P08.S05g; before it, this line would have taken the
      # delivery arm with it and the round would have had nobody to reach for a different reason.
      curl -sS -X POST "${URLS[$((i - 1))]}/api/session/disarm" \
        -H "X-CSRF-Token: ${CSRFS[$((i - 1))]}" -o /dev/null || true
      # The PORT is what is observed; the host is loopback because that is where these instances
      # are. A delivery arm binds `0.0.0.0:0` and reports `0.0.0.0:<port>` — a bind, not a dialable
      # address — and dialling it verbatim is how an earlier run of this clause hung.
      adx="$(curl -fsS "${URLS[$((i - 1))]}/api/session/status" | jget address 2>/dev/null || true)"
      [ -n "$adx" ] && adx="127.0.0.1:${adx##*:}"
      [ -n "$adx" ] \
        || fail "[$transport] instance $i reports no armed address after signing — it never armed
      for delivery, so the round has nobody to reach. P08.S05g arms the delivery slot when a hop is
      mirrored; a blank here means that trigger did not fire."
      fpx="${FPS[$((i - 1))]}"
      # **Read for the assertion, SENT only off the link (P08.S05h).** The address is what
      # proves the party armed for delivery at all, so it is still observed and still asserted
      # above. Handing it to the round is the crutch ADR-010 names: under `--lan` the point is
      # that the convener finds the party the way a convener on one office network does — over
      # the link — and a round given the answer proves nothing about the tier S05h added. Off
      # the link this namespace-free mode has no multicast and no DHT, so the typed address is
      # the only thing that can carry it and it is sent.
      if [ "$mode" != "lan" ]; then
        addrs="$(python3 -c "
import json,sys
d=json.loads('''$addrs'''); d['$fpx']='$adx'; print(json.dumps(d))")"
      fi
    done
    local DELIVER_BODY
    DELIVER_BODY="$(python3 -c "
import json,sys
print(json.dumps({'ceremony': sys.argv[1], 'addresses': json.loads(sys.argv[2])}))" "$rel_cid" "$addrs")"

    # ── C10's injected failure, applied BEFORE the first round ───────────────────
    #
    # *"An injected write failure at party 3 of 4, after which the party is NOT recorded as
    # acknowledged and the re-run delivers to that party and to no other."* The write is made to
    # fail by taking away the party's `~/nib/signed` directory — the save then returns
    # `ErrNotStored`, which since P08.S05a is what stops an acknowledgement being a lie.
    #
    # **It has to be a write failure at the PARTY, and the first cut of this clause removed the
    # convener's marker instead.** That models a crash between delivering and recording, where the
    # party already HAS the document and its one-shot arm is spent — so the recovery run had nobody
    # to reach and hung. The two failures are not interchangeable, and only this one leaves the
    # party both unacknowledged and still listening.
    local victim vfp vsigned before_mtime after_mtime witness
    victim=3
    [ "$N" -ge 4 ] || victim=2
    # **The witness is a party that is neither the convener nor the victim, chosen rather than
    # assumed.** It was hard-coded to instance 2 while the victim moves with N — so at N=3, where
    # the victim IS instance 2, the witness was the party whose signed/ directory had just been
    # wiped, and the run failed on its own setup with "no witness file on instance 2". Measured
    # 2026-09-02 against the committed harness at v1.117.320: red at N=3, and only N=2/4/9 are
    # ever run, so nothing had asked.
    witness=2
    [ "$victim" != "2" ] || witness=3
    vfp="$(printf '%s' "${FPS[$((victim - 1))]}" | tr 'A-Z' 'a-z')"
    vsigned="${HOMES[$((victim - 1))]}/home/nib/signed"
    rm -rf "$vsigned" && : > "$vsigned" \
      || fail "[$transport] could not block party $victim's signed/ directory"
    # STIMULUS: the block is really in place. A regular file where a directory must be makes
    # MkdirAll fail; without this the "failed" party below could simply have succeeded.
    [ -f "$vsigned" ] \
      || fail "[$transport] party $victim's signed/ is not blocked, so no write failure is injected"

    echo "[$transport] delivering the finished document to $((N - 1)) parties…"
    dcode="$(curl -sS -X POST "${URLS[0]}/api/ceremony/deliver" -H 'Content-Type: application/json' \
      -H "X-CSRF-Token: ${CSRFS[0]}" \
      -d "$DELIVER_BODY" \
      -o "$WORK/deliver.$transport.json" -w '%{http_code}')"
    [ "$dcode" = "200" ] \
      || fail "[$transport] the delivery round returned HTTP $dcode: $(head -c 400 "$WORK/deliver.$transport.json")"

    # Per party, on their own disk. `signed/` is where S05d routes a delivered document, and the
    # name is deterministic — which is what makes "exactly one file per party" checkable at all.
    local delivered_ok=0 i
    for i in $(seq 2 "$N"); do
      local n_files want_files=1
      # The injected party has no directory to hold anything, by construction.
      [ "$i" = "$victim" ] && want_files=0
      n_files="$(find "${HOMES[$((i - 1))]}/home/nib/signed" -maxdepth 1 -name "*-$rel_cid.pdf" 2>/dev/null | wc -l)"
      [ "$n_files" = "$want_files" ] \
        || fail "[$transport] instance $i holds $n_files copies of ceremony $rel_cid under
      ~/nib/signed, want exactly $want_files. C08 says the finished document reaches EVERY party, and C10
      says a round leaves exactly one file each — 0 means this party never got the document it
      signed, and >1 means the deterministic filename S05d built is not deterministic."
      delivered_ok=$((delivered_ok + 1))
    done
    # STIMULUS: the loop ran. Without this, "every party has exactly one" is true of a loop that
    # checked nobody — the vacuous green this file has been burned by before.
    [ "$delivered_ok" = "$((N - 1))" ] \
      || fail "[$transport] the delivery check ran over $delivered_ok parties, not $((N - 1))"
    echo "[$transport] every party holds exactly one copy of the finished document (C08)"


    # The convener must NOT have recorded the party whose write failed — that is the whole of
    # C10's first half, and P08.S05a's ack-means-persisted is what makes it observable at all.
    [ ! -e "${HOMES[0]}/home/nib/ceremonies/$rel_cid/delivered/$vfp" ] \
      || fail "[$transport] the convener recorded party $victim as delivered although its write
      FAILED. An acknowledgement that outruns the disk is the false receipt P08.S05a removed one
      layer down, and it makes C10's re-run skip the one party that needs reaching."
    python3 - "$WORK/deliver.$transport.json" "$transport" "$vfp" <<'PYFAIL' || exit 1
import json, sys
path, t, victim = sys.argv[1], sys.argv[2], sys.argv[3]
parties = json.load(open(path)).get("parties") or []
bad = [p for p in parties if p.get("fingerprint", "").lower() == victim]
if len(bad) != 1:
    print(f"FAIL: [{t}] the round did not report the injected party at all", file=sys.stderr)
    sys.exit(1)
if bad[0].get("delivered"):
    print(f"FAIL: [{t}] the party whose write failed is reported DELIVERED", file=sys.stderr)
    sys.exit(1)
if not bad[0].get("reason"):
    print(f"FAIL: [{t}] the failed party carries no reason, so the convener is told a party is "
          f"missing and nothing about why", file=sys.stderr)
    sys.exit(1)
others = [p for p in parties if p.get("fingerprint", "").lower() != victim]
if not all(p.get("delivered") for p in others):
    print(f"FAIL: [{t}] one party's write failure stopped the round reaching the others — a round "
          f"must not end on the first failure, or one bad disk denies everybody their copy",
          file=sys.stderr)
    sys.exit(1)
print(f"[{t}] the party whose write failed is unrecorded and named; the others were reached")
PYFAIL

    # ── The RECOVERY run: the party is repaired, and the re-run reaches it and no other ──
    rm -f "$vsigned" && mkdir -p "$vsigned" \
      || fail "[$transport] could not repair party $victim's signed/ directory"
    # A witness on a party that already succeeded: its file must not be rewritten. mtime is the
    # only thing that can see it, because the deterministic filename makes a re-delivery
    # byte-identical to what is already there.
    before_mtime="$(find "${HOMES[$((witness - 1))]}/home/nib/signed" -name "*-$rel_cid.pdf" -printf '%T@\n' 2>/dev/null | head -1)"
    [ -n "$before_mtime" ] || fail "[$transport] no witness file on instance $witness before the recovery run"

    dcode="$(curl -sS -X POST "${URLS[0]}/api/ceremony/deliver" -H 'Content-Type: application/json' \
      -H "X-CSRF-Token: ${CSRFS[0]}" -d "$DELIVER_BODY" \
      -o "$WORK/deliver3.$transport.json" -w '%{http_code}')"
    [ "$dcode" = "200" ] || fail "[$transport] the recovery re-run returned HTTP $dcode: $(head -c 300 "$WORK/deliver3.$transport.json")"

    python3 - "$WORK/deliver3.$transport.json" "$transport" "$vfp" <<'PYONE' || exit 1
import json, sys
path, t, victim = sys.argv[1], sys.argv[2], sys.argv[3]
parties = json.load(open(path)).get("parties") or []
redelivered = [p for p in parties if not p.get("skipped")]
if len(redelivered) != 1:
    print(f"FAIL: [{t}] the recovery re-run delivered to {len(redelivered)} parties, want exactly "
          f"1. C10 is explicit that a re-run reaches the party that failed AND NO OTHER — a round "
          f"that re-delivers to everybody is 'satisfied completely by a round that delivers "
          f"twice', which the criterion names as the defect it exists to catch.", file=sys.stderr)
    sys.exit(1)
if redelivered[0].get("fingerprint", "").lower() != victim:
    print(f"FAIL: [{t}] the re-run delivered to {redelivered[0].get('fingerprint','')[:12]}, want "
          f"the party whose write failed ({victim[:12]})", file=sys.stderr)
    sys.exit(1)
if not all(p.get("delivered") for p in parties):
    print(f"FAIL: [{t}] a party is not reported delivered after the recovery run", file=sys.stderr)
    sys.exit(1)
print(f"[{t}] the recovery re-run reached exactly the party whose write had failed (C10)")
PYONE

    n_files="$(find "$vsigned" -maxdepth 1 -name "*-$rel_cid.pdf" 2>/dev/null | wc -l)"
    [ "$n_files" = "1" ] \
      || fail "[$transport] after the recovery run the repaired party holds $n_files copies, want 1"
    after_mtime="$(find "${HOMES[$((witness - 1))]}/home/nib/signed" -name "*-$rel_cid.pdf" -printf '%T@\n' 2>/dev/null | head -1)"
    [ "$before_mtime" = "$after_mtime" ] \
      || fail "[$transport] the recovery run REWROTE an already-acknowledged party's file.
      The deterministic filename hides that from any count or content check, so mtime is the only
      thing that can see 'skipped' rather than 'delivered again' — and re-delivering to a party
      that already has it is what C10 forbids."
    echo "[$transport] exactly one file per party, and no already-delivered party was rewritten (C10)"

    # ── Every party disarms before the next transport's relay ────────────────────
    #
    # **Without this the SECOND transport's relay could not arm at all, and had not been running.**
    # Measured 2026-08-31: a `-n 4` run completed all three QUIC hops and then failed at
    # `instance 2 could not arm before hop 1 (HTTP 409): a session is already armed` — so the whole
    # TCP arm of the N-party relay was unreachable, on a harness whose own contract line says it
    # runs "over BOTH transports". The same shape as /pending 344 one level up.
    #
    # **It is the product behaving correctly, not a bug being papered over.** A party that has
    # signed holds a re-delivery window of `connectDeadline` (300s, `clocks.go:53`) so a writeback
    # lost in flight can be re-served without re-signing — D24/D18, and `runCeremonyReceive` arms
    # it at the first signature. The QUIC relay finishes in seconds, well inside that window, so
    # the collision is DETERMINISTIC rather than flaky: every party is still legitimately armed.
    #
    # A real second ceremony on one machine within five minutes hits the same 409, and the user's
    # answer is the same as this one — disarm first. That the harness has to say so is the honest
    # cost of a window that exists to protect a signature.
    local d
    for d in $(seq 2 "$N"); do
      curl -sS -X POST "${URLS[$((d-1))]}/api/session/disarm" \
        -H "X-CSRF-Token: ${CSRFS[$((d-1))]}" -o /dev/null || true
    done

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
    # **GREEN since P07.S05e (v1.117.207), and this is the criterion it took four phases to reach.**
    #
    # P03's exit criterion is *"a LAN ceremony completes with NO outbound internet traffic"*, and it
    # was false for every ceremony carrying an invitation — which is every ceremony P07 builds. It
    # survived four phases because the only `--lan` run was the TWO-PARTY one, which has no
    # invitation and therefore no `cer` at all: the run that existed to prove the criterion was the
    # one shape that could not reach the defect. `--lan -n N` was impossible until S05c removed
    # three separate barriers to it.
    #
    # The arithmetic it took, in order, because each step looked sufficient and was not:
    #   120 packets  four parties, nothing suppressing the bootstrap at all
    #     9 packets  S05d: bootstrap lazy behind one door, fetch windowed, dial side holding on
    #                its own browse result rather than a two-second timer
    #     0 packets  S05e: the ARM holding on evidence — every sighting of its own expected peer
    #                renews the hold, and an arm that hears nothing pays one budget and publishes
    #
    # **A regression here is most likely a hold that stopped renewing**, not a new eager caller:
    # the bootstrap has an AST guard asserting it has exactly one door, and the arm's half does
    # not. Probe per instance before assuming otherwise — a stack trace on `ensureBootstrapped` is
    # what found the arm in the first place, and `link_report` above says what each end hears.
    # **The reading is taken AFTER `decline_round`, not here (P08.S05h).** The end-state round is a
    # delivery round with a different payload — the same arms, the same rendezvous, the same
    # publish — and it ran entirely OUTSIDE this window, so its egress was measured by nothing at
    # all. Deferring the reading is what puts it inside; see the assertion below the decline.
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

  decline_round

  # ── P03's exit criterion, over EVERYTHING this run emitted ───────────────────
  #
  # Read here rather than after the relays, so the delivery rounds AND the end-state round are
  # both inside the measured window. Before S05h the reading was taken before `decline_round` ran,
  # so a whole round — arms, rendezvous, publish and all — emitted into a window nobody read.
  if [ "$LAN" = "1" ]; then
    after="$(offlink_packets)"
    [ "$after" = "$baseline" ] \
      || fail "a $N-party LAN run emitted $((after - baseline)) packets destined off the link — P03's exit criterion says a LAN ceremony completes with NO outbound internet traffic, and this went to ZERO at P07.S05e over 16 hops and two transports. This is a REGRESSION, not a known remainder. Probe per instance: a stack trace on ensureBootstrapped names the caller, and link_report says what each end hears."
    echo "[lan] $(( (N - 1) * 2 )) hops, FOUR delivery rounds (each relay runs one and re-runs it after the injected failure) and an end-state round over two transports, and nothing left the link"
  fi

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
