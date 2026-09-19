// Package guard converts a panic at a package boundary into an error.
package guard

import "fmt"

// WithRecover runs fn, storing its results through result and err, and turns a
// panic into an error rather than letting it leave the package.
//
// A panic never reaches a normal return, so converting one needs a deferred
// function that writes the results through pointers. This is the single shared
// place that does it: nothing else needs its own defer and recover, and no
// function needs a named return to support it.
//
// A panic carrying an error is stored as it is, so errors.Is and errors.As keep
// working through the recovery. Anything else is wrapped with %v, because a
// panic value may be any type and there is nothing to unwrap.
//
// Revisions:
//   - 2026-09-19 21:18: initial creation
func WithRecover[T any](result *T, err *error, fn func() (T, error)) {
	defer func() {
		raised := recover()
		if raised == nil {
			return
		}

		var empty T

		*result = empty

		failure, ok := raised.(error)
		if ok {
			*err = fmt.Errorf("recovered: %w", failure)

			return
		}

		*err = fmt.Errorf("recovered: %v", raised)
	}()

	*result, *err = fn()
}
