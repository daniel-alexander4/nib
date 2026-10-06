package browser

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// windowClass is the WM_CLASS Open gives Nib's app window on Linux, and what Raise looks for.
const windowClass = "Nib"

// raiseLimit bounds each helper. A raise that has not happened in this long is not going to.
const raiseLimit = 2 * time.Second

// Raise brings Nib's existing window to the front, and reports whether it asked anything to.
//
// **X11 only, and only with a helper the user already has** (`wmctrl`, then `xdotool`). ADR-006
// stands: no raise exists that works everywhere — Wayland refuses one by design, and a page
// cannot raise its own window (measured, ADR-086). This is the one place where the machine can be
// asked, so it is asked; everywhere else the window takes its tab where it is and marks its title.
// Nothing depends on the answer.
//
// The window is found by the class Open set, so a Nib running as a tab in the user's own browser
// is not found, and is not raised.
func Raise() bool {
	if runtime.GOOS != "linux" || os.Getenv("DISPLAY") == "" {
		return false
	}
	if path, err := exec.LookPath("wmctrl"); err == nil {
		if out, ok := run(path, "-lx"); ok {
			if id := wmctrlWindow(out, windowClass); id != "" {
				if _, ok := run(path, "-i", "-a", id); ok {
					return true
				}
			}
		}
	}
	if path, err := exec.LookPath("xdotool"); err == nil {
		out, _ := run(path, "search", "--class", "^"+windowClass+"$")
		ids := strings.Fields(out)
		// Newest first: a Chromium app also owns an unmapped helper window of the same class,
		// listed before the one on screen, and activating that one fails.
		for i := len(ids) - 1; i >= 0; i-- {
			if _, ok := run(path, "windowactivate", ids[i]); ok {
				return true
			}
		}
	}
	return false
}

// wmctrlWindow picks the id of the first window in `wmctrl -lx` output whose class is exactly
// class. The third column is "instance.Class"; a substring match would take "x.Nibbles".
func wmctrlWindow(listing, class string) string {
	for _, line := range strings.Split(listing, "\n") {
		f := strings.Fields(line)
		if len(f) >= 3 && strings.HasSuffix(f[2], "."+class) {
			return f[0]
		}
	}
	return ""
}

func run(name string, args ...string) (string, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), raiseLimit)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	return string(out), err == nil
}
