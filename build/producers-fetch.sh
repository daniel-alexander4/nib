#!/usr/bin/env bash
# producers-fetch.sh — fetch the sourced producers (Word, Acrobat and the other producers this machine cannot run) named
# in build/producers/sourced.tsv into the real-producer corpus (PLAN-ua-coverage.md P08.S03). build/producers.sh makes
# the local half; this is the other half, and the only one that touches the network.
#
#   build/producers-fetch.sh [out-dir]   # default: $NIB_UA_PRODUCERS, else ~/nib/producers
#
# **A file whose SHA-256 does not match the manifest is REFUSED, never scored**: it is not written, the script says
# which, and it exits non-zero after the rest. A publisher that re-issues a document at the same URL therefore changes
# nothing in the corpus until someone measures the new file and updates the manifest's hash on purpose: the corpus's
# copy, if it still matches, is KEPT (and so is one a transient outage made unavailable). Each sourced
# producer's directory is replaced whole, like build/producers.sh's, so a row removed from the manifest leaves no file.
# The files are never committed (an out-dir inside the repository is refused).
#
# What this harness CANNOT discharge
#   - Availability: the documents live on other people's servers. A 404 or 403 is reported per row and the corpus is
#     narrower, not wrong. Several agencies refuse a plain HTTP client outright (measured: DOL, SEC, GAO, HHS, ED — 403).
#   - The producer claim: the directory is what the file's /Producer said when the manifest was written, measured then;
#     the hash pins the bytes, so the claim cannot drift after that.
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
manifest="$here/producers/sourced.tsv"
out=${1:-${NIB_UA_PRODUCERS:-$HOME/nib/producers}}
repo=$(cd "$here/.." && pwd -P)
mkdir -p "$out"
out=$(cd "$out" && pwd -P)
case "$out/" in "$repo"/*)
	rmdir "$out" 2>/dev/null || true
	echo "producers-fetch.sh: refusing to write the corpus inside the repository ($out) — it is never committed" >&2
	exit 2
	;;
esac
command -v curl >/dev/null || { echo "SKIP: curl is not installed, so the sourced producers are not fetched"; exit 0; }

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

fetched=0 refused=0 missing=0
while IFS=$'\t' read -r producer name sha url _; do
	case "$producer" in '' | '#'*) continue ;; esac
	mkdir -p "$work/stage-$producer"
	f="$work/stage-$producer/$name.pdf"
	# Publishers disagree about clients (measured: CDC refuses a browser-like agent and serves curl's; FDA the reverse),
	# so curl's own agent is tried first and a browser-like one second.
	if ! curl -sSfL --max-time 120 -o "$f" "$url" 2>"$work/curl.err" &&
		! curl -sSfL --max-time 120 -A 'Mozilla/5.0' -o "$f" "$url" 2>"$work/curl.err"; then
		echo "MISSING: $producer/$name — $(tr -d '\n' <"$work/curl.err") ($url)" >&2
		rm -f "$f"
		missing=$((missing + 1))
		continue
	fi
	got=$(sha256sum "$f" | cut -c1-64)
	if [ "$got" != "$sha" ]; then
		echo "REFUSED: $producer/$name — SHA-256 $got, the manifest pins $sha; the file was not written ($url)" >&2
		rm -f "$f"
		refused=$((refused + 1))
		continue
	fi
	fetched=$((fetched + 1))
done <"$manifest"

# A row this run could not fetch, or refused, keeps the copy already in the corpus IF that copy still matches the
# manifest's hash — so an outage or a re-issue at the same URL never shrinks the corpus — and a file the manifest no
# longer names is dropped when its producer's directory is replaced.
while IFS=$'\t' read -r producer name sha url _; do
	case "$producer" in '' | '#'*) continue ;; esac
	f="$work/stage-$producer/$name.pdf" old="$out/$producer/$name.pdf"
	if [ ! -e "$f" ] && [ -e "$old" ] && [ "$(sha256sum "$old" | cut -c1-64)" = "$sha" ]; then
		cp "$old" "$f"
		echo "KEPT: $producer/$name — this run did not fetch it, and the corpus's copy still matches the manifest" >&2
	fi
done <"$manifest"
# Each sourced producer's directory carries a `.sourced` marker, so one whose LAST manifest row was removed — no stage
# directory this run — is still recognised as this script's and removed, rather than scored forever unpinned (the P08
# phase-close review, R8-5). The local producers' directories carry no marker and are never touched here.
for d in "$out"/*/; do
	p=${d%/}; p=${p##*/}
	if [ -e "$out/$p/.sourced" ] && [ ! -d "$work/stage-$p" ]; then
		echo "DROPPED: $p — the manifest names no file for it any more" >&2
		rm -rf "${out:?}/$p"
	fi
done
for d in "$work"/stage-*; do
	[ -d "$d" ] || continue
	p=${d##*/stage-}
	: >"$d/.sourced"
	rm -rf "${out:?}/$p"
	mv "$d" "$out/$p"
done

echo "producers-fetch.sh: $fetched fetched, $refused refused on hash, $missing unavailable, under $out"
[ "$refused" -eq 0 ] || exit 1
