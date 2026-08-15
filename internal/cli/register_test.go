package cli

import (
	"strings"
	"testing"
)

// The registry layout is table-tested from any platform on purpose: the write
// itself is Windows-only, so without this the entries would be exercised only on
// the one OS this repo's tests never run on.
func TestAssociationEntries(t *testing.T) {
	// A path with a space, because "C:\Program Files\..." is the normal case and
	// an unquoted command breaks on it.
	const exe = `C:\Program Files\Nib\nib-1.101.2-windows-amd64.exe`
	want := `"` + exe + `" "%1"`

	byPath := map[string]regEntry{}
	for _, e := range associationEntries(exe) {
		byPath[e.Path+"|"+e.Name] = e
	}

	for _, tc := range []struct{ key, value string }{
		{`Nib.Document\shell\open\command|`, want},
		{`Applications\nib-1.101.2-windows-amd64.exe\shell\open\command|`, want},
	} {
		got, ok := byPath[tc.key]
		if !ok {
			t.Fatalf("no entry for %q; got %v", tc.key, byPath)
		}
		if got.Value != tc.value {
			t.Errorf("%s = %q, want %q", tc.key, got.Value, tc.value)
		}
		// The document argument must be quoted separately, or a PDF whose name
		// contains a space arrives as two arguments and neither is the file.
		if !strings.HasSuffix(got.Value, `"%1"`) {
			t.Errorf("%s does not pass a quoted %%1: %q", tc.key, got.Value)
		}
	}

	// Presence in OpenWithProgids is what actually lists Nib for PDFs.
	if _, ok := byPath[`.pdf\OpenWithProgids|Nib.Document`]; !ok {
		t.Error("no .pdf\\OpenWithProgids\\Nib.Document entry — Nib would not appear in Open with")
	}
	if _, ok := byPath[`Applications\nib-1.101.2-windows-amd64.exe\SupportedTypes|.pdf`]; !ok {
		t.Error("no SupportedTypes .pdf entry")
	}
}

// The Applications key is keyed by the file name the user actually runs. The
// published binaries are nib-<version>-windows-<arch>.exe, so a hardcoded
// "nib.exe" would register a program that does not exist on their disk.
func TestAssociationEntriesUseTheRealBinaryName(t *testing.T) {
	for _, name := range []string{`nib.exe`, `nib-1.101.2-windows-arm64.exe`, `renamed by the user.exe`} {
		entries := associationEntries(`C:\tools\` + name)
		found := false
		for _, e := range entries {
			if strings.HasPrefix(e.Path, `Applications\`+name+`\`) {
				found = true
			}
		}
		if !found {
			t.Errorf("no Applications\\%s key; entries: %v", name, entries)
		}
	}
}

// Unregister must not delete .pdf\OpenWithProgids: that key is shared with every
// other PDF application, so removing it would silently deregister the user's
// other readers. Only our value inside it may go.
func TestUnregisterNeverDeletesTheSharedKey(t *testing.T) {
	for _, k := range ownedKeys(`C:\tools\nib.exe`) {
		if strings.Contains(k, "OpenWithProgids") {
			t.Errorf("ownedKeys includes the shared key %q — deleting it would deregister other PDF apps", k)
		}
		if k == ".pdf" || strings.HasPrefix(k, `.pdf\`) {
			t.Errorf("ownedKeys includes %q, which is not ours to delete", k)
		}
	}
}

// Keys are deleted deepest-first, because Windows refuses to delete a key that
// still has subkeys — a shallow-first order silently leaves the tree behind.
func TestOwnedKeysAreDeepestFirst(t *testing.T) {
	keys := ownedKeys(`C:\tools\nib.exe`)
	for i, k := range keys {
		for _, later := range keys[i+1:] {
			if strings.HasPrefix(k, later+`\`) {
				continue // correct: the deeper key k comes before its parent
			}
			if strings.HasPrefix(later, k+`\`) {
				t.Errorf("%q is deleted before its child %q, which will fail", k, later)
			}
		}
	}
}
