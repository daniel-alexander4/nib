package server

import (
	"os"
	"path/filepath"
	"strings"

	nibbrowser "nib/internal/browser"
	"nib/internal/vault"
)

// Where a downloaded update goes (ADR-102).
//
// Dan, 2026-10-07: *"nib download folder should always be the browser default folder unless
// configured differently in settings. It should not ask me where I want to download it."*
//
// **ONE door decides it, and the page never does** (ADR-009). The download route, the status the
// Settings card shows and the folder the popup names all come from `downloadDir`, so what is shown
// is what is written. Until ADR-102 the page sent the folder, defaulting to `~/nib`.

// The four places a folder can come from, in the order they are tried. Published on /api/status as
// `downloadDirFrom`; the page turns each into words.
const (
	downloadFromSetting = "setting" // Settings → Updates → Download folder
	downloadFromBrowser = "browser" // set in the browser Nib's window is running in
	downloadFromSystem  = "system"  // the system Downloads folder — every browser's own default
	downloadFromNib     = "nib"     // ~/nib, when none of the above is an existing folder
)

// downloadPlace is the door's answer.
type downloadPlace struct {
	Dir     string // absolute; exists, except for the last rung, which the download creates
	From    string // one of the four above
	Browser string // "Chrome", "Firefox", … when the window's browser is known; "" otherwise
}

// browserDownloadFolder and systemDownloads are the two readers of the machine. Vars so a test can
// answer for a browser without a browser — and so none reads the developer's own profile.
var (
	browserDownloadFolder = nibbrowser.DownloadFolder
	systemDownloads       = nibbrowser.SystemDownloads
)

// downloadDir resolves the folder a download is written to.
//
// **A folder that is not there is skipped, never created** — a folder the user named in Settings or
// in their browser and then removed, or one on a drive that is not plugged in, is not Nib's to bring
// back, and the next rung is used instead. Only `~/nib` is Nib's own to create, so the last rung
// always answers. The page shows the folder actually used, and says so when the setting was skipped.
func downloadDir(set vault.Settings) downloadPlace {
	if dir, ok := nibbrowser.UsableDir(set.DownloadDir); ok {
		return downloadPlace{Dir: dir, From: downloadFromSetting}
	}
	dir, name := browserDownloadFolder()
	// Checked again here though the reader checked it: the var above is the seam a test replaces,
	// and the rule that nothing is written to a folder that is not there belongs to the door.
	if dir, ok := nibbrowser.UsableDir(dir); ok {
		return downloadPlace{Dir: dir, From: downloadFromBrowser, Browser: name}
	}
	if dir, ok := nibbrowser.UsableDir(systemDownloads()); ok {
		return downloadPlace{Dir: dir, From: downloadFromSystem, Browser: name}
	}
	return downloadPlace{Dir: defaultOutputDir(), From: downloadFromNib, Browser: name}
}

// checkDownloadDirSetting validates a folder typed into Settings and returns the form stored.
//
// "" clears the setting, which returns the choice to the browser. Anything else must be a folder
// that is already there: the refusal is a sentence the field shows, so it says which thing is wrong.
func checkDownloadDirSetting(typed string) (stored, problem string) {
	typed = strings.TrimSpace(typed)
	if typed == "" {
		return "", ""
	}
	p := expandHome(typed)
	if len(p) > 4096 || strings.ContainsRune(p, 0) || !filepath.IsAbs(p) {
		return "", "Type the whole path to the folder, starting from the top — for example ~/Downloads."
	}
	p = filepath.Clean(p)
	info, err := os.Stat(p)
	switch {
	case err != nil:
		return "", "Nib could not find that folder. Check the spelling, or create it first."
	case !info.IsDir():
		return "", "That is a file, not a folder."
	}
	return p, ""
}
