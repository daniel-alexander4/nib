# ADR-214 — A save dialog starts in the document's own folder, and the server vets the suggestion

Status: accepted, 2026-11-07 (Dan: "if I opened a document from another directory, the save dialog for that document should open back to that dir, not ~/nib"). Extends ADR-009, ADR-212.

1. **The server names the folder** (`docResponse.dir`, from `document.folder()`): the folder of `path`, or of `srcPath` for a document converted from a photo or an
   Office file, which installs pathless on purpose (ADR-185) and so would otherwise lose its folder. The client never takes a dirname: only the server knows the
   separator. Empty for an upload, a URL, a combine, a scan and an arrival (the browser reveals no folder), and for a path that is not absolute (`destDir` refuses
   one at the write). No syscall: `docResponse` runs under `s.mu` on every answer.
2. **`GET /api/listdir?start=1` makes the path a suggestion** (`usableStart`). One that is gone, is a file, cannot be listed, or lies in Nib's ceremony store
   (`ceremony.InStore`, the question `recordRecent` asks) is answered as if nothing was asked: `~/nib`, with no error row. A path typed or clicked to (`start=0`)
   is unchanged, so a person can still browse into the store.
3. **One door in the window** (`startFolder`, app.js): Save As and both splits call it, and `test/jsdom/startfolder.test.mjs` fails a destination dialog that
   calls `browseDir('')`. Every export goes through `openSaveAs`, so every export starts there. Open keeps its own remembered folder.
4. **The `start` key is written FIRST in the query, with its ternary outside the literal.** The request-field scan (`requestfields_test.go`) cuts a literal at its
   first quote and at its last `?`; `start=${start ? 1 : 0}&path=…` made it read ` 1 : 0}` as a key and report `start` unsent. Red: the send deleted.
5. **Declared.** A folder that lists but cannot be written to (a read-only mount) opens fine and fails at Save with the existing toast. The dialog reads the active
   view when it opens, so switching tabs during a long export starts it in the new tab's folder (the folder is shown). `TestEveryPublishedObservableHasANamedReader`
   cannot tell `docMeta.dir` from any other `.dir` in app.js, so it is no reader for this field; the jsdom test is. Windows is compiled, not run.
