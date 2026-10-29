package unpack

import (
	"fmt"

	"go.starlark.net/starlark"
)

// Positional refuses a call that is not exactly count positional arguments,
// with the text starlark.UnpackPositionalArgs gives when its minimum and its
// maximum are both count. A builtin then reads each argument with StringAt,
// CallableAt or args[idx].
//
// Not UnpackPositionalArgs itself, because it takes a pointer to each local
// through ...any and can call a method through that pointer. The compiler
// cannot see where the pointer goes, so it moves each local to the heap: one
// allocation for each argument, on every call of the builtin.
//
// The checks come in the order UnpackPositionalArgs makes them, so a call that
// is wrong in two ways is told the same one.
//
// Revisions:
//   - 2026-10-29 01:32: initial creation
func Positional(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
	count int,
) error {
	if len(kwargs) > 0 {
		return fmt.Errorf(
			"%s: unexpected keyword arguments",
			fn.Name(),
		)
	}

	if len(args) != count {
		return fmt.Errorf(
			"%s: got %d arguments, want %d",
			fn.Name(),
			len(args),
			count,
		)
	}

	return nil
}

// StringAt reads the argument at idx as a string, with the refusal
// UnpackPositionalArgs gives. idx counts from zero, and the refusal counts
// from one, as UnpackPositionalArgs does.
//
// The caller has checked the count with Positional, so idx is in range.
//
// Revisions:
//   - 2026-10-29 01:32: initial creation
func StringAt(
	fn *starlark.Builtin,
	args starlark.Tuple,
	idx int,
) (string, error) {
	text, ok := starlark.AsString(args[idx])
	if !ok {
		return "", fmt.Errorf(
			"%s: for parameter %d: got %s, want string",
			fn.Name(),
			idx+1,
			args[idx].Type(),
		)
	}

	return text, nil
}

// CallableAt reads the argument at idx as something a script can call, with
// the refusal UnpackPositionalArgs gives. idx counts as it does for StringAt.
//
// The caller has checked the count with Positional, so idx is in range.
//
// Revisions:
//   - 2026-10-29 01:32: initial creation
func CallableAt(
	fn *starlark.Builtin,
	args starlark.Tuple,
	idx int,
) (starlark.Callable, error) {
	callable, ok := args[idx].(starlark.Callable)
	if !ok {
		return nil, fmt.Errorf(
			"%s: for parameter %d: got %s, want callable",
			fn.Name(),
			idx+1,
			args[idx].Type(),
		)
	}

	return callable, nil
}
