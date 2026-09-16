#!/usr/bin/env bash
# The refusals `./build.sh --publish` runs BEFORE it builds anything (/pending 505).
#
# Usage: build/publish-preflight.sh <version>     (from the repository root; non-zero = refuse)
#
# `--publish` pushes the current branch, tags it `v<version>` remotely and uploads binaries built
# from the WORKING TREE. With nothing checking, a release could carry uncommitted edits — binaries
# nobody can rebuild from the tag they are attached to — or a version string the tagged commit's
# own VERSION file does not carry, which is the number every running Nib's update check compares
# against. Both are refused here. What `--publish` publishes is unchanged.
#
# It checks, and does not fix: it never stages, commits, stashes or cleans anything.
#
# What it does NOT refuse: files git is told to ignore (`.gitignore`, `.git/info/exclude`). The
# repository keeps local-only files that way on purpose, so an ignored file that is compiled in is
# outside this check by construction. `TestThePublishPreflightRefuses` drives each refusal.
set -euo pipefail

refuse() { echo "publish refused: $*" >&2; exit 1; }

want="${1:-}"
[ -n "$want" ] || refuse "no version given"
git rev-parse --is-inside-work-tree >/dev/null 2>&1 || refuse "not inside a git work tree, so nothing ties the binaries to a commit"

# Tracked files, staged or not, must match HEAD: the binaries are built from the working tree.
if ! git diff --quiet HEAD --; then
  refuse "tracked files differ from HEAD ($(git diff --name-only HEAD -- | head -5 | tr '\n' ' ')) — the binaries would be built from code the tag does not point at. Commit or set the change aside first."
fi

# Untracked source that `go build` or the embedded web assets would pick up. Not every untracked
# file: a scratch note in the root changes nothing the build reads.
stray="$(git ls-files --others --exclude-standard -- '*.go' 'go.mod' 'go.sum' 'web/' 'build/nfpm.yaml' | head -5)"
[ -z "$stray" ] || refuse "untracked files the build would read: $(printf '%s' "$stray" | tr '\n' ' ')— commit or remove them first."

# The version being published is the one the tagged commit carries.
head_version="$(git show HEAD:VERSION 2>/dev/null | tr -d '[:space:]')" || true
[ -n "$head_version" ] || refuse "HEAD has no VERSION file"
[ "$head_version" = "$want" ] || refuse "publishing $want but HEAD's VERSION is $head_version — the tag would name a release its own commit does not claim to be."

echo "publish preflight: tree matches HEAD and HEAD's VERSION is $want"
