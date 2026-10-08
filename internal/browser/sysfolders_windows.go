//go:build windows

package browser

import (
	"path/filepath"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

// systemFolder is the user's Downloads or Desktop folder as Windows defines it: the known folder,
// which follows a folder the user has moved to another drive, and the profile's own sub-folder if
// the shell will not say.
func systemFolder(kind folderKind, e env) string {
	id, name := windows.FOLDERID_Downloads, "Downloads"
	if kind == desktopFolder {
		id, name = windows.FOLDERID_Desktop, "Desktop"
	}
	if p, err := windows.KnownFolderPath(id, 0); err == nil && p != "" {
		return p
	}
	return filepath.Join(e.home, name)
}

// defaultBrowserID names the browser a tab opens in: the ProgId the user chose for `http`
// (`ChromeHTML`, `MSEdgeHTM`, `FirefoxURL-…`). "" when nothing is recorded.
func defaultBrowserID(e env) string {
	k, err := registry.OpenKey(registry.CURRENT_USER,
		`Software\Microsoft\Windows\Shell\Associations\UrlAssociations\http\UserChoice`, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer k.Close()
	id, _, err := k.GetStringValue("ProgId")
	if err != nil {
		return ""
	}
	return id
}
