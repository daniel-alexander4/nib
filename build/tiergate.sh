# tiergate.sh — what a tier's verdict means, for the harnesses that run tiers 2-6. Sourced, never run.
#
# Two questions every harness answered for itself, each slightly differently, and both wrongly in the
# same direction (/pending 650, 605, 822):
#
#   1. **Did this tier look at anything?** A missing dependency printed a sentence and exited 0, so a
#      tier that drove no browser read exactly like one that passed — to `suiterun`, to a `make`
#      chain, to a caller's `&&`. Only the printed line told them apart. `nib_skip` keeps that
#      default, because a fresh clone runs tiers 0 and 1 unaided and must not fail on tiers it has
#      no setup for (CONTRIBUTING.md); under `NIB_REQUIRE_TIERS=1` a skip exits **77** — non-zero,
#      so every caller fails it, and distinct, so a caller that wants to can say "skipped" rather
#      than "red".
#
#   2. **Did every test FILE contribute?** The node:test harnesses compared `# tests N` with the file
#      count. That cannot fire: N is a TOTAL (≈532 against 93 files, 179 against 42), so one file
#      contributing nothing is hidden whenever another contributes two. It also counts skipped tests,
#      and a file with no tests at all is reported as ONE passing test named after the file — so even
#      an empty file cleared it. `nib_population` reads the per-file counts `build/filecount.mjs`
#      prints and judges each file on its own.

# nib_skip <why> — this tier cannot run here. Exits the harness.
nib_skip() {
	if [ "${NIB_REQUIRE_TIERS:-}" = "1" ]; then
		echo "SKIP (refused, NIB_REQUIRE_TIERS=1): $1" >&2
		exit 77
	fi
	echo "SKIP: $1"
	exit 0
}

# NIB_FILECOUNT — the node --test arguments that add the per-file count to the ordinary TAP stream.
# Both go to stdout: the TAP reporter must be named explicitly once a second reporter is.
NIB_FILECOUNT=(--test-reporter=tap --test-reporter-destination=stdout
	"--test-reporter=$(dirname "${BASH_SOURCE[0]}")/filecount.mjs" --test-reporter-destination=stdout)

# nib_population <tier> <dir> <expected file count> <runner output> — returns 0 when every test file
# in <dir> ran at least one test, 1 when the population is wrong, and 77 under NIB_REQUIRE_TIERS=1
# when a file's tests ALL skipped (a dependency that file needs is absent: tagcorrect.test.mjs gates
# all seven of its tests on LibreOffice). Prints its findings to stderr, after the runner's totals.
nib_population() {
	local tier="$1" dir="$2" expect="$3" out="$4"
	local files n
	files="$(find "$dir" -maxdepth 1 -name '*.test.mjs' | sort)"
	n="$(printf '%s\n' "$files" | grep -c .)"
	if [ "$n" -ne "$expect" ]; then
		echo "FAIL: expected $expect $tier test files, found $n — a test file was added or dropped." >&2
		echo "      If deliberate, update the declared count in this script." >&2
		return 1
	fi
	local verdict
	verdict="$(printf '%s\n' "$out" | awk -v files="$files" '
		/^# nib-file [0-9]+ [0-9]+ / { ran[$5] = $3; skipped[$5] = $4; next }
		END {
			nf = split(files, f, "\n"); total = 0; skip = 0
			for (i = 1; i <= nf; i++) {
				p = f[i]; sub(/^\.\//, "", p)
				if (!(p in ran) || ran[p] + skipped[p] == 0) { print "EMPTY " p; continue }
				total += ran[p]; skip += skipped[p]
				if (ran[p] == 0) print "ALLSKIP " p " " skipped[p]
			}
			print "TOTAL " total " " skip
		}')"
	local total skipped bad=0 skipfile=0 line
	read -r _ total skipped <<<"$(printf '%s\n' "$verdict" | grep '^TOTAL ')"
	while IFS= read -r line; do
		case "$line" in
			EMPTY\ *)
				echo "FAIL: ${line#EMPTY } contributed no test — its tests are silently not running" >&2
				bad=1 ;;
			ALLSKIP\ *)
				set -- ${line#ALLSKIP }
				echo "SKIPPED: every test in $1 skipped ($2) — that file looked at nothing on this machine" >&2
				skipfile=1 ;;
		esac
	done <<<"$verdict"
	echo "population: $n $tier files, $total tests ran, $skipped skipped" >&2
	if [ "$total" -eq 0 ]; then
		echo "FAIL: the $tier suite ran but ran no tests — a green with nothing in it" >&2
		return 1
	fi
	[ "$bad" = 0 ] || return 1
	if [ "$skipfile" = 1 ] && [ "${NIB_REQUIRE_TIERS:-}" = "1" ]; then
		echo "SKIP (refused, NIB_REQUIRE_TIERS=1): a whole $tier file skipped" >&2
		return 77
	fi
	return 0
}
