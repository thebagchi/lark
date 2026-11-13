package deep

import (
	"fmt"

	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/guard"
	"github.com/thebagchi/lark/v1/runtime/plugin/deep/internal/clone"
	"github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
)

// ShowsCode is the first call to module.member in tree whose second argument
// the source plainly shows is a function, and what it shows: "the function
// helper" or "a lambda". Answers nil and "" when there is none.
//
// The half of IsData a compiler can answer. Only what is visible counts: a
// name the file declares with def, or a lambda written in place, is certain
// before anything runs, and a script author would rather hear it then.
// Everything else - a call's result, a value read from somewhere - is left to
// IsData when it runs.
//
// Here rather than in each plugin for the reason IsData is: state.set and
// event.post each take a value across threads, and two readings of "the source
// shows a function" would be two sets of cases to keep in step.
//
// Only a call of exactly two arguments. Any other count is refused when it
// runs, for its arity, and that says more than naming the second of them would.
//
// Revisions:
//   - 2026-09-24 20:10: initial creation, as state's Check
//   - 2026-09-30 22:41: a question any plugin can ask of its own call, rather than
//     state's
//   - 2026-11-04 19:30: a nil tree shows no code (C87)
//   - 2026-11-04 19:30: returns an error for a tree that panics the walk; the
//     error wraps the recovered panic (C87 fix round 1)
func ShowsCode(
	tree *syntax.File,
	module string,
	member string,
) (*syntax.CallExpr, string, error) {
	if tree == nil {
		return nil, "", nil
	}

	declared := map[string]bool{}

	for _, stmt := range tree.Stmts {
		def, ok := stmt.(*syntax.DefStmt)
		if ok && def != nil && def.Name != nil {
			declared[def.Name.Name] = true
		}
	}

	var (
		found *syntax.CallExpr
		named string
	)

	// syntax.Walk panics on a nil node in the tree. The panic is an error the
	// caller sees: a tree that cannot be read must not pass as one that shows
	// no code.
	raised := guard.Contained(func() {
		found, named = _Seek(
			tree,
			module,
			member,
			declared,
		)
	})
	if raised != nil {
		return nil, "", fmt.Errorf(
			"read the source for %s.%s: %w",
			module,
			member,
			raised,
		)
	}

	return found, named, nil
}

// _Seek walks tree for the first call of module.member that shows a function,
// and returns it with what it shows.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation, the walk ShowsCode had (C87)
//   - 2026-11-10 11:32: uses clone.Calls, which moved to internal/clone (C120b, Q315)
func _Seek(
	tree *syntax.File,
	module string,
	member string,
	declared map[string]bool,
) (*syntax.CallExpr, string) {
	var (
		found *syntax.CallExpr
		named string
	)

	syntax.Walk(
		tree,
		func(node syntax.Node) bool {
			if found != nil {
				return false
			}

			call, ok := node.(*syntax.CallExpr)
			if !ok || len(call.Args) != 2 || !clone.Calls(
				call,
				module,
				member,
			) {
				return true
			}

			named = common.Names(
				call.Args[1],
				declared,
			)
			if named != "" {
				found = call
			}

			return found == nil
		},
	)

	return found, named
}
