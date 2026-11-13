package common

import (
	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
)

// Called is the name of fn for an error, or common.ANONYMOUS when fn is nil,
// which only a Go caller can pass and whose Name would dereference it.
//
// One helper for core, event and flow. unpack keeps its own _NameOf, because
// this package imports unpack and unpack cannot import it back.
//
// Revisions:
//   - 2026-11-06 00:20: initial creation, from the copies in core, event and flow (C100b)
//   - 2026-11-08 00:59: names read from common (C91w12c, Q244)
func Called(fn *starlark.Builtin) string {
	if fn == nil {
		return common.ANONYMOUS
	}

	return fn.Name()
}
