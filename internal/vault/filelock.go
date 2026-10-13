package vault

import (
	"errors"
	"os"
	"time"
)

// The vault's cross-process lock (ADR-154).
//
// Every write of the vault file is a read of it, a change, and a rename over it, and a second Nib
// process on the same config directory does the same from its own copy. This lock is what makes the
// three one step BETWEEN processes; v.mu makes them one step inside a process. Order is always v.mu
// first, then this.
//
// **It is the kernel's lock on an open file, never the file's presence.** `vault.nib.lock` is
// created once and never removed (removing it would let two processes each lock a different file
// of the same name); what excludes is flock on Linux and macOS and LockFileEx on Windows, both of
// which the kernel releases when the holder exits or crashes. A lock file left behind holds
// nothing, and nothing reads a pid or a marker out of it.
//
// What it does not cover, declared: a config directory on a filesystem that does not honour these
// locks between machines (a synced folder, some network mounts), and a build that predates it,
// which takes no lock at all.

// lockSuffix names the lock file beside the vault. Not the vault file itself: the vault is replaced
// by a rename on every save, so a lock on its inode would be a lock on a file that is gone.
const lockSuffix = ".lock"

// lockWait is how long a writer waits for another process's write before it is refused with
// ErrBusy. A save is a durable write of the whole vault — 8 to 75 ms measured from 621 B to 7.3 MB,
// 312 ms at worst for a key removal's read-back (ADR-144) — so ten seconds is a process that is
// stopped, not one that is slow. A var so a test can shorten it.
var lockWait = 10 * time.Second

// lockPoll is how often a waiting writer asks again. The lock is asked for without blocking so the
// wait can be bounded; a blocking call cannot be given up on.
const lockPoll = 5 * time.Millisecond

// ErrBusy: another process held the vault's lock for the whole of lockWait, so nothing was read or
// written.
var ErrBusy = errors.New("another running Nib was writing the vault and did not finish, so the " +
	"change was not stored; try again, and if it keeps happening close the other Nib")

// fileLock is one held lock on a vault's lock file.
type fileLock struct{ f *os.File }

func lockPath(vaultPath string) string { return vaultPath + lockSuffix }

// lockVaultFile takes the cross-process lock for the vault at vaultPath, waiting up to lockWait.
//
// The lock file is opened fresh for every acquisition: the lock belongs to the open file, so two
// `*Vault` values in one process exclude each other as two processes do, and closing the file is
// the release even where unlocking fails.
func lockVaultFile(vaultPath string) (*fileLock, error) {
	f, err := os.OpenFile(lockPath(vaultPath), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(lockWait)
	for {
		got, err := tryLock(f)
		if err != nil {
			f.Close()
			return nil, err
		}
		if got {
			return &fileLock{f: f}, nil
		}
		if !time.Now().Before(deadline) {
			f.Close()
			return nil, ErrBusy
		}
		time.Sleep(lockPoll)
	}
}

// release gives the lock up. Safe to call twice.
func (l *fileLock) release() {
	if l == nil || l.f == nil {
		return
	}
	unlock(l.f)
	l.f.Close()
	l.f = nil
}
