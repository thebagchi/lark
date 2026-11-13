// _Distinct is why a UI that sends two Function entries named greet is
// refused before anything compiles them: two bodies cannot both be the one a
// Call names. Nodes may repeat a name across threads; functions may not.

package graph

import (
	"errors"
	"fmt"
	"maps"
	"slices"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// ErrDuplicate is returned when a flow declares one name twice: two functions,
// a function named main, which is the spine, or a function, constant or
// argument sharing a name with another. It is also returned when two modules
// declare one name, and the wrap names both.
//
// The limit is real and is the cost of the shape a user interface wants: a
// Flow has functions and no module, so a flat list cannot hold two things
// called the same thing. The merge renames and splices every body that
// uses the name. Derivation never rewrites a body, so it refuses.
//
// Revisions:
//   - 2026-09-20 19:57: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-13 17:40: names the merge, formerly the amalgam merge (C134, Q446)
var (
	ErrDuplicate = errors.New("a name is declared twice")
)

// _Defined is every Function.name in flow, in the order listed.
//
// Revisions:
//   - 2026-09-20 19:20: initial creation
//   - 2026-10-02 00:13: reads a Flow
//   - 2026-10-03 08:26: unexported as _Defined, nothing outside this package reading it
func _Defined(flow *workflowpb.Flow) []string {
	var (
		fns   = flow.GetFunctions()
		names = make(
			[]string,
			0,
			len(fns),
		)
	)

	for _, fn := range fns {
		names = append(
			names,
			fn.GetName(),
		)
	}

	return names
}

// _Distinct reports that no name in flow is declared twice.
//
// Functions, constants and arguments share one namespace: each binds a
// module-level name in the generated script, so a name in two of them is a
// script whose second binding hides the first. main is the spine, written from
// Flow.main, so a function of that name would be a second def main. A nil
// flow, or one that declares nothing, has no duplicates.
//
// Check calls this, so a host validating a whole flow asks once.
//
// Revisions:
//   - 2026-09-20 19:20: initial creation
//   - 2026-09-21 16:25: says that Check covers it
//   - 2026-10-02 00:13: reads a Flow, and refuses main, and a name both a
//     function and a constant or an argument
//   - 2026-10-03 08:26: unexported, a host checking a flow through Check
//   - 2026-11-05 23:52: quotes or bounds a name a script chose (C63, B128, Q4)
//   - 2026-11-06 00:16: quotes a name a flow can hold (C63, B128, Q208)
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
func _Distinct(flow *workflowpb.Flow) error {
	seen := make(
		map[string]bool,
		len(flow.GetFunctions()),
	)

	for _, name := range _Defined(flow) {
		if seen[name] || name == common.ENTRY {
			return fmt.Errorf(
				"%s: %w",
				unpack.QuotePrefix(name),
				ErrDuplicate,
			)
		}

		seen[name] = true
	}

	for _, name := range slices.Sorted(maps.Keys(flow.GetConstants())) {
		if seen[name] {
			return fmt.Errorf(
				"%s: %w",
				unpack.QuotePrefix(name),
				ErrDuplicate,
			)
		}

		seen[name] = true
	}

	for name := range flow.GetArgs() {
		if seen[name] {
			return fmt.Errorf(
				"%s: %w",
				unpack.QuotePrefix(name),
				ErrDuplicate,
			)
		}
	}

	return nil
}
