package browser

import (
	"strings"
	"testing"
)

// TestWindowsHuntsABrowserInstalledForOneUser — /pending 610. A browser installed without
// administrator rights lives under the user's %LOCALAPPDATA%, not under Program Files and not on
// PATH, and the Windows list named neither — so that user was given the plain-tab fallback.
//
// This checks the LIST. Whether Windows then starts the browser it names was not run: there is
// no Windows machine behind this test.
func TestWindowsHuntsABrowserInstalledForOneUser(t *testing.T) {
	const local = `C:\Users\pat\AppData\Local`
	got := chromiumCandidatesFor("windows", local)

	at := func(want string) int {
		for i, c := range got {
			if c == want {
				return i
			}
		}
		return -1
	}
	// The stimulus: the list being read is the Windows one.
	machine, onPath := at(`C:\Program Files\Google\Chrome\Application\chrome.exe`), at("chrome.exe")
	if machine < 0 || onPath < 0 {
		t.Fatalf("setup: this is not the Windows list (%v)", got)
	}
	for _, want := range []string{
		local + `\Google\Chrome\Application\chrome.exe`,
		local + `\Microsoft\Edge\Application\msedge.exe`,
		local + `\BraveSoftware\Brave-Browser\Application\brave.exe`,
		local + `\Chromium\Application\chrome.exe`,
	} {
		i := at(want)
		if i < 0 {
			t.Errorf("Windows does not look for %s — a browser installed for one user is not found, and that user gets a plain tab", want)
			continue
		}
		// After the machine-wide install, which every user on the machine was already given.
		if i < machine {
			t.Errorf("%s is tried before the machine-wide install", want)
		}
	}

	// With no directory there is nothing to look under: `\Google\…` would be the root of
	// whatever drive Nib was started from.
	for _, c := range chromiumCandidatesFor("windows", "") {
		if strings.HasPrefix(c, `\`) {
			t.Errorf("with no %%LOCALAPPDATA%% the list still names %s — a path on the current drive's root", c)
		}
	}
	// And no other platform is given Windows paths.
	for _, goos := range []string{"darwin", "linux"} {
		for _, c := range chromiumCandidatesFor(goos, local) {
			if strings.Contains(c, local) {
				t.Errorf("%s is offered the Windows path %s", goos, c)
			}
		}
	}
}
