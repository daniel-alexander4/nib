// Package browser opens Nib's UI in a chromeless app-mode window.
//
// We prefer a Chromium-family browser's --app mode, which gives a dedicated,
// address-bar-free window that looks like a native desktop app while reusing an
// engine that's already installed. If none is found we fall back to opening the
// URL as an ordinary tab in the default browser. (Safari has no app mode, so a
// Safari-only Mac takes the tab fallback — by design.)
package browser

import (
	"log"
	"nib/internal/safe"
	"os"
	"os/exec"
	"runtime"
	"time"
)

// fileExists reports whether a path names something that is not a directory. It does not check that it is runnable.
func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// Open launches url as an app-mode window, or a default-browser tab if no
// app-mode-capable browser is found. It returns the started command (already
// running), or an error if nothing could launch.
//
// The returned command is REAPED here, in a background goroutine, rather than left
// to the caller. The doc comment used to say the caller "can wait on it" and the
// only caller discards it, so nobody ever called Wait — and on Linux and macOS the
// launched browser became a zombie in nib's process table for the rest of the
// session the moment the user closed the window. Returning it is still useful for a
// caller that wants the handle; nothing now depends on that caller remembering.
func Open(url string) (*exec.Cmd, error) {
	if path, ok := findChromium(); ok {
		appArgs := []string{"--app=" + url, "--new-window"}
		if runtime.GOOS == "linux" {
			// Set a stable WM_CLASS so the panel can match this window to
			// nib.desktop (StartupWMClass=Nib) and show the themed icon —
			// rasterized sharply at the panel's size — instead of upscaling
			// the small icon Chromium derives from the page favicon.
			appArgs = append(appArgs, "--class="+windowClass)
		}
		cmd := exec.Command(path, appArgs...)
		if err := cmd.Start(); err == nil {
			// Started is not the same as running.
			//
			// `Start` reports only exec-level failure, so a browser that launches and
			// exits immediately — a locked user-data-dir, snap or flatpak confinement, an
			// Edge policy, a broken profile — was reported as success. There was no
			// fallback once Start had succeeded, and the only diagnostic was a line on
			// stderr that a double-clicked launch has nowhere to print. The user's entire
			// report is "I double-clicked Nib and nothing happened".
			//
			// A short wait is the whole fix: a browser that is going to fail this way
			// fails at once, and one that is working is still running. It costs a
			// quarter-second on the failing path and nothing on the working one.
			if launched(cmd, appModeSettle) {
				// Which branch opened the window is logged, because nothing else records it and the two behave
				// differently — printing among them (/pending 788). Never the URL: it carries the launch key.
				log.Printf("opened the window in app mode with %s", path)
				// And recorded, because the download folder is that browser's (ADR-102).
				recordOpened(Window{AppMode: true, Path: path})
				return cmd, nil
			}
			// It failed. Fall through to the tab fallback rather than serving a window
			// nobody can see.
		}
		// fall through to the tab fallback if the app-mode launch failed
	}

	name, args := tabOpener(url)
	log.Printf("no Chromium-family browser opened an app window; opening a tab in the default browser with %s", name)
	cmd := exec.Command(name, args...)
	err := cmd.Start()
	if err == nil {
		recordOpened(Window{})
		reap(cmd)
	}
	return cmd, err
}

// appModeSettle is how long Open waits to see whether an app-mode launch survives.
//
// Long enough that a browser refusing its profile has exited, short enough that a user
// never notices. Measured against nothing — it is a settle window, not a threshold — but
// the failure it catches is immediate by nature: the process is gone before it draws.
const appModeSettle = 250 * time.Millisecond

// launched reports whether an app-mode launch worked: cmd is still running after d, OR it
// exited within d with status 0. It reaps cmd either way.
//
// It replaces the bare reap on the app-mode path: the goroutine still waits, so nothing
// lingers as a zombie, but the result is now observed instead of discarded.
//
// **An early exit with status 0 is the browser handing the window to itself** (/pending 502).
// With Chrome, Edge or Brave already running, the launched process passes `--app=` to the
// running one over its process-singleton socket and exits normally at once; the window opens.
// Chromium's own handling of that case returns its normal exit code. This used to read "not
// alive" and fall through to the tab opener, so a user with their browser open got the app
// window AND a second tab of the same page. The failures this wait exists to catch — a
// profile in use by another host, confinement, a policy — exit non-zero. Read, not launched:
// no browser was started on a display to establish this.
func launched(cmd *exec.Cmd, d time.Duration) bool {
	done := make(chan error, 1)
	go func() {
		defer safe.Recover("browser reap")
		done <- cmd.Wait()
	}()
	select {
	case err := <-done:
		return err == nil // exited within the settle window: status 0 is a hand-off
	case <-time.After(d):
		return true
	}
}

// reap waits on a launched browser so it does not linger as a zombie once the user
// closes the window. The error is deliberately dropped: the browser exiting non-zero
// is not Nib's problem, and by this point the UI is either open or the user has gone.
func reap(cmd *exec.Cmd) {
	go func() {
		defer safe.Recover("browser reap")
		_ = cmd.Wait()
	}()
}

// findChromium returns the first Chromium-family browser found for the OS.
func findChromium() (string, bool) {
	for _, c := range chromiumCandidates() {
		if path, err := exec.LookPath(c); err == nil {
			return path, true
		}
		if fileExists(c) { // absolute paths (macOS .app bundles)
			return c, true
		}
	}
	return "", false
}

// chromiumCandidates lists browser binaries to try, per OS.
func chromiumCandidates() []string {
	return chromiumCandidatesFor(runtime.GOOS, os.Getenv("LOCALAPPDATA"))
}

// chromiumCandidatesFor is the list for one OS. The OS and the Windows user's local
// application-data directory are parameters so every list can be checked from any platform.
func chromiumCandidatesFor(goos, localAppData string) []string {
	switch goos {
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		}
	case "windows":
		c := []string{
			`C:\Program Files\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files (x86)\Google\Chrome\Application\chrome.exe`,
			`C:\Program Files\Microsoft\Edge\Application\msedge.exe`,
			`C:\Program Files (x86)\Microsoft\Edge\Application\msedge.exe`,
			// Brave and Chromium: the README promises both ("Chrome / Edge / Brave /
			// Chromium, app mode") and neither appeared here at all, so a Brave-only
			// Windows user silently took the rundll32 tab fallback — an ordinary
			// tabbed window, not the chromeless one the README describes. The x64
			// Edge path was missing too; only the x86 one was listed.
			`C:\Program Files\BraveSoftware\Brave-Browser\Application\brave.exe`,
			`C:\Program Files (x86)\BraveSoftware\Brave-Browser\Application\brave.exe`,
		}
		c = append(c, windowsPerUserCandidates(localAppData)...)
		return append(c, "chrome.exe", "msedge.exe", "brave.exe", "chromium.exe")
	default: // linux and friends
		return []string{
			"google-chrome", "google-chrome-stable", "chromium", "chromium-browser",
			"microsoft-edge", "brave-browser",
		}
	}
}

// tabOpener returns the OS command that opens a URL in the default browser.
func tabOpener(url string) (string, []string) {
	switch runtime.GOOS {
	case "darwin":
		return "open", []string{url}
	case "windows":
		return "rundll32", []string{"url.dll,FileProtocolHandler", url}
	default:
		return "xdg-open", []string{url}
	}
}

// windowsPerUserCandidates lists where a browser installed for ONE Windows user lives, under
// that user's %LOCALAPPDATA% (/pending 610). An installer run without administrator rights puts
// the browser there and nowhere under Program Files, and it is not on PATH either — so that user
// had a Chromium-family browser and was given the plain-tab fallback.
//
// The directory is a parameter, and the paths are joined by hand rather than with
// filepath.Join, so the list can be checked on a machine that is not Windows. With no
// directory there are no candidates: a path starting at `\Google` would name the root of
// whatever drive Nib was started from.
func windowsPerUserCandidates(localAppData string) []string {
	if localAppData == "" {
		return nil
	}
	return []string{
		localAppData + `\Google\Chrome\Application\chrome.exe`,
		localAppData + `\Microsoft\Edge\Application\msedge.exe`,
		localAppData + `\BraveSoftware\Brave-Browser\Application\brave.exe`,
		localAppData + `\Chromium\Application\chrome.exe`,
	}
}
