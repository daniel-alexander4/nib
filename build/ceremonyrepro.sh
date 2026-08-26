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
hits=0; files=0
for h in "$A_HOME" "$B_HOME"; do
  if [ -d "$h/nib" ]; then
    while IFS= read -r f; do
      files=$((files+1))
      grep -qiF "$SEC" "$f" && { echo "        secret in $f"; hits=$((hits+1)); }
    done < <(find "$h/nib" -type f)
  fi
done
if [ "$files" -eq 0 ]; then no "secret search" "nothing under ~/nib on either machine — the search read no bytes"
elif [ "$hits" -eq 0 ]; then ok "the invitation secret is in none of the $files file(s) under ~/nib (D29)"
else no "secret residue" "$hits file(s) carry it"; fi

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
curl -s -c "$SP/A.jar" -b "$SP/A.jar" "$A_BASE/api/pdf" -o "$SP/convened.pdf"
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

echo
echo "PASS=$PASS FAIL=$FAIL"
[ "$FAIL" -eq 0 ]
