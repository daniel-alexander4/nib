# ADR-040 — a converter nib cannot find is one door, and the claim is about the SEARCH

**Status:** accepted · v1.133.0

## Context

Nib shells out to two optional converters: LibreOffice for the twelve office extensions, and
Ghostscript for the general PDF/A path. Both are detected at runtime and never bundled.

Four sites classified "it is not there", each with its own `errors.Is` branch and its own
literal — `internal/server/office.go`, `internal/server/pdfa.go`, and two verbs in
`internal/cli/commands.go`. That produced **six strings and three different wordings per
tool**, and the sentinel's own text made two more that no user ever saw, because every site
substituted its own.

Worse, all six said *"is not installed"*, and nib could not know that. `exec.LookPath` answers
"is this name on PATH" and nothing else — not the registry, not `/Applications`. The stock
install of both tools is **off PATH on two of the three platforms nib ships to**:
`/Applications/LibreOffice.app/Contents/MacOS/soffice`, and `%ProgramFiles%\LibreOffice\…` on
Windows. So "not installed" and "installed where we did not look" were the same boolean, and
the same sentence.

This mattered little while the absence was silent — the file picker just narrowed to Markdown
and said nothing. It stops being tolerable the moment nib says it out loud, which is what the
user asked for: a line telling them the converter is missing, with a link to get it. A line is
a claim, and a claim has to be true for the user who already installed it.

`internal/browser` had already met this exact problem and solved it the other way:
`findChromium` tries `LookPath` **and then** absolute per-OS candidates, because a bundle
install is invisible to PATH.

## Decision

**One door classifies; every surface words it for its own audience.**
`pdfops.MissingToolFor(err) (MissingTool, bool)` returns a typed kind. The HTTP doors, the CLI
verbs and the File card each write their own sentence from it. This is ADR-009 as this repo
reads it, stated where `readInstallablePDF` is consumed: *"ADR-009 unifies the CHECKS; it
explicitly does not require every site to print the same sentence."* The shape is a direct copy
of `pathRefusal{kind, status, msg}`, whose `openHandedOff` switches on `kind` and speaks in its
own voice.

**Discovery looks where the tools actually are.** `internal/pdfops/toolpath.go` tries PATH
first, then per-OS install locations, expressed as **globs** because Windows Ghostscript lives
in a version-numbered directory (`gs\gs10.03.1\bin`). Roots come from `%ProgramFiles%` and
friends rather than a hardcoded `C:`. The fallback is **monotonic** — it can only ever find
more than the old probe, and a wrong candidate stats false and costs nothing.

**A candidate must be RUNNABLE, not merely present.** `browser.fileExists` is `os.Stat` +
`!IsDir()` while its comment claims "a regular, runnable file", and that shape is deliberately
not copied. Promoting a present-but-unrunnable install is the one failure worse than reporting
absent: it widens the file picker and then an unrunnable `soffice` that starts and hangs burns
the full two-minute timeout and returns *"timed out converting this document"* — no path, no
reason. Mode bits narrow that window; a quarantined `.app`, an ACL'd WindowsApps install and a
`noexec` mount still pass and fail at exec, and that residue is stated rather than papered over.

**A found path is cached for the process; an empty answer is re-probed on every call.** The
old `sync.Once` cached both, so a user who installed the converter while nib was running stayed
refused until the process ended — and *"restart Nib"* is not a simple instruction here: a
relaunch **hands off to the running instance** (`handedOff`), and idle-exit needs every window
closed plus a ten-second grace and never fires at all in a `noBrowser` run. Measured: a miss
costs ~107 µs (two names) / ~159 µs (three) on a 19-entry PATH, against `/api/status`, which is
on no timer and is fetched about once per page load — the ~1 s poller is `/api/session/status`,
a different route. That asymmetry is what makes the UI's **Check again** honest.

**The claim is about the search.** Every surface says nib *could not find* the tool, never that
it is not installed.

**The remedy URL is a client-side constant and never travels on the wire.** The web client's
link is authored statically in `index.html`. ADR-039 refused a route that accepts a URL because
it "would be a general 'fetch this and write it to my disk' primitive, with the request body
choosing both host and path"; the same reasoning forbids a response body choosing where the
page **navigates**. The CLI prints `MissingTool.Vendor()`, which is the same fact reachable from
code rather than from a response.

## Consequences

- Office conversion starts working on macOS and Windows machines that had it all along.
- The File card states the gap and offers **Check again**; the PDF/A modal gains the branch it
  never had, where Ghostscript is absent — that refusal was previously **unreachable from the
  GUI**, because `#pdfaGsGo` is revealed only when Ghostscript is present.
- The missing-converter branch becomes testable for the first time: `lookTool` is a pure
  function, so the absent case is driven directly rather than skipped. No reset seam is added
  to shipped code, which `build/redproof.sh` forbids.
- `TestEveryMissingToolRefusalGoesThroughOneDoor` asserts routing, with a floor — because once
  the door absorbs the sentinels, "no site names them" is also what a scan that stopped
  matching would report.
- **Not addressed, deliberately:** a dropped `.docx` is still discarded silently, and
  `/api/open` still refuses one with "that file isn't a PDF" — which it does identically when
  LibreOffice *is* installed, so neither is a missing-converter defect. Both are filed
  separately.
