package rewrite

import (
	"strconv"

	"go.starlark.net/resolve"
	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/common"
)

// At is where a lambda begins: the file's name, a line and a column.
//
// Not a syntax.Position, which carries its file as a pointer: two parses of one
// source hold two pointers, so the same lambda would be two keys. The name is
// kept because a merged tree holds the nodes of many files, and two of them
// can have a lambda at one line and column.
//
// Revisions:
//   - 2026-09-30 20:58: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect and renamed _At to At, for the tests in
//     tests/ (C120b, Q315, Q314)
type At string

// Marked copies the marks off the written tree, which is resolved, onto the
// compiled tree, which is not.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-10-28 22:49: marks the copy Compile made rather than parsing the
//     source a second time
//   - 2026-11-10 01:37: moved from v1/runtime/dialect and renamed _Marked to Marked, for the
//     tests in tests/ (C120b, Q315, Q314)
func Marked(
	compiled *syntax.File,
	written *syntax.File,
) {
	_Take(
		compiled,
		Taken(written),
	)
	_Apply(
		compiled,
		Marks(written),
	)
}

// Taken is every lambda written as spawn's argument, with the variables each
// one reads, in the order the resolver met them.
//
// A lambda taking *args or **kwargs is left out: nothing may follow either in
// a parameter list, and spawn refuses a function taking them anyway.
//
// Revisions:
//   - 2026-09-29 23:30: initial creation
//   - 2026-11-08 10:35: a nil file takes nothing (C108, B199)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect and renamed _Taken to Taken, for the
//     tests in tests/ (C120b, Q315, Q314)
func Taken(written *syntax.File) map[At][]string {
	taken := make(map[At][]string)

	if written == nil {
		return taken
	}

	syntax.Walk(
		written,
		func(node syntax.Node) bool {
			lambda := _Spawned(node)
			if lambda == nil {
				return true
			}

			fn, ok := lambda.Function.(*resolve.Function)
			if !ok || fn.HasVarargs || fn.HasKwargs {
				return true
			}

			at := _Where(lambda.Lambda)

			for _, bind := range fn.FreeVars {
				taken[at] = append(
					taken[at],
					bind.First.Name,
				)
			}

			return true
		},
	)

	return taken
}

// _Spawned is the lambda a node hands the builtin spawn as its one argument, or
// nil when the node is anything else.
//
// Revisions:
//   - 2026-09-29 23:30: initial creation
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect (C120b, Q315)
func _Spawned(node syntax.Node) *syntax.LambdaExpr {
	call, ok := node.(*syntax.CallExpr)
	if !ok || len(call.Args) != 1 {
		return nil
	}

	name, ok := call.Fn.(*syntax.Ident)
	if !ok || name.Name != common.SPAWN || !_Predeclared(name) {
		return nil
	}

	lambda, ok := call.Args[0].(*syntax.LambdaExpr)
	if !ok {
		return nil
	}

	return lambda
}

// _Predeclared reports whether a resolved name is the environment's rather
// than one the script bound.
//
// Revisions:
//   - 2026-09-29 23:30: initial creation
//   - 2026-11-10 01:37: moved from v1/runtime/dialect (C120b, Q315)
func _Predeclared(name *syntax.Ident) bool {
	bind, ok := name.Binding.(*resolve.Binding)

	return ok && bind.Scope == resolve.Predeclared
}

// _Take gives each lambda in taken a parameter per variable it reads,
// defaulting to that variable.
//
// Revisions:
//   - 2026-09-29 23:30: initial creation
//   - 2026-11-10 01:37: moved from v1/runtime/dialect (C120b, Q315)
func _Take(
	compiled *syntax.File,
	taken map[At][]string,
) {
	if len(taken) == 0 {
		return
	}

	syntax.Walk(
		compiled,
		func(node syntax.Node) bool {
			lambda, ok := node.(*syntax.LambdaExpr)
			if !ok {
				return true
			}

			for _, name := range taken[_Where(lambda.Lambda)] {
				lambda.Params = append(
					lambda.Params,
					_Default(
						name,
						lambda.Lambda,
					),
				)
			}

			return true
		},
	)
}

// _Default is the parameter name=name.
//
// Placed where the lambda begins, so an error about the default - a variable
// read before it was assigned - points at the lambda that read it.
//
// Revisions:
//   - 2026-09-29 23:30: initial creation
//   - 2026-11-10 01:37: moved from v1/runtime/dialect (C120b, Q315)
func _Default(
	name string,
	at syntax.Position,
) syntax.Expr {
	return &syntax.BinaryExpr{
		X: &syntax.Ident{
			NamePos: at,
			Name:    name,
		},
		OpPos: at,
		Op:    syntax.EQ,
		Y: &syntax.Ident{
			NamePos: at,
			Name:    name,
		},
	}
}

// _Where is where a lambda begins.
//
// Revisions:
//   - 2026-09-29 23:30: initial creation
//   - 2026-11-02 22:58: keeps the name of the file, since a tree may hold many (C44a)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect (C120b, Q315)
func _Where(pos syntax.Position) At {
	return At(pos.Filename() + ":" +
		strconv.Itoa(int(pos.Line)) + ":" +
		strconv.Itoa(int(pos.Col)))
}
