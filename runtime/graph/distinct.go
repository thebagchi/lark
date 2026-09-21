// Distinct is why a UI that sends two Function entries named greet is
// refused before anything compiles them: two bodies cannot both be the one a
// Call names. Nodes may repeat a name across threads; functions may not.

package graph

import (
	"errors"
	"fmt"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
)

// ErrDuplicate is returned when two Function entries share a name.
var ErrDuplicate = errors.New("duplicate function")

// Names is every Function.name in graph, in the order listed.
//
// Revisions:
//   - 2026-09-20 19:20: initial creation
func Names(graph *workflowpb.Graph) []string {
	fns := graph.GetFunctions()
	names := make([]string, 0, len(fns))

	for _, fn := range fns {
		names = append(names, fn.GetName())
	}

	return names
}

// Distinct reports that no two functions in graph share a name.
//
// Nodes are not functions. first on two threads is two statuses of one
// name, which Names never sees. A nil graph, or one with no functions,
// has no duplicates.
//
// Revisions:
//   - 2026-09-20 19:20: initial creation
func Distinct(graph *workflowpb.Graph) error {
	seen := make(map[string]bool, len(graph.GetFunctions()))

	for _, name := range Names(graph) {
		if seen[name] {
			return fmt.Errorf("%s: %w", name, ErrDuplicate)
		}

		seen[name] = true
	}

	return nil
}
