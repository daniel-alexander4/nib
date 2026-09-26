#!/usr/bin/env bash
# producers.sh — render the committed sources in build/producers/ through every producer this machine has, into the
# real-producer corpus the checker is scored against (PLAN-ua-coverage.md P08.S02; the harness is
# internal/uacheck/producers_test.go, TestTheCheckerAgreesWithVeraPDFOnRealProducers).
#
#   build/producers.sh [out-dir]        # default: $NIB_UA_PRODUCERS, else ~/nib/producers
#
# Writes <out>/libreoffice/{writer,form}-{tagged,ua,untagged}.pdf, <out>/pdflatex/{article,times,tagged}.pdf,
# <out>/chromium/web.pdf and <out>/ghostscript/<each of those>.pdf (Ghostscript pdfwrite re-distils them, re-encoding
# Type 1 as CFF). Chromium's is the one SOURCE rendered into Type 0 fonts (CIDFontType2, Identity-H); Ghostscript's copy
# of it keeps them. A producer that is not installed is SKIPPED and said so; the corpus is then narrower, never silently
# wrong. The corpus is NOT committed — the sourced producers (Word, Acrobat — P08.S03) are third-party documents and one
# directory holds both — so an out-dir inside this repository is refused.
#
# **Each producer replaces its OWN directory, and only when it succeeded.** A producer's files are built in scratch space
# and swapped in whole, so a file from an earlier run (a removed variant, a producer that failed this time) is never
# scored beside fresh ones. A producer that is installed and FAILS says which and why, removes its directory rather than
# leave last run's files to be scored, and the script exits non-zero after the others have run. Other producers'
# directories — including P08.S03's `word/` and `acrobat/` — are never touched.
#
# What this harness CANNOT discharge
#   - Word and Acrobat: neither runs here; their files come from P08.S03's pinned manifest.
#   - LuaLaTeX: measured failing on this machine (luaotfload cannot load lmroman10-regular), so LaTeX's tagging is
#     exercised through pdfLaTeX's \DocumentMetadata only.
#   - Byte-identity across runs: producers stamp dates and IDs, so two runs differ in bytes. What is stable is the
#     verdicts, which the harness scores (measured: two regenerations score identically).
#   - The same fonts on every machine: pdfLaTeX falls back to a Type 3 bitmap for a glyph with no Type 1 program
#     installed, so the font population depends on the TeX installation.
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
src="$here/producers"
out=${1:-${NIB_UA_PRODUCERS:-$HOME/nib/producers}}
repo=$(cd "$here/.." && pwd -P)
mkdir -p "$out"
out=$(cd "$out" && pwd -P)
case "$out/" in "$repo"/*)
	rmdir "$out" 2>/dev/null || true # the empty directory this run just made, if it was this run's
	echo "producers.sh: refusing to write the corpus inside the repository ($out) — it is never committed" >&2
	exit 2
	;;
esac

work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# A file:// URL for a local path, with the two characters that break one encoded.
url() { local p=${1// /%20}; echo "file://${p//#/%23}"; }

made=0
failed=()
skip() { echo "SKIP: $1 is not installed, so its part of the corpus is not made (the corpus is narrower, not wrong)"; }
# publish <producer> — swap the staged directory in for the producer's own.
publish() {
	rm -rf "${out:?}/$1"
	mv "$work/stage-$1" "$out/$1"
	made=$((made + $(find "$out/$1" -name '*.pdf' | wc -l)))
}
# fail <producer> <why> [log] — say which producer and why, and remove its directory so no stale file is scored.
fail() {
	echo "FAIL: $1 is installed and did not produce its files — $2" >&2
	if [ -n "${3:-}" ] && [ -f "$3" ]; then tail -n 20 "$3" >&2; fi
	rm -rf "${out:?}/$1"
	failed+=("$1")
}

# LibreOffice: the Writer source and the form source, each exported three ways. Each variant is its own filter-options
# JSON; an HTML source goes through Writer/Web's export filter, which is what imports its controls as form fields, and
# ExportFormFields is explicit because without it the form exports no fields (measured).
lo() {
	local doc stem filter v name opts
	mkdir -p "$work/stage-libreoffice"
	for doc in writer.fodt form.html; do
		stem=${doc%.*}
		filter=writer_pdf_Export
		[ "$doc" = form.html ] && filter=writer_web_pdf_Export
		for v in \
			'tagged|{"UseTaggedPDF":{"type":"boolean","value":"true"},"ExportFormFields":{"type":"boolean","value":"true"}}' \
			'ua|{"PDFUACompliance":{"type":"boolean","value":"true"},"UseTaggedPDF":{"type":"boolean","value":"true"},"ExportFormFields":{"type":"boolean","value":"true"}}' \
			'untagged|{"UseTaggedPDF":{"type":"boolean","value":"false"},"ExportFormFields":{"type":"boolean","value":"true"}}'; do
			name=${v%%|*} opts=${v#*|}
			mkdir -p "$work/lo-$stem-$name"
			# A private profile, so a running LibreOffice (or its lock) does not swallow the conversion.
			soffice -env:UserInstallation="$(url "$work/lo-profile")" --headless \
				--convert-to "pdf:$filter:$opts" --outdir "$work/lo-$stem-$name" "$src/$doc" >"$work/lo.log" 2>&1 || return 1
			[ -s "$work/lo-$stem-$name/$stem.pdf" ] || return 1
			cp "$work/lo-$stem-$name/$stem.pdf" "$work/stage-libreoffice/$stem-$name.pdf"
		done
	done
}
if command -v soffice >/dev/null; then
	if lo; then publish libreoffice; else fail libreoffice "a conversion wrote no PDF" "$work/lo.log"; fi
else
	skip LibreOffice
fi

# pdfLaTeX: two untagged sources (Computer Modern, Times) and one tagged. Run twice so references settle.
tex() {
	local t
	mkdir -p "$work/stage-pdflatex" "$work/tex"
	for t in article times tagged; do
		for _ in 1 2; do
			if ! (cd "$work/tex" && pdflatex -interaction=batchmode -halt-on-error "$src/$t.tex" >/dev/null); then
				cp "$work/tex/$t.log" "$work/tex.log" 2>/dev/null || true
				return 1
			fi
		done
		cp "$work/tex/$t.pdf" "$work/stage-pdflatex/$t.pdf"
	done
}
if command -v pdflatex >/dev/null; then
	if tex; then publish pdflatex; else fail pdflatex "a source did not compile" "$work/tex.log"; fi
else
	skip pdfLaTeX
fi

# Chromium (headless, Skia/PDF): tagged output with Type 0 CIDFontType2 fonts. Any Chromium-family binary will do.
chrome=$(command -v chromium || command -v chromium-browser || command -v google-chrome || true)
if [ -n "$chrome" ]; then
	mkdir -p "$work/stage-chromium"
	if "$chrome" --headless=new --disable-gpu --no-sandbox --user-data-dir="$work/chrome-profile" --no-pdf-header-footer \
		--print-to-pdf="$work/stage-chromium/web.pdf" "$(url "$src/web.html")" >"$work/chrome.log" 2>&1 &&
		[ -s "$work/stage-chromium/web.pdf" ]; then
		publish chromium
	else
		fail chromium "headless print wrote no PDF" "$work/chrome.log"
	fi
else
	skip Chromium
fi

# Ghostscript: re-distil everything above. pdfwrite drops or rebuilds the structure and re-encodes fonts, which is the
# point — its output is where the font family diverges most from what the first producer wrote. It runs over the
# directories published THIS run, so a producer that failed contributes nothing here either.
if command -v gs >/dev/null; then
	mkdir -p "$work/stage-ghostscript"
	ok=1
	for p in libreoffice pdflatex chromium; do
		for f in "$out/$p"/*.pdf; do
			[ -e "$f" ] || continue
			gs -q -dNOPAUSE -dBATCH -dSAFER -sDEVICE=pdfwrite -o "$work/stage-ghostscript/$p-$(basename "$f")" "$f" \
				>>"$work/gs.log" 2>&1 || ok=0
		done
	done
	if [ "$ok" = 1 ]; then publish ghostscript; else fail ghostscript "a re-distil failed" "$work/gs.log"; fi
else
	skip Ghostscript
fi

echo "producers.sh: $made file(s) written under $out"
if [ ${#failed[@]} -gt 0 ]; then
	echo "producers.sh: FAILED — ${failed[*]}; their directories were removed rather than left stale" >&2
	exit 1
fi
