# ADR-144 — removing an authorized key replaces the vault's content key

**Status:** accepted
**Date:** 2026-10-12
**Extends:** ADR-009 (one door: `mutateLocked` for a vault change, `atomicfile` for a durable write), ADR-073
(how a durable write is described: staged, synced, then renamed).
**Context:** /pending 668 #6 (P06 review R5). The vault's contents are encrypted under one random content key,
and each authorized SSH key has a slot holding that key sealed to it. `RemoveKey` dropped the slot and nothing
else. The removed machine still had its SSH private key, and any `vault.nib` copied while its slot existed
gives it the content key — which never changed, so it opened every vault written afterwards, ceremony secrets
pruned since included. The window said removing a key revoked its access. Dan decided 2026-10-12: rotate.
**Applies:** `internal/vault` (`RemoveKey`, `resealSlots`, `mutateLocked`, `save`, `beforeReplacing`,
`generationOf`), `internal/atomicfile` (`WriteDurableChecked`), `internal/server/keys.go`, the Authorized keys
wording in `web/`.

## Decision

1. **`RemoveKey` draws a new content key** (`crypto/rand`, as `newSealed` draws the first), encrypts the
   contents under it and seals it to every slot that stays. The removed key's old slot now unwraps to a key
   that opens nothing written from then on.
2. **Values only — no format change.** A remaining slot is rebuilt from the `PubKey` it stores (age, to an SSH
   recipient); `PubKey` and `KeyPath` are carried. `newSealed` is not used: it enrols the built-in keys and would
   put back a built-in slot the user removed.
3. **All or nothing.** A slot that cannot be sealed to refuses the whole removal and names that slot
   (`ErrKeyNotSealable`, 409) — skipping it would lock a second key out unasked. The change goes through
   `mutateLocked`, which now snapshots and puts back the key (its slice and its bytes), its generation and
   `unproven` beside the contents and slots; the door census counts all three.
4. **The old file stays until the new one has been opened.** `atomicfile.WriteDurableChecked` asks one question
   between the staging and the rename. For a key that has opened nothing on disk (`unproven`: after a removal,
   and a vault just sealed by Create or Migrate) the STAGED file is read back and must carry the generation and
   slots that were written and decrypt with the new key to a payload this build decodes.
5. **A generation in the envelope stops a stale writer.** `gen` (`omitempty`) counts key replacements. No lock
   is held on the vault file, so a second Nib process opened before the removal holds the old key and the old
   slots, and its next save of anything would write them back. Every save reads the file's generation and
   refuses (`ErrKeyChanged`) when it is above the one this process last read or wrote. A lower one is not
   refused: that is a stale writer having got there first, and this process's save is what restores the removal.

`gen` is not a format break. The envelope version stays 2. A build without the field ignores it when reading
and omits it when writing; a file without it reads as 0; a vault never rotated is written without it.

## What it does not do

- **It takes back nothing the removed key could already open.** A copy of the vault file made before the
  removal still opens with it, and the signing identity key, an imported P12 and ceremony secrets inside may
  have been copied. After a lost machine the answer is a new signing identity (/pending 872), not this.
- **Importing a backup made before the removal** brings the old key and the removed slot back.
- **The other slots are not proven to unwrap** — their private keys are on other machines. They are compared
  byte for byte with what was sealed.
- **The stale-writer guard is a check, not a lock.** A stale process that passes its check and renames after a
  rotation's rename overwrites the rotated file; the rotating process's next save restores it and nothing says
  so in between. A build that predates `gen` is not stopped at all. Closing either needs a cross-process lock.
- A process refused with `ErrKeyChanged` stays refused until it is restarted; only the key-removal route words
  it for the user, every other route answers as it does for any failed vault write.

## Refused

- **Parsing the whole file for the generation.** Measured: 12.7 µs / 1.4 ms / 33.7 ms at 621 B / 274 KB / 7.3 MB,
  against saves of 7.8 / 10.5 / 74.7 ms — on every `AddRecent`. `generationOf` reads the first 128 bytes: every
  writer is `json.Marshal` of `envelope`, which puts `gen` directly after `version`. Measured 7–10 µs at all
  three sizes. A file some other program wrote with the key further in reads as 0 and is not defended.
- **Proving every save against its staged file.** The read-back took a save from 10.5 to 12.7–14.3 ms at 274 KB
  and from 52–75 to 170–312 ms at 7.3 MB (two runs each). It is paid where the key is new, and nowhere else.
- **Refusing on any generation that differs.** It would stop the rotating process repairing a file an older
  build wrote back.

## Who can write the vault file (traced)

`save()` is called by `mutateLocked` (every mutator) and `persist` (Create, OpenSSHAt, Migrate — each on a vault
nobody else holds yet). `Replace` (the backup import) writes bytes the caller validated and retires the open
vault; it asks no question. Outside the package the vault is opened only in `internal/server/auth.go`
(`grep -rn 'vault\.\(Open\|Create\|Migrate\|AutoSetup\|Replace\)' --include='*.go'`, tests excluded); the CLI
never opens it. So one process holds one `*Vault`, and a second writer is a second Nib process on the same
config directory — or an older build.

## Consequences

- The window's sentence is the true one: a removed key cannot open the vault as saved from then on, and what it
  could already open is not taken back.
- Removing a key costs one read-back of the vault file. Every save costs one open and a 128-byte read.
- `envelope`'s field order is now load-bearing for `generationOf`; `TestAStaleProcessCannotWriteItsOldKeyOverTheRotatedFile`
  goes red if `gen` moves behind the ciphertext.
