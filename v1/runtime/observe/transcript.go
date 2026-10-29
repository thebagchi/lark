package observe

import (
	"slices"
	"sync"
)

// _Transcript is every line a run printed, in the order the lines arrived.
//
// Locked, because each thread of a run prints on its own goroutine. The order
// is the order the lock was taken in: print order on one thread, and the order
// they met for two.
type _Transcript struct {
	mu     sync.Mutex
	prints []*Print
}

// _Add keeps line as the latest line printed.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation
func (t *_Transcript) _Add(line *Print) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.prints = append(
		t.prints,
		line,
	)
}

// _Lines is every line kept so far, as a slice of its own, so a line printed
// after it is read does not change what the reader holds.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation
func (t *_Transcript) _Lines() []*Print {
	t.mu.Lock()
	defer t.mu.Unlock()

	return slices.Clone(t.prints)
}
