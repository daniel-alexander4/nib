//go:build !windows

package browser

import "path/filepath"

// systemFolder is the user's Downloads or Desktop folder as the desktop defines it.
//
// Linux asks `user-dirs.dirs`, which is where a renamed or translated folder is recorded
// (`Téléchargements`), and falls back to the English name every browser falls back to. macOS has no
// such file: the folders are ~/Downloads and ~/Desktop whatever the Finder displays them as.
func systemFolder(kind folderKind, e env) string {
	key, name := "DOWNLOAD", "Downloads"
	if kind == desktopFolder {
		key, name = "DESKTOP", "Desktop"
	}
	if e.goos == "linux" {
		if d := xdgUserDir(e, key); d != "" {
			return d
		}
	}
	return filepath.Join(e.home, name)
}

// defaultBrowserID names the browser a tab opens in.
//
// **macOS answers "" and that is a declared gap, not an oversight** (ADR-102): the record is a
// binary property list under ~/Library/Preferences/com.apple.LaunchServices, and Nib has no reader
// for that format. A tab is opened there only when none of Chrome, Edge, Brave or Chromium is in
// /Applications, so the browser is Safari or Firefox, and the caller falls to the system Downloads
// folder — which is what both use unless the user changed it.
func defaultBrowserID(e env) string {
	if e.goos == "linux" {
		return linuxDefaultBrowser(e)
	}
	return ""
}
