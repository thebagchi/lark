package unpack

import (
	"fmt"
	"strconv"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
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
//   - 2026-11-04 23:50: names a nil builtin ANONYMOUS, and refuses a missing argument or a nil
//     value (C86 fix round 3)
func Positional(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
	count int,
) error {
	if len(kwargs) > 0 {
		return fmt.Errorf(
			"%s: unexpected keyword arguments",
			_NameOf(fn),
		)
	}

	if len(args) != count {
		return fmt.Errorf(
			"%s: got %d arguments, want %d",
			_NameOf(fn),
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
//   - 2026-11-04 23:50: names a nil builtin ANONYMOUS, and refuses a missing argument or a nil
//     value (C86 fix round 3)
func StringAt(
	fn *starlark.Builtin,
	args starlark.Tuple,
	idx int,
) (string, error) {
	err := _Missing(
		fn,
		args,
		idx,
	)
	if err != nil {
		return "", err
	}

	text, ok := starlark.AsString(args[idx])
	if !ok {
		return "", fmt.Errorf(
			"%s: for parameter %d: got %s, want string",
			_NameOf(fn),
			idx+1,
			_TypeOf(args[idx]),
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
//   - 2026-11-04 23:50: names a nil builtin ANONYMOUS, and refuses a missing argument or a nil
//     value (C86 fix round 3)
func CallableAt(
	fn *starlark.Builtin,
	args starlark.Tuple,
	idx int,
) (starlark.Callable, error) {
	err := _Missing(
		fn,
		args,
		idx,
	)
	if err != nil {
		return nil, err
	}

	callable, ok := args[idx].(starlark.Callable)
	if !ok {
		return nil, fmt.Errorf(
			"%s: for parameter %d: got %s, want callable",
			_NameOf(fn),
			idx+1,
			_TypeOf(args[idx]),
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
//   - 2026-11-04 23:50: names a nil builtin ANONYMOUS, and refuses a missing argument or a nil
//     value (C86 fix round 3)
func _IntAt(
	fn *starlark.Builtin,
	args starlark.Tuple,
	idx int,
) (starlark.Int, error) {
	err := _Missing(
		fn,
		args,
		idx,
	)
	if err != nil {
		return starlark.Int{}, err
	}

	number, ok := args[idx].(starlark.Int)
	if !ok {
		return starlark.Int{}, fmt.Errorf(
			"%s: for parameter %d: got %s, want int",
			_NameOf(fn),
			idx+1,
			_TypeOf(args[idx]),
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
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-04 23:50: names a nil builtin ANONYMOUS, and refuses a missing argument or a nil
//     value (C86 fix round 3)
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
			_NameOf(fn),
			idx+1,
			ShortText(number.String()),
			strconv.IntSize,
		)
	}

	return int(wide), nil
}

// _NameOf is fn's name, or ANONYMOUS for a nil builtin, which only a Go caller
// can pass and whose Name would dereference it.
//
// Revisions:
//   - 2026-11-04 23:50: initial creation (C86 fix round 3)
//   - 2026-11-08 00:59: names read from common (C91w12c, Q244)
func _NameOf(fn *starlark.Builtin) string {
	if fn == nil {
		return common.ANONYMOUS
	}

	return fn.Name()
}

// _TypeOf is the type of a value, or "nothing" for a nil value, which has no
// Type method to ask.
//
// Revisions:
//   - 2026-11-04 23:50: initial creation (C86 fix round 3)
func _TypeOf(value starlark.Value) string {
	if value == nil {
		return "nothing"
	}

	return value.Type()
}

// _Missing is the refusal for an argument position the caller did not fill:
// out of range, since there is no argument there to take.
//
// Revisions:
//   - 2026-11-04 23:50: initial creation (C86 fix round 3)
func _Missing(
	fn *starlark.Builtin,
	args starlark.Tuple,
	idx int,
) error {
	if idx >= 0 && idx < len(args) {
		return nil
	}

	return fmt.Errorf(
		"%s: for parameter %d: no such argument: %w",
		_NameOf(fn),
		idx+1,
		ErrRange,
	)
}
