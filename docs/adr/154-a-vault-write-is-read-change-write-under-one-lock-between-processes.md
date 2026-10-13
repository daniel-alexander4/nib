# ADR-154 — a vault write is read, change, write under one lock between processes

**Status:** accepted
**Date:** 2026-10-12
**Supersedes:** ADR-144's "the stale-writer guard is a check, not a lock" and "only the key-removal route words
it for the user" — those two sentences only. ADR-144's decisions 1-5 stand, the generation and its "higher only"
rule included.
**Extends:** ADR-009 (one door: `beginWrite` for the lock and the read, `httpError` for the refusal's wording),
ADR-136 (a refusal recognised by its sentence at the one door every handler answers through).
**Context:** /pending 873. Three parts were filed; the trace found the first was one case of a wider one.
**Applies:** `internal/vault` (`beginWrite`, `refresh`, `headOf`, `persist`, `Replace`, `OpenSSHAt`, `Migrate`,
`filelock*.go`), `internal/server` (`vaulterror.go`, `httpError`), three toasts in `web/app.js`.

## What was measured

`save()` writes the whole vault from one process's memory. With two `*Vault` values on one file — two opens of the
lock file, so what two processes do — and no lock:

- process A pinned a peer and authorized a second key; process B recorded one recent file. The file then held
  **0 pinned peers and 1 key**: the new key's slot was gone, with no error anywhere. A's next write put both back
  and dropped B's recent file. This needs no key removal; it is every pair of writes.
- ADR-144's own window (a stale process between its generation check and its rename while a removal lands) is the
  same shape with a content key in it.

A lock around the check and the rename orders two such writes and loses the same update: the second writer still
writes a copy that predates the first. So the lock has to span the READ.

## Decision

1. **Every vault mutator begins with `beginWrite`**: `v.mu`, then the cross-process lock, then `refresh`, and only
   then does it judge anything (is this key already authorized, is there anything to prune). The lock is held to
   the end of that one write. `save()` refuses without it, so a new road to the file cannot skip it.
2. **`refresh` makes memory the file where another process wrote it.** Every save draws a fresh nonce, the third
   key of the envelope and inside the 128 bytes `generationOf` already read; a value remembers the nonce it last
   read or wrote. Same generation and same nonce: nothing to read — every write of a Nib running alone. Same
   generation, another nonce: the file is read whole, opened with this value's key, and becomes its contents and
   slots. Generation above: `ErrKeyChanged` (ADR-144). Below: memory is kept and the save restores the removal
   (ADR-144 decision 5).
3. **A file this process cannot open is not written over** — `ErrChangedElsewhere`. Another process imported a
   backup (a different content key at the same generation, which the generation cannot see), or the file is
   damaged where the head says another write happened. Before this the import was silently replaced.
4. **The lock is the kernel's, on a sibling file.** `vault.nib.lock`, created once and never removed; `flock`
   off Windows and `LockFileEx` on it, both released by the kernel when the holder exits or crashes. Nothing reads
   a pid or a marker, so a lock file left behind holds nothing. Not on `vault.nib` itself: a save replaces that
   inode. Asked for without blocking every 5 ms for up to 10 s, then `ErrBusy` — a save is 8-75 ms, 312 ms at
   worst.
5. **The other three roads to the file take the same lock.** `OpenSSHAt` and `Migrate` read the file, change it
   and write it back, so they hold the lock from the read (`persist(held)`); `Replace` holds it across its rename;
   `Create` is ordered by its `O_EXCL` placeholder and takes the lock for its write.
6. **A refused vault write says why on every route.** `ErrKeyChanged`, `ErrChangedElsewhere`, `ErrBusy` and
   `ErrReplaced` are recognised by sentence in `httpError` — 409, `cause: "vault-not-written"` — because handlers
   that reach the vault through another function voice its error inside a sentence of their own. A handler with a
   fixed "could not …" answers through `vaultWriteFailed`, which hands the sentence to the same recognition.
   409 because the key-removal route has always answered this refusal 409.

Cost, measured on one machine (n = 2000 / 200): the lock and its release 10 µs, the extra head read 16 µs, against
9.3 ms for the `AddRecent` they sit in.

## What it does not do

- **The read accessors are not refreshed.** A process shows the vault as of its own last write; a second
  process's pinned peer appears after this process next stores anything. Refreshing them is a file open on every
  `Settings()` and `Keys()` on the request path.
- **A build that predates this takes no lock and reads nothing.** Its writes still lose updates, and it still
  writes a generation-less file over a rotated one (873 part 2). Nothing in this build can stop a process that is
  already running. What would stop an old build from OPENING a rotated vault is writing envelope version 3 once
  the key has been replaced — every build since the version ceiling refuses a newer envelope. That is a stored
  format change, a rotated vault would stop opening on any machine not yet updated, and it is not done here.
- **A lower generation is still written over**, so a backup made before a removal and imported by ANOTHER process
  is replaced by a process past the removal. The generation cannot tell that from an old build's write-back.
- **A process refused with `ErrKeyChanged` or `ErrChangedElsewhere` stays refused until restarted.**
- **Filesystems that do not honour these locks between machines** — a synced folder, some network mounts.
- **Windows: compiled and vetted (`GOOS=windows go build ./...`, `go vet`), not run.** `LockFileEx` on one byte
  with `LOCKFILE_FAIL_IMMEDIATELY`; `ERROR_LOCK_VIOLATION` is "held".
- `identity()`'s callers still answer a fixed sentence; it writes the vault only when the first signing identity
  is minted.

## Refused

- **A lock around check-and-rename only** (what 873 proposed): closes ADR-144's window and leaves the lost update
  measured above.
- **Refusing any write over a file another process wrote**, instead of reading it: two Nibs on one directory
  would each freeze the other on every recent file.
- **Reading the file inside `mutateLocked`**: the mutators validate before the door and several hold an index
  into the contents; replacing the contents under them is wrong. The read is before the validation.
- **File identity (inode, size, mtime) as "which write"**: an inode number is reused as soon as the rename frees
  it. The nonce is already in the file and already random.

## Consequences

- `TestEveryVaultMutatorBeginsAWrite` holds the routing in the vault; `TestNoHandlerWordsAFailedVaultWriteWithoutTheVaultsSentence`
  holds it in the server. Both count what they matched.
- A config directory holds one more file, `vault.nib.lock`, empty.
- `envelope`'s field order now carries the nonce as well as `gen` for `headOf`.
