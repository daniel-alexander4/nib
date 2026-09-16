//go:build windows

package pdfops

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
)

// killTreeOnCancel makes a converter's cancellation kill its whole process tree, so `soffice.exe`'s
// `soffice.bin` dies with it — `/pending 503`, and see `runConvert`.
//
// Windows has no process group a signal can address, so the tree is ended by `taskkill /T`, which walks
// child processes, resolved from `%SystemRoot%` rather than PATH. **Not exercised by any test on this
// repo's machines**, which are Linux: the Unix sibling's tests drive the behaviour, and this one is
// compiled (`GOOS=windows go vet`) and nothing more — the declared gap.
func killTreeOnCancel(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		taskkill := "taskkill"
		if root := os.Getenv("SystemRoot"); root != "" {
			taskkill = filepath.Join(root, "System32", "taskkill.exe")
		}
		// taskkill's own failure is not acted on: the likeliest is that the tree has already gone, and the
		// launcher is killed directly below either way, whose error is the one returned.
		_ = exec.Command(taskkill, "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid)).Run()
		return cmd.Process.Kill()
	}
}
