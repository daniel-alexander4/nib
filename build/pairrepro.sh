#!/usr/bin/env bash
# Tier 4 of Nib's test harness: run TWO real nib binaries against each other and
# complete a ceremony between them — once per transport, TCP and QUIC (D14).
#
# Usage: ./build/pairrepro.sh [--keep]
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

for dep in go curl python3; do
  command -v "$dep" >/dev/null 2>&1 || {
    echo "$dep not installed; skipping the two-instance ceremony tests"; exit 0; }
done

KEEP=0
[ "${1:-}" = "--keep" ] && KEEP=1

PORT_A="${NIB_PAIR_PORT_A:-18541}"
PORT_B="${NIB_PAIR_PORT_B:-18542}"
A="http://127.0.0.1:$PORT_A"
B="http://127.0.0.1:$PORT_B"
SESSION_PORT="${NIB_PAIR_SESSION_PORT:-18543}"
# A second port for the QUIC run. Distinct rather than reused because the two
# ceremonies run back to back and a listener's teardown is not instantaneous —
# a reused port would make a bind race look like a transport failure.
SESSION_PORT_QUIC="${NIB_PAIR_SESSION_PORT_QUIC:-18544}"
WORK="$(mktemp -d)"
PID_A=""; PID_B=""

cleanup() {
  if [ "$KEEP" = "1" ]; then
    echo "--keep: A at $A (pid $PID_A), B at $B (pid $PID_B), work dir $WORK"
    return
  fi
  [ -n "$PID_A" ] && kill "$PID_A" >/dev/null 2>&1
  [ -n "$PID_B" ] && kill "$PID_B" >/dev/null 2>&1
  rm -rf "$WORK"
}
trap cleanup EXIT INT TERM

fail() { echo "FAIL: $*" >&2; exit 1; }

# Refuse ports someone else holds, BEFORE building — the same lesson tier 3
# learned: a leftover --keep run makes the failure surface four steps later and
# blames the wrong thing.
for p in "$PORT_A" "$PORT_B"; do
  if curl -fsS -o /dev/null --max-time 2 "http://127.0.0.1:$p/api/status" 2>/dev/null; then
    fail "something is already serving 127.0.0.1:$p — a leftover --keep run? Stop it, or set NIB_PAIR_PORT_A/B."
  fi
done

echo "building nib…"
go build -o "$WORK/nib" ./cmd/nib || fail "could not build nib"

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

PID_A="$(start A "$PORT_A" "$WORK/a")"
PID_B="$(start B "$PORT_B" "$WORK/b")"
wait_up "$A" || { cat "$WORK/a/nib.log" >&2; fail "instance A did not come up"; }
wait_up "$B" || { cat "$WORK/b/nib.log" >&2; fail "instance B did not come up"; }

# Each instance enrols its OWN key under its OWN home, which is what gives them
# two different identities. A shared vault would make every assertion below
# vacuous — the two "peers" would be one key agreeing with itself.
for pair in "$A:$WORK/a" "$B:$WORK/b"; do
  url="${pair%%:/*}"; url="${pair%:*}"; home="${pair##*:}"
  curl -fsS -X POST "$url/api/ssh/enroll" -H 'Content-Type: application/json' \
    -d "{\"mode\":\"create\",\"keyPath\":\"$home/home/.ssh/id_ed25519\"}" >/dev/null \
    || fail "could not enrol a key on $url"
done

csrf() { curl -fsS "$1/api/status" | jget csrf; }
CSRF_A="$(csrf "$A")"; CSRF_B="$(csrf "$B")"
[ -n "$CSRF_A" ] && [ -n "$CSRF_B" ] || fail "no CSRF token from one of the instances"

FP_A="$(curl -fsS "$A/api/peers" | jget fingerprint)"
FP_B="$(curl -fsS "$B/api/peers" | jget fingerprint)"
NAME_A="$(curl -fsS "$A/api/peers" | jget name)"
[ "${#FP_A}" = 64 ] && [ "${#FP_B}" = 64 ] || fail "one instance has no identity fingerprint"

# THE assertion that makes this a two-instance harness rather than one process
# talking to itself.
#
# **Defence in depth, and the probe said so.** Trying to make the two share a
# vault does not reach here: the second instance's enrolment returns 409 first,
# because a vault already exists. So this line guards a state the enrolment guard
# already prevents — kept because it is the assertion a future refactor of either
# guard would need, and because it fires correctly when the two fingerprints are
# equal (probed directly, 2026-08-19).
[ "$FP_A" != "$FP_B" ] || fail "both instances have the SAME identity ($FP_A) — they are sharing a vault, and every assertion below would be one key agreeing with itself"
# The six-word name came along with it (P01.S02), so this tier sees it too.
[ "$(echo "$NAME_A" | wc -w)" = 6 ] || fail "instance A reports the name '$NAME_A', which is not six words"

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
ceremony() { # transport port outfile
  local transport="$1" port="$2" out="$3"

  # A pristine document each time: a ceremony over the previous ceremony's output
  # would carry three signatures and the count below would assert the wrong thing.
  curl -fsS -X POST "$A/api/open" -H 'Content-Type: application/json' -H "X-CSRF-Token: $CSRF_A" \
    -d "{\"path\":\"$WORK/doc.pdf\"}" >/dev/null || fail "[$transport] A could not open the document"

  # B arms a receive session for co-signing, on this transport.
  curl -fsS -X POST "$B/api/session/arm" -H 'Content-Type: application/json' -H "X-CSRF-Token: $CSRF_B" \
    -d "{\"fingerprint\":\"$FP_A\",\"bind\":\"127.0.0.1:$port\",\"mode\":\"cosign\",\"transport\":\"$transport\"}" >/dev/null \
    || fail "[$transport] B could not arm a session"

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
  if (exec 3<>"/dev/tcp/127.0.0.1/$port") 2>/dev/null; then
    exec 3<&- 3>&-
    [ "$transport" = "tcp" ] || fail "[$transport] port $port answers TCP — the QUIC run is listening on a TCP socket, so it is the TCP path wearing a different label"
  else
    [ "$transport" != "tcp" ] || fail "[$transport] port $port does not answer TCP, so the TCP run is not on the transport it asked for"
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
    -F "pdf=@$WORK/doc.pdf" -F "appearance=@$WORK/sig.png" \
    -F "params={\"fingerprint\":\"$FP_B\",\"intent\":\"I agree to co-sign\"}" \
    -F "address=127.0.0.1:$port" -F "transport=$transport" \
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
  [ -n "$words_a" ] || fail "[$transport] instance A was never shown the verification words — the ceremony reached the document exchange without the spoken check (L2)"
  [ -n "$words_b" ] || fail "[$transport] instance B was never shown the verification words"
  [ "$(echo "$words_a" | wc -w)" = 4 ] || fail "[$transport] A's verification string is not four words: $words_a"
  # The property only two instances can see.
  [ "$words_a" = "$words_b" ] || fail "[$transport] the two instances derived DIFFERENT verification words — A saw '$words_a' and B saw '$words_b'. Two people comparing these would read a mismatch as an attack."

  [ "$code" = "200" ] || { cat "$init_out" >&2; cat "$WORK/b/nib.log" >&2; fail "[$transport] initiate returned HTTP $code"; }

  # The ceremony completed only if the document carries BOTH signatures — read
  # from B's side, because asking A whether A signed is asking the thing under test.
  curl -fsS "$B/api/pdf" -o "$out" || fail "[$transport] could not fetch B's document"
  python3 - "$out" "$transport" <<'PYSIG' || exit 1
import sys
b = open(sys.argv[1], "rb").read()
n = b.count(b"/ByteRange")
if n < 2:
    print(f"FAIL: [{sys.argv[2]}] the co-signed document carries {n} signature byte-ranges, "
          f"want 2 — the ceremony did not produce a document signed by both parties",
          file=sys.stderr)
    sys.exit(1)
print(f"[{sys.argv[2]}] the finished document carries {n} signatures")
PYSIG
  WORDS="$words_a"
}

ceremony tcp "$SESSION_PORT" "$WORK/final.tcp.pdf"
WORDS_TCP="$WORDS"
ceremony quic "$SESSION_PORT_QUIC" "$WORK/final.quic.pdf"
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

echo "PASS: a ceremony completed between two instances over BOTH transports"
echo "      tcp:  $WORDS_TCP"
echo "      quic: $WORDS_QUIC"
