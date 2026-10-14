package server

import (
	"sync"

	"nib/internal/vault"
)

// An older copy of the vault was written over it, and Nib put the current one back (ADR-159,
// /pending 873 part 2).
//
// The vault records it (vault.OlderCopyRestored); this is how a window hears. It rides the window
// stream as `event: vault`, because nothing else fits: the write that found the older copy
// SUCCEEDED, so no response carries a refusal; it is as likely to be a recent file recorded on an
// open, or the delivery arm, as anything the user pressed; `/api/status` is read about once a page
// load; and the session notice is polled only while this window is armed. The stream is held by
// every window for its whole life and already carries what the machine has to say unasked.
//
// It is said while it can be acted on — the other writer is still running, and will write its old
// copy back at its next save — which is why it is pushed and not left for the next status read.

// vaultRestoredWhat is the notice's stable key (noticeView.What).
const vaultRestoredWhat = "vault-restored"

// The words. One sentence for what happened, and what to do about it behind the dash the window
// joins them with. Both readings of a lower generation are named, because nothing in the file
// tells them apart (vault.restoreOverOlderCopy).
const (
	vaultRestoredSummary = "Another Nib wrote an older copy of your vault over it after a key was " +
		"removed, and Nib has put the removal back"
	vaultRestoredDetail = "close any other Nib that is still running — an older version does this " +
		"each time it saves. If you imported a backup in another Nib just now, it was replaced: " +
		"import it again in this one"
)

// vaultRestoreWake wakes every window's stream when the open vault reports a restore. It holds no
// state of its own — the count and the time are the vault's — so a window that connects later reads
// the same fact from the same place.
type vaultRestoreWake struct {
	mu    sync.Mutex
	watch chan struct{}
}

// announce is what the vault calls (vault.NotifyOlderCopyRestored). It runs under the vault's lock,
// so it closes a channel and nothing else.
func (k *vaultRestoreWake) announce() {
	k.mu.Lock()
	if k.watch != nil {
		close(k.watch)
		k.watch = nil
	}
	k.mu.Unlock()
}

// changes returns a channel closed on the next restore. Same contract as armedChanges: take it
// BEFORE reading, or a restore landing in between is lost (/pending 464).
func (k *vaultRestoreWake) changes() <-chan struct{} {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.watch == nil {
		k.watch = make(chan struct{})
	}
	return k.watch
}

// vaultRestoredNotice is what a window is told about v, or nil when v has restored nothing. `At`
// is the last restore's time and is the window's key for "have I said this one".
func vaultRestoredNotice(v *vault.Vault) *noticeView {
	if v == nil {
		return nil
	}
	n, at := v.OlderCopyRestored()
	if n == 0 {
		return nil
	}
	return &noticeView{What: vaultRestoredWhat, Summary: vaultRestoredSummary, Detail: vaultRestoredDetail, At: at}
}
