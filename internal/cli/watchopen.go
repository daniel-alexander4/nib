package cli

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"nib/internal/safe"
)

// watchOpen hands a PDF that arrived in the watched directory to Nib, through the SAME door a
// double-click uses: the nib binary launched with the file's path (/pending 403). That launch
// hands off to a running Nib — a tab in the window already open (ADR-086), queued if the vault is
// locked, refused with a notice past the open-document cap (ADR-005) — or becomes the Nib that
// shows it. Nothing here decides any of that, so nothing here can disagree with it (ADR-009).
//
// It is the "Print to Nib" path: every platform's print dialog can save a PDF into a folder, and a
// watch on that folder puts the printout in front of the user. A printer driver is /pending 902.
//
// Opening rewrites nothing, so a signed PDF is opened like any other.
func watchOpen(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	if err := launchNib(abs); err != nil {
		return "", err
	}
	return "handed to Nib", nil
}

// launchNib starts the launch watchOpen hands a file to. A var so a test can see what would be
// launched without starting a Nib — under `go test` the executable is the test binary.
var launchNib = func(path string) error {
	cmd, err := nibLaunch(path)
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("could not start Nib: %w", err)
	}
	// Reaped, never awaited: a launch that hands off exits at once, and one that becomes the
	// running Nib lives until its last window closes.
	go func() {
		defer safe.Recover("watch open launch")
		_ = cmd.Wait()
	}()
	return nil
}

// nibLaunch builds the launch: this binary, the file's absolute path as its only argument, the
// watch's own stderr so a refusal the launch logs is seen where the watch reports, and a process
// group of its own (detachLaunch) so stopping the watch does not stop a Nib it started.
func nibLaunch(path string) (*exec.Cmd, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("could not find the nib binary: %w", err)
	}
	cmd := exec.Command(exe, path)
	cmd.Stderr = os.Stderr
	detachLaunch(cmd)
	return cmd, nil
}
