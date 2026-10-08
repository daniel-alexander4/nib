# ADR-102 — an update downloads to the browser's own folder, and nothing asks

**Status:** accepted
**Date:** 2026-10-07
**Context:** Dan: *"nib download folder should always be the browser default folder unless configured
differently in settings. It should not ask me where I want to download it. It should pop up a message
saying Downloading to <folder>."* He had to change the dialog's folder by hand from `~/nib` to
`~/Downloads` to download v1.191.2. Offered "the system Downloads folder" as a stand-in, he refused
it as an assumption: he means the folder **the browser Nib's window is running in** downloads to.
`internal/browser/downloads.go`; `internal/server/downloaddir.go`; `handleUpdateDownload`;
`startDownload` in `web/app.js`.
**Supersedes:** two clauses of ADR-039 and nothing else — *"The client sends a destination folder and
nothing else"* and the table row *"a chosen folder, default `~/nib`, displayed"*. The server still
fetches, the write still goes through the atomic door at `0o644`, there is still one download at a
time, Cancel still owns the abort, and Nib still never installs.
**Applies:** anything that decides where a file Nib fetched is written.

## Decision

**One server-side function, `downloadDir`, decides the folder. The page sends none and asks for
none.** It tries four places in order and uses the first that is a folder that exists:

| | where | shown in Settings as |
| --- | --- | --- |
| 1 | Nib's own setting (`vault.Settings.DownloadDir`, Settings → Updates → Download folder) | "set here" |
| 2 | the download folder **set in the browser this process opened its window in** | "Chrome's download folder" |
| 3 | the system Downloads folder — what each of those browsers uses when nothing is set in it | "your Downloads folder" |
| 4 | `~/nib` | "Nib's own folder, because no Downloads folder was found" |

**A folder that is not there is skipped, never created.** A folder the user named and then removed,
or one on a drive that is not plugged in, is not Nib's to bring back. Only `~/nib` is Nib's to
create, so the last rung always answers. The popup and the Settings line name the folder actually
used, and Settings says so when its own entry was the one skipped.

**Clicking the pill starts the download.** The popup says *Downloading to &lt;folder&gt;* — the
`dir` in the route's answer, never worked out of a path by the page — then progress, then
*Downloaded to &lt;path&gt;* and **Show in folder**. The Folder box and the Download button are gone.

## What is read from the browser, and nothing else

`browser.Open` used to log which branch opened the window; it now records it (`browser.Opened`).

- **Chrome, Chromium, Edge, Brave:** `profile.last_used` from `Local State` (else `Default`), then
  `download.default_directory` from that profile's `Preferences`.
- **Firefox:** the default profile from `profiles.ini` (an `[Install…]` section's `Default`, else the
  profile marked `Default=1`, else the only one), then `browser.download.folderList` (0 the Desktop,
  1 the system Downloads folder, 2 `browser.download.dir`; absent is 1) from its `prefs.js`.
- **Linux, to know which browser a tab opened in:** the `x-scheme-handler/http` line under
  `[Default Applications]` in `mimeapps.list`. **Windows:** the `http` `UserChoice` ProgId.
- **For rung 3:** `XDG_DOWNLOAD_DIR` (and `XDG_DESKTOP_DIR`, for Firefox's 0) from `user-dirs.dirs`,
  parsed and never run; on Windows the Downloads known folder; on macOS `~/Downloads`.

**The privacy reviewer's question is residue, and the answer is: none.** Each file is decoded into
those keys alone. No other value is kept in memory past the decode, none is logged, none is stored
in the vault, and the only thing that leaves the function is one folder path and the browser's
name, which go to `/api/status` and the popup on the loopback window and nowhere else.

## Why a browser's preference file is treated as hostile

It is text another program wrote — the browser, a sync service, an installer, anything running as
the user. So:

- a folder from one is used only if it is an **absolute path to a directory that exists** after
  cleaning (`browser.UsableDir`), and the door checks again;
- a profile name is joined onto a folder only if it is one plain path element; a relative Firefox
  profile path must stay under Firefox's folder;
- each file is read up to **8 MiB** and no further (measured here: `Preferences` 54 KB and 127 KB,
  `Local State` 12 KB, `prefs.js` 9–53 KB);
- a file that is missing, too large, or does not parse means **"nothing set"** — the next rung —
  and is never an error the user sees;
- nothing read is executed. `user-dirs.dirs` is shell syntax and is read as text: only the two forms
  the format allows, `"$HOME/…"` and `"/…"`.

What the worst file can do is therefore choose **which existing folder** a non-executable release
artifact with a server-chosen name lands in. That is the same power the browser's own setting has
over every other download.

## The route

`POST /api/update/download` reads nothing from the request. A `dir` still sent — a stale page — is
**not read**, the same way the `url` field never was; refusing it would add a server read of a
field no client sends, and would break a page loaded before an upgrade for nothing. Guards are
unchanged: `requireUnlocked`, the session token, no new route.

**The 412 names the file and the folder**, because the popup has nothing else to say and nothing to
choose. **Reveal still takes no path**, and now also opens the folder of a file the server just
refused to overwrite: both that and a finished download are folders the *server* resolved, which is
the property ADR-039's rule protects. The refusal's folder is forgotten when a download begins.

## Settings

A text field beside *Check for updates on startup*. Empty follows the browser. A typed folder is
expanded (`~`), cleaned and stored absolute, and **refused with a sentence shown beside the field**
unless it is a folder that is already there. Empty is stored as absence, so a build that drops the
key returns the user to the browser's folder.

## What this cannot know, declared

- **A window this process did not open** — a hand-typed URL, a headless run (`NIB_NO_BROWSER`), a
  second launch surfacing the window of a Nib started headless — has no recorded browser. Nothing is
  read and rung 3 answers. Predicting the browser from what `Open` *would* pick was refused: it
  reads someone's profile on a guess.
- **macOS does not identify a tab's browser.** The default-browser record is a binary property list
  Nib has no reader for. A tab is opened there only when none of Chrome, Edge, Brave or Chromium is
  installed, and rung 3 is what Safari and Firefox use unless the user changed it. A Mac whose only
  browser is a Firefox with a custom folder gets `~/Downloads`.
- **A browser not named above is not read**: Safari, Waterfox, LibreWolf, Vivaldi, Opera. Measured:
  this machine's own default `http` handler is a Waterfox launcher. Rung 3 answers.
- **Chrome's "ask where to save each file" and Firefox's "always ask"** are not read. Nib does not
  ask; that is the request.
- **The snap and flatpak profile locations are from those packages' layouts, not from a machine that
  has them.** The Windows and macOS paths were compiled (`GOOS=windows`, `GOOS=darwin`) and their
  tables tested on Linux; nothing here was run on either.
- **Writability is not part of the test.** A folder that exists and cannot be written fails the
  download with the door's own message rather than falling to the next rung.

## What it costs

`/api/status` now calls `downloadDir`, which reads at most two files from the browser's profile plus
`user-dirs.dirs`. **Measured on the development machine, 200 calls each:** 1.2 ms per call for
Chrome (a 127 KB `Preferences`), 0.6 ms for Chromium (54 KB), 12 µs when the browser is not one
Nib reads, 8 µs for the system folder alone. Nearly all of it is decoding the browser's JSON. Status
is fetched when the page loads, after an unlock, after this setting is saved and when a request
fails — not on a timer — so nothing caches the answer, and a folder changed in the browser is seen
at the next of those.

**It cannot be exercised end to end here**, for ADR-039's reason: the newest release is older than
the running build. The route was driven against a stub release server; the browser reader against
fixture profiles in temp directories, never a real one.
