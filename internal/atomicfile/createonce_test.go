package atomicfile

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// TestCreateOnceHasOneWinnerAndItsWholeContent — /pending 668.
//
// The door a write-once record with several writers goes through. Two properties, and the second
// is what `CreateDurable` cannot give: exactly one creator succeeds, and anyone who reads the name
// at any moment reads a winner's complete bytes.
func TestCreateOnceHasOneWinnerAndItsWholeContent(t *testing.T) {
	const writers = 12
	const size = 4 << 20
	// Large enough that a file published before its content landed would be caught short.
	body := func(i int) []byte { return bytes.Repeat([]byte{byte('a' + i)}, size) }
	for round := 0; round < 4; round++ {
		dir := t.TempDir()
		p := filepath.Join(dir, "receipt.json")
		errs := make([]error, writers)
		start := make(chan struct{})
		var wg sync.WaitGroup
		for i := 0; i < writers; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				<-start
				errs[i] = CreateOnce(p, body(i), 0o600)
				if errs[i] != nil {
					// The loser reads back at once, as a write-once caller does.
					if got, rerr := os.ReadFile(p); rerr != nil || len(got) != size {
						t.Errorf("a creator that lost read %d bytes (err %v) under the name — the "+
							"name was published before its content", len(got), rerr)
					}
				}
			}(i)
		}
		// Readers that watch the name the whole time: whenever it can be read, it is whole.
		done := make(chan struct{})
		var rg sync.WaitGroup
		for r := 0; r < 4; r++ {
			rg.Add(1)
			go func() {
				defer rg.Done()
				for {
					if got, rerr := os.ReadFile(p); rerr == nil && len(got) != size {
						t.Errorf("the name held %d of %d bytes — published before its content", len(got), size)
						return
					}
					select {
					case <-done:
						return
					default:
					}
				}
			}()
		}
		close(start)
		wg.Wait()
		close(done)
		rg.Wait()
		winner := -1
		for i, err := range errs {
			switch {
			case err == nil && winner >= 0:
				t.Fatalf("round %d: creators %d and %d both succeeded", round, winner, i)
			case err == nil:
				winner = i
			case !os.IsExist(err):
				t.Fatalf("round %d: a creator that lost was answered %v, want an exists error", round, err)
			}
		}
		if winner < 0 {
			t.Fatalf("round %d: no creator succeeded", round)
		}
		if got, _ := os.ReadFile(p); !bytes.Equal(got, body(winner)) {
			t.Fatalf("round %d: the file does not hold the winner's content", round)
		}
		left, _ := os.ReadDir(dir)
		for _, e := range left {
			if strings.HasSuffix(e.Name(), ".tmp") {
				t.Fatalf("round %d: a temporary file was left behind: %s", round, e.Name())
			}
		}
	}
}
