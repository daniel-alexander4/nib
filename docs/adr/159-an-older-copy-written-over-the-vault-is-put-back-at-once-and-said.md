# ADR-159 — an older copy written over the vault is put back at once, and said

**Status:** accepted
**Date:** 2026-10-12
**Supersedes:** ADR-154 decision 2's last clause ("Below: memory is kept and the save restores the removal") and
ADR-144 decision 5's "its save is what restores the removal" — the WHEN and the silence only. A lower generation
is still written over; ADR-154's refusal of envelope version 3 stands, and this is what was chosen instead of it.
**Extends:** ADR-154 (`beginWrite` is where a write learns what the file is), ADR-086 (the window stream carries
what the machine has to say unasked), ADR-009 (one door: `restoreOverOlderCopy`).
**Context:** /pending 873 part 2. A build that predates the generation and the lock — every published build to
v1.208.8 — checks the envelope version only when it OPENS a vault. One that is already running writes its whole
in-memory copy over the file at every save: the old content key, and the slot of a key removed since.
**Applies:** `internal/vault` (`beginWrite`, `refresh`, `restoreOverOlderCopy`, `OlderCopyRestored`,
`NotifyOlderCopyRestored`), `internal/server` (`vaultrestored.go`, `handleWindow`, `adoptVault`), `web/app.js`
(`reflectVaultRestored`).

## What was read, and measured

- `refresh` answered a generation below this process's with `nil`: memory was kept and whatever save came next
  put the rotated vault back. Nothing recorded that it had happened and nothing told anyone.
- **That save did not always come.** A mutator judges after `beginWrite` and several return without saving — a
  key already authorized, nothing to prune. Measured with `PruneCeremonyPeers` on a ceremony that holds nothing,
  over a pre-removal file written back onto a rotated vault: the call answered `0, nil` and the file stayed at
  generation 0 with three slots — the removed key authorized, the file under the old content key — until some
  later write happened to change something.
- The older writer goes on. Each of its saves writes the old copy back; a restore nobody hears about is undone at
  that program's next save, for as long as it runs.

## Decision

1. **The restore is a write of its own, in `beginWrite`.** `refresh` reports a lower generation
   (`olderCopy`) and `beginWrite` writes memory back through `restoreOverOlderCopy` — the mutation door with an
   empty change — before the mutator judges anything. The mutator then runs as usual. One more durable write, on
   an event that needs two programs on one config directory and a key removal between them.
2. **It is counted, in memory.** `Vault.OlderCopyRestored()` gives the count and the time of the last one, from
   two atomics. Only a restore that LANDED is counted: one that fails is the mutator's error, by every route's
   usual road (ADR-154 decision 6), and the next write finds the older copy again. Not stored in the vault and
   not kept across a restart: it is something this process saw about a process that is running now.
3. **The window is told on its stream, once per restore.** `event: vault`, carrying a `noticeView`
   (`what: "vault-restored"`, the sentence, what to do, the restore's time). The vault wakes the stream through
   `NotifyOlderCopyRestored`, installed in `adoptVault`; the state is read from the vault and sent on difference,
   so a window that opens afterwards hears the last one too. The page shows it in `#sessionNotice` — the alert
   that stays until dismissed — keyed on the time in `sessionStorage`, so a reconnect or a reload does not repeat
   it. Not `/api/status` (read about once a page load), not the session poll (runs only while this window is
   armed), not a refusal (the write succeeded).
4. **The words name both things a lower generation can be.** The generation moves only at a key removal, so the
   file is either an older Nib's write-back or a backup, made before the removal, that ANOTHER process imported.
   Nothing in the file tells them apart — both are the old key and the old slots. So: *"Another Nib wrote an
   older copy of your vault over it after a key was removed, and Nib has put the removal back — close any other
   Nib that is still running — an older version does this each time it saves. If you imported a backup in
   another Nib just now, it was replaced: import it again in this one"*.
5. **An import by THIS process is not one.** `Replace` retires the value that did the removal and the vault
   opened from the imported file starts at that file's generation; nothing is restored over it and nothing is
   said (`TestAnImportByThisProcessIsNotAnOlderCopy`).

## What it does not do

- **Detection is at this process's next vault write, not before.** An older copy sits on disk, unreported, for
  as long as this process stores nothing. Reading the file's head on a timer or on the read accessors is the
  request-path cost ADR-154 refused.
- **Nothing stops the older program.** It is told nothing and writes again at its next save; this build restores
  again and says so again. The sentence asks the user to close it.
- **What the older writer stored in between is lost** — its recent file, its pinned peer. Its copy is under a
  content key this process no longer holds, so there is nothing to merge.
- **A backup imported by another process is still replaced** (ADR-154), now with a sentence saying so.
- **With no window open** — the last-open record written as the last window closes — the restore is counted and
  the next window to connect is told.

## Refused

- **Envelope version 3 after a rotation** (ADR-154's parked option; Dan, 2026-10-12): it stops an old build from
  OPENING a rotated vault and does nothing about one already running, which is the case here; and a rotated
  vault would stop opening on every machine not yet updated.
- **Refusing the write instead of restoring**: this process would stop storing anything while the removed key
  stayed authorized on disk.
- **A new status field or a new poll** for one rare event, beside a stream every window already holds.

## Consequences

- `refresh` returns `(olderCopy bool, err error)`. `TestEveryVaultMutatorBeginsAWrite` names
  `restoreOverOlderCopy` as `beginWrite`'s own second half.
- The window stream carries a fourth event. `noticeView` gains a second producer and no field.
- Tests: `TestAnOlderCopyWrittenOverTheVaultIsRestoredAndCounted`,
  `TestAWriteThatChangesNothingStillRestoresTheOlderCopy`, `TestARestoreThatFailsIsTheWritesErrorAndIsNotCounted`,
  `TestAnImportByThisProcessIsNotAnOlderCopy` (vault); `TestAnOlderCopyWrittenOverTheVaultIsSaidToEveryWindow`
  (server); `test/jsdom/handoffpush.test.mjs` (window).
