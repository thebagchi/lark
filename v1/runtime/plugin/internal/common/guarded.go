package common

import (
	"fmt"

	"go.starlark.net/starlark"
)

// Guarded is a builtin that runs check on its arguments and then calls inner.
//
// It has the name of inner. An error from check is wrapped as "name: error",
// so the check need not name the builtin. A call that passes the check goes to
// inner untouched, so its text and its result are inner's, byte for byte.
//
// Revisions:
//   - 2026-11-04 21:49: initial creation, from json's decode and core's str, repr, print
//     and fail (C59, A4e)
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
func Guarded(
	inner *starlark.Builtin,
	check func(
		*starlark.Thread,
		starlark.Tuple,
	) error,
) *starlark.Builtin {
	return starlark.NewBuiltin(
		Called(inner),
		func(
			thread *starlark.Thread,
			fn *starlark.Builtin,
			args starlark.Tuple,
			kwargs []starlark.Tuple,
		) (starlark.Value, error) {
			err := check(
				thread,
				args,
			)
			if err != nil {
				return nil, fmt.Errorf(
					"%s: %w",
					Called(fn),
					err,
				)
			}

			return inner.CallInternal(
				thread,
				args,
				kwargs,
			)
		},
	)
}
