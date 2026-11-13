package unpack

import (
	"strings"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/unpack/internal/spec"
)

// Named matches a call's arguments to a builtin's parameters, by position and
// by keyword, and puts each value in the slot of its parameter. A parameter the
// call leaves out keeps a nil slot.
//
// It answers true only for a call that is regular: no more positional arguments
// than parameters, every keyword the name of a parameter, no parameter given
// twice, and every parameter not marked OPTIONAL given. It reads no type. For any
// other call it answers false, and the caller hands the call to
// starlark.UnpackArgs, which gives the refusal. So the text of a refusal stays
// that of starlark.UnpackArgs, and this function can not drift from it.
//
// Not starlark.UnpackArgs itself, because it takes a pointer to each local
// through ...any. The compiler cannot see where the pointers go, so it moves
// each local to the heap: one allocation for each parameter, on every call.
// The slots are a slice of the caller's array, which stays on the stack.
//
// names are written as starlark.UnpackArgs takes them: the required ones first,
// then the ones that end with OPTIONAL. names and slots have the same length,
// and slots start nil. The caller owns both and Named keeps neither.
//
// A slots shorter than names, and a keyword entry that holds fewer than two
// values, are not regular calls: Named answers false and reads no further.
//
// Revisions:
//   - 2026-11-04 00:35: initial creation (C38c)
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
//   - 2026-11-10 16:30: refuses slots shorter than names and a keyword entry shorter than two
//     items (C117, B246, Q65, Q206)
func Named(
	args starlark.Tuple,
	kwargs []starlark.Tuple,
	names []string,
	slots []starlark.Value,
) bool {
	if len(args) > len(names) || len(slots) < len(names) {
		return false
	}

	copy(
		slots,
		args,
	)

	// PAIR_LEN is the length of a keyword argument: a name and a value.
	const PAIR_LEN = 2

	for _, item := range kwargs {
		if len(item) < PAIR_LEN {
			return false
		}

		key, ok := item[0].(starlark.String)
		if !ok {
			return false
		}

		idx := _Index(
			names,
			string(key),
		)
		if idx < 0 || slots[idx] != nil {
			return false
		}

		slots[idx] = item[1]
	}

	for idx, name := range names {
		if strings.HasSuffix(
			name,
			spec.OPTIONAL,
		) {
			break
		}

		if slots[idx] == nil {
			return false
		}
	}

	return true
}

// _Index is where name is in names, with or without OPTIONAL, or -1.
//
// Revisions:
//   - 2026-11-04 00:35: initial creation (C38c)
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
func _Index(
	names []string,
	name string,
) int {
	for idx, held := range names {
		if strings.TrimSuffix(
			held,
			spec.OPTIONAL,
		) == name {
			return idx
		}
	}

	return -1
}
