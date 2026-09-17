//go:build windows

package pdfops

import (
	"os"
	"os/exec"
)

// killTreeOnCancel makes a converter's cancellation kill its whole process tree, so `soffice.exe`'s
// `soffice.bin` dies with it — `/pending 503`, and see `runConvert`.
//
// Windows has no process group a signal can address, so the tree is ended by `taskkill /T`, which walks
// child processes. **The declared gap narrowed rather than closed** (`/pending 516`): the command it runs
// is built by `taskkillCommand` in the untagged sibling, so which program is launched and with which
// arguments is asserted on every machine this repo is worked on, while whether `taskkill` then ends the
// tree is still exercised by nothing here. The Unix sibling's tests drive the behaviour; this half is
// compiled (`TestEveryPlatformCompiles`) and its argv is tested, and that is all.
func killTreeOnCancel(cmd *exec.Cmd) {
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		name, args := taskkillCommand(os.Getenv("SystemRoot"), cmd.Process.Pid)
		// taskkill's own failure is not acted on: the likeliest is that the tree has already gone, and the
		// launcher is killed directly below either way, whose error is the one returned.
		_ = exec.Command(name, args...).Run()
		return cmd.Process.Kill()
	}
}
