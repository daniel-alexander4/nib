# ADR-039 — the download is Nib's, and the install is not

**Status:** accepted
**Date:** 2026-09-16
**Context:** Dan: *"when I click the pill to download a new version, I should see a popup with
download progress and a way to open/execute/install the new version. Download location should be
clearly displayed."* `internal/server/update.go`; `startDownload` in `web/app.js`;
`internal/server/window.go`'s stream; `internal/server/saveas.go`'s write door.
**Supersedes:** the download half of the v1.95.0 decision (a native `confirm()` then
`location.assign(downloadUrl)`). The pill's tri-state colouring and click-to-check are unchanged.
**Applies:** anything that fetches bytes from the network on the user's behalf.

## Decision

**Nib performs the download itself**, writes it to a folder the user can see and change, reports
progress, and stops there. **It does not execute, install, or replace anything**, and the file it
writes is not executable.

| | before | now |
| --- | --- | --- |
| who fetches the bytes | the browser (`location.assign`) | the server, streaming to disk |
| progress | none possible | `event: download` on the existing window stream |
| destination | the browser's download folder, unnameable | a chosen folder, default `~/nib`, displayed |
| after it lands | nothing | **Show in folder** |
| install | never | **still never** |

## Why the server, and not the page

The obvious cheaper shape — fetch the asset in the page, read `response.body` for progress, hand the
bytes to the existing `POST /api/write` — **was measured and refused.** A real release asset
answers with **no `Access-Control-Allow-Origin`**, on either hop: the `github.com` 302 or the
`release-assets.githubusercontent.com` 200. A page fetch is blocked outright, so there is no
client-side path to the bytes. Server-side is forced rather than preferred, and the measurement is
recorded here because the next person will have the same idea.

Two numbers from the same probe shaped the implementation: the linux-amd64 asset is
**94,658,744 bytes (~95 MB)**, so the transfer streams to disk rather than buffering; and the
redirect target is a **signed URL valid for about an hour**, so the server resolves and follows it
at download time instead of holding one.

## Why the install half is refused

**There is nothing to verify the bytes against.** `build.sh:72` publishes
`assets=("$DIST/nib-$VERSION"-* "$DIST/nib_${VERSION}_"*.deb LICENSE THIRD-PARTY-NOTICES.md)` and
`gh release upload/create` attaches exactly that — **no checksum file, no signature, no manifest.**
The crypto review's standing check asks that verify-before-use be *unrepresentable otherwise*; here
there is no verification to make unrepresentable. Executing a ~95 MB binary fetched over the
network with no integrity check is the supply-chain shape, and no dialog makes it safe.

**It also contradicts a written promise.** `README.md`: *"Nib only notifies and downloads — it never
installs or replaces itself; you apply the update the way you installed."* That sentence is kept.

**And on two platforms there is nothing to install.** macOS and Windows ship a **bare binary**
(`build.sh:41-46`), so "install" there means overwriting the running executable — the exact act the
sentence above rules out. Only the Linux `.deb` has an installer, and applying it needs privileges
Nib does not have and should not ask for.

So the dialog offers **Show in folder**, and the file is written `0o644`: Nib will not hand back a
ready-to-run artifact it cannot vouch for. **The path to the install half is publishing checksums
from `build.sh` and verifying them here** — a separate change, and the precondition for reopening
this.

## What the route may be told, and what it may not

**The client sends a destination folder and nothing else.** The asset URL is re-resolved server-side
by the same `latestRelease` + `assetURL` the check uses. A route that accepted a URL would be a
general "fetch this and write it to my disk" primitive, with the request body choosing both host and
path. For the same reason, **reveal takes no path**: it opens the folder of the download this
process just wrote, and `browser.OpenFolder` cannot express "open that file" — a desktop asked to
open a binary or a `.deb` may run it.

**`requireUnlocked` + CSRF, not the check route's `requirePublicLoopback`.** That route is public
because it only queries out (`server.go:327-329`); these write bytes into the user's filesystem,
which is what `POST /api/write` does and takes the same guard. Inheriting the check's gate would
have left a disk-writing route reachable without a CSRF token.

The asset's **file name is remote-controlled** — it comes from GitHub's JSON — so it is reduced to a
base name, refused if anything but a plain name survives, and then contained by `containedJoin`.

## Progress rides the stream that already exists

A new `event: download` on `/api/window`, never a second `EventSource`: **each connection is counted
as a window** (`window.go`), so another stream would inflate the count and defeat the idle exit that
fires at zero. The loop now serves two broadcasters, which forced two changes: both wakes are taken
**before** either state is read — `/pending 464`'s lost-wakeup rule, which applies per broadcaster —
and each event is emitted **only when its payload changes**, or a percentage tick would re-send the
armed state a hundred times per download.

Throttling is at whole percents on the server, and the client dedupes on rendered text before
writing into an `aria-live` region — the guard `#srvWaitTiers` already carries.

## What this costs, stated

**A partial file is the risk this design most has to answer**, because the artifact a user finds in
that folder is one they might run. The transfer writes `<name>.part` and renames on success only;
every exit — failure, cancel, short read against a stated length, oversize — removes it. Cancel owns
the abort because Escape *clicks* a dialog's Cancel rather than hiding it.

**One download at a time**, refused rather than queued: two streams onto one path is a corrupt file,
and a refusal is something a user can act on.

**It cannot be exercised end to end on the development machine.** The newest published release is
older than the running build, so the pill reads green and the download path is reachable only
against a stub — the same gap `/pending 10` has carried since v1.95.0. This ships verified by tests
and by stubs, not by a real release.
