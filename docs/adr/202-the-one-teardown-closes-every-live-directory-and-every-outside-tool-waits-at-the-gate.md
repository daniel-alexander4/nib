# ADR-202 — The one teardown closes every live directory, and every outside tool waits at the gate

Status: accepted, 2026-11-04 (/pending 940, review residue of v2.33.0). Extends ADR-185, ADR-009.

**A registry of live directories.** `tempdir.New` enters each `Dir` in a set and `Close` removes it, under a mutex that is
never held across a removal. `tempdir.CloseAll` snapshots the set and closes each outside the lock, and `Server.Shutdown` calls
it AFTER `KillConverters`, so nothing is writing into a directory as it goes. `Close` is once-guarded: an owner closing at the
same moment waits for the first and removes nothing twice. The claimed spill directory is skipped; it is `Release`'s, last. A
directory made after `CloseAll` is not refused (refusing would poison every later test in a process that called it); the
process is leaving. Liveness stays the kernel lock — a closed directory is simply removed early.

**The PDF/A Ghostscript route takes ctx and a gate slot.** `ConvertPDFAGhostscript(ctx, pdf)` runs under `ImportGate.RunWait`
(a person pressed the button; no refusal for a long queue), its two-minute budget starting at the slot, and the request's context
reaches the process kill path. A closed request, and Ctrl-C at the command line, end the group.

**The command line is interruptible.** `cli.interruptContext` (SIGINT/SIGTERM, a second signal restored to default) is what
`import`/`office`, `merge` and `pdfa --gs` hand to `pdfops`.

**Declared gaps are said.** `pdfops.DeclaredLimits` is the one wording of "no reaper" and "no memory ceiling" off Linux; `cmd/nib`
logs it at a desktop start. The window has no surface for it.
