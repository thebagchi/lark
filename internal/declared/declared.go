// Package declared reads which arguments a script declares: the names of every
// arg() call at its top level, with or without parentheses round the callee.
// It is internal: a run and the lark command both check -args keys against it,
// and neither exports it.
//
// Revisions:
//   - 2026-11-05 22:40: initial creation, moved from package graph (C71, Q196)
//   - 2026-11-10 11:52: reads a parenthesised callee (C118, B251, Q301)
package declared

import (
	"errors"
	"fmt"
	"slices"

	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/dialect"
)

const (
	// ARG is the builtin a declaration is written with.
	ARG = "arg"
)

// Of is the sorted names every arg() call at the top level of text
// declares, whether the call is assigned, passed on, listed or only printed.
//
// A call in the body of a def or a lambda declares nothing, since a run
// refuses it. A script that binds the name arg itself, with a def, an
// assignment, a load or a for, declares nothing at all: its calls are calls of
// its own, as the compile reads them (B261). A script is read as it is, so a
// caller reading a merged script reads what a run does. A name the walk cannot
// know, an arg() whose first argument is not a string literal, is refused, so a
// caller never takes the names for all of them.
//
// Returns a plain error for such a call, "arg names something that is not a string
// literal", and a wrapped parse error.
//
// Revisions:
//   - 2026-11-05 22:10: initial creation (C71, Q193)
//   - 2026-11-05 22:40: moved to internal/declared (Q196)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 11:47: reads (arg)("x"), and refuses a bytes name (C118, B251, B252, Q301)
//   - 2026-11-11 00:30: a script that binds arg itself declares nothing (C122, B261)
func Of(
	entry string,
	text []byte,
) ([]string, error) {
	tree, err := common.OPTIONS.Parse(
		entry,
		text,
		0,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"%s: %w",
			entry,
			err,
		)
	}

	bound := map[string]bool{}

	dialect.Bound(
		tree.Stmts,
		bound,
	)

	if bound[ARG] {
		return nil, nil
	}

	names, unknown := _Calls(tree)
	if unknown {
		return nil, fmt.Errorf(
			"%s: %w",
			ARG,
			errors.New("arg names something that is not a string literal"),
		)
	}

	slices.Sort(names)

	return slices.Compact(names), nil
}

// _Calls is the name of every arg() call at the top level of tree, and whether
// a call names something the walk cannot know.
//
// Revisions:
//   - 2026-11-11 00:50: initial creation, the walk Of had (C122, B261)
func _Calls(tree *syntax.File) ([]string, bool) {
	var (
		names   []string
		unknown bool
	)

	syntax.Walk(
		tree,
		func(node syntax.Node) bool {
			switch node := node.(type) {
			case *syntax.DefStmt, *syntax.LambdaExpr:
				return false

			case *syntax.CallExpr:
				callee := Callee(node)
				if callee == nil || callee.Name != ARG {
					return true
				}

				name, ok := _Named(node)
				if !ok {
					unknown = true

					return true
				}

				names = append(
					names,
					name,
				)
			}

			return true
		},
	)

	return names, unknown
}

// _Named is the string an arg() call names its argument with, and false when
// the call has no argument or its first is not a string literal.
//
// Revisions:
//   - 2026-11-05 22:10: initial creation (C71, Q193)
//   - 2026-11-10 16:04: a bytes literal names nothing; reads the literal with Spelled (C118, B252)
func _Named(call *syntax.CallExpr) (string, bool) {
	if len(call.Args) == 0 {
		return "", false
	}

	return Spelled(call.Args[0])
}

// Spelled is the string a string literal states, and false for anything else.
//
// Only a string literal names. A bytes literal b"x" also holds a Go string, so
// the token decides, and a bytes name is refused as any other name the walk
// cannot know. Not a bare word: arg(host) names nothing, since a declaration
// states its name. A nil expression, or a typed nil literal, spells nothing.
//
// Exported for the derivation of a flow, which reads a declaration as a run
// does: one place decides what a declaration names.
//
// Revisions:
//   - 2026-09-22 22:31: initial creation
//   - 2026-11-08 10:35: a typed nil literal spells nothing (C108, B215)
//   - 2026-11-10 00:25: renamed from _Spelled, as a test in tests/ needs it (C120a, Q314)
//   - 2026-11-10 16:04: moved from graph.Spelled and merged with the rule of _Named; a bytes
//     literal spells nothing (C118, B252)
func Spelled(expr syntax.Expr) (string, bool) {
	literal, ok := expr.(*syntax.Literal)
	if !ok || literal == nil || literal.Token != syntax.STRING {
		return "", false
	}

	word, ok := literal.Value.(string)

	return word, ok
}

// Callee is the name a call calls, or nil when it calls something that is not
// a plain name. Parentheses round the name are looked through, since
// (arg)("x", 1) is the call arg("x", 1) (Q301).
//
// Exported for the args plugin, whose compile check must read a callee as Of
// does, and for the derivation of a flow.
//
// Revisions:
//   - 2026-11-05 22:40: initial creation (C71, Q196)
//   - 2026-11-10 16:04: renamed from _Callee; looks through parentheses, returns the name
//     node, is exported, and a nil call or typed nil parenthesis has no callee (C118, B251, Q301)
func Callee(call *syntax.CallExpr) *syntax.Ident {
	if call == nil {
		return nil
	}

	fn := call.Fn

	for {
		paren, ok := fn.(*syntax.ParenExpr)
		if !ok || paren == nil {
			break
		}

		fn = paren.X
	}

	name, _ := fn.(*syntax.Ident)

	return name
}
