package clone

import (
	"go.starlark.net/syntax"
)

// Calls reports whether a call is module.member.
//
// Revisions:
//   - 2026-09-24 20:10: initial creation, as state's _Stores
//   - 2026-09-30 22:41: names the call it looks for, rather than state.set
//   - 2026-11-06 00:08: a nil call, or a dotted call with no name, is no match (C100a, B175, B176)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/deep and renamed _Calls to Calls, for the
//     tests in tests/ (C120b, Q315, Q314)
func Calls(
	call *syntax.CallExpr,
	module string,
	member string,
) bool {
	if call == nil {
		return false
	}

	dotted, ok := call.Fn.(*syntax.DotExpr)
	if !ok || dotted == nil || dotted.Name == nil || dotted.Name.Name != member {
		return false
	}

	held, ok := dotted.X.(*syntax.Ident)

	return ok && held != nil && held.Name == module
}
