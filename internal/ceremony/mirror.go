package ceremony

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// The on-disk mirror: ~/nib/ceremonies/<id>/ (D20, D29).
//
// A ceremony can outlive a sitting — a party is interrupted, closes Nib, and comes back —
// so the record and the document in flight are written beside each other under the
// ceremony's own id. The id is what makes "continue" name a ceremony rather than guess at
// one.
//
// **What must never be written here is the invitation secret** (D29). The secret lives in
// the vault, which is sealed to the user's SSH key; this directory is ordinary files under
// the user's home. A test that asserts the vault *contains* the secret cannot see a copy
// left on disk beside the document, which is why the check that matters is an absence
// check over this directory.

// idPattern is what a ceremony id must look like before it is allowed near a path.
//
// The id comes out of a record, and a record can arrive from another party — so it is
// attacker-controlled input being used to build a directory name. 32 hex characters and
// nothing else: no separators, no dots, no room for traversal. Validated rather than
// escaped, because a validator has one answer and an escaper has a list of cases.
var idPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

// ErrBadID is returned for an id that cannot safely name a directory.
var ErrBadID = errors.New("ceremony id is not 32 hex characters")

// MirrorDir returns the directory for a ceremony, under root (normally ~/nib).
func MirrorDir(root, id string) (string, error) {
	if !idPattern.MatchString(id) {
		return "", fmt.Errorf("%w: %q", ErrBadID, id)
	}
	return filepath.Join(root, "ceremonies", id), nil
}

// WriteMirror creates the ceremony's directory and writes the record and the document.
//
// The record is written as JSON beside the document rather than only inside it: a party
// resuming needs to know which ceremony a file belongs to before opening the PDF, and
// reading a PDF attachment to find out is a slower answer to a question the directory
// name already half-answers.
func WriteMirror(root string, r Record, pdf []byte) (string, error) {
	dir, err := MirrorDir(root, r.ID)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	b, err := r.Encode()
	if err != nil {
		return "", err
	}
	// 0600 on both: these are the documents of a signing in progress, and the directory is
	// under the user's home where the default umask would otherwise make them world-readable
	// on a shared machine.
	if err := os.WriteFile(filepath.Join(dir, "record.json"), b, 0o600); err != nil {
		return "", err
	}
	if pdf != nil {
		if err := os.WriteFile(filepath.Join(dir, "document.pdf"), pdf, 0o600); err != nil {
			return "", err
		}
	}
	return dir, nil
}

// ReadMirror loads a ceremony back off disk.
func ReadMirror(root, id string) (Record, []byte, error) {
	dir, err := MirrorDir(root, id)
	if err != nil {
		return Record{}, nil, err
	}
	b, err := os.ReadFile(filepath.Join(dir, "record.json"))
	if err != nil {
		return Record{}, nil, err
	}
	r, err := Decode(b)
	if err != nil {
		return Record{}, nil, err
	}
	pdf, err := os.ReadFile(filepath.Join(dir, "document.pdf"))
	if err != nil && !os.IsNotExist(err) {
		return r, nil, err
	}
	return r, pdf, nil
}

// RemoveMirror deletes a ceremony's directory — the close-out prune (D29).
func RemoveMirror(root, id string) error {
	dir, err := MirrorDir(root, id)
	if err != nil {
		return err
	}
	return os.RemoveAll(dir)
}
