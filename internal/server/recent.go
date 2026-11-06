package server

import (
	"nib/internal/ceremony"
	"nib/internal/vault"
)

// recordRecent is the ONE door a path reaches Open Recent through (/pending 490, ADR-009). Two routes
// open a document by a path the person or the OS named — `handleOpen` and `openHandedOff` — and both
// call it; `TestOnlyTheRecentDoorAddsToRecent` fails a third caller of `AddRecent`.
//
// **A ceremony's own copy is never listed** (Dan, 2026-09-14: "the ceremony process is more important
// than the open recent"). A recent entry would open the document without the ceremony's state, would go
// dead when close-out moves the folder (ADR-012), and would be a second way into a surface whose only
// door is the Signing Ceremonies panel. The refusal is by PATH, so typing a path inside the store into
// Open… is refused too, which is the walk-around a refusal by route would leave. A signed copy the
// person keeps elsewhere is an ordinary file and is listed.
//
// Best-effort: a locked vault has nowhere to write, and failing to remember a file changes nothing the
// person asked for. Routes with no path (a drop, an upload, a URL, a combine, an arrival) never reach
// this door; that gap is declared in /pending 490's follow-up, not hidden here.
func recordRecent(v *vault.Vault, path string) {
	if v == nil || path == "" {
		return
	}
	if root, err := ceremony.StoreRoot(); err == nil && ceremony.InStore(root, path) {
		return
	}
	_ = v.AddRecent(path)
}
