package browser

import (
	"fmt"
	"os/exec"
	"runtime"

	"nib/internal/safe"
)

// OpenFolder shows a directory in the desktop's file manager (ADR-039).
//
// **A folder, never a file, and that is a safety property rather than a convenience.** Handing a
// downloaded artifact to the desktop opener would ask the desktop to DO something with it, and on
// a binary or a .deb that can mean run it or hand it to an installer — the act this whole feature
// refuses, because nothing is published to verify the bytes against. The caller passes the
// containing directory; this function has no way to express "open that file".
//
// **Not `tabOpener`, though it is the same neighbourhood.** That one is documented as "the OS
// command that opens a URL in the default browser", and on Windows it is
// `rundll32 url.dll,FileProtocolHandler` — a URL protocol handler, which is the wrong tool for a
// path. Windows wants `explorer` for a folder. Reusing the URL opener here would have worked on
// Linux and macOS and been subtly wrong on the one platform nobody tests.
func OpenFolder(dir string) error {
	name, args := folderOpener(dir)
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not open %s: %w", dir, err)
	}
	// Reaped, like the browser launch above: a Start with no Wait leaves a zombie for the life of
	// the process, and this one can be called repeatedly from the download dialog.
	go func() {
		defer safe.Recover("open folder reaper")
		_ = cmd.Wait()
	}()
	return nil
}

// folderOpener returns the OS command that shows a directory in a file manager.
func folderOpener(dir string) (string, []string) {
	switch runtime.GOOS {
	case "darwin":
		return "open", []string{dir}
	case "windows":
		return "explorer", []string{dir}
	default:
		return "xdg-open", []string{dir}
	}
}
