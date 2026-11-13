// Package rewrite holds the syntax-tree rewriting of the dialect: the copier
// that copies a script's tree, the marker that marks its calls, the finders of
// the names and scripts it uses, and the match helpers they share. The compile
// entry points of the dialect stay in package dialect.
//
// Revisions:
//   - 2026-11-10 01:37: initial creation (C120b, Q315)
package rewrite

import (
	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/internal/nilcheck"
)

// Targets records in into every name an assignment's or a loop's left side
// binds, which may be one name or a shape of them.
//
// A nil into records nothing. A shape that holds itself is read once, so a tree
// a host builds by hand with a cycle ends the walk in place of a stack overflow
// no recover catches.
//
// Revisions:
//   - 2026-10-03 20:51: initial creation, from two copies of it
//   - 2026-11-04 19:30: skips a nil node (C87)
//   - 2026-11-08 10:35: skips a nil map, and reads a shape that holds itself once
//     (C108, B195, B196)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect and renamed _Targets to Targets, for the
//     tests in tests/ (C120b, Q315, Q314)
func Targets(
	expr syntax.Expr,
	into map[string]bool,
) {
	if into == nil {
		return
	}

	var seen map[any]bool

	_Collect(
		expr,
		into,
		&seen,
	)
}

// _Collect is Targets for one node, with seen holding every shape already
// read. A shape is a tuple, a list or a parenthesis, and seen is made when the
// first one is met, so a plain name costs nothing.
//
// Revisions:
//   - 2026-11-08 10:35: initial creation, the walk _Targets had (C108, B196)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect (C120b, Q315)
//   - 2026-11-11 00:40: finds a nil node with nilcheck.Absent (C122, B268)
func _Collect(
	expr syntax.Expr,
	into map[string]bool,
	seen *map[any]bool,
) {
	if nilcheck.Absent(expr) {
		return
	}

	switch held := expr.(type) {
	case *syntax.Ident:
		into[held.Name] = true

	case *syntax.TupleExpr:
		if Seen(
			expr,
			seen,
		) {
			return
		}

		for _, part := range held.List {
			_Collect(
				part,
				into,
				seen,
			)
		}

	case *syntax.ListExpr:
		if Seen(
			expr,
			seen,
		) {
			return
		}

		for _, part := range held.List {
			_Collect(
				part,
				into,
				seen,
			)
		}

	case *syntax.ParenExpr:
		if Seen(
			expr,
			seen,
		) {
			return
		}

		_Collect(
			held.X,
			into,
			seen,
		)
	}
}

// Seen reports whether a node was read already, and marks it read.
//
// Revisions:
//   - 2026-11-08 10:35: initial creation (C108, B196)
//   - 2026-11-10 01:37: moved from v1/runtime/dialect and renamed _Seen to Seen, for the tests
//     in tests/ (C120b, Q315, Q314)
func Seen(
	expr any,
	seen *map[any]bool,
) bool {
	if *seen == nil {
		*seen = make(map[any]bool)
	}

	if (*seen)[expr] {
		return true
	}

	(*seen)[expr] = true

	return false
}
