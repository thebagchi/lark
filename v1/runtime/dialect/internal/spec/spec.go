// Package spec is internal, not public: the tables and counts dialect reads while it marks a tree.
//
// Revisions:
//   - 2026-11-08 00:52: initial creation, from dialect (Q243, C91w12d)
package spec

import "github.com/thebagchi/lark/v1/runtime/common"

// HANDED is where each builtin that runs a function it is handed takes that
// function: spawn first, and each wrapper after the count or the budget it
// takes first.
//
// Revisions:
//   - 2026-10-04 01:25: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:52: moved here from dialect (Q243, C91w12d)
var (
	HANDED = map[string]int{
		common.SPAWN:   0,
		common.REPEAT:  1,
		common.RETRY:   1,
		common.TIMEOUT: 1,
	}
)

// CLOSED is the fewest statements a suite holds when one of them is a closer:
// the closer, and one statement it closes.
//
// Revisions:
//   - 2026-10-04 01:25: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 00:52: moved here from dialect (Q243, C91w12d)
const (
	CLOSED = 2
)
