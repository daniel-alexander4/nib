package server

import (
	"net/http"
	"strings"

	"nib/internal/vault"
)

// A vault write the vault itself refused, and said why (ADR-154, /pending 873 part 3).
//
// Four refusals are not "the write failed": another running Nib removed an authorized key
// (ErrKeyChanged), replaced the vault with one this process cannot open (ErrChangedElsewhere), was
// still writing it (ErrBusy), or this process's own import retired the value (ErrReplaced). Each is
// a sentence the user can act on — close and reopen Nib, or try again — and only the key-removal
// route passed the first of them on. The other twenty-odd routes that store something answered
// "could not save settings", "could not pin peer", and so on: true, and no use to anyone.
//
// **One recognition, by sentence, in httpError** — where ADR-136's `associated-file` already is and
// for its reason: the handlers that reach the vault through another function (the ceremony pin, the
// convene, the signing identity) voice its error inside a sentence of their own, so the sentinel is
// gone by the time they answer and the sentence is what is left. A handler that worded the failure
// itself, with no room for the vault's sentence, answers through vaultWriteFailed below, which
// hands the sentence to the same recognition.

// vaultRefusalCause is the `cause` a refused vault write carries, on every route.
const vaultRefusalCause = "vault-not-written"

// vaultRefusals are the sentences recognised. The vault's own sentinels, read at the comparison,
// so rewording one there cannot leave this recognising the old words.
var vaultRefusals = []error{
	vault.ErrKeyChanged,
	vault.ErrChangedElsewhere,
	vault.ErrBusy,
	vault.ErrReplaced,
}

// vaultRefusal reports the vault's sentence in msg, if msg carries one.
func vaultRefusal(msg string) (sentence string, ok bool) {
	for _, e := range vaultRefusals {
		if s := e.Error(); strings.Contains(msg, s) {
			return s, true
		}
	}
	return "", false
}

// vaultWriteFailed answers a failed vault write for a handler that words the failure itself: the
// vault's own sentence where it gave one, else the handler's status and words.
//
// 409, as the key-removal route has always answered this refusal — a second status would make one
// refusal read differently by the door that reached it (ADR-008's reasoning).
func vaultWriteFailed(w http.ResponseWriter, err error, code int, msg string) {
	if s, ok := vaultRefusal(err.Error()); ok {
		// The sentence alone, as a sentence: the handler's "could not …" adds nothing to it.
		writeVaultRefusal(w, strings.ToUpper(s[:1])+s[1:]+".")
		return
	}
	httpError(w, code, msg)
}

// writeVaultRefusal is the one answer, for httpError's recognition and for vaultWriteFailed.
func writeVaultRefusal(w http.ResponseWriter, msg string) {
	writeJSONStatus(w, http.StatusConflict, map[string]string{"error": msg, "cause": vaultRefusalCause})
}
