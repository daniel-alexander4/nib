//go:build !linux

package singleton

// ReplaceOthers is a no-op on platforms without /proc. The --replace flag is
// used by the packaged Linux desktop launcher.
func ReplaceOthers() int { return 0 }
