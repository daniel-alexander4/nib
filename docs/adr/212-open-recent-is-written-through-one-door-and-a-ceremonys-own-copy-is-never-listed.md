# ADR-212 — Open Recent is written through one door, and a ceremony's own copy is never listed

Status: accepted, 2026-11-05 (/pending 490, v2.49.17). Narrows /pending 490's first design; the vault format is unchanged.

1. **`recordRecent` is the ONE door** a path reaches Open Recent through (`internal/server/recent.go`). The two routes that open a document by a path
   the person or the OS named, `handleOpen` and `openHandedOff`, call it; `TestOnlyTheRecentDoorAddsToRecent` fails a third caller of `AddRecent`.
   Before this, the hand-off (a double-clicked file) was never listed, and `handleOpen` listed any path it was given.
2. **A ceremony's own copy is never listed** (Dan, 2026-09-14). The refusal is by path, `ceremony.InStore(root, path)`: inside `ceremonies/` or `ended/`
   under the store root, compared as written and with the folder's symlinks resolved. A recent entry would open the document without the ceremony's
   state, would go dead when close-out moves the folder (ADR-012), and would be a second way into a surface whose only door is the Signing Ceremonies
   panel. A signed copy kept outside those folders is an ordinary file and is listed. Before this, a path typed into Open… inside the store was listed.
3. **Typed entries (`{kind, ref, name}`) and a `Contents.Version` bump were not built.** The routes that would need them have no path to record: a drop and
   an upload carry bytes (a browser never gives the source path), a URL is not a file, and a combine or an arrival has no source file until it is saved.
   A URL would also store its query string, which can hold a credential, in the vault. These are the follow-up, not a gap in this decision.
4. **Verified** red against each half: the ceremony check removed (live, ended and hand-off rows list the path), the hand-off call removed, `ended/`
   dropped from the store folders, the symlink resolution dropped, and a direct `AddRecent` call added.
5. **A file named `.pdf` written by Save As is listed** (`handleWriteFile`, /pending 957, v2.49.18; decided with Dan 2026-11-05). It is the one way a dropped file, an
   upload, a URL document or a combine gets a path; it goes through the same door, so a PDF saved inside the ceremony folder is refused and an export in
   another format (Markdown, a ZIP) is not listed. **A URL document is never listed**, saved or not until then: a URL can carry a credential in its query string, and
   stripping it can break the link. So there are no typed entries and still no vault change. Red against: the call removed, the PDF test dropped to
   "any file name".
