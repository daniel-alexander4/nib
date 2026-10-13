# ADR-153 — a merged-in alternate configuration shows of the host what the host's default shows

**Status:** accepted
**Date:** 2026-10-12
**Extends:** ADR-131 (a merge may not reveal what a merged document hid), closing its first declared gap and
amending decision 3's last bullet for `/AS` only. ADR-009 (one door).
**Context:** `/pending 870`. ADR-131 joined a merged-in document's optional-content groups to the host's and
left the source's ALTERNATE configurations (`/OCProperties /Configs`) behind, on the ground that "two documents'
lists of them have no pairing". The entry filed it as a judgment: a source alternate carried into the merged
document would have to say something about the HOST's groups, which it never held.

Read with ADR-131's own rule as the test:

- **Dropping them reveals nothing.** A configuration is a state a reader can switch TO; removing one removes a
  state, and the default — which decision 3 already keeps for every group — is untouched. What dropping costs is
  the source author's own named states: a "Print" that hides the working layers is gone from the merged file.
- **Carrying one naively DOES reveal.** A configuration's `/BaseState` (default `/ON`) applies to every group
  no list in it names (ISO 32000-1 §8.11.4.3, Table 101). The source's alternate names none of the host's groups,
  so appended as it stands it shows every layer the host hid — or, under `/BaseState /OFF`, hides every layer
  the host showed.
- **There is a state for the host's groups that neither reveals nor invents**, and it is decision 3 read the
  other way round: the state the host's OWN default gives them. ADR-131 already writes each source group into
  every host alternate in the source default's state; the mirror writes each host group into every source
  alternate in the host default's state. No pairing is needed, and the only states a reader can reach are ones
  an author chose for the groups that author owned.

Reading the code for the mirror found a second reveal of the same family in ADR-131 itself: a usage application
(`/AS`) can switch a group off with no list naming it, and the source `/D`'s `/AS` was appended to the host's
`/D` only — so under a HOST alternate, a merged-in group its own default hides by usage was shown.
**Applies:** `pdfread.UnionOptionalContent` — every loop that calls `pdfcpu.MergeXRefTables` (ADR-131).

## Decision

**1. The source's alternate configurations are carried.** Each is appended to the host's `/Configs`, after the
host's own, in the source's order.

**2. In each, the host's groups stand as the host's default has them.** A group the host's `/D` has off — in
its `/OFF`, or under its `/BaseState /OFF` and not in its `/ON` — is listed in the carried configuration's
`/OFF`; one it has on is listed in `/ON` wherever the carried configuration's `/BaseState` is not `/ON`. The
host's states are read BEFORE the source's groups join the host's lists. One function sorts a document's groups
by its default (`defaultStates`) and one writes them into a configuration (`giveStates`); both directions use
both.

**3. A default's usage applications go wherever its groups' states go.** The source `/D`'s `/AS` is appended to
every host configuration, and the host `/D`'s `/AS` to every carried source alternate. (ADR-131 appended the
source's to the host's `/D` alone.)

**4. A carried configuration's own entries are not restated.** Its `/Name`, `/BaseState`, `/ON`, `/OFF`,
`/Order`, `/Locked`, `/AS`, `/RBGroups`, `/Intent` and `/ListMode` name only the source's groups and stand; the
host's groups are not added to its `/Order`, so they are not in the layers panel under it — which hides a
control, never content.

**5. The host's own state is still never touched** (ADR-131 decision 4), and a later merge treats a carried
configuration as one of the host's: a third document's groups are written into it by ADR-131's decision 3.

## Declared gaps

- **Two documents' alternates are not paired.** A host "Print" and a source "Print" are two configurations with
  one name; each shows the other document's layers as that document's default does.
- **A group hidden only by an alternate of its own document** is shown under the other document's
  configurations, as it is under its own default (ADR-131's second gap, unchanged).
- **No reader nib measures against switches configurations.** pdf.js, poppler and Ghostscript read `/D` only, so
  this is held by what the file SAYS (the lists), not by pixels; ADR-131's pixel measurements are of `/D`.

## Consequences

- `Append`, `Combine`, `InsertPDF`, the two splits, `RedactPages` and `ImagesToPDF` carry a merged-in
  document's alternates. Where the host has no `/OCProperties` the source's dictionary is adopted whole, as
  before, alternates included.
- Test: `TestAMergedInAlternateConfigurationShowsOfTheHostWhatTheHostsDefaultShows`
  (`internal/pdfops/optionalmerge_test.go`), red under each of: host groups not written, either `/AS` not
  carried, the alternates dropped.
