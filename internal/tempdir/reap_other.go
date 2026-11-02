//go:build !linux

package tempdir

// ReapSupported is false off Linux: there is no /proc to find an orphaned converter's group in.
// The gap is DECLARED in the tool's missing-tool wording (R7) and `TestReaperIsHonestAboutSupport`
// fails a sibling whose constant says true while its body does nothing.
const ReapSupported = false

// NewReaper returns nil: nothing is reaped. The caller checks ReapSupported to say so.
func NewReaper(known []string) Reaper { return nil }
