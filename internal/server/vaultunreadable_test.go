package server

import (
	"os"
	"strings"
	"testing"

	"nib/internal/vault"
)

// TestAnUnreadableVaultIsNotReportedAsAMissingKey — /pending 502.
//
// Any vault file that could not be read reached the client as "key-missing", whose screen asks the
// user to find the key Nib was set up with. The client half is test/jsdom/vaultunreadable.test.mjs.
func TestAnUnreadableVaultIsNotReportedAsAMissingKey(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(vault.Path(dir), []byte("{garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	s := New(os.DirFS("."), os.DirFS("."), dir, "test")
	// STIMULUS: the file is present, so this is not the setup state under another name.
	if !vault.Exists(dir) {
		t.Fatal("setup: the corrupt vault file is not seen as present")
	}
	st := s.vaultStatus()
	if st.State != "vault-unreadable" {
		t.Fatalf("a corrupt vault reported state %q, want vault-unreadable", st.State)
	}
	if st.VaultPath != vault.Path(dir) || !strings.Contains(st.Problem, "corrupt") {
		t.Errorf("the status does not say which file or why: path %q problem %q", st.VaultPath, st.Problem)
	}
}
