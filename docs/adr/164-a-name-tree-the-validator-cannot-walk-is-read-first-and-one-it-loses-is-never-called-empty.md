# ADR-164 — a name tree the validator cannot walk is read first, and one it loses is never called empty

**Status:** accepted
**Date:** 2026-10-12
**Extends:** ADR-129 (one door writes; the validator changes what it reads), ADR-136 (the write door refuses by
name, on a mark kept from the read), ADR-145 (what the validator would lose is read BEFORE validation),
ADR-072 (a refusal about a document is a 422 naming its cause), ADR-009 (one door). ADR-013 and ADR-143 are
untouched: `ContentDigestVersion` does not move.
**Context:** `/pending 880`. A document with 204 embedded files lost all of them, silently. Traced in pdfcpu
v0.13.0 and measured:

- **The writer.** `model.Node.Add` (`model/nameTree.go:309-353`) sends a key to a leaf and `HandleLeaf`
  (`:275-292`, `maxEntries = 3` at `:30`) splits a leaf of four into two leaves of two under the node that was
  the leaf. Nothing rebalances. Keys added in sorted order always land in the right-most leaf, so the tree
  grows one node deeper for every two keys: 204 keys → 101 deep, 1,000 → 499, 10,000 → 4,999 (the same in
  reverse order; in random order 16 and 23). A merge adds a source's tree key by key through the same `Add`
  (`AddTree`, `:356-379`), in key order.
- **The validator.** `validateNameTreeDepth` (`validate/nameTree.go:706-709`) refuses a node more than
  `MaxRecursionDepth` = 100 (`model/configuration.go:361`) below the root. In the relaxed mode nib reads in,
  the caller swallows the error and goes on to the next kid (`:750-756`) — after assigning the failed call's
  results, so the parent's upper bound is now the empty string, which `validateNameTreeDictLimitsEntry`
  (`:688-690`) also writes into the file's own `/Limits`. A tree whose bound is empty is not kept
  (`validate/xReftable.go:325-329`), and the catalog's `/Names` is deleted and rebuilt from what was kept
  (`:333-336`). So a tree too deep on its LAST kid is dropped whole, and one too deep on an earlier kid loses
  that kid and everything under it.
- **Measured through nib's own doors** (`pdfops.AddAttachment`, one file at a time, names in order): after the
  204th, `Attachments` lists 0 with no error; `pdfdetach -list` on the same bytes lists 204; `SetLang` then
  writes a document holding none; `ContentDigest` equals the digest of the document with no files (the digest
  HAS covered embedded files since rule 3 — two documents differing only in their attachments digested alike);
  `CeremonyRecord` answers "none". 1,000 files added in reverse order list 200. *Remove files & media* did
  remove them, by accident: the write dropped the tree whatever was asked.

**Applies:** `internal/pdfread` (`nametree.go`, `validated`, `Write`, `MergeRaw`), `internal/pdfops`
(`nameTreeEntries`, `CarryAttachments`, the merge, `StripActive`, `removeAllAttachments`), `internal/ceremony`
(`Extract`), `internal/server` (`httpError`).

## Decision

**1. Each catalog name tree is read before validation, by a walk of nib's own** (`settleNameTrees`, beside
`xfaIn`): a loop, not recursion; every node at most once; at most 2^20 names. **A tree read whole that nests
deeper than the validator follows is written flat into its own root dictionary, in the context, before the
validator sees it** — every key and value as the file wrote them, in key order. The validator then reads
every name and validates every value itself. This is not the put-back ADR-129 §2 refused at the read: nothing
is carried past the validator; the tree is given a shape it can walk, and a name tree's shape means nothing
(ISO 32000-1 §7.9.6). It applies to all ten trees the validator reads under `/Names`.

**2. What the validator still drops is marked, and never called empty** (`rememberNameTrees`). After
validation the names the context holds for each tree are counted against what the walk counted. Fewer — a
loop in `/Kids`, a kid the validator refused for a value in it, a key the validator will not keep — or a walk
that stopped at its bound, and the tree is marked unread against the context's cross-reference table.

- A READER asks (`pdfread.NameTreeUnread`) and answers with an error. `pdfops.nameTreeEntries`, the one
  enumeration of `/EmbeddedFiles` and `/JavaScript`, returns `ErrNameTreeUnread`: so do `Attachments`,
  `ReadAttachment`, `CeremonyRecord` and an attach's own duplicate check. `ContentDigest` hashes its existing
  `#unreadable-tree` marker, so such a document does not digest as one with no files. `ceremony.Extract`
  answers `ErrRecordUnreadable` and NOT `ErrNoRecord`, which every gate reads as "an ordinary PDF".
- A WRITE is refused: `pdfread.Write` returns `ErrNameTreeLost`, the server answers 422 with cause
  `name-tree`. Both merges ask `RefuseNameTreeLoss` of each source (a merge carries a source's trees and the
  mark does not follow them), and `CarryAttachments` refuses rather than carrying nothing.
- **The removal of that very tree is not refused** (`ForgetNameTree`, named at the two sites): *Remove files &
  media* and *Remove active content* take the mark off the tree they remove. The validator has already taken
  it out of the context and the write leaves it out, which is what was asked; `verifyStripped` still re-reads
  the output with a second reader.

**3. No write nests a name tree deeper than 32** (`shapeNameTrees`, called by `Write` and by nothing else).
A deeper tree in `ctx.Names` is rebuilt — the same keys and values in the same order, leaves of 64 under nodes
of 64, the root keeping its dictionary — before pdfcpu binds it. Whatever built the chain (an attach, a merge
of sorted destinations), nib does not write a tree an older nib, or pdfcpu, reads as empty. 32 is above
pdfcpu's own trees over random keys (16 at 1,000 names, 23 at 10,000) and a third of what the validator reads.

## Refused

- **Carrying a dropped tree across the validator and putting it back for the write** (ADR-129's shape). The
  validator deletes the catalog's key, so at the write "the validator took it" and "the operation removed it"
  are the same catalog — the reason ADR-136 and ADR-145 refused a carry — and a tree nobody validated would be
  read by `Attachments`, the digest and the record reader in between.
- **Refusing every deep tree at the write and reading none.** It would leave nib unable to open its own
  204-attachment documents for anything but reading, and would still list no files.
- **Patching pdfcpu's `Node.Add` to balance.** The write door already sees every tree pdfcpu is about to
  bind; a patched module is a second carried fork (ADR-142) for what forty lines at the door do.
- **A bound and a refusal on the number of attachments.** No bound exists: 10,000 files are written 2 deep.

## Consequences

- Measured after the change: 210 files attached one at a time list 210 at every step, and after an unrelated
  rewrite; a tree 150 deep holding one file (which no nib writer makes) lists it, digests with it, keeps it
  through a rewrite and loses it to *Remove files & media*; 203 / 204 / 1,000 / 10,000 names added in sorted,
  reverse and random order are written at most 32 deep and pdfcpu's validator alone reads every one back in
  order.
- **A document nib used to read as having no files now digests with them.** A ceremony record written over
  such a document by an earlier build (204 or more attachments at convene) no longer matches; the earlier
  value was the digest of a different document.
- A document with a looped or otherwise unreadable catalog name tree can be opened, read and stripped of that
  tree, and nothing else: no other operation writes it. On `~/nib/producers` and `~/nib/verapdfs` (333 files,
  23 with a catalog name tree, deepest 1, most names 567) no tree is marked.
- Cost, on every validating read of a document that has a `/Names` (one catalog lookup otherwise), a flat tree
  of 10 / 1,000 / 100,000 names on a loaded machine: the walk 6 µs / 0.48 ms / 127 ms and the count afterwards
  1.4 µs / 56 µs / 17 ms, against validated reads of 0.13 ms / 6.7 ms / 8.3 s.
- **The other trees, measured.** A number tree — `/PageLabels`, the structure `/ParentTree` — 150 deep is
  REFUSED by the validator with the whole document (`validate/numberTree.go:173`, `:199-202` returns the
  error), so nothing is lost silently; nib writes both flat. The structure root's `/IDTree` is a name tree the
  validator walks the same way and does not internalize (`validate/structTree.go:674`): 150 deep it stays in
  the document and is written with its one entry, but 100 of its nodes come back with an emptied upper
  `/Limits`. nib never deepens an `/IDTree` (`setIDTreeEntry` adds to a leaf). Declared, not fixed.
- **Declared.** A key of `/Names` the validator does not know is dropped by the same rebuild
  (`xReftable.go:300-305`) and is not marked. `xt.Names["Dests"]`'s direct readers (`anchors.go`,
  `outlinecarry.go`, `pageselect.go`) do not ask `NameTreeUnread`: on a marked document they resolve no named
  destination, and every write is refused. The mark does not follow pages into a cut or an imposition, which
  carry no catalog tree. A tree flattened at the read is written flat (one array) unless something deepens it.
  Handlers that do not pass the error text through `httpError` keep their own status, as under ADR-136.
- Guards: `TestATreeDeeperThanTheValidatorFollowsIsReadWhole`, `TestATreeTheValidatorDropsIsMarkedAndNotWritten`,
  `TestNoWriteNestsANameTreePastWhatAReaderFollows` (`internal/pdfread`);
  `TestTheTwoHundredAndFourthAttachmentDoesNotLoseTheOthers`, `TestADeepTreeHidesNoFileFromNib`,
  `TestAnUnreadTreeIsNeverCalledEmpty` (`internal/pdfops`);
  `TestADocumentWhoseFilesCouldNotBeReadIsNotAnOrdinaryPDF` (`internal/ceremony`);
  `TestAnUnreadNameTreeRefusalIsA422NamingItsCause` (`internal/server`).
