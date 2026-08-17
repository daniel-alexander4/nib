// Package browser opens Nib's UI in a chromeless app-mode window.
//
// We prefer a Chromium-family browser's --app mode, which gives a dedicated,
// address-bar-free window that looks like a native desktop app while reusing an
// engine that's already installed. If none is found we fall back to opening the
// URL as an ordinary tab in the default browser. (Safari has no app mode, so a
// Safari-only Mac takes the tab fallback — by design.)
package browser

import (
	"nib/internal/safe"
	"os"
	"os/exec"
	"runtime"
)

// fileExists reports whether an absolute path is a regular, runnable file.
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
			appArgs = append(appArgs, "--class=Nib")
		}
		cmd := exec.Command(path, appArgs...)
		if err := cmd.Start(); err == nil {
			reap(cmd)
			return cmd, nil
		}
		// fall through to the tab fallback if the app-mode launch failed
	}

	name, args := tabOpener(url)
	cmd := exec.Command(name, args...)
	err := cmd.Start()
	if err == nil {
		reap(cmd)
	}
	return cmd, err
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
	switch runtime.GOOS {
	case "darwin":
		return []string{
			"/Applications/Google Chrome.app/Contents/MacOS/Google Chrome",
			"/Applications/Microsoft Edge.app/Contents/MacOS/Microsoft Edge",
			"/Applications/Brave Browser.app/Contents/MacOS/Brave Browser",
			"/Applications/Chromium.app/Contents/MacOS/Chromium",
		}
	case "windows":
		return []string{
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
			"chrome.exe", "msedge.exe", "brave.exe", "chromium.exe",
		}
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
