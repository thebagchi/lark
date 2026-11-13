package declare

import (
	"fmt"

	"go.starlark.net/resolve"
	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/internal/declared"
)

// Aliased refuses a use of the arg builtin that is not a call of it: arg
// bound to another name, passed on, listed, or read for an attribute.
//
// Every declaration is then an arg() call written at its place, with or
// without parentheses round the callee, which is what declared.Of reads (Q301).
// An alias hides a declaration from that reader, so a run would refuse the key
// the script declares through it.
//
// Only the builtin counts. The resolver has bound every name of tree, so a
// parameter, a local or a lambda parameter named arg is that variable and not
// the builtin, and so is a keyword f(arg=1) or an attribute x.arg, which the
// resolver does not bind. A file that takes the name arg at its top level is
// not asked, since the compiler skips the plugin that supplies a shadowed
// name. A tree that is not resolved has no binding to read and is passed.
//
// Returns ErrDeclare, opening with the file, line and column of the first
// use, as a resolve error does.
//
// Revisions:
//   - 2026-11-10 11:52: initial creation (C118, B251, Q301)
//   - 2026-11-10 17:15: uses ErrDeclare, formerly ErrNotDeclaring (C121, Q349)
func Aliased(tree *syntax.File) error {
	if tree == nil {
		return nil
	}

	var (
		called  = map[*syntax.Ident]bool{}
		refused *syntax.Ident
	)

	syntax.Walk(
		tree,
		func(node syntax.Node) bool {
			switch node := node.(type) {
			case *syntax.CallExpr:
				callee := declared.Callee(node)
				if callee != nil {
					called[callee] = true
				}

			case *syntax.Ident:
				if refused == nil && _Builtin(node) && !called[node] {
					refused = node
				}
			}

			return refused == nil
		},
	)

	if refused == nil {
		return nil
	}

	return fmt.Errorf(
		"%s: %s is bound or passed on, not called (call it where it is written): %w",
		refused.NamePos,
		declared.ARG,
		ErrDeclare,
	)
}

// _Builtin reports whether name is the arg builtin: the resolver bound it to a
// predeclared name.
//
// Revisions:
//   - 2026-11-10 11:52: initial creation (C118, B251, Q301)
func _Builtin(name *syntax.Ident) bool {
	if name.Name != declared.ARG {
		return false
	}

	bind, ok := name.Binding.(*resolve.Binding)

	return ok && bind != nil && bind.Scope == resolve.Predeclared
}
