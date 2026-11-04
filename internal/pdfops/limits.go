package pdfops

import "nib/internal/tempdir"

// ReapUnsupportedNote is the sentence for a system where a converter an earlier Nib left running cannot be
// found: the leftover directory is still swept, the process is not.
const ReapUnsupportedNote = "On this system Nib cannot find a converter that an earlier run left running, so it may have to be ended by hand."

// DeclaredLimits are the platform gaps ADR-185 declares — an orphaned converter is not reaped and a converter's
// memory is not capped — as sentences, empty where both are enforced (Linux). It is the ONE wording door:
// `cmd/nib` logs each line at startup, so a gap that is declared in the ADR is also said where the person running
// Nib can read it. The two constants it reads are build-tagged siblings.
func DeclaredLimits() []string {
	return declaredLimits(tempdir.ReapSupported, CeilingUnenforcedNote)
}

func declaredLimits(reapSupported bool, ceilingNote string) []string {
	var out []string
	if !reapSupported {
		out = append(out, ReapUnsupportedNote)
	}
	if ceilingNote != "" {
		out = append(out, ceilingNote)
	}
	return out
}
