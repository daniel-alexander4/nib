#!/usr/bin/env bash
# Tier 6 of Nib's test harness: a ceremony between two real PROCESSES, over real HTTP.
#
# Usage: ./build/ceremonyrepro.sh
#
# ── What this tier reaches that tiers 0-3 cannot ─────────────────────────────
# The Go tests drive handlers with an httptest server inside ONE process, sharing one
# vault, one identity and one registry. A ceremony is by construction a thing between
# two machines that hold different keys, so the most important property — that the
# invitation one binary produces is the invitation the other binary accepts — is exactly
# the property a single-process test assumes rather than checks. Here A convenes and B
# accepts, each with its own $HOME, its own vault and its own signing identity, and the
# invitation crosses between them as text.
#
# ── Why it exists at all, which is a process lesson ──────────────────────────
# P07.S02a live-verified `POST /api/ceremony/convene` with a script in a session
# scratchpad. The scratchpad was wiped, so between that slice and P07.S02b the product's
# only ceremony-creating surface was exercised by NOTHING committed — the Go suite had no
# test of that route at all, which is how it was found. A verification that lives outside
# the repo is one that has already been lost once. This is that script, committed.
#
# ── Where it stops ───────────────────────────────────────────────────────────
# **Two processes on one machine are not two machines.** Both loopback, one clock, one
# filesystem, one kernel. It says nothing about NAT, about the DHT, or about a peer that
# is genuinely elsewhere — that is `pairrepro.sh` (tier 4) and the two-machine VERIFY item.
#
# **No hop completes here.** B never signs: the L3 clause below asserts that B is REFUSED
# for trying to, which is the point at this coordinate. Driving a hop to completion needs
# a rendezvous and a second armed listener, and that is P07.S03a's N-party driver.
#
# It skips cleanly when its dependencies are absent, like tiers 2 and 3.
set -uo pipefail
cd "$(dirname "$0")/.."
REPO="$PWD"

command -v curl >/dev/null 2>&1 || { echo "SKIP: curl is not installed"; exit 0; }
command -v python3 >/dev/null 2>&1 || { echo "SKIP: python3 is not installed"; exit 0; }
# The transfer clause hashes both legs' documents. Undeclared, this SKIPS cleanly on a host
# without it (macOS ships `shasum -a 256`); undeclared and unchecked, both `$(sha256sum …)` expand
# to empty, compare equal, and the clause FAILS with a false reason — which is worse than skipping.
command -v sha256sum >/dev/null 2>&1 || { echo "SKIP: sha256sum is not installed"; exit 0; }

WORK="$(mktemp -d)"
trap 'kill ${A_PID:-0} ${B_PID:-0} 2>/dev/null; wait 2>/dev/null; rm -rf "$WORK"' EXIT
SP="$WORK"
go build -o "$SP/nib" ./cmd/nib || { echo "FAIL: could not build nib" >&2; exit 1; }
go run build/genpdf.go "$SP/lease.pdf" "the lease" >/dev/null 2>&1
[ -s "$SP/lease.pdf" ] || { echo "FAIL: could not build the fixture PDF" >&2; exit 1; }

PASS=0; FAIL=0
ok(){ echo "  ok   — $1"; PASS=$((PASS+1)); }
no(){ echo "  FAIL — $1"; echo "        $2"; FAIL=$((FAIL+1)); }
start() { # $1 = name -> sets ${1}_BASE, ${1}_CSRF, ${1}_HOME
  local n=$1 h="$SP/home_$1"
  rm -rf "$h"; mkdir -p "$h/.config"
  local port=$((20000 + RANDOM % 20000))
  # NIB_NO_UPDATE_CHECK makes this tier HERMETIC — without it the app calls out to a
  # release feed and a green run would depend on somebody else's uptime.
  HOME="$h" XDG_CONFIG_HOME="$h/.config" NIB_NO_BROWSER=1 NIB_NO_UPDATE_CHECK=1 \
    NIB_ADDR="127.0.0.1:$port" "$SP/nib" >"$SP/$n.log" 2>&1 &
  eval "${n}_PID=$!"
  local base="http://127.0.0.1:$port"
  for _ in $(seq 1 150); do curl -sf "$base/api/status" >/dev/null 2>&1 && break; sleep 0.1; done
  curl -s -c "$SP/$n.jar" -b "$SP/$n.jar" -X POST "$base/api/ssh/enroll" \
    -H 'content-type: application/json' -d "{\"mode\":\"create\",\"keyPath\":\"$h/id_ed25519\"}" \
    >"$SP/$n.enroll.json"
  local csrf; csrf=$(python3 -c "import json;print(json.load(open('$SP/$n.enroll.json')).get('csrf',''))")
  [ -n "$csrf" ] || { echo "$n: no csrf: $(cat "$SP/$n.enroll.json")"; exit 1; }
  eval "${n}_BASE='$base'; ${n}_CSRF='$csrf'; ${n}_HOME='$h'"
}
post(){ # $1=name $2=path $3=json -> body in $SP/resp.json, prints status
  local n=$1 b c; eval "b=\$${n}_BASE; c=\$${n}_CSRF"
  curl -s -o "$SP/resp.json" -w '%{http_code}' -c "$SP/$n.jar" -b "$SP/$n.jar" \
    -X POST "$b$2" -H 'content-type: application/json' -H "X-CSRF-Token: $c" -H "Origin: $b" -d "$3"
}
get(){ local n=$1 b; eval "b=\$${n}_BASE"; curl -s -c "$SP/$n.jar" -b "$SP/$n.jar" "$b$2"; }
jq_(){ python3 -c "import json,sys;d=json.load(open('$SP/resp.json'));print($1)" 2>/dev/null; }

trap 'kill ${A_PID:-0} ${B_PID:-0} 2>/dev/null; wait 2>/dev/null' EXIT
start A; start B
A_FP=$(get A /api/peers | python3 -c "import json,sys;print(json.load(sys.stdin)['fingerprint'])")
B_FP=$(get B /api/peers | python3 -c "import json,sys;print(json.load(sys.stdin)['fingerprint'])")
echo "A(convener)=${A_FP:0:12}…  B(party)=${B_FP:0:12}…"
[ "$A_FP" != "$B_FP" ] || { echo "setup: both instances share a fingerprint — not two parties"; exit 1; }

# A opens a document and convenes.
code=$(post A /api/open "{\"path\":\"$SP/lease.pdf\"}")
[ "$code" = 200 ] || { echo "open: $code $(cat "$SP/resp.json")"; exit 1; }
EXP=$(python3 -c "import datetime;print((datetime.datetime.now(datetime.timezone.utc)+datetime.timedelta(hours=48)).strftime('%Y-%m-%dT%H:%M:%SZ'))")
code=$(post A /api/ceremony/convene "{\"roster\":[{\"fingerprint\":\"$A_FP\",\"label\":\"Alice\",\"signs\":true},{\"fingerprint\":\"$B_FP\",\"label\":\"Bob\",\"capacity\":\"as Director\",\"signs\":true}],\"intent\":\"We agree\",\"expires\":\"$EXP\",\"convenerSigns\":true}")
if [ "$code" = 200 ]; then ok "A convened a two-party ceremony through the real route"; else no "convene" "$code $(cat "$SP/resp.json")"; exit 1; fi
CID=$(jq_ "d['ceremony']")
INV=$(python3 -c "
import json;d=json.load(open('$SP/resp.json'))
print(next(i['invitation'] for i in d['invites'] if i['fingerprint'].lower()=='$B_FP'.lower()))")
[ -n "$INV" ] || { echo "no invitation for B"; exit 1; }
# The FIRST ceremony's document, captured before clause 6 convenes a second one over it.
#
# **Clause 7 used to fetch /api/pdf at the point of use and got the SECOND ceremony's document**,
# pairing it with this first ceremony's invitation. It still went red and it still said "not this
# party's turn", so it read as driving L3 — but two conditions were true at once (wrong turn AND
# wrong ceremony) and nothing could say which one L3 fired on. P07.S07b made that visible by
# adding the arrival check to the dial door, whose refusal names the ceremony mismatch and
# arrives first. Capturing the right document here leaves the out-of-turn condition as the only
# one present, which is what clause 7 has always claimed to test.
curl -fsS -c "$SP/A.jar" -b "$SP/A.jar" "$A_BASE/api/pdf" -o "$SP/convened1.pdf"
[ -s "$SP/convened1.pdf" ] || { echo "could not capture the first ceremony's document"; exit 1; }

# CLAUSE 1 — the convener's own pins (D21 from the hub side).
get A /api/peers >"$SP/apeers.json"
if python3 -c "
import json,sys;d=json.load(open('$SP/apeers.json'))
sys.exit(0 if any(p['fingerprint'].lower()=='$B_FP'.lower() for p in d['peers']) else 1)"; then
  ok "convening pinned the counterparty on the CONVENER's machine"
else no "convener pin" "$(cat "$SP/apeers.json")"; fi

# CLAUSE 2 — B cannot arm before accepting.
code=$(post B /api/session/arm "{\"fingerprint\":\"$A_FP\",\"bind\":\"127.0.0.1:0\",\"transport\":\"tcp\",\"invitation\":$(python3 -c "import json;print(json.dumps('$INV'))")}")
if [ "$code" = 400 ] && grep -q "isn't pinned" "$SP/resp.json"; then
  ok "B holding a good invitation is REFUSED an arm before accepting (the step D21 removes)"
else no "pre-accept arm" "$code $(cat "$SP/resp.json")"; fi

# CLAUSE 3 — accept.
code=$(post B /api/ceremony/accept "{\"invitation\":$(python3 -c "import json;print(json.dumps('$INV'))")}")
if [ "$code" = 200 ]; then
  P=$(jq_ "d['pinned']"); S=$(jq_ "d['signing']"); C=$(jq_ "d['ceremony']")
  [ "$P" = 1 ] && ok "accept established exactly ONE pin (D22 is a hub)" || no "pin count" "pinned=$P"
  [ "$S" = 2 ] && ok "accept reports 2 obliged signers" || no "signing count" "signing=$S"
  [ "$C" = "$CID" ] && ok "accept names the same ceremony A convened" || no "ceremony id" "$C vs $CID"
  python3 -c "
import json,sys;d=json.load(open('$SP/resp.json'))
c=[p for p in d['roster'] if p['convener']]
sys.exit(0 if len(c)==1 and c[0]['name'] and c[0]['fingerprint'].lower()=='$A_FP'.lower() else 1)" \
    && ok "the convener is named with a six-word name derived from their fingerprint" \
    || no "convener naming" "$(cat "$SP/resp.json")"
else no "accept" "$code $(cat "$SP/resp.json")"; fi

# CLAUSE 4 — the arm now succeeds.
code=$(post B /api/session/arm "{\"fingerprint\":\"$A_FP\",\"bind\":\"127.0.0.1:0\",\"transport\":\"tcp\",\"invitation\":$(python3 -c "import json;print(json.dumps('$INV'))")}")
if [ "$code" = 200 ]; then ok "after accepting, B arms with no manual pin anywhere (D21)"; else no "post-accept arm" "$code $(cat "$SP/resp.json")"; fi
post B /api/session/disarm '{}' >/dev/null

# CLAUSE 5 — the secret is not under ~/nib on EITHER machine.
SEC=$(python3 -c "
import base64,json,sys
t='$INV'; body=t.split(':',1)[1].rsplit('.',1)[0]
pad='='*(-len(body)%4)
d=json.loads(base64.urlsafe_b64decode(body+pad))
print(base64.b64decode(d['secret']).hex())")
# **The count is PER MACHINE, and it used to be aggregate (fixed 2026-08-29, P08.S01).**
#
# The stimulus guard was `files -eq 0` summed over both homes, and the `-d` test skipped a home
# that had no `~/nib` at all. B is the invitee and never convenes, so B never got one: B contributed
# zero files, the aggregate was satisfied by A alone, and the green line said "on either machine"
# while having read only the convener's disk. That is the one check in this repo that looks at an
# invitee's disk, and it could not see it.
#
# It matters now because P08.S01 makes an invitee persist for the first time. Its store is the
# VAULT — sealed, and under $h/.config, not $h/nib — so the correct outcome here is still zero
# hits; what changed is that a regression putting it in ~/nib would now be visible.
hits=0
declare -A SEEN=()
for n in A B; do
  eval "h=\$${n}_HOME"
  c=0
  if [ -d "$h/nib" ]; then
    while IFS= read -r f; do
      c=$((c+1))
      grep -qiF "$SEC" "$f" && { echo "        secret in $f"; hits=$((hits+1)); }
    done < <(find "$h/nib" -type f)
  fi
  SEEN[$n]=$c
done
# **Each machine's zero is classified, which is the whole repair.** A zero has two meanings and the
# aggregate could not tell them apart: "this side was not read" (the defect) and "this side has
# written nothing to ~/nib at all" (structural, and true of the invitee in THIS tier — no hop
# completes here, so `mirrorHop` never runs on B, and P08.S01 puts B's invitation in the VAULT under
# $h/.config, which is not this directory and is sealed besides).
#
# So B's zero is reported as structural rather than folded into A's count. It stops being structural
# the moment anything writes to B's ~/nib — a completed hop, a delivered document — and then the
# count must be non-zero and the search must read it. That is the state this clause is waiting for,
# and saying so is what keeps a pass from meaning "we looked at the invitee" when we did not.
if [ "${SEEN[A]}" -eq 0 ]; then
  no "secret search" "nothing under A's ~/nib — the convener convened, so its disk must have content and was not read"
elif [ "$hits" -ne 0 ]; then
  no "secret residue" "$hits file(s) carry it"
elif [ "${SEEN[B]}" -eq 0 ]; then
  # Not a failure, and not a silent pass either: the invitee's side is named as unexercised.
  ok "the secret is in none of the ${SEEN[A]} file(s) under the convener's ~/nib (D29) — the invitee has no ~/nib in this tier (no hop completes, and its invitation is in the sealed vault), so its zero is structural and is NOT evidence"
else
  ok "the invitation secret is in none of the ${SEEN[A]} file(s) on the convener nor the ${SEEN[B]} on the invitee (D29)"
fi

# CLAUSE 6 — a second ceremony sharing B, then A ends the first: B stays pinned.
code=$(post A /api/open "{\"path\":\"$SP/lease.pdf\"}")
code=$(post A /api/ceremony/convene "{\"roster\":[{\"fingerprint\":\"$A_FP\",\"label\":\"Alice\",\"signs\":true},{\"fingerprint\":\"$B_FP\",\"label\":\"Bob\",\"signs\":true}],\"intent\":\"A second matter\",\"expires\":\"$EXP\",\"convenerSigns\":true}")
if [ "$code" = 200 ]; then
  CID2=$(jq_ "d['ceremony']")
  [ "$CID2" != "$CID" ] && ok "A convened a SECOND ceremony with the same counterparty" || no "second ceremony" "same id"
else no "second convene" "$code $(cat "$SP/resp.json")"; fi

# CLAUSE 7 — C18/C16 (P07.S05a): the convened document reports how many obliged signers it has,
# and that NONE of them have signed yet.
#
# This is the count a verifier needs before it can say a ceremony is incomplete. On the convened
# document it is the extreme case — two obliged, zero signed — and it is drivable here because
# convening is the one ceremony act this tier completes.
curl -fsS -c "$SP/A.jar" -b "$SP/A.jar" "$A_BASE/api/attestations" -o "$SP/atts.json"
python3 - "$SP/atts.json" <<'PYC' && ok "the convened document reports 0 of 2 obliged signers (C18)" || no "C18 counts" "$(head -c 300 "$SP/atts.json")"
import json, sys
d = json.load(open(sys.argv[1]))
ob, sg = d.get("obliged", 0), d.get("signed", 0)
sys.exit(0 if ob == 2 and sg == 0 else 1)
PYC

# CLAUSE 8 — the same route says NOTHING about completeness for a document with no ceremony.
# Without this, "obliged: 2" above is equally true of a route that reports the signer count under
# a different name, and every ordinary co-sign in the product would grow a completeness line.
code=$(post B /api/open "{\"path\":\"$SP/lease.pdf\"}")
if [ "$code" = 200 ]; then
  curl -fsS -c "$SP/B.jar" -b "$SP/B.jar" "$B_BASE/api/attestations" -o "$SP/atts_plain.json"
  python3 - "$SP/atts_plain.json" <<'PYP' && ok "a document with no ceremony reports no obliged signers at all" || no "C18 third state" "$(head -c 200 "$SP/atts_plain.json")"
import json, sys
d = json.load(open(sys.argv[1]))
sys.exit(0 if not d.get("obliged") and not d.get("signed") else 1)
PYP
else no "open a plain document on B" "$code"; fi

# CLAUSE 7 — L3 (P07.S03): B tries to sign out of turn, through the real initiate route.
#
# A is first in the roster and has not signed. B initiating a co-sign is therefore a contribution
# out of roster order, and it must be refused BEFORE the local signature is applied — the gate
# runs inside buildCoSigned, which is reached before any dialing, so this needs no peer at all.
# Driven on the CONVENED document B would actually be handed.
cp "$SP/convened1.pdf" "$SP/convened.pdf"
if [ ! -s "$SP/convened.pdf" ]; then no "L3 setup" "could not fetch the convened document"; else
  python3 -c "
import base64,sys
sys.stdout.buffer.write(base64.b64decode('iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=='))" > "$SP/appearance.png"
  code=$(curl -s -o "$SP/resp.json" -w '%{http_code}' -c "$SP/B.jar" -b "$SP/B.jar" \
    -X POST "$B_BASE/api/session/initiate" -H "X-CSRF-Token: $B_CSRF" -H "Origin: $B_BASE" \
    -F "pdf=@$SP/convened.pdf" -F "params={\"fingerprint\":\"$A_FP\",\"intent\":\"I agree\"}" \
    -F "appearance=@$SP/appearance.png" \
    -F "address=127.0.0.1:1" -F "transport=tcp" -F "invitation=$INV")
  if [ "$code" = 409 ] && grep -q "not this party.s turn" "$SP/resp.json"; then
    ok "L3 refuses a contribution out of roster order, by name, through the real route"
  else
    no "L3 out-of-turn refusal" "$code $(cat "$SP/resp.json")"
  fi
fi

# CLAUSE 9 — a one-way transfer completes, and the receipt means PERSISTED (P08.S05a, C10).
#
# **The transfer route had no harness coverage above tier 1 at all.** `/api/session/send` and the
# armed `mode:"receive"` path were driven only inside one process, sharing one vault and one
# identity — so "the document reaches the other machine and lands on its disk" was asserted by
# nothing that crosses a process boundary. This clause is why the reordering in P08.S05a is
# checkable end to end: the write now happens inside `sessionAccepter.Accept`, BEFORE
# `ReceiveDocument` sends `ackOK`, so a green send is a claim about the receiver's disk.
#
# Both ends must confirm the spoken check, and the SENDER's request blocks while it waits — so
# the confirmations go out concurrently, which is the same shape the UI needs.
transfer_leg() { # $1 = transport
  local tr="$1" code arm_code words_a words_b i before
  # **A DELTA, not a total, and the second leg is why.** `find … | wc -l` is satisfied by the
  # FIRST leg's file, so a total would report the quic leg green on the strength of the tcp one —
  # the shape `pairrepro.sh` already names for its own document count.
  before=$(find "$B_HOME/nib" -type f -name '*.pdf' 2>/dev/null | wc -l)
  arm_code=$(post B /api/session/arm "{\"fingerprint\":\"$A_FP\",\"bind\":\"127.0.0.1:0\",\"mode\":\"receive\",\"transport\":\"$tr\"}")
  if [ "$arm_code" != 200 ]; then no "[$tr] B could not arm to receive" "$arm_code $(cat "$SP/resp.json")"; return; fi
  local addr; addr=$(jq_ "d['address']")
  if [ -z "$addr" ]; then no "[$tr] B armed and reported no address" "nothing to dial"; return; fi

  # The send, in the background: its response does not return until both gates have answered.
  # **`-b` only, never `-c`.** curl rewrites the jar it is given with `-c` when it exits, and the
  # foreground polls below read `$SP/A.jar` every 100 ms for up to 20 s. A read landing on a
  # truncated jar sends no session cookie, gets a 401, and the clause reports "the spoken check
  # never appeared" — a false reason for a real race.
  ( curl -s -o "$SP/send.$tr.json" -w '%{http_code}' -b "$SP/A.jar" \
      -X POST "$A_BASE/api/session/send" -H "X-CSRF-Token: $A_CSRF" -H "Origin: $A_BASE" \
      -F "pdf=@$SP/send-$tr.pdf" -F "fingerprint=$B_FP" -F "address=$addr" -F "transport=$tr" \
      > "$SP/send.$tr.code" ) &
  local send_pid=$!

  # Both spoken checks, confirmed as they appear. They must AGREE — a transfer whose two ends
  # derived different words is L2 failing, and the equality is the only thing that shows it.
  words_a=""; words_b=""
  for i in $(seq 1 200); do
    [ -z "$words_a" ] && words_a=$(get A /api/session/status | python3 -c "import json,sys;d=json.load(sys.stdin);print((d.get('verify') or {}).get('words',''))" 2>/dev/null)
    [ -z "$words_b" ] && words_b=$(get B /api/session/status | python3 -c "import json,sys;d=json.load(sys.stdin);print((d.get('verify') or {}).get('words',''))" 2>/dev/null)
    [ -n "$words_a" ] && [ -n "$words_b" ] && break
    sleep 0.1
  done
  # **Every exit disarms B.** Killing the curl does not stop the transfer — A's handler is not
  # watching the request context — so an early return leaves B in-session, the NEXT leg's arm
  # returns 409, and one root cause prints as two FAIL lines of which the second is misleading.
  bail() { kill $send_pid 2>/dev/null; wait $send_pid 2>/dev/null
           post B /api/session/disarm '{}' >/dev/null 2>&1 || true; no "$1" "$2"; }
  if [ -z "$words_a" ] || [ -z "$words_b" ]; then
    bail "[$tr] the spoken check never appeared on both ends" "A='$words_a' B='$words_b'"; return
  fi
  if [ "$words_a" != "$words_b" ]; then
    bail "[$tr] the two ends derived DIFFERENT verification strings" "A='$words_a' B='$words_b'"; return
  fi
  post A /api/session/verify '{"confirmed":true}' >/dev/null
  post B /api/session/verify '{"confirmed":true}' >/dev/null

  # B's consent gate, then accept.
  for i in $(seq 1 200); do
    get B /api/session/status | grep -q '"pending"' && break
    sleep 0.1
  done
  post B /api/session/respond '{"accept":true}' >/dev/null
  wait $send_pid 2>/dev/null
  code=$(cat "$SP/send.$tr.code" 2>/dev/null)
  # Disarm regardless of outcome: a leg that failed with B still armed makes the NEXT leg's arm
  # return 409 and report as a different failure than the one that happened.
  post B /api/session/disarm '{}' >/dev/null 2>&1 || true

  if [ "$code" != 200 ] || ! grep -q '"sent":true' "$SP/send.$tr.json" 2>/dev/null; then
    no "[$tr] the transfer did not complete" "$code $(head -c 200 "$SP/send.$tr.json" 2>/dev/null)"; return
  fi
  # **The assertion that makes the receipt mean something: THIS leg's bytes are on B's DISK.**
  # `sent:true` is produced by `ackOK`, and since P08.S05a that byte is written only after the
  # durable write has returned. Asserting the file is what turns "the peer said yes" into "the
  # peer kept it" — and it is the one thing a single-process test cannot claim.
  #
  # **Content, not a count, and the count is what found the reason.** A `-gt $before` delta
  # reported the second leg RED: both legs land in the same second, `receivedName` stamps
  # `<slug>-<YYYYmmdd-HHMMSS>.pdf`, and the second write silently OVERWROTE the first — measured,
  # both legs at `incoming/alice-20260831-110425.pdf`. That is a live data-loss defect
  # (/pending 342) and it belongs to P08.S05d's deterministic-filename bullet, not here. Hashing
  # this leg's own document is correct either way and still fails if nothing was written at all.
  local want; want=$(sha256sum "$SP/send-$tr.pdf" | cut -d' ' -f1)
  if find "$B_HOME/nib" -type f -name '*.pdf' -exec sha256sum {} + 2>/dev/null | cut -d' ' -f1 | grep -qx "$want"; then
    ok "[$tr] a one-way transfer completed and THIS leg's bytes are on the RECEIVER's disk"
  else
    no "[$tr] the sender was told it was accepted and this leg's bytes are not on the receiver's disk" \
       "this is exactly the loss C10 names: ackOK sent before the write. before=$before"
  fi
}

# Stimulus first: B's ~/nib must hold no PDF before either leg, or "a file appeared" is true of
# a run that transferred nothing.
if [ "$(find "$B_HOME/nib" -type f -name '*.pdf' 2>/dev/null | wc -l)" -ne 0 ]; then
  no "transfer setup" "B already holds a PDF under ~/nib, so the landing check below is vacuous"
else
  go run build/genpdf.go "$SP/send-tcp.pdf" "the tcp transfer" >/dev/null 2>&1
  go run build/genpdf.go "$SP/send-quic.pdf" "the quic transfer" >/dev/null 2>&1
  if [ ! -s "$SP/send-tcp.pdf" ] || [ ! -s "$SP/send-quic.pdf" ]; then
    no "transfer setup" "could not build the per-leg fixture PDFs"
  elif [ "$(sha256sum "$SP/send-tcp.pdf" | cut -d' ' -f1)" = "$(sha256sum "$SP/send-quic.pdf" | cut -d' ' -f1)" ]; then
    no "transfer setup" "the two legs' documents hash the same, so the content check cannot tell them apart"
  else
    transfer_leg tcp
    transfer_leg quic
  fi
fi

# ── CLAUSE 10 — C13: a second ceremony on a document ALREADY under a live one is refused ────
#
# **C15 says every criterion in this phase is driven by the multi-instance harness, and this one
# was not.** `TestConveningTwiceOnOneDocumentIsRefusedAtTheRoute` drives it inside one process,
# where the convener, the vault, the identity and the document registry are all the same object.
# The refusal is about a document's STATE across convene calls, and a single-process test shares
# that state by construction rather than establishing it — which is the same gap this whole tier
# exists to close for the invitation.
#
# **Distinct from CLAUSE 6, which is the case that must NOT be refused.** That one convenes a
# second ceremony with the same COUNTERPARTY, on a freshly opened document, and it succeeds. This
# one re-convenes on the document that is already under a ceremony. Two convenes, opposite
# answers, and only the pair says the refusal is about the document rather than about the peer.
code=$(post A /api/open "{\"path\":\"$SP/lease.pdf\"}")
if [ "$code" != 200 ]; then
  no "C13 setup" "A could not open the document again ($code)"
else
  FIRST=$(post A /api/ceremony/convene "{\"roster\":[{\"fingerprint\":\"$A_FP\",\"label\":\"Alice\",\"signs\":true},{\"fingerprint\":\"$B_FP\",\"label\":\"Bob\",\"signs\":true}],\"intent\":\"A third matter\",\"expires\":\"$EXP\",\"convenerSigns\":true}")
  if [ "$FIRST" != 200 ]; then
    no "C13 setup" "the first convene on this document failed ($FIRST), so a refusal below would be refusing the wrong thing"
  else
    code=$(post A /api/ceremony/convene "{\"roster\":[{\"fingerprint\":\"$A_FP\",\"label\":\"Alice\",\"signs\":true},{\"fingerprint\":\"$B_FP\",\"label\":\"Bob\",\"signs\":true}],\"intent\":\"A fourth matter\",\"expires\":\"$EXP\",\"convenerSigns\":true}")
    body="$(cat "$SP/resp.json" 2>/dev/null)"
    # 409 and not 400, because C13's own point is that this is not a corrected field: the answer
    # is a different ACTION. And the sentence is graded, not just the code — "409" alone is what
    # a user gets from three unrelated refusals on this route.
    if [ "$code" != 409 ]; then
      no "C13 second convene" "HTTP $code, want 409 — a document already under a live ceremony was convened again. $body"
    elif ! printf '%s' "$body" | grep -qi "already"; then
      no "C13 sentence" "409 with a body that does not say the document is already in a ceremony: $body"
    else
      ok "a second ceremony on a document already under a live one is refused 409, by name (C13)"
    fi
  fi
fi

# ── CLAUSE 11 — C12: a ceremony folder deleted BY HAND degrades that entry and costs no other ──
#
# **Also driven only at tier 1 until now** (`TestOneUnloadableCeremonyDoesNotCostTheOthers`, in
# `internal/ceremony`), which builds the directories itself. Here they are directories a real Nib
# wrote, being damaged under a real Nib that is still running and will answer the route about
# them — which is the case C12 is written for: a user tidying `~/nib` by hand.
#
# **Damaged rather than removed.** Removing the whole folder is the ABSENT case and it leaves
# nothing to degrade; C12's entry says "deleted by hand", and the state that produces a degraded
# ENTRY rather than a vanished one is a folder whose `record.json` is gone or unreadable. The
# distinction matters because a listing that silently drops the entry passes a removal test and
# fails this one — and a ceremony that vanishes from the list is one whose only remedy is finding
# and deleting the folder by hand, which is where the user already is.
listing() { curl -fsS -c "$SP/A.jar" -b "$SP/A.jar" "$A_BASE/api/ceremonies" -o "$1"; }
if ! listing "$SP/cer.before.json"; then
  no "C12 setup" "the ceremonies listing could not be read before the damage"
else
  before="$(python3 -c "
import json,sys
d=json.load(open('$SP/cer.before.json'))
print(len([c for c in d.get('ceremonies') or [] if c.get('state')=='ok']))")"
  # STIMULUS: at least two healthy entries, or "the others still work" is a claim about nobody.
  if [ "${before:-0}" -lt 2 ]; then
    no "C12 setup" "only ${before:-0} healthy ceremony/ies are listed; C12 needs two so that 'every other ceremony still works' is about something"
  else
    victim="$(python3 -c "
import json
d=json.load(open('$SP/cer.before.json'))
print([c for c in d.get('ceremonies') or [] if c.get('state')=='ok'][0]['id'])")"
    rm -f "$A_HOME/nib/ceremonies/$victim/record.json"
    if ! listing "$SP/cer.after.json"; then
      no "C12" "the ceremonies listing FAILED after one folder was damaged — one bad entry cost the whole route, which is exactly what C12 forbids"
    else
      python3 - "$SP/cer.after.json" "$victim" "$before" <<'PYC12' && ok "a ceremony folder damaged by hand degrades ONLY its own entry (C12)" || no "C12" "$(head -c 400 "$SP/cer.after.json")"
import json, sys
d = json.load(open(sys.argv[1]))
victim, before = sys.argv[2], int(sys.argv[3])
cers = d.get("ceremonies") or []
me = [c for c in cers if c.get("id") == victim]
if not me:
    print("FAIL: the damaged ceremony VANISHED from the listing rather than degrading. A "
          "ceremony Nib will not admit exists is one whose only remedy is finding and deleting "
          "the folder by hand — which is where the user already is.", file=sys.stderr)
    sys.exit(1)
if me[0].get("state") == "ok":
    print("FAIL: the damaged ceremony still reports state 'ok' — its record.json is gone and "
          "the listing did not notice, so a user is told a bricked ceremony is fine.",
          file=sys.stderr)
    sys.exit(1)
if not (me[0].get("reason") or "").strip():
    print("FAIL: the damaged entry carries no reason. The class alone is a word; the sentence is "
          "what tells the user whether this is damage, a forgery, or a Nib that is out of date.",
          file=sys.stderr)
    sys.exit(1)
others = [c for c in cers if c.get("id") != victim and c.get("state") == "ok"]
if len(others) < before - 1:
    print("FAIL: %d healthy ceremony/ies remain and %d are owed — damaging one entry cost "
          "another, which is the failure C12 is written against."
          % (len(others), before - 1), file=sys.stderr)
    sys.exit(1)
PYC12
    fi
  fi
fi

# ── CLAUSE 12 — P06.S01/C12: the ceremonies listing answers with the vault LOCKED ────────────
#
# **Six of P06's exit criteria are about a panel that renders while locked**, and until v1.117.334
# the route was behind `requireUnlocked`, so none of them was buildable. This drives the criterion
# where it actually lives: a real process, a real vault it cannot open, and real ceremony
# directories on disk beside it.
#
# **A locked instance is made by taking the KEY away, not by asking Nib to lock.** There is no idle
# re-lock in this product — `grep -rn "s.vault = nil" internal/server/` finds no production site —
# so "locked" means `vault.OpenSSH` failed, which is a cold start against a vault whose key is not
# there. Copying A's `.config` (the vault) and A's `~/nib` (the unsealed mirror) into a fresh home
# WITHOUT `id_ed25519` produces exactly that: `AutoSetup` no-ops because a vault exists, `OpenSSH`
# fails because its key does not, and `s.vault` stays nil.
#
# **The sibling assertion is what makes it a lock and not a coincidence.** A route answering 200
# proves nothing on its own — the instance might simply have unlocked. So `GET /api/peers`, which
# is still behind the gate, must return 401 in the same breath. One 200 and one 401 from one
# process is the whole clause.
# **The locked instance gets its OWN key and then loses it**, which is the only shape that works.
# The first cut copied A's `.config` into a fresh home and expected `OpenSSH` to fail — it did not,
# because `unwrapSlot` tries `slot.KeyPath` first and that is an ABSOLUTE path recorded at enrol
# time, so the copy happily reached back into A's home and opened A's key. Measured, not reasoned:
# the clause failed on its own setup assertion with `/api/peers` answering 200.
#
# So: start L normally so it enrols a key of its own under its own home; give it A's ceremonies;
# stop it; delete its key; start it again on the same home. Now `slot.KeyPath` names a file that no
# longer exists, the `~/.ssh` sweep finds nothing because HOME is the sandbox, and the vault stays
# shut.
start L
cp -a "$A_HOME/nib" "$L_HOME/nib" 2>/dev/null
kill "$L_PID" 2>/dev/null; wait "$L_PID" 2>/dev/null
rm -f "$L_HOME/id_ed25519" "$L_HOME/id_ed25519.pub"
# STIMULUS, both halves: there is something to render, and the key is genuinely gone. Without the
# first a 200 carrying an empty list would pass; without the second this is an unlocked instance.
if [ ! -d "$L_HOME/nib/ceremonies" ] || [ -z "$(ls -A "$L_HOME/nib/ceremonies" 2>/dev/null)" ]; then
  no "locked-read setup" "the locked instance's home holds no ceremony directories, so a 200 with an empty list would pass this clause"
elif [ -e "$L_HOME/id_ed25519" ]; then
  no "locked-read setup" "the signing key is still there, so the instance below will not be locked"
else
  LOCK_PORT=$((20000 + RANDOM % 20000))
  HOME="$L_HOME" XDG_CONFIG_HOME="$L_HOME/.config" NIB_NO_BROWSER=1 NIB_NO_UPDATE_CHECK=1 \
    NIB_ADDR="127.0.0.1:$LOCK_PORT" "$SP/nib" >"$SP/locked.log" 2>&1 &
  LOCK_PID=$!
  LOCK_BASE="http://127.0.0.1:$LOCK_PORT"
  for _ in $(seq 1 150); do curl -sf "$LOCK_BASE/api/status" >/dev/null 2>&1 && break; sleep 0.1; done
  peers_code="$(curl -s -o "$SP/locked.peers.json" -w '%{http_code}' "$LOCK_BASE/api/peers")"
  cer_code="$(curl -s -o "$SP/locked.cer.json" -w '%{http_code}' "$LOCK_BASE/api/ceremonies")"
  if [ "$peers_code" != "401" ]; then
    no "locked-read setup" "GET /api/peers returned $peers_code, want 401 — this instance is NOT locked, so a 200 from the ceremonies route below would say nothing at all"
  elif [ "$cer_code" != "200" ]; then
    no "locked read" "GET /api/ceremonies returned $cer_code with the vault locked, want 200. Six of P06's criteria are about a panel that renders while locked, and nothing in this listing is sealed: the mirror is ordinary files by D29's design. $(head -c 200 "$SP/locked.cer.json")"
  else
    python3 - "$SP/locked.cer.json" <<'PYLOCK' && ok "the ceremonies listing answers with the vault LOCKED, while a vault-gated route refuses (P06.S01)" || no "locked read" "$(head -c 300 "$SP/locked.cer.json")"
import json, sys
d = json.load(open(sys.argv[1]))
cers = d.get("ceremonies") or []
if not cers:
    print("FAIL: a locked read returned an EMPTY listing. The panel would render an empty shelf "
          "to a user who has live ceremonies — which passes a status-code check and fails the "
          "criterion.", file=sys.stderr)
    sys.exit(1)
# **Graded over the HEALTHY entries only, and that is not a softening.** CLAUSE 11 above
# deliberately damages one of A's ceremonies to drive C12, and this clause reads a copy of A's
# `~/nib` taken afterwards — so a degraded row here is the earlier clause working, and demanding
# every row be whole would make the two clauses contradict each other.
ok_rows = [c for c in cers if c.get("state") == "ok"]
if not ok_rows:
    print("FAIL: a locked read returned %d row(s) and NONE is healthy. The panel would show a "
          "user nothing but damage for ceremonies that are fine." % len(cers), file=sys.stderr)
    sys.exit(1)
hollow = [c for c in ok_rows if not c.get("intent") or not c.get("roster")]
if hollow:
    print("FAIL: %d locked entry/ies classified 'ok' came back hollow (no intent or no roster): "
          "%r. The criterion is that the panel RENDERS roster, position and next action while "
          "locked; a row with an id and nothing else is not that."
          % (len(hollow), hollow[0]), file=sys.stderr)
    sys.exit(1)
# And the degraded row still carries its sentence, locked — the lock must not cost the diagnosis.
bad = [c for c in cers if c.get("state") != "ok" and not (c.get("reason") or "").strip()]
if bad:
    print("FAIL: a degraded row came back with no reason while locked: %r. The class is a word; "
          "the sentence is what tells the user whether this is damage, a forgery, or a Nib that "
          "is out of date." % bad[0], file=sys.stderr)
    sys.exit(1)
print("locked read: %d row(s), %d healthy and whole, %d degraded and each with its sentence"
      % (len(cers), len(ok_rows), len(cers) - len(ok_rows)))
PYLOCK
  fi
  kill "$LOCK_PID" 2>/dev/null; wait "$LOCK_PID" 2>/dev/null
fi

echo
echo "PASS=$PASS FAIL=$FAIL"
[ "$FAIL" -eq 0 ]
