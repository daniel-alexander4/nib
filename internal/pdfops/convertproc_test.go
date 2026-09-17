package pdfops

import (
	"strings"
	"testing"
)

// TestTheWindowsTreeKillDoesNotComeFromPATH — `/pending 516`: what a machine that is not Windows can
// still say about Windows-only code.
//
// `taskkill` ends a process tree, and `%SystemRoot%\System32` is where Windows' own copy of it lives.
// Taking whatever PATH answers first is how a program that kills processes becomes a program that does
// something else, and `internal/browser.findChromium` learned the general form of this the other way
// round (ADR-040): PATH is not where Windows keeps things.
//
// **What it cannot assert is the spelling.** `filepath.Join` answers with the host's separator, so on
// Linux this is `C:\Windows/System32/taskkill.exe`. The rule — the program comes from `%SystemRoot%` when
// the environment names one, and `/T` is what makes the kill a TREE kill — is platform-independent, and
// that is the half this pins. Whether `taskkill` then ends the tree is the gap `convertproc_windows.go`
// declares and this does not close.
func TestTheWindowsTreeKillDoesNotComeFromPATH(t *testing.T) {
	name, args := taskkillCommand(`C:\Windows`, 4321)
	if !strings.HasPrefix(name, `C:\Windows`) || !strings.Contains(name, "System32") ||
		!strings.HasSuffix(name, "taskkill.exe") {
		t.Errorf("taskkillCommand resolved %q, which is not %%SystemRoot%%'s System32\\taskkill.exe — "+
			"the converter's tree kill is then whatever the user's PATH answers", name)
	}
	if got := strings.Join(args, " "); got != "/T /F /PID 4321" {
		t.Errorf("taskkill's arguments are %q, want %q — without /T only the launcher dies and "+
			"`soffice.bin` keeps the pipe the conversion is waiting on", got, "/T /F /PID 4321")
	}
	// With no %SystemRoot% there is nothing to resolve against, and the bare name is the deliberate
	// fallback rather than a missing case: a tree kill that never runs is worse than one through PATH.
	if bare, _ := taskkillCommand("", 1); bare != "taskkill" {
		t.Errorf("with no SystemRoot the command is %q, want the bare name", bare)
	}
}
