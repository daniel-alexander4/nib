//go:build windows

package cli

// oNoFollow is zero on Windows, where neither O_NOFOLLOW nor O_NONBLOCK exists for files.
//
// **This is a real gap, not a no-op standing in for one**, and it is named here rather than
// left to be discovered: on Windows the watch keeps only scanOnce's Lstat check plus
// readNoFollow's regular-file check on the open handle, so the narrow race between them
// stays open. Closing it there needs CreateFile with FILE_FLAG_OPEN_REPARSE_POINT, which is
// a different API from os.OpenFile and worth doing when a Windows user watches a shared
// drop directory — which is not a configuration anyone has reported.
const oNoFollow = 0
