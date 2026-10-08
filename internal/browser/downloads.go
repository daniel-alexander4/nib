package browser

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
)

// The folder the browser Nib's window is running in downloads to (ADR-102).
//
// Dan, 2026-10-07: *"nib download folder should always be the browser default folder unless
// configured differently in settings."* He means it literally — the browser's own setting, not a
// stand-in for it — so this file reads that setting from the browser's preference file.
//
// ── What is read, and nothing else ───────────────────────────────────────────
// Chrome, Chromium, Edge, Brave: `profile.last_used` from `Local State`, then
// `download.default_directory` from that profile's `Preferences`.
// Firefox: the default profile's path from `profiles.ini`, then `browser.download.folderList` and
// `browser.download.dir` from its `prefs.js`.
// Linux only: the `x-scheme-handler/http` line of `mimeapps.list` (which browser a tab opens in) and
// the `XDG_DOWNLOAD_DIR` / `XDG_DESKTOP_DIR` lines of `user-dirs.dirs`.
// Each file is decoded into those keys alone; no other value in it is kept, logged or sent.
//
// ── Every value is another program's text ────────────────────────────────────
// A preference file is written by the browser, by a sync service, by an extension's installer, by
// anything running as the user. So a folder read from one is used only if it is an absolute path to
// a directory that exists (`UsableDir`); a file over `maxPrefBytes`, one that does not parse, and a
// path that fails that test all mean "nothing set" — never an error anyone sees. Nothing here runs
// anything or writes anything.
//
// ── What it cannot know, declared ────────────────────────────────────────────
//   - A window this process did not open (a hand-typed URL, a headless run, a second launch that
//     surfaced the window) has no recorded browser: nothing is read, and the caller uses the system
//     Downloads folder.
//   - On macOS a tab's browser is not identified (the default-browser record is a binary property
//     list this does not parse). A tab is only opened there when no Chromium-family browser is
//     installed, and the system Downloads folder is what Safari uses unless told otherwise.
//   - A browser this does not name (Waterfox, LibreWolf, Vivaldi, Opera, Safari) is not read.

// maxPrefBytes bounds each preference file read. Measured on the development machine: two Chrome
// `Preferences` files of 54 KB and 127 KB, a `Local State` of 12 KB, three Firefox `prefs.js` of
// 9–53 KB. 8 MiB is far above all of them and is what stops a file somebody grew on purpose from
// being pulled into memory on every status request.
const maxPrefBytes = 8 << 20

// maxPathBytes bounds a folder read from a preference file. PATH_MAX on Linux.
const maxPathBytes = 4096

// Window is how this process's window was opened: an app-mode window of the Chromium-family binary
// at Path, or (AppMode false) a tab in whatever the default browser is.
type Window struct {
	AppMode bool
	Path    string
}

var opened struct {
	mu  sync.Mutex
	win Window
	ok  bool
}

// recordOpened notes the branch `Open` took. Before ADR-102 that was only logged.
func recordOpened(w Window) {
	opened.mu.Lock()
	opened.win, opened.ok = w, true
	opened.mu.Unlock()
}

// Opened reports how this process opened its window, and false if it never opened one.
func Opened() (Window, bool) {
	opened.mu.Lock()
	defer opened.mu.Unlock()
	return opened.win, opened.ok
}

// DownloadFolder is the download folder SET in the browser this process opened its window in, and
// that browser's name ("Chrome", "Firefox", …).
//
// dir is "" when that browser has none set, or names one that is not an existing directory; name is
// "" when the browser is not known at all. Both empty is the ordinary answer, not a failure.
func DownloadFolder() (dir, name string) {
	w, ok := Opened()
	if !ok {
		return "", ""
	}
	return downloadFolderFor(w, hostEnv())
}

// SystemDownloads is the user's Downloads folder as the desktop defines it — what every browser
// named here uses when nothing is set in it. It may not exist; the caller checks.
func SystemDownloads() string {
	return systemFolder(downloadsFolder, hostEnv())
}

// UsableDir cleans p and reports whether it is an absolute path to an existing directory. It is the
// one test a folder Nib was TOLD about must pass before anything is written into it.
func UsableDir(p string) (string, bool) {
	if p == "" || len(p) > maxPathBytes || strings.ContainsRune(p, 0) || !filepath.IsAbs(p) {
		return "", false
	}
	p = filepath.Clean(p)
	info, err := os.Stat(p)
	if err != nil || !info.IsDir() {
		return "", false
	}
	return p, true
}

// env is everything about the machine this file reads, gathered in one value so a test can point it
// at a temp directory instead of the developer's own browser profile.
type env struct {
	goos       string
	home       string
	configHome string   // Linux: $XDG_CONFIG_HOME, else ~/.config
	localApp   string   // Windows: %LOCALAPPDATA%
	roamingApp string   // Windows: %APPDATA%
	sysDirs    []string // Linux: system directories that may hold a mimeapps.list
	// defaultID names the default browser in the desktop's own terms — a .desktop file name on
	// Linux, a ProgId on Windows — or "" when that cannot be told.
	defaultID func(env) string
}

func hostEnv() env {
	e := env{goos: runtime.GOOS, defaultID: defaultBrowserID}
	e.home, _ = os.UserHomeDir()
	e.configHome = os.Getenv("XDG_CONFIG_HOME")
	if !filepath.IsAbs(e.configHome) && e.home != "" {
		e.configHome = filepath.Join(e.home, ".config")
	}
	e.localApp = os.Getenv("LOCALAPPDATA")
	e.roamingApp = os.Getenv("APPDATA")
	e.sysDirs = []string{"/etc/xdg", "/usr/local/share/applications", "/usr/share/applications"}
	return e
}

type folderKind int

const (
	downloadsFolder folderKind = iota
	desktopFolder
)

func downloadFolderFor(w Window, e env) (dir, name string) {
	if e.home == "" {
		return "", ""
	}
	id := w.Path
	if !w.AppMode {
		if e.defaultID == nil {
			return "", ""
		}
		id = e.defaultID(e)
	}
	product := productOf(id)
	if product == "" {
		return "", ""
	}
	name = productNames[product]
	for _, root := range dataRoots(product, packagingOf(id), e) {
		if info, err := os.Stat(root); err != nil || !info.IsDir() {
			continue
		}
		// The first data folder that exists is the browser's; what it says is the answer, set or not.
		if product == "firefox" {
			return firefoxFolder(root, e), name
		}
		return chromiumFolder(root), name
	}
	return "", name
}

var productNames = map[string]string{
	"chrome": "Chrome", "chromium": "Chromium", "edge": "Edge", "brave": "Brave", "firefox": "Firefox",
}

// productOf names the browser a binary path, a .desktop file name or a Windows ProgId belongs to,
// or "" for one this file does not read. Only the last path element is judged, so a browser is
// never recognised from the folder it happens to be installed under.
func productOf(id string) string {
	base := id
	if i := strings.LastIndexAny(base, `/\`); i >= 0 {
		base = base[i+1:]
	}
	base = strings.ToLower(base)
	switch {
	case strings.Contains(base, "chromium"):
		return "chromium"
	case strings.Contains(base, "chrome"):
		return "chrome"
	case strings.Contains(base, "brave"):
		return "brave"
	case strings.Contains(base, "edge"):
		return "edge"
	case strings.Contains(base, "firefox"):
		return "firefox"
	}
	return ""
}

// packagingOf says whether a Linux browser is a snap or a flatpak, from where its binary is or how
// its .desktop file is named (`firefox_firefox.desktop` is the snap's, `org.mozilla.firefox.desktop`
// the flatpak's). It only ORDERS the folders tried: a machine upgraded from a packaged Firefox to
// the snap keeps the old ~/.mozilla beside the live one.
func packagingOf(id string) string {
	switch {
	case strings.HasPrefix(id, "/snap/"), strings.HasPrefix(id, "/var/lib/snapd/"):
		return "snap"
	case strings.ContainsAny(id, `/\`):
		return ""
	case strings.Contains(id, "_"):
		return "snap"
	case strings.Count(strings.TrimSuffix(id, ".desktop"), ".") >= 2:
		return "flatpak"
	}
	return ""
}

// dataRoots lists where a browser keeps its profiles on this OS, the likeliest first.
//
// The Linux and macOS/Windows native locations are each browser's documented default. The snap and
// flatpak ones are from those packages' layouts and were NOT exercised on a machine that has them.
func dataRoots(product, packaging string, e env) []string {
	type root struct{ dir, kind string }
	var roots []root
	add := func(kind string, parts ...string) { roots = append(roots, root{filepath.Join(parts...), kind}) }
	switch e.goos {
	case "darwin":
		lib := filepath.Join(e.home, "Library", "Application Support")
		switch product {
		case "chrome":
			add("", lib, "Google", "Chrome")
		case "chromium":
			add("", lib, "Chromium")
		case "edge":
			add("", lib, "Microsoft Edge")
		case "brave":
			add("", lib, "BraveSoftware", "Brave-Browser")
		case "firefox":
			add("", lib, "Firefox")
		}
	case "windows":
		if product == "firefox" {
			if e.roamingApp != "" {
				add("", e.roamingApp, "Mozilla", "Firefox")
			}
			break
		}
		if e.localApp == "" {
			break
		}
		switch product {
		case "chrome":
			add("", e.localApp, "Google", "Chrome", "User Data")
		case "chromium":
			add("", e.localApp, "Chromium", "User Data")
		case "edge":
			add("", e.localApp, "Microsoft", "Edge", "User Data")
		case "brave":
			add("", e.localApp, "BraveSoftware", "Brave-Browser", "User Data")
		}
	default:
		flat := filepath.Join(e.home, ".var", "app")
		switch product {
		case "chrome":
			add("", e.configHome, "google-chrome")
			add("flatpak", flat, "com.google.Chrome", "config", "google-chrome")
		case "chromium":
			add("", e.configHome, "chromium")
			add("snap", e.home, "snap", "chromium", "common", "chromium")
			add("flatpak", flat, "org.chromium.Chromium", "config", "chromium")
		case "edge":
			add("", e.configHome, "microsoft-edge")
			add("flatpak", flat, "com.microsoft.Edge", "config", "microsoft-edge")
		case "brave":
			add("", e.configHome, "BraveSoftware", "Brave-Browser")
			add("snap", e.home, "snap", "brave", "current", ".config", "BraveSoftware", "Brave-Browser")
			add("flatpak", flat, "com.brave.Browser", "config", "BraveSoftware", "Brave-Browser")
		case "firefox":
			add("", e.home, ".mozilla", "firefox")
			add("snap", e.home, "snap", "firefox", "common", ".mozilla", "firefox")
			add("flatpak", flat, "org.mozilla.firefox", ".mozilla", "firefox")
		}
	}
	var first, rest []string
	for _, r := range roots {
		if packaging != "" && r.kind == packaging {
			first = append(first, r.dir)
		} else {
			rest = append(rest, r.dir)
		}
	}
	return append(first, rest...)
}

// readCapped returns a file's bytes, or nil if it is missing, unreadable or over maxPrefBytes.
func readCapped(path string) []byte {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxPrefBytes+1))
	if err != nil || len(b) > maxPrefBytes {
		return nil
	}
	return b
}

// plainName reports whether s is one path element and nothing else — the test a profile name read
// from a file must pass before it is joined onto a folder.
func plainName(s string) bool {
	return s != "" && s != "." && s != ".." && len(s) <= 255 && !strings.ContainsAny(s, "/\\\x00")
}

// chromiumFolder reads `download.default_directory` for the profile last used under root.
func chromiumFolder(root string) string {
	profile := "Default"
	var state struct {
		Profile struct {
			LastUsed string `json:"last_used"`
		} `json:"profile"`
	}
	if b := readCapped(filepath.Join(root, "Local State")); b != nil {
		if json.Unmarshal(b, &state) == nil && plainName(state.Profile.LastUsed) {
			profile = state.Profile.LastUsed
		}
	}
	var prefs struct {
		Download struct {
			DefaultDirectory string `json:"default_directory"`
		} `json:"download"`
	}
	b := readCapped(filepath.Join(root, profile, "Preferences"))
	if b == nil || json.Unmarshal(b, &prefs) != nil {
		return ""
	}
	dir, _ := UsableDir(prefs.Download.DefaultDirectory)
	return dir
}

// firefoxFolder reads the default profile's download folder under root.
//
// `browser.download.folderList` is 0 for the Desktop, 1 for the system Downloads folder and 2 for
// `browser.download.dir`; absent is 1. For 1 this returns "" — "nothing set" — and the caller's next
// rung is that same Downloads folder.
func firefoxFolder(root string, e env) string {
	profile := firefoxProfile(root)
	if profile == "" {
		return ""
	}
	b := readCapped(filepath.Join(profile, "prefs.js"))
	if b == nil {
		return ""
	}
	const listKey, dirKey = `user_pref("browser.download.folderList",`, `user_pref("browser.download.dir",`
	list, dir := 1, ""
	for _, line := range bytes.Split(b, []byte("\n")) {
		s := strings.TrimSpace(string(line))
		switch {
		case strings.HasPrefix(s, listKey):
			v := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s[len(listKey):]), ");"))
			if n, err := strconv.Atoi(v); err == nil {
				list = n
			}
		case strings.HasPrefix(s, dirKey):
			v := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(s[len(dirKey):]), ");"))
			// Firefox writes the value as a double-quoted string with backslash escapes, which is
			// the form strconv reads. One that does not read that way is not a folder.
			if u, err := strconv.Unquote(v); err == nil {
				dir = u
			} else {
				dir = ""
			}
		}
	}
	switch list {
	case 0:
		d, _ := UsableDir(systemFolder(desktopFolder, e))
		return d
	case 2:
		d, _ := UsableDir(dir)
		return d
	}
	return ""
}

// firefoxProfile picks the profile Firefox opens by default from `profiles.ini`: the one an
// `[Install…]` section names, else the `[Profile…]` marked `Default=1`, else the only one.
//
// A relative path must stay under root. An absolute one is allowed — Firefox allows it — and all
// that is ever done with it is read one file named `prefs.js` from it.
func firefoxProfile(root string) string {
	b := readCapped(filepath.Join(root, "profiles.ini"))
	if b == nil {
		return ""
	}
	var installDefault, marked string
	var paths []string
	section, path, isDefault := "", "", false
	flush := func() {
		if strings.HasPrefix(section, "Profile") && path != "" {
			paths = append(paths, path)
			if isDefault && marked == "" {
				marked = path
			}
		}
		path, isDefault = "", false
	}
	for _, line := range bytes.Split(b, []byte("\n")) {
		s := strings.TrimSpace(string(line))
		if strings.HasPrefix(s, "[") && strings.HasSuffix(s, "]") {
			flush()
			section = s[1 : len(s)-1]
			continue
		}
		k, v, ok := strings.Cut(s, "=")
		if !ok {
			continue
		}
		switch {
		case strings.HasPrefix(section, "Install") && k == "Default" && installDefault == "":
			installDefault = v
		case strings.HasPrefix(section, "Profile") && k == "Path":
			path = v
		case strings.HasPrefix(section, "Profile") && k == "Default":
			isDefault = v == "1"
		}
	}
	flush()
	pick := installDefault
	if pick == "" {
		pick = marked
	}
	if pick == "" && len(paths) == 1 {
		pick = paths[0]
	}
	if pick == "" || len(pick) > maxPathBytes || strings.ContainsRune(pick, 0) {
		return ""
	}
	if filepath.IsAbs(pick) {
		return filepath.Clean(pick)
	}
	full := filepath.Join(root, filepath.FromSlash(pick))
	if rel, err := filepath.Rel(root, full); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return ""
	}
	return full
}

// linuxDefaultBrowser is the .desktop file a `http:` link opens with, read from the first
// `mimeapps.list` that names one — the user's, then the system's — in the order the desktop's own
// opener looks. "" when none does.
func linuxDefaultBrowser(e env) string {
	dirs := []string{e.configHome, filepath.Join(e.home, ".local", "share", "applications")}
	dirs = append(dirs, e.sysDirs...)
	for _, d := range dirs {
		if d == "" {
			continue
		}
		b := readCapped(filepath.Join(d, "mimeapps.list"))
		if b == nil {
			continue
		}
		section := ""
		for _, line := range bytes.Split(b, []byte("\n")) {
			s := strings.TrimSpace(string(line))
			if strings.HasPrefix(s, "[") {
				section = s
				continue
			}
			if section != "[Default Applications]" {
				continue
			}
			k, v, ok := strings.Cut(s, "=")
			if !ok || strings.TrimSpace(k) != "x-scheme-handler/http" {
				continue
			}
			// A list is allowed (`a.desktop;b.desktop`); the first is the one used.
			first, _, _ := strings.Cut(strings.TrimSpace(v), ";")
			if first = strings.TrimSpace(first); first != "" {
				return first
			}
		}
	}
	return ""
}

// xdgUserDir reads one folder from `user-dirs.dirs` — `XDG_DOWNLOAD_DIR="$HOME/Downloads"` — the
// file the desktop writes and every browser here reads for its default. Parsed, never run: the
// file is shell syntax, and the only forms the format allows are `"$HOME/…"` and `"/…"`.
//
// A folder set to the home directory itself is the format's way of saying "none", and reads as "".
func xdgUserDir(e env, name string) string {
	b := readCapped(filepath.Join(e.configHome, "user-dirs.dirs"))
	if b == nil {
		return ""
	}
	key, found := "XDG_"+name+"_DIR", ""
	for _, line := range bytes.Split(b, []byte("\n")) {
		k, v, ok := strings.Cut(strings.TrimSpace(string(line)), "=")
		if !ok || strings.TrimSpace(k) != key {
			continue
		}
		v = strings.TrimSpace(v)
		if len(v) < 2 || v[0] != '"' || v[len(v)-1] != '"' {
			continue
		}
		v = v[1 : len(v)-1]
		var out strings.Builder
		for i := 0; i < len(v); i++ {
			if v[i] == '\\' && i+1 < len(v) {
				i++
			}
			out.WriteByte(v[i])
		}
		v = out.String()
		switch {
		case v == "$HOME" || v == "$HOME/":
			found = ""
		case strings.HasPrefix(v, "$HOME/"):
			found = filepath.Join(e.home, v[len("$HOME/"):])
		case filepath.IsAbs(v):
			found = filepath.Clean(v)
		}
	}
	if found == filepath.Clean(e.home) {
		return ""
	}
	return found
}
