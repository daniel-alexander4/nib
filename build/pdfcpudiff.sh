#!/usr/bin/env bash
# The pdfcpu comparison instrument (PLAN-pdfcpu-upgrade P01.S01): per document, what each of two trees reads, digests,
# verifies and maps — and only the DIFFERENCE is the output.
#
#     ./build/pdfcpudiff.sh --manifest <list>              write <list>.sha256 (one sha256 per listed file)
#     ./build/pdfcpudiff.sh <list> <treeA> <treeB>         compare the two trees over the list
#     ./build/pdfcpudiff.sh --rows <list> <tree> <out.tsv> one tree's rows alone: the BASELINE a later tree is compared with
#     ./build/pdfcpudiff.sh --selftest                     prove the instrument can see a change (non-vacuity)
#
# The list is a LOCAL file of document paths, one per line, never committed (as build/accuracy.sh and formrt.sh): real
# documents are somebody's. What may be committed is the list's hash manifest, so a run can say the list it read is the
# list it was given. A tree is a checkout (or `oldtree` export) of the repo; the row writer, build/pdfcpudiff.go.src,
# is laid into each through `go test -overlay`, so nothing is written into either.
#
# **It FAILS, never skips**, on an absent, empty or changed list or manifest, an unreadable listed document, or a tree
# that read fewer documents than were listed — a skip reads as green to every caller, and an instrument that reads
# nothing also diffs empty. It prints "N read of N listed" for each tree even when the diff is empty.
#
# **No path, name or value is printed or kept**: rows carry an index, and a refusal is a class (a hash of its text with
# digits and quoted strings removed). NIB_PDFCPUDIFF_EXPLAIN=<file> keeps the unreduced text locally for the operator.
#
# Exit: 0 the comparison ran (the diff is the output); 2 the instrument could not run honestly.
set -uo pipefail
HERE="$(cd "$(dirname "$0")/.." && pwd)"
SRC="$HERE/build/pdfcpudiff.go.src"
die() { echo "pdfcpudiff: FAIL: $1" >&2; exit 2; }

# Scratch space is torn down on every exit. What a run may KEEP for diagnosis is the rows (an index and classes, never a
# path) and only under NIB_PDFCPUDIFF_KEEP=1; the go test logs, which can echo a document's text on a panic, never
# survive a run that passed.
WORKS=()
cleanup() {
  [ "${NIB_PDFCPUDIFF_KEEP:-}" = "1" ] && return
  local w
  for w in "${WORKS[@]:-}"; do [ -n "$w" ] && rm -rf "$w"; done
}
trap cleanup EXIT

listed() { grep -vc '^[[:space:]]*\(#\|$\)' "$1"; }
paths() { grep -v '^[[:space:]]*\(#\|$\)' "$1"; }

manifest() { # <list>
  local list="$1"
  [ -f "$list" ] || die "no list at $list"
  [ "$(listed "$list")" -gt 0 ] || die "the list at $list names no documents"
  : >"$list.sha256.tmp"
  while IFS= read -r p; do
    [ -f "$p" ] || { rm -f "$list.sha256.tmp"; die "a listed document is not a file (line $(grep -nF -- "$p" "$list" | head -1 | cut -d: -f1))"; }
    sha256sum "$p" | cut -d' ' -f1 >>"$list.sha256.tmp"
  done < <(paths "$list")
  mv "$list.sha256.tmp" "$list.sha256"
  echo "manifest: $(listed "$list") documents → $list.sha256"
}

check_manifest() { # <list> — the one rule is tiergate.sh's (ADR-009); this instrument always requires the manifest
  local list="$1"
  [ -f "$list" ] || die "no list at $list"
  ( . "$HERE/build/tiergate.sh"; NIB_REQUIRE_TIERS=1 nib_corpus_check "$list" ) || exit 2
}

rows() { # <list> <tree> <out>  — runs the row writer inside the tree
  local list tree out ov explain=""
  list="$(realpath "$1")"; tree="$(realpath "$2" 2>/dev/null)" || die "no tree at $2"; out="$(realpath -m "$3")"
  [ -d "$tree/internal/pdfops" ] || die "$tree is not a nib tree"
  # one explain file per run: tree B must not erase tree A's
  [ -n "${NIB_PDFCPUDIFF_EXPLAIN:-}" ] && explain="$(realpath -m "$NIB_PDFCPUDIFF_EXPLAIN").$(basename "$out")"
  ov="$(mktemp /tmp/pdfcpudiff-overlay.XXXXXX)"
  printf '{"Replace":{"%s/internal/pdfops/zz_pdfcpudiff_test.go":"%s"}}' "$tree" "$SRC" >"$ov"
  (cd "$tree" && NIB_PDFCPUDIFF_LIST="$list" NIB_PDFCPUDIFF_OUT="$out" NIB_PDFCPUDIFF_EXPLAIN="$explain" \
    go test -timeout 0 -overlay "$ov" -count=1 -run '^TestPdfcpuDiffRows$' ./internal/pdfops/ >"$out.log" 2>&1) \
    || { tail -20 "$out.log" >&2; rm -f "$ov"; die "the row writer failed in $tree (log: $out.log)"; }
  rm -f "$ov" "$out.log"
}

# apart <rows> — rows that are not a normal read, counted apart (a timeout or an unreadable file is a result, never a "read")
apart() {
  local to un pa
  to="$(grep -c "$(printf '\ttimeout$')" "$1")"; un="$(grep -c 'unreadable-file' "$1")"; pa="$(grep -c "$(printf '\tpanic=')" "$1")"
  [ "$to$un$pa" = "000" ] || printf ' (%s timed out, %s unreadable, %s panicked)' "$to" "$un" "$pa"
}

compare() { # <list> <treeA> <treeB>
  local list="$1" a="$2" b="$3" work n ra rb
  check_manifest "$list"
  n="$(listed "$list")"
  work="$(mktemp -d /tmp/pdfcpudiff.XXXXXX)"; WORKS+=("$work")
  rows "$list" "$a" "$work/a.tsv"
  rows "$list" "$b" "$work/b.tsv"
  ra="$(wc -l <"$work/a.tsv")"; rb="$(wc -l <"$work/b.tsv")"
  echo "tree A: $ra read of $n listed$(apart "$work/a.tsv")"
  echo "tree B: $rb read of $n listed$(apart "$work/b.tsv")"
  [ "$ra" -eq "$n" ] && [ "$rb" -eq "$n" ] || die "a tree read fewer documents than were listed"
  echo "--- differing rows (index: column A → B) ---"
  paste -d'\n' "$work/a.tsv" "$work/b.tsv" | awk -F'\t' '
    NR % 2 == 1 { delete a; for (i = 1; i <= NF; i++) a[i] = $i; na = NF; next }
    { diff = 0; out = ""
      m = (NF > na) ? NF : na
      for (i = 2; i <= m; i++) if (a[i] != $i) { diff = 1; out = out "  " a[i] " → " $i "\n" }
      if (diff) { d++; printf "%s:\n%s", $1, out } }
    END { printf "--- %d of %d documents differ ---\n", d + 0, NR / 2 }'
  if [ "${NIB_PDFCPUDIFF_KEEP:-}" = "1" ]; then echo "rows kept in $work (index and classes only; delete when read)"; fi
}

selftest() {
  command -v git >/dev/null || die "git needed"
  local work doc a b out
  work="$(mktemp -d /tmp/pdfcpudiff-selftest.XXXXXX)"; WORKS+=("$work")
  doc="$work/doc.pdf"
  (cd "$HERE" && go run build/genpdf.go "$doc" "selftest page") || die "could not generate a fixture"
  printf '%s\n' "$doc" >"$work/list.txt"
  "$0" --manifest "$work/list.txt" >/dev/null || exit 2
  a="$(mktemp -d "$work/ta.XXXXXX")"; b="$(mktemp -d "$work/tb.XXXXXX")"
  git -C "$HERE" archive HEAD | tar -x -C "$a"
  git -C "$HERE" archive HEAD | tar -x -C "$b"
  # (b) the known one-line, digest-affecting mutation
  grep -q 'hashChunk(h, \[\]byte("nib-content-digest"))' "$b/internal/pdfops/attachments.go" || die "selftest: the mutation site moved"
  sed -i 's/hashChunk(h, \[\]byte("nib-content-digest"))/hashChunk(h, []byte("nib-content-digest-SELFTEST"))/' "$b/internal/pdfops/attachments.go"
  echo "== same tree against itself =="
  local same changed
  same="$(compare "$work/list.txt" "$a" "$a")" || { echo "$same"; die "selftest: the identical comparison failed"; }
  echo "$same" | grep -q '^1 read of 1 listed\|tree A: 1 read of 1 listed' || { echo "$same"; die "selftest: it did not say it read the document"; }
  echo "$same" | grep -q -- '--- 0 of 1 documents differ ---' || { echo "$same"; die "selftest: identical trees differ"; }
  echo "== a tree with one digest-affecting line changed =="
  changed="$(NIB_PDFCPUDIFF_KEEP=1 compare "$work/list.txt" "$a" "$b")" || { echo "$changed"; die "selftest: the mutated comparison failed"; }
  echo "$changed" | grep -q -- '--- 1 of 1 documents differ ---' || { echo "$changed"; die "selftest: the mutation went UNSEEN — the instrument is vacuous"; }
  echo "$changed" | grep -q 'digest6=' || { echo "$changed"; die "selftest: the difference was not in the digest column"; }
  echo "== no path, name or text of the document appears in anything kept =="
  local kept
  kept="$(echo "$changed" | sed -n 's/^rows kept in \([^ ]*\) .*/\1/p')"
  [ -d "$kept" ] || die "selftest: could not find the kept rows"
  if grep -rl -e "doc.pdf" -e "$doc" -e "selftest page" "$kept" "$work/list.txt.sha256" 2>/dev/null | grep -q .; then
    echo "$changed" >&2; die "selftest: a document's path, name or text reached the kept output"
  fi
  echo "$changed" | grep -q -e "doc.pdf" -e "selftest page" && die "selftest: a document's name reached the printed output"
  echo "== an empty list and a changed list must fail =="
  : >"$work/empty.txt"
  out="$("$0" "$work/empty.txt" "$a" "$a" 2>&1)"; case "$out" in *'names no documents'*) ;; *) die "selftest: an empty list was not refused for being empty" ;; esac
  echo "selftest changed" >>"$doc"
  out="$("$0" "$work/list.txt" "$a" "$a" 2>&1)"; case "$out" in *'differs from its manifest hash'*) ;; *) die "selftest: a changed document was not refused for differing from its manifest" ;; esac
  rm -f "$work/list.txt.sha256"
  out="$("$0" "$work/list.txt" "$a" "$a" 2>&1)"; case "$out" in *'no manifest'*) ;; *) die "selftest: a list with no manifest was not refused for that" ;; esac
  echo "selftest: PASS (identical → 0 differ; one mutated line → named in digest6; empty and changed lists refuse)"
}

case "${1:-}" in
  --manifest) [ $# -eq 2 ] || die "usage: $0 --manifest <list>"; manifest "$2" ;;
  --rows) [ $# -eq 4 ] || die "usage: $0 --rows <list> <tree> <out.tsv>"
    check_manifest "$2"; rows "$2" "$3" "$4"
    [ "$(wc -l <"$4")" -eq "$(listed "$2")" ] || die "the tree read fewer documents than were listed"
    echo "tree: $(wc -l <"$4") read of $(listed "$2") listed$(apart "$4") → $4" ;;
  --selftest) selftest ;;
  "") die "usage: $0 <list> <treeA> <treeB> | --manifest <list> | --selftest" ;;
  *) [ $# -eq 3 ] || die "usage: $0 <list> <treeA> <treeB>"; compare "$@" ;;
esac
