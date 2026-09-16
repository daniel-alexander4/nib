//go:build !windows

package pdfops

import (
	"os/exec"
	"syscall"
)

// killTreeOnCancel makes a converter's cancellation kill its whole process group, so a launcher's children
// (`soffice` → `soffice.bin`) die with it — `/pending 503`, and see `runConvert`.
//
// The converter is started as the leader of its own group, and cancellation signals the group.
//
// **The trade this makes, declared.** A process in its own group is out of the terminal's foreground group,
// so a Ctrl-C to a `nib` CLI conversion no longer reaches the converter directly: nib exits and a converter
// that is working finishes its file and exits on its own, where before the interrupt killed it too. A
// converter that HANGS is exactly what this exists for, and it was the case with no bound at all.
func killTreeOnCancel(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// A negative pid addresses the process group whose id is that pid — the group Setpgid created.
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
