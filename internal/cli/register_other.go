//go:build !windows

package cli

import (
	"fmt"
	"os"
)

// The association verbs exist on every platform so the command list stays the
// same everywhere and a Windows instruction pasted onto a Mac gets a real
// answer instead of "unknown command". Elsewhere the job belongs to the
// desktop: Linux installs nib.desktop from the .deb (MimeType=application/pdf),
// and macOS reads the bundle's declared types.
func cmdRegister(args []string) int   { return notWindows("register") }
func cmdUnregister(args []string) int { return notWindows("unregister") }

func notWindows(verb string) int {
	fmt.Fprintf(os.Stderr, "nib %s: only needed on Windows.\n", verb)
	fmt.Fprintln(os.Stderr, "On Linux the .deb installs a desktop entry that already claims PDFs;")
	fmt.Fprintln(os.Stderr, "on macOS the file association comes from the application bundle.")
	return 1
}
