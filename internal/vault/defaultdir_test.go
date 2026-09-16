package vault

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// TestDefaultDirIsAbsoluteWithNoConfigDirectory — /pending 502 (info).
//
// With no config directory resolvable, DefaultDir returned "nib" relative to wherever the process
// was started, so the vault moved with the working directory.
func TestDefaultDirIsAbsoluteWithNoConfigDirectory(t *testing.T) {
	if runtime.GOOS == "windows" || runtime.GOOS == "darwin" || runtime.GOOS == "plan9" {
		t.Skip("the unset-environment case is driven through $HOME and $XDG_CONFIG_HOME")
	}
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("HOME", "")
	// STIMULUS: the config directory really cannot be resolved, or this asserts about the ordinary path.
	if _, err := os.UserConfigDir(); err == nil {
		t.Fatal("setup: os.UserConfigDir still resolves with HOME and XDG_CONFIG_HOME unset")
	}
	if d := DefaultDir(); !filepath.IsAbs(d) {
		t.Errorf("DefaultDir() = %q with no config directory; it must be absolute", d)
	}
}
