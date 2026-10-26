//go:build !unix

package cli

import "errors"

// mkfifo: no named pipes here, and the test that asks skips with this reason.
func mkfifo(string) error { return errors.New("named pipes are not available on this platform") }
