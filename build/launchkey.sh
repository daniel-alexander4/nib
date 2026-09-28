# launchkey.sh — how a headless Nib is reached by a harness (ADR-053, ADR-054). Sourced, never run.
#
# Every route but three requires the per-process token, and the token is handed out only for a
# launch key. A headless Nib (NIB_NO_BROWSER) opens no window, so it logs the keyed URL for whoever
# started it — `open Nib at http://127.0.0.1:PORT/#k=KEY` (cmd/nib/main.go) — and that log line is
# the only way in. This file is the ONE reader of that line: four harnesses each carried a copy of
# the regex, so a rewording of the log would have broken all four separately (the P08 phase-close
# review, R8-10).

# nib_answers <base> — true once anything answers HTTP at <base>, whatever the status. `/api/status`
# requires the token too (ADR-054), so a 403 from it is how an up, unauthenticated Nib looks, and
# `curl -f` would read it as down.
nib_answers() {
	[ "$(curl -s -o /dev/null -w '%{http_code}' --max-time 2 "$1/api/status" 2>/dev/null)" != 000 ]
}

# launch_key <log> — the most recent launch key in a headless Nib's log. Retried: the server can
# answer a moment before the line is written.
launch_key() {
	local key="" _
	for _ in $(seq 1 40); do
		key="$(sed -n 's/.*open Nib at [^#]*#k=\([A-Za-z0-9_-]*\).*/\1/p' "$1" | tail -1)"
		[ -n "$key" ] && break
		sleep 0.1
	done
	[ -n "$key" ] && printf '%s\n' "$key"
}

# launch_token <base> <log> — trade the logged key at POST /api/launch and print the token. A key
# trades once, so each call needs a fresh line in the log (a restarted process writes one).
launch_token() {
	local key
	key="$(launch_key "$2")" || return 1
	curl -fsS -X POST "$1/api/launch" -H "X-Nib-Launch: $key" | sed -n 's/.*"csrf":"\([^"]*\)".*/\1/p'
}
