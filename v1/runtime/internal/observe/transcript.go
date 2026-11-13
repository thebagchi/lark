package observe

import (
	"slices"
	"sync"

	"github.com/thebagchi/lark/v1/runtime/common"
)

// _Transcript is every line a run printed, in the order the lines arrived.
//
// Locked, because each thread of a run prints on its own goroutine. The order
// is the order the lock was taken in: print order on one thread, and the order
// they met for two.
//
// Revisions:
//   - 2026-10-29 06:17: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 23:23: keeps common.Print lines (C91w7)
type _Transcript struct {
	mu     sync.Mutex
	prints []*common.Print
}

// _Add keeps line as the latest line printed.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation
//   - 2026-11-07 23:23: takes a common.Print (C91w7)
func (t *_Transcript) _Add(line *common.Print) {
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
//   - 2026-11-07 23:23: returns common.Print lines (C91w7)
func (t *_Transcript) _Lines() []*common.Print {
	t.mu.Lock()
	defer t.mu.Unlock()

	return slices.Clone(t.prints)
}
