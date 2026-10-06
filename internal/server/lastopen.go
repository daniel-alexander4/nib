package server

import (
	"log"
	"net/http"
	"path/filepath"
)

// The session boundary (ADR-085).
//
// Closing Nib's last window closes its documents. The process outlives the window by
// `idleExitGrace` so that a reload finds what it left, and a launch arriving inside that grace is
// handed to this process — which used to mean it came up holding everything the closed window had.
// A launch is not a reload: it starts with nothing open but what it was asked to open. What was
// open is remembered BY PATH, so the user can ask for it back.

// These are the lines a test and a user's log read, literals for `windowConnectedMsg`'s reason.
const (
	lastOpenRecordedMsg = "recorded the open documents for resume:"
	lastOpenSkippedMsg  = "open documents not recorded for resume:"
	sessionEndedMsg     = "previous session ended by a launch:"
)

// recordLastOpen stores the paths of the open documents, in tab order. The ONE door, with two
// callers: the last window going, and Quit.
//
// **Best-effort, and it says so in the log rather than to anyone.** A locked vault has nowhere to
// write and a failed save changes nothing the user asked for; neither may hold up a window closing.
//
// **A set with no file in it leaves the stored one alone.** Opening Nib, looking at nothing and
// closing it is not a session worth resuming, and it must not cost the user the one that was.
func (s *Server) recordLastOpen() {
	s.mu.Lock()
	v := s.vault
	var paths []string
	pathless := 0
	for _, d := range s.docs {
		if d.path == "" {
			pathless++
			continue
		}
		paths = append(paths, d.path)
	}
	s.mu.Unlock()
	if len(paths) == 0 {
		if pathless > 0 {
			log.Printf("%s %d open, none with a file behind it", lastOpenSkippedMsg, pathless)
		}
		return
	}
	if v == nil {
		log.Printf("%s Nib is locked", lastOpenSkippedMsg)
		return
	}
	if err := v.SetLastOpen(paths); err != nil {
		log.Printf("%s %v", lastOpenSkippedMsg, err)
		return
	}
	log.Printf("%s %d with a file, %d without", lastOpenRecordedMsg, len(paths), pathless)
}

// endSessionForLaunch closes every document the closed window left, through the same door Close
// uses. Called only by a hand-off that cancelled a grace.
//
// **Nothing is recorded here**: the window that went already did, before the grace this launch
// cancelled was armed. The unsaved count is logged because it is the one thing this loses that the
// exit ten seconds later would also have lost — the close prompt asked about it, and a browser
// that died without asking is the case this line exists to make visible.
func (s *Server) endSessionForLaunch() {
	s.mu.Lock()
	n, unsaved := len(s.docs), 0
	for _, d := range s.docs {
		if len(d.undo) > 0 {
			unsaved++
		}
	}
	s.mu.Unlock()
	if n == 0 {
		return
	}
	s.setDoc(nil)
	log.Printf("%s %d closed, %d with unsaved changes", sessionEndedMsg, n, unsaved)
}

// handleLastOpen returns what Resume would reopen, in `handleRecent`'s shape and for its reasons:
// the name is derived here, and an empty set is [] rather than null.
func (s *Server) handleLastOpen(w http.ResponseWriter, r *http.Request) {
	paths := vaultFrom(r).LastOpen()
	out := make([]recentEntry, 0, len(paths))
	for _, p := range paths {
		out = append(out, recentEntry{Path: p, Name: filepath.Base(p)})
	}
	writeJSON(w, out)
}
