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
)

// ERR_DUPLICATE is returned when a flow declares one name twice: two functions,
// a function named main, which is the spine, or a function, constant or
// argument sharing a name with another.
var ERR_DUPLICATE = errors.New("a name is declared twice")

// _Defined is every Function.name in flow, in the order listed.
//
// Revisions:
//   - 2026-09-20 19:20: initial creation
//   - 2026-10-02 00:13: reads a Flow
//   - 2026-10-03 08:26: unexported as _Defined, nothing outside this package reading it
func _Defined(flow *workflowpb.Flow) []string {
	fns := flow.GetFunctions()
	names := make([]string, 0, len(fns))

	for _, fn := range fns {
		names = append(names, fn.GetName())
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
func _Distinct(flow *workflowpb.Flow) error {
	seen := make(map[string]bool, len(flow.GetFunctions()))

	for _, name := range _Defined(flow) {
		if seen[name] || name == ENTRY {
			return fmt.Errorf("%s: %w", name, ERR_DUPLICATE)
		}

		seen[name] = true
	}

	for _, name := range slices.Sorted(maps.Keys(flow.GetConstants())) {
		if seen[name] {
			return fmt.Errorf("%s: %w", name, ERR_DUPLICATE)
		}

		seen[name] = true
	}

	for name := range flow.GetArgs() {
		if seen[name] {
			return fmt.Errorf("%s: %w", name, ERR_DUPLICATE)
		}
	}

	return nil
}
