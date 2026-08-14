package server

// drivesFromBitmask turns a GetLogicalDrives mask — bit 0 is A:, bit 1 is B:, and
// so on — into browsable root paths like "C:\". It lives apart from the Windows
// call that produces the mask so it can be table-tested from any platform, the
// same reason assetURL takes goos as a parameter instead of reading runtime.GOOS.
func drivesFromBitmask(mask uint32) []string {
	roots := make([]string, 0, 26)
	for i := 0; i < 26; i++ {
		if mask&(1<<uint(i)) != 0 {
			roots = append(roots, string(rune('A'+i))+`:\`)
		}
	}
	return roots
}
