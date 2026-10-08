package browser

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// The browser's own download folder (ADR-102).
//
// **Every test here builds its own machine in a temp directory.** The code under test reads a
// browser's preference files, and a test that let it reach the developer's real profile would pass
// or fail on what that person's Chrome happens to be set to — and would be reading it. `testEnv`
// is the only source of paths.

func testEnv(t *testing.T, goos string) env {
	t.Helper()
	home := t.TempDir()
	return env{
		goos:       goos,
		home:       home,
		configHome: filepath.Join(home, ".config"),
		localApp:   filepath.Join(home, "AppData", "Local"),
		roamingApp: filepath.Join(home, "AppData", "Roaming"),
		sysDirs:    []string{filepath.Join(home, "sys")},
		defaultID:  func(env) string { return "" },
	}
}

func put(t *testing.T, path string, content []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatal(err)
	}
}

func mkdir(t *testing.T, path string) string {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func chromePrefs(dir string) []byte {
	return []byte(`{"browser":{"x":1},"download":{"default_directory":` + jsonString(dir) + `,"prompt_for_download":true}}`)
}

func jsonString(s string) string {
	return `"` + strings.NewReplacer(`\`, `\\`, `"`, `\"`).Replace(s) + `"`
}

// One row per browser per OS: the binary Nib found, and where that browser keeps its profiles.
func TestAChromiumFamilyBrowsersFolderIsReadFromItsOwnProfile(t *testing.T) {
	for _, tc := range []struct {
		goos, binary, name string
		root               func(e env) string
	}{
		{"linux", "/usr/bin/google-chrome", "Chrome", func(e env) string { return filepath.Join(e.configHome, "google-chrome") }},
		{"linux", "/usr/bin/google-chrome-stable", "Chrome", func(e env) string { return filepath.Join(e.configHome, "google-chrome") }},
		{"linux", "/usr/bin/chromium", "Chromium", func(e env) string { return filepath.Join(e.configHome, "chromium") }},
		{"linux", "/snap/bin/chromium", "Chromium", func(e env) string { return filepath.Join(e.home, "snap/chromium/common/chromium") }},
		{"linux", "/usr/bin/microsoft-edge", "Edge", func(e env) string { return filepath.Join(e.configHome, "microsoft-edge") }},
		{"linux", "/usr/bin/brave-browser", "Brave", func(e env) string { return filepath.Join(e.configHome, "BraveSoftware/Brave-Browser") }},
		{"darwin", "/Applications/Google Chrome.app/Contents/MacOS/Google Chrome", "Chrome", func(e env) string { return filepath.Join(e.home, "Library/Application Support/Google/Chrome") }},
		{"darwin", "/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge", "Edge", func(e env) string { return filepath.Join(e.home, "Library/Application Support/Microsoft Edge") }},
		{"darwin", "/Applications/Brave Browser.app/Contents/MacOS/Brave Browser", "Brave", func(e env) string {
			return filepath.Join(e.home, "Library/Application Support/BraveSoftware/Brave-Browser")
		}},
		{"darwin", "/Applications/Chromium.app/Contents/MacOS/Chromium", "Chromium", func(e env) string { return filepath.Join(e.home, "Library/Application Support/Chromium") }},
		{"windows", `C:\Program Files\Google\Chrome\Application\chrome.exe`, "Chrome", func(e env) string { return filepath.Join(e.localApp, "Google/Chrome/User Data") }},
		{"windows", `C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`, "Edge", func(e env) string { return filepath.Join(e.localApp, "Microsoft/Edge/User Data") }},
		{"windows", `C:\Program Files\BraveSoftware\Brave-Browser\Application\brave.exe`, "Brave", func(e env) string { return filepath.Join(e.localApp, "BraveSoftware/Brave-Browser/User Data") }},
		{"windows", "chromium.exe", "Chromium", func(e env) string { return filepath.Join(e.localApp, "Chromium/User Data") }},
	} {
		e := testEnv(t, tc.goos)
		want := mkdir(t, filepath.Join(e.home, "Saved Files"))
		put(t, filepath.Join(tc.root(e), "Default", "Preferences"), chromePrefs(want))
		dir, name := downloadFolderFor(Window{AppMode: true, Path: tc.binary}, e)
		if dir != want || name != tc.name {
			t.Errorf("%s %s: got (%q, %q), want (%q, %q)", tc.goos, tc.binary, dir, name, want, tc.name)
		}
	}
}

func TestTheProfileLastUsedIsTheOneRead(t *testing.T) {
	e := testEnv(t, "linux")
	root := filepath.Join(e.configHome, "google-chrome")
	inDefault := mkdir(t, filepath.Join(e.home, "from-default"))
	inWork := mkdir(t, filepath.Join(e.home, "from-work"))
	put(t, filepath.Join(root, "Default", "Preferences"), chromePrefs(inDefault))
	put(t, filepath.Join(root, "Profile 2", "Preferences"), chromePrefs(inWork))
	win := Window{AppMode: true, Path: "/usr/bin/google-chrome"}

	if dir, _ := downloadFolderFor(win, e); dir != inDefault {
		t.Errorf("with no Local State got %q, want the Default profile's %q", dir, inDefault)
	}
	put(t, filepath.Join(root, "Local State"), []byte(`{"profile":{"last_used":"Profile 2"}}`))
	if dir, _ := downloadFolderFor(win, e); dir != inWork {
		t.Errorf("with last_used = Profile 2 got %q, want %q", dir, inWork)
	}
	// The profile NAME is another program's text too. One that is not a plain name is not joined
	// onto anything: here it points at a Preferences file outside the browser's folder.
	outside := mkdir(t, filepath.Join(e.home, "from-outside"))
	put(t, filepath.Join(e.configHome, "elsewhere", "Preferences"), chromePrefs(outside))
	for _, hostile := range []string{"../elsewhere", filepath.Join(e.configHome, "elsewhere"), "..", ""} {
		put(t, filepath.Join(root, "Local State"), []byte(`{"profile":{"last_used":`+jsonString(hostile)+`}}`))
		if dir, _ := downloadFolderFor(win, e); dir != inDefault {
			t.Errorf("last_used = %q read %q, want the Default profile's %q", hostile, dir, inDefault)
		}
	}
}

// What a preference file is allowed to make Nib do: nothing, unless it names a folder that is
// really there. Each of these must read as "nothing set", with the browser still named.
func TestAHostilePreferenceFileSetsNothing(t *testing.T) {
	e := testEnv(t, "linux")
	root := filepath.Join(e.configHome, "google-chrome")
	prefs := filepath.Join(root, "Default", "Preferences")
	win := Window{AppMode: true, Path: "/usr/bin/google-chrome"}
	real := mkdir(t, filepath.Join(e.home, "real"))
	file := filepath.Join(e.home, "a-file")
	put(t, file, []byte("x"))

	// The control: the same fixture with an honest value is read. Without it every case below
	// could pass because the fixture is in the wrong place.
	put(t, prefs, chromePrefs(real))
	if dir, _ := downloadFolderFor(win, e); dir != real {
		t.Fatalf("control: an honest Preferences read %q, want %q", dir, real)
	}

	huge := append(chromePrefs(real)[:len(chromePrefs(real))-1], []byte(`,"pad":"`+strings.Repeat("a", 10<<20)+`"}`)...)
	for name, content := range map[string][]byte{
		"a relative path":           chromePrefs("Downloads"),
		"a relative path with dots": chromePrefs("../../real"),
		"a folder that is missing":  chromePrefs(filepath.Join(e.home, "missing")),
		"dots into a missing place": chromePrefs(filepath.Join(real, "..", "missing")),
		"a file, not a folder":      chromePrefs(file),
		"an empty value":            chromePrefs(""),
		"a NUL in the path":         []byte(`{"download":{"default_directory":"` + real + `\u0000x"}}`),
		"a number, not a string":    []byte(`{"download":{"default_directory":7}}`),
		"invalid JSON":              []byte(`{"download":{"default_directory":` + jsonString(real)),
		"not JSON at all":           []byte("\x00\x01\x02 rm -rf ~"),
		"a 10 MB file":              huge,
		"a path longer than a path": chromePrefs("/" + strings.Repeat("a/", 3000)),
	} {
		put(t, prefs, content)
		dir, who := downloadFolderFor(win, e)
		if dir != "" || who != "Chrome" {
			t.Errorf("%s: read (%q, %q), want nothing set in Chrome", name, dir, who)
		}
	}
	// The 10 MB case is only a test of the cap if the file WOULD have been read without it.
	if !bytes.Contains(huge, []byte(real)) || len(huge) <= maxPrefBytes {
		t.Fatal("the oversize fixture is not a valid oversize fixture")
	}

	// A path with dots that cleans to a real folder is that folder — the browser itself would
	// download there — and what comes back is the cleaned form, which is what the popup names.
	put(t, prefs, chromePrefs(filepath.Join(real, "sub", "..")))
	if dir, _ := downloadFolderFor(win, e); dir != real {
		t.Errorf("a dotted path to a real folder read %q, want the cleaned %q", dir, real)
	}
}

const firefoxProfiles = `[Install4F96D1932A9F858E]
Default=Profiles/abc.default-release
Locked=1

[Profile1]
Name=default
IsRelative=1
Path=Profiles/old.default
Default=1

[Profile0]
Name=default-release
IsRelative=1
Path=Profiles/abc.default-release

[General]
StartWithLastProfile=1
Version=2
`

func firefoxEnv(t *testing.T, id string) (env, string) {
	e := testEnv(t, "linux")
	e.defaultID = func(env) string { return id }
	return e, filepath.Join(e.home, ".mozilla", "firefox")
}

func TestFirefoxsFolderFollowsItsFolderList(t *testing.T) {
	e, root := firefoxEnv(t, "firefox.desktop")
	put(t, filepath.Join(root, "profiles.ini"), []byte(firefoxProfiles))
	prefs := filepath.Join(root, "Profiles", "abc.default-release", "prefs.js")
	custom := mkdir(t, filepath.Join(e.home, "My \"Saved\" Files"))
	desktop := mkdir(t, filepath.Join(e.home, "Bureau"))
	put(t, filepath.Join(e.configHome, "user-dirs.dirs"), []byte("XDG_DESKTOP_DIR=\"$HOME/Bureau\"\n"))
	// The profile marked Default=1 is NOT the one read when an install names another: a wrong
	// pick here answers with this folder.
	put(t, filepath.Join(root, "Profiles", "old.default", "prefs.js"),
		[]byte(`user_pref("browser.download.folderList", 2);`+"\n"+`user_pref("browser.download.dir", `+jsonString(mkdir(t, filepath.Join(e.home, "old")))+`);`+"\n"))
	dirLine := `user_pref("browser.download.dir", ` + jsonString(custom) + `);` + "\n"

	for _, tc := range []struct{ name, prefs, want string }{
		{"2 is the folder it names", `user_pref("browser.download.folderList", 2);` + "\n" + dirLine, custom},
		{"0 is the Desktop, as the desktop names it", `user_pref("browser.download.folderList", 0);` + "\n" + dirLine, desktop},
		{"1 is the system Downloads folder: nothing set", `user_pref("browser.download.folderList", 1);` + "\n" + dirLine, ""},
		{"absent is 1", `user_pref("app.update.auto", false);` + "\n" + dirLine, ""},
		{"2 with no folder named", `user_pref("browser.download.folderList", 2);` + "\n", ""},
		{"2 naming a relative folder", `user_pref("browser.download.folderList", 2);` + "\n" + `user_pref("browser.download.dir", "Downloads");` + "\n", ""},
		{"2 naming a missing folder", `user_pref("browser.download.folderList", 2);` + "\n" + `user_pref("browser.download.dir", "/no/such/folder");` + "\n", ""},
		{"a value that is not a quoted string", `user_pref("browser.download.folderList", 2);` + "\n" + `user_pref("browser.download.dir", ` + custom + `);` + "\n", ""},
		// The size cap, on a format where a cut-off file still parses: the folder is in the first
		// lines and ten megabytes of comment follow. Too large is "nothing set", not "read the start".
		{"a 10 MB prefs.js", `user_pref("browser.download.folderList", 2);` + "\n" + dirLine + "// " + strings.Repeat("a", 10<<20) + "\n", ""},
	} {
		put(t, prefs, []byte("// Mozilla User Preferences\n"+tc.prefs))
		dir, name := downloadFolderFor(Window{}, e)
		if dir != tc.want || name != "Firefox" {
			t.Errorf("%s: got (%q, %q), want (%q, Firefox)", tc.name, dir, name, tc.want)
		}
	}
}

func TestFirefoxsProfileIsPickedTheWayFirefoxPicksIt(t *testing.T) {
	e, root := firefoxEnv(t, "firefox.desktop")
	mk := func(profile string) string {
		dir := mkdir(t, filepath.Join(e.home, "dl-"+filepath.Base(profile)))
		put(t, filepath.Join(profile, "prefs.js"),
			[]byte(`user_pref("browser.download.folderList", 2);`+"\n"+`user_pref("browser.download.dir", `+jsonString(dir)+`);`+"\n"))
		return dir
	}
	a := mk(filepath.Join(root, "Profiles", "a"))
	b := mk(filepath.Join(root, "Profiles", "b"))
	abs := filepath.Join(e.home, "profile-elsewhere")
	c := mk(abs)
	outside := filepath.Join(e.home, ".mozilla", "outside")
	mk(outside)

	for _, tc := range []struct{ name, ini, want string }{
		{"the install's default", "[Install1]\nDefault=Profiles/b\n[Profile0]\nPath=Profiles/a\nDefault=1\n[Profile1]\nPath=Profiles/b\n", b},
		{"else the one marked default", "[Profile0]\nPath=Profiles/a\n[Profile1]\nPath=Profiles/b\nDefault=1\n", b},
		{"else the only one", "[Profile0]\nPath=Profiles/a\n", a},
		{"two and neither marked: none", "[Profile0]\nPath=Profiles/a\n[Profile1]\nPath=Profiles/b\n", ""},
		{"an absolute profile path is allowed", "[Profile0]\nIsRelative=0\nPath=" + abs + "\nDefault=1\n", c},
		{"a relative one may not leave the folder", "[Profile0]\nIsRelative=1\nPath=../outside\nDefault=1\n", ""},
		{"an empty file", "", ""},
	} {
		put(t, filepath.Join(root, "profiles.ini"), []byte(tc.ini))
		if dir, _ := downloadFolderFor(Window{}, e); dir != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, dir, tc.want)
		}
	}
}

// A machine upgraded from a packaged Firefox to the snap keeps the old ~/.mozilla beside the live
// one, so which is read follows how the DEFAULT browser is packaged.
func TestASnapOrFlatpakBrowserIsReadFromItsOwnFolder(t *testing.T) {
	prefsFor := func(dir string) []byte {
		return []byte(`user_pref("browser.download.folderList", 2);` + "\n" + `user_pref("browser.download.dir", ` + jsonString(dir) + `);` + "\n")
	}
	for _, tc := range []struct{ id, live string }{
		{"firefox.desktop", ".mozilla/firefox"},
		{"firefox_firefox.desktop", "snap/firefox/common/.mozilla/firefox"},
		{"org.mozilla.firefox.desktop", ".var/app/org.mozilla.firefox/.mozilla/firefox"},
	} {
		e, _ := firefoxEnv(t, tc.id)
		for _, rel := range []string{".mozilla/firefox", "snap/firefox/common/.mozilla/firefox", ".var/app/org.mozilla.firefox/.mozilla/firefox"} {
			root := filepath.Join(e.home, rel)
			put(t, filepath.Join(root, "profiles.ini"), []byte("[Profile0]\nPath=p\n"))
			put(t, filepath.Join(root, "p", "prefs.js"), prefsFor(mkdir(t, filepath.Join(e.home, "dl", rel))))
		}
		want := filepath.Join(e.home, "dl", tc.live)
		if dir, _ := downloadFolderFor(Window{}, e); dir != want {
			t.Errorf("default browser %s: read %q, want %q", tc.id, dir, want)
		}
	}
}

func TestATabsBrowserIsTheDesktopsDefault(t *testing.T) {
	e := testEnv(t, "linux")
	user := filepath.Join(e.configHome, "mimeapps.list")
	sys := filepath.Join(e.sysDirs[0], "mimeapps.list")
	put(t, sys, []byte("[Default Applications]\ntext/html=firefox.desktop\nx-scheme-handler/http=firefox.desktop\n"))
	if got := linuxDefaultBrowser(e); got != "firefox.desktop" {
		t.Errorf("with only the system list got %q, want firefox.desktop", got)
	}
	// The user's own list is asked first, only its [Default Applications] counts, and a list of
	// handlers means the first.
	put(t, user, []byte("[Added Associations]\nx-scheme-handler/http=brave-browser.desktop;\n\n"+
		"[Default Applications]\nx-scheme-handler/https=chromium.desktop\nx-scheme-handler/http=google-chrome.desktop;firefox.desktop;\n"))
	if got := linuxDefaultBrowser(e); got != "google-chrome.desktop" {
		t.Errorf("with a user list got %q, want google-chrome.desktop", got)
	}

	// And through the whole reader: a tab in Chrome reads Chrome's folder.
	e.defaultID = linuxDefaultBrowser
	want := mkdir(t, filepath.Join(e.home, "chrome-dl"))
	put(t, filepath.Join(e.configHome, "google-chrome", "Default", "Preferences"), chromePrefs(want))
	if dir, name := downloadFolderFor(Window{}, e); dir != want || name != "Chrome" {
		t.Errorf("a tab in the default browser read (%q, %q), want (%q, Chrome)", dir, name, want)
	}

	// A browser this does not name — measured: the development machine's default is a Waterfox
	// launcher — is not read and not named, though the Chrome profile above is sitting right there.
	put(t, user, []byte("[Default Applications]\nx-scheme-handler/http=userapp-Waterfox-N0BYH3.desktop\n"))
	if dir, name := downloadFolderFor(Window{}, e); dir != "" || name != "" {
		t.Errorf("an unnamed default browser read (%q, %q), want nothing", dir, name)
	}
}

func TestTheSystemFoldersComeFromTheDesktopsOwnFile(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows asks the shell for its known folders; user-dirs.dirs is the Linux desktop's file")
	}
	e := testEnv(t, "linux")
	file := filepath.Join(e.configHome, "user-dirs.dirs")
	for _, tc := range []struct{ name, content, want string }{
		{"no file: the English name", "", filepath.Join(e.home, "Downloads")},
		{"a translated folder", "# comment\nXDG_DESKTOP_DIR=\"$HOME/Bureau\"\nXDG_DOWNLOAD_DIR=\"$HOME/Téléchargements\"\n", filepath.Join(e.home, "Téléchargements")},
		{"an absolute folder", "XDG_DOWNLOAD_DIR=\"/mnt/big/dl\"\n", "/mnt/big/dl"},
		{"an escaped character", "XDG_DOWNLOAD_DIR=\"$HOME/a\\\"b\"\n", filepath.Join(e.home, `a"b`)},
		{"set to home means none", "XDG_DOWNLOAD_DIR=\"$HOME/\"\n", filepath.Join(e.home, "Downloads")},
		{"set to home by its whole path means none", "XDG_DOWNLOAD_DIR=\"" + e.home + "/\"\n", filepath.Join(e.home, "Downloads")},
		{"a relative value is not a folder", "XDG_DOWNLOAD_DIR=\"dl\"\n", filepath.Join(e.home, "Downloads")},
		{"an unquoted value is not read", "XDG_DOWNLOAD_DIR=$HOME/dl\n", filepath.Join(e.home, "Downloads")},
		// Shell syntax is never run: a command substitution is text, and text that is not a path.
		{"a command is not run", "XDG_DOWNLOAD_DIR=\"$(touch " + filepath.Join(e.home, "ran") + ")\"\n", filepath.Join(e.home, "Downloads")},
	} {
		if tc.content == "" {
			_ = os.Remove(file)
		} else {
			put(t, file, []byte(tc.content))
		}
		if got := systemFolder(downloadsFolder, e); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
	if _, err := os.Stat(filepath.Join(e.home, "ran")); err == nil {
		t.Error("reading user-dirs.dirs ran what was in it")
	}
	// macOS has no such file, and one lying around is not asked.
	mac := testEnv(t, "darwin")
	put(t, filepath.Join(mac.configHome, "user-dirs.dirs"), []byte("XDG_DOWNLOAD_DIR=\"/mnt/big/dl\"\n"))
	if got := systemFolder(downloadsFolder, mac); got != filepath.Join(mac.home, "Downloads") {
		t.Errorf("macOS: got %q, want ~/Downloads", got)
	}
}

// TestOpenRecordsWhichBrowserItOpened drives the real `Open` against stand-in programs on PATH —
// a script named `google-chrome`, and one named `xdg-open` — because the fact under test is which
// BRANCH ran, and that used to be a log line and nothing else.
func TestOpenRecordsWhichBrowserItOpened(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("the stand-ins are shell scripts found on PATH, which is how the Linux candidates are found")
	}
	reset := func() {
		opened.mu.Lock()
		opened.win, opened.ok = Window{}, false
		opened.mu.Unlock()
	}
	t.Cleanup(reset)
	reset()
	if _, ok := Opened(); ok {
		t.Fatal("a process that opened nothing reports a window")
	}
	// With nothing opened nothing is read — a headless run, or a URL typed by hand.
	if dir, name := DownloadFolder(); dir != "" || name != "" {
		t.Errorf("with no window opened DownloadFolder is (%q, %q), want nothing", dir, name)
	}

	script := []byte("#!/bin/sh\nexit 0\n")
	bin := t.TempDir()
	chrome := filepath.Join(bin, "google-chrome")
	if err := os.WriteFile(chrome, script, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
	if _, err := Open("http://127.0.0.1:1/"); err != nil {
		t.Fatalf("Open with a stand-in Chrome failed: %v", err)
	}
	if w, ok := Opened(); !ok || !w.AppMode || w.Path != chrome {
		t.Errorf("after an app-mode launch Opened is (%+v, %v), want app mode with %s", w, ok, chrome)
	}

	// No Chromium-family browser at all: the tab opener runs, and that is what is recorded.
	reset()
	tabBin := t.TempDir()
	if err := os.WriteFile(filepath.Join(tabBin, "xdg-open"), script, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", tabBin)
	if _, err := Open("http://127.0.0.1:1/"); err != nil {
		t.Fatalf("Open with only a tab opener failed: %v", err)
	}
	if w, ok := Opened(); !ok || w.AppMode || w.Path != "" {
		t.Errorf("after a tab launch Opened is (%+v, %v), want a tab", w, ok)
	}

	// Nothing could launch: nothing is recorded.
	reset()
	t.Setenv("PATH", t.TempDir())
	if _, err := Open("http://127.0.0.1:1/"); err == nil {
		t.Fatal("Open with nothing on PATH reported success")
	}
	if w, ok := Opened(); ok {
		t.Errorf("a launch that failed recorded a window: %+v", w)
	}
}

func TestUsableDirIsAnAbsolutePathToAFolderThatIsThere(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	put(t, file, []byte("x"))
	for _, tc := range []struct {
		in, want string
		ok       bool
	}{
		{dir, dir, true},
		{dir + string(filepath.Separator), dir, true},
		{filepath.Join(dir, "x", ".."), dir, true},
		{"", "", false},
		{"relative", "", false},
		{".", "", false},
		{file, "", false},
		{filepath.Join(dir, "missing"), "", false},
		{dir + "\x00", "", false},
	} {
		got, ok := UsableDir(tc.in)
		if ok != tc.ok || got != tc.want {
			t.Errorf("UsableDir(%q) = (%q, %v), want (%q, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}
