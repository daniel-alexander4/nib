//go:build windows

package server

import (
	"log"

	"golang.org/x/sys/windows"
)

// browseRoots lists the drives the folder browser can jump between.
//
// Windows has no single filesystem root: filepath.Dir(`C:\`) is a fixed point, so
// the parent walk dead-ends at whichever drive holds %USERPROFILE% and a document
// on D:, a USB stick, or a mapped share cannot be reached by clicking at all.
// These are the roots the UI offers once the walk runs out.
//
// GetLogicalDrives reads a bitmask and touches no drive. Stat-ing A: through Z:
// would answer the same question but can block for seconds on a mapped drive
// whose server is gone — precisely the setup this exists to serve.
func browseRoots() []string {
	mask, err := windows.GetLogicalDrives()
	if err != nil {
		log.Printf("could not list drives: %v", err)
		return nil
	}
	return drivesFromBitmask(mask)
}
