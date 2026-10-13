# ADR-157 — a launch key trades only for the user it was minted for

**Status:** accepted
**Date:** 2026-10-12
**Amends:** ADR-053's declared gap (the launch key in the browser's command line, `/pending 701`), which
ADR-054 restated. Neither decision changes: the key still rides in the URL fragment, still trades once.
ADR-006's refusal of peer credentials is not reopened — it refused a second TRANSPORT (a Unix socket, named
pipes on Windows); this adds none. ADR-009 (one door; a declared platform split).
**Context:** `browser.Open` starts the browser with `--app=http://127.0.0.1:P/#k=KEY`, and the tab fallback
passes the same URL to `xdg-open`. A command line is readable by every user of the machine
(`/proc/<pid>/cmdline` is 0444; `/proc` here is mounted without `hidepid`), and TCP loopback is not per-user,
so another user could read the key and trade it at `POST /api/launch` before the page did.
**Applies:** `internal/server` (`launch.go`, `launchpeer*.go`).

## Measured

On the development machine: Linux Mint, Chrome 155, Chromium and Firefox as .deb packages; no snap or
flatpak browser is installed.

- **The hole.** A process polling `/proc` read the key from the launcher's command line and traded it 118 ms
  after the browser was started, and was given the token. The real window (Chrome, a new profile) traded
  5.6 s after the launch and was answered 403, which the page shows as *Open Nib again*. Two more launches
  on new profiles traded about 3.5 s in; a profile already in use was not measured. The key stays in the browser's command line for the life of
  the browser process; after the trade it opens nothing.
- **The trampoline ADR-053 proposed** (a 0600 file the browser is pointed at, which redirects to the keyed
  URL) works where it could be tried: Chrome and Chromium keep the app window (no bar: outer height − inner
  height 0, against 131 and 87 for a tab), the fragment arrives whole, no referrer, one history entry, from
  `$XDG_RUNTIME_DIR`, `~/nib`, `/tmp` and `~/.config`, by script and by meta refresh; Firefox follows it in a
  tab. **What it could not be shown to do is open under confinement** — no snap or flatpak browser is
  installed here — and a file a confined browser cannot read is a Nib that does not open. A second reason
  it is not built: the tab fallback hands the path to `xdg-open`, which picks the handler for an HTML FILE,
  not for an `http` URL; where the two differ the file opens in an editor.
- **The kernel names the owner of a loopback socket.** One exact `NETLINK_SOCK_DIAG` lookup (the interface
  `ss` reads) answered user 991 for a socket of `systemd-resolved`, 0 for `cupsd`'s, and this user for this
  process's own, over IPv4 and IPv6, in about 90 µs for the two lookups a trade makes.
- **Asked with no interface, a socket bound to `lo` is not found** — and binding a socket to a device takes
  no privilege. Asked with the loopback interface's index it is found.
- **A socket its owner closed is reported with owner 0** and no inode while it finishes.

## Decision

**1. `POST /api/launch` asks the kernel who owns the connection it arrived on, and refuses a key offered by
another user.** One question, `tradedByAnotherUser`, asked of the request's own two addresses. The far end
of a loopback connection is a socket in this machine's own table, whatever browser made it and however that
browser is confined, so nothing about how the window is opened changes.

**2. The refusal comes BEFORE the key is looked at, and does not spend it.** The window the key was minted
for still trades it. A refusal that spent the key would leave the thief with nothing and the user with *Open
Nib again*.

**3. One answer refuses: the kernel names an owner who is neither this user nor root.** Everything else
trades as it did before —

- **root**, because it reads this process's memory already and a relay root runs is root's to vouch for;
- **no such socket**, because the far end is then not a socket of this machine (a peer reached through
  address translation, which only root can arrange) or has closed and cannot be answered;
- **the kernel will not say** — another platform, or a sandbox without the diagnostics — said ONCE in the
  log, because there the key is as exposed as it was.

A Nib that does not open is worse than the gap, so a doubt refuses nothing.

**4. The question is asked the way the kernel delivers a packet**: an exact lookup for the socket that
receives on the loopback interface, never a walk of `/proc/net/tcp` (read in pieces while the table changes,
so a row can be skipped) and never with no interface (a socket bound to `lo` is then invisible). It asks
about this process's OWN end first; a kernel that cannot find a socket this process is holding is one whose
"no such socket" means nothing, and is treated as "will not say". A listener answering in a connection's
place, a connection waiting out its timer, and a socket with no inode are "no such socket", not an owner.

**5. Linux only, and the others say so.** `launchpeer_linux.go` asks; `launchpeer_other.go` answers "cannot
tell". No second transport, no change to the launch, the hand-off or the harnesses' log line.

## Consequences

- On Linux another user who reads the key from a command line cannot trade it. The key is still IN the
  command line; this ADR does not take it out.
- **Declared — macOS and Windows are unchanged.** Read, not measured: on macOS `ps` shows every user's
  command lines, so the gap stands as ADR-053 wrote it; on Windows another user's command line needs a
  process handle a standard user is not given.
- **Declared — a relay the user runs vouches for whoever uses it.** A proxy or port forward running as this
  user (or as root) that will connect to `127.0.0.1` for another user makes that user's trade look like this
  user's.
- **Declared — another user can still spend the key, and gets nothing for it.** Offering the key on a
  connection and closing it at once reads as "no such socket", which trades: the token is written to nobody
  and the real window shows *Open Nib again*. Refusing "no such socket" would close this and would also
  refuse any peer the machine's own table does not hold; that was not shown to be an empty set (WSL2's
  mirrored networking, a cross-namespace forward), so it is not done.
- **A browser sent through a loopback proxy that runs as a third user is refused**, with the log naming both
  users. That proxy's owner reads the token in passing; the refusal is the point.
- Two netlink round trips per trade, on no path a window uses after its first request.
- The trampoline stays unbuilt. What it still needs: a snap Chromium and a snap Firefox (stock Ubuntu), and a
  flatpak Chromium, each shown to open the file in app mode from the directory chosen, and an answer for
  `xdg-open` on a machine whose HTML handler is not its browser.

## Verification

`internal/server/launchpeer_test.go`: a key offered on another user's connection is refused and the same key
then trades for this user; root, "no such socket" and "will not say" each trade; the question names the
request's own two ends. `launchpeer_linux_test.go`, with nothing replaced: this process's own peer is found
and owned by this user over IPv4 and IPv6; a peer bound to `lo` is still found; a closed peer is "no such
socket" and not root; a real trade is asked of the kernel and passes. Each was turned red by its own
mutation (no check; the key spent before the question; root refused; "gone" refused; no interface in the
lookup; the inode check removed; the two ends swapped; the owner read from the wrong offset). A real window
of each browser kind on the machine — Chrome and Chromium in app mode, Firefox through `xdg-open` — traded
through the built binary with the log silent.
