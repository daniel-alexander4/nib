package server

import (
	"reflect"
	"runtime"
	"testing"
)

// The bitmask conversion is kept apart from the Windows syscall precisely so it
// can be checked from any platform — the same reason assetURL takes goos as a
// parameter. Without this the drive list would only ever be exercised on Windows,
// which is the one platform this repo's tests never run on.
func TestDrivesFromBitmask(t *testing.T) {
	cases := []struct {
		name string
		mask uint32
		want []string
	}{
		{"none", 0, []string{}},
		{"C only", 1 << 2, []string{`C:\`}},
		{"A and B (floppies still count)", 0b11, []string{`A:\`, `B:\`}},
		{"C and D", 1<<2 | 1<<3, []string{`C:\`, `D:\`}},
		{"C and a mapped Z", 1<<2 | 1<<25, []string{`C:\`, `Z:\`}},
		{"all 26", 0x03FFFFFF, []string{
			`A:\`, `B:\`, `C:\`, `D:\`, `E:\`, `F:\`, `G:\`, `H:\`, `I:\`, `J:\`, `K:\`, `L:\`, `M:\`,
			`N:\`, `O:\`, `P:\`, `Q:\`, `R:\`, `S:\`, `T:\`, `U:\`, `V:\`, `W:\`, `X:\`, `Y:\`, `Z:\`,
		}},
		{"bits above Z are ignored", 1<<2 | 1<<26 | 1<<31, []string{`C:\`}},
	}
	for _, c := range cases {
		if got := drivesFromBitmask(c.mask); !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: drivesFromBitmask(%#b) = %v, want %v", c.name, c.mask, got, c.want)
		}
	}
}

// Off Windows "/" already contains every mounted filesystem, so the parent walk
// reaches everything and there is nothing to jump to. Pinning this keeps the
// POSIX response byte-identical to what it was before roots existed.
func TestBrowseRootsEmptyOffWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("windows has real drives to enumerate")
	}
	if got := browseRoots(); len(got) != 0 {
		t.Errorf("browseRoots() = %v, want empty off Windows", got)
	}
}
