package browser

import "testing"

// TestTheRaiseFindsNibsWindowByItsExactClass — the listing's third column is "instance.Class", and
// a substring match would raise somebody else's window.
func TestTheRaiseFindsNibsWindowByItsExactClass(t *testing.T) {
	listing := "0x04600003  0 nibbles.Nibbles       host Nibbles\n" +
		"0x04e00007  0 gnome-terminal.Gnome-terminal  host Terminal\n" +
		"0x05000003  3 127.0.0.1.Nib         host Nib\n"
	if got := wmctrlWindow(listing, windowClass); got != "0x05000003" {
		t.Errorf("picked %q, want Nib's own window 0x05000003", got)
	}
	if got := wmctrlWindow("0x04600003  0 nibbles.Nibbles  host Nibbles\n", windowClass); got != "" {
		t.Errorf("picked %q with no Nib window listed", got)
	}
}
