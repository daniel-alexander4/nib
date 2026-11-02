// Package filelock is the one kernel file lock Nib takes between processes (ADR-009: a primitive
// gets one door). The vault's cross-process write lock (ADR-154) and the temporary-directory
// door's liveness lock (ADR-185) both ask it, and neither reads a pid or a marker out of a file:
// what excludes is the kernel's lock on an open file, released when the holder exits or crashes.
package filelock
