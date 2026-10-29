#!/usr/bin/env bash
# The release half of `./build.sh --publish` (/pending 868): upload the assets, make sure the
# release is PUBLISHED, and say "published" only when the update check's own endpoint says so.
#
# Usage: build/publish-release.sh <version> <branch> <asset>...   (from the repository root)
#
# `gh release create` with assets makes a DRAFT, uploads, then publishes. A run killed during the
# upload leaves that draft behind — a release with assets and no tag. The next run found it with
# `gh release view`, replaced its assets and printed "published": nothing had asked whether it was
# a draft, and a draft is served to nobody. `releases/latest` — the one address every running Nib
# asks (internal/server/update.go) — still answered the release before it.
#
# So, after the assets are up:
#   1. the release's draft state is READ, and a draft is published at HEAD — the commit the
#      preflight tied these binaries to. The target is given as a commit, never left to the
#      draft's own, because a draft has no tag for the preflight's tag check to have seen;
#   2. the last line comes from what `releases/latest` answers, not from what this script meant
#      to do. Anything else exits non-zero and names what is being served instead.
#
# `TestThePublishPublishesADraftAndReportsWhatIsServed` drives it with a stub gh. Never run it,
# or build.sh --publish, to test it: it publishes.
set -euo pipefail

fail() { echo "NOT published: $*" >&2; exit 1; }

version="${1:-}"; branch="${2:-}"
[ -n "$version" ] && [ -n "$branch" ] || fail "usage: build/publish-release.sh <version> <branch> <asset>..."
shift 2
[ "$#" -gt 0 ] || fail "no assets to upload"
tag="v$version"

if gh release view "$tag" >/dev/null 2>&1; then
  gh release upload "$tag" "$@" --clobber
else
  # A version with a hand-written note (docs/release-notes/<version>.md) is published WITH it: the digest
  # flag day (ADR-175) has to reach the people it strands, and "Nib 2.4.0" tells them nothing.
  notes=(--notes "Nib $version")
  [ -f "docs/release-notes/$version.md" ] && notes=(--notes-file "docs/release-notes/$version.md")
  gh release create "$tag" "$@" \
    --target "$branch" --title "Nib $version" "${notes[@]}"
fi

draft="$(gh release view "$tag" --json isDraft --jq .isDraft)" \
  || fail "the assets are up, but gh could not say whether $tag is a draft."
case "$draft" in
  false) ;;
  true)
    head_commit="$(git rev-parse HEAD)"
    echo "$tag was left as a draft (an interrupted publish) — publishing it at ${head_commit:0:12}"
    gh release edit "$tag" --draft=false --latest --target "$head_commit"
    ;;
  *) fail "the assets are up, but gh answered \"$draft\" when asked whether $tag is a draft." ;;
esac

# {owner}/{repo} are gh's own placeholders for the repository this directory belongs to.
latest="$(gh api 'repos/{owner}/{repo}/releases/latest' --jq .tag_name)" \
  || fail "the assets are up, but releases/latest could not be read, so nothing shows $tag is being served."
[ "$latest" = "$tag" ] \
  || fail "the assets are up, but releases/latest answers ${latest:-nothing}, not $tag — every running Nib is still told ${latest:-that} is current."
echo "published $tag — releases/latest answers $latest"
