//go:build !windows

package server

// browseRoots reports no roots off Windows. "/" already contains every mounted
// filesystem, so the parent walk reaches everything on its own and the browser
// needs no separate jump list; the UI renders nothing when this is empty.
func browseRoots() []string { return nil }
