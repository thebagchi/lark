package unpack

import (
	"fmt"
	"strconv"

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

// _IntAt reads the argument at idx as a Starlark int, with the refusal
// UnpackPositionalArgs gives for a *starlark.Int. idx counts as it does for
// StringAt.
//
// Revisions:
//   - 2026-10-29 23:58: initial creation
func _IntAt(
	fn *starlark.Builtin,
	args starlark.Tuple,
	idx int,
) (starlark.Int, error) {
	number, ok := args[idx].(starlark.Int)
	if !ok {
		return starlark.Int{}, fmt.Errorf(
			"%s: for parameter %d: got %s, want int",
			fn.Name(),
			idx+1,
			args[idx].Type(),
		)
	}

	return number, nil
}

// _SizeAt reads the argument at idx as a Go int, with the refusals
// UnpackPositionalArgs gives for an *int: one for a value that is not an int,
// and one for an int that does not fit. idx counts as it does for StringAt.
//
// Revisions:
//   - 2026-10-29 23:58: initial creation
func _SizeAt(
	fn *starlark.Builtin,
	args starlark.Tuple,
	idx int,
) (int, error) {
	number, err := _IntAt(
		fn,
		args,
		idx,
	)
	if err != nil {
		return 0, err
	}

	var (
		wide, ok = number.Int64()
		fits     = ok && int64(int(wide)) == wide
	)
	if !fits {
		format := "%s: for parameter %d: %s out of range " +
			"(want value in signed %d-bit range)"

		return 0, fmt.Errorf(
			format,
			fn.Name(),
			idx+1,
			number,
			strconv.IntSize,
		)
	}

	return int(wide), nil
}
