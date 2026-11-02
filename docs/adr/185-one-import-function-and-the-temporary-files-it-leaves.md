# ADR-185 — One import function, and the temporary files and processes it owns

**Status:** accepted
**Date:** 2026-11-01
**Context:** `PLAN-import-export.md` P01.S02a (D8, D9, D11, D13, D14, D31, D32, D33, D35, L1, R4–R9, X1–X3, Y4–Y5, Y10).
Four routes admit bytes — a path, an upload, a URL and the OS hand-off — and two more reach them. Each kept its own refusal: the same
Office file was converted at one door and refused at four, `handleOffice` and `cmdOffice` each carried their own copy of the language and
PDF/UA handling, and nothing owned what a conversion left behind. LibreOffice ran on `context.Background()`, eight double-clicked files
started eight of them, Quit and SIGTERM left `soffice.bin` running with the user's source in its temp directory, and four `os.MkdirTemp`
calls in the shared system temp directory named no owner.

## Decision

- **`pdfops.Import(ctx, src io.ReaderAt, size, name, ImportOptions)` is the one function** a non-PDF becomes a PDF through. The doors —
  `readInstallablePDF` (path, hand-off, reload), `importOrRefuse` (upload, URL, Import…) and Combine — say where the bytes are and who is
  asking. `asOpenableDocument`, the `openability` enum and `convertibleRefusal` are gone, and `TestEveryOpenRouteDecidesThroughTheImportFunction`
  fails on a server file that classifies or converts for itself. The source is a ReaderAt read **after** the gate, so a queued import holds a
  descriptor, not its bytes. The source is never written (L1): Import has no path to write to, and `TestNoRouteWritesTheFileItWasGiven` hashes it,
  its inode and its directory after every route and every failure.
- **Bytes decide, the name decides only what bytes cannot say** (D32). `sniffFamily` names image / package / legacy Office / RTF from the head;
  a name that contradicts it is `ErrExtensionDisagrees`, text with a NUL named `.md` is `ErrNotText`, a package's central directory is read
  (never decompressed) against the entry, decoded-size and ratio caps and against its own name (`word/`, `xl/`, `ppt/`, or an ODF `mimetype`).
  A cap refuses and never truncates. The caps are **provisional** (P01.S03 tunes them); each has a test at cap and cap+1.
- **A converted document is pathless and named `<base>.pdf`** (D11). A PDF is returned as it came, at once, and never waits for the gate.
- **The temp door.** `internal/tempdir` is the only place a temporary directory is made (`TestEveryTempDirectoryComesThroughTheDoor`). Liveness
  is a **kernel lock** on `<dir>/.live` (`internal/filelock`, the vault's primitive extracted — ADR-154 delegates to it), never a name, never an
  age: the sweep, run at every start including the CLI, removes exactly the directories whose lock it can take, acts only on real directories
  owned by this user at 0700 under a per-user root, and a directory with no `.live` is spared for a minute only to cover the window in which
  `New` is creating it. `Claim` makes a `nib-spill-…` directory the process's TMPDIR, so `multipart-*` overflow, pdfcpu's and Ghostscript's scratch
  files land under a lock and are swept; uploads spill past 8 MiB instead of holding 200 MiB in RAM.
- **One teardown.** `Server.Shutdown`, called from the single place Quit, the idle exit and SIGTERM arrive (`cmd/nib/main.go`), stops installs and
  kills every registered converter process group synchronously (bounded at 1 s). A converter's group id is also written beside its input
  (`.pgid`) so the next start's sweep can find an orphan: on Linux it scans `/proc` for members of that group that started no earlier than the
  directory and whose executable is a known converter. SIGKILL cannot be handled; the window until the next start is declared.
- **The gate.** `internal/gate`: two conversions at once, sixteen waiting, then `ErrQueueFull` by name; the budget starts at acquire; `Admit`
  (the server's `roomToOpen`) is asked after the wait; the slot is released by `defer`. It is taken once, at the top of `Import`, and **only for a
  format whose producer needs an outside tool** — native producers never wait. The OS hand-off waits however long the queue is (`WaitForever`),
  on a context detached from the launcher's 30 s wait. A request that goes away while queued or converting releases its slot and kills its tree.
- **LibreOffice is contained, and the plan's mechanism was falsified by measurement.** The plan said "links never updating". A loopback
  listener showed LibreOffice's defaults make requests while converting (a DOCX's linked picture: 2-3 hits) and that setting does **not** stop
  them; a manual proxy nothing listens on does (0 hits, with a positive control that requires the hits under an empty profile and a red proof that
  restoring the default turns it red). The profile also sets macro security to its highest level. A link to a file on the local disk is not a
  network fetch and is untouched: that is the package scan of X11(a), P04.S01's.
- **Converters get an allowlisted environment** (`toolEnv`): PATH, HOME, locale, XDG and fontconfig, the Windows roots, and a TMPDIR inside the job
  directory — no proxy variables, no `SSH_AUTH_SOCK`, no `NIB_*`. HOME stays real so installed fonts keep working (declared: an isolated HOME is
  stricter and costs that fidelity). `TestEveryProcessNibStartsIsDeclared` names every site that starts a process and requires a converter to
  set `Env`.
- **A memory ceiling is a watchdog over the converter's process group**, never an address-space limit (Chromium dumped core under `ulimit -v`).
  Linux only; the other build says so in a constant and a sentence (`CeilingEnforced`, `CeilingUnenforcedNote`).
- **"The same bytes" at every door means the same content** (`ContentDigest`): a PDF carries its creation time and a fresh `/ID`, so two
  conversions of one file never agree byte for byte.

## Declared limits

- The hand-off backlog's FIFO ahead of the gate, the window's list of unopened names and the per-item Cancel (Y4) are not built: a hand-off waits
  on the gate however long it is, but the window cannot yet list what is waiting. Folded into P01.S02b.
- Windows and macOS: no reaper for an orphaned converter, no memory ceiling, and `taskkill` is compiled and argv-tested only.
- An isolated HOME for converters; local-file links in Office documents (X11(a)); the 1,000-page model read (P02.S03c).
- `import.ok` / `import.refused` are atomics read by the per-door census; they are not yet on `/api/status`.
