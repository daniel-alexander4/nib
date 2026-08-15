//go:build windows

package cli

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"

	"golang.org/x/sys/windows/registry"
)

// classesRoot is where a per-user file association lives. HKCU means no
// administrator rights and no effect on other accounts on the machine.
const classesRoot = `Software\Classes`

// selfPath is the absolute path of the running binary — the command Windows will
// invoke, so it must be resolved rather than taken from argv.
func selfPath() (string, error) {
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return filepath.Abs(exe)
}

// notifyShell tells Explorer the association table changed, so the "Open with"
// list is right immediately instead of after the next sign-in. Best-effort: the
// registration is already written by this point, and a stale shell cache is a
// cosmetic delay, not a failure.
func notifyShell() {
	const shcneAssocChanged = 0x08000000
	const shcnfIdList = 0x0000
	shell32 := syscall.NewLazyDLL("shell32.dll")
	_, _, _ = shell32.NewProc("SHChangeNotify").Call(shcneAssocChanged, shcnfIdList, 0, 0)
}

func cmdRegister(args []string) int {
	if len(args) > 0 {
		fmt.Fprintln(os.Stderr, "nib register: takes no arguments")
		return 1
	}
	exe, err := selfPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "nib: could not resolve this program's path: %v\n", err)
		return 1
	}
	for _, e := range associationEntries(exe) {
		k, _, err := registry.CreateKey(registry.CURRENT_USER, classesRoot+`\`+e.Path, registry.SET_VALUE)
		if err != nil {
			fmt.Fprintf(os.Stderr, "nib: could not write %s: %v\n", e.Path, err)
			return 1
		}
		err = k.SetStringValue(e.Name, e.Value)
		k.Close()
		if err != nil {
			fmt.Fprintf(os.Stderr, "nib: could not set %s\\%s: %v\n", e.Path, e.Name, err)
			return 1
		}
	}
	notifyShell()
	fmt.Println("Nib is now offered for PDFs in Explorer's \"Open with\" menu.")
	fmt.Println("Windows reserves the default handler for you to choose: right-click a PDF →")
	fmt.Println("Open with → Choose another app → Nib, and tick \"Always use this app\".")
	fmt.Println("Registered for " + exe)
	return 0
}

func cmdUnregister(args []string) int {
	if len(args) > 0 {
		fmt.Fprintln(os.Stderr, "nib unregister: takes no arguments")
		return 1
	}
	exe, err := selfPath()
	if err != nil {
		fmt.Fprintf(os.Stderr, "nib: could not resolve this program's path: %v\n", err)
		return 1
	}
	failed := false

	// Remove only our own value from the shared OpenWithProgids key — the key
	// itself belongs to .pdf and carries every other PDF app's registration.
	if k, err := registry.OpenKey(registry.CURRENT_USER, classesRoot+`\.pdf\OpenWithProgids`, registry.SET_VALUE); err == nil {
		if err := k.DeleteValue("Nib.Document"); err != nil && !errors.Is(err, registry.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "nib: could not remove the .pdf association: %v\n", err)
			failed = true
		}
		k.Close()
	}

	// Deepest first, so each key is empty when it is deleted.
	for _, path := range ownedKeys(exe) {
		err := registry.DeleteKey(registry.CURRENT_USER, classesRoot+`\`+path)
		if err != nil && !errors.Is(err, registry.ErrNotExist) && !errors.Is(err, fs.ErrNotExist) {
			fmt.Fprintf(os.Stderr, "nib: could not remove %s: %v\n", path, err)
			failed = true
		}
	}
	notifyShell()
	if failed {
		return 1
	}
	fmt.Println("Nib is no longer offered for PDFs in Explorer.")
	return 0
}
