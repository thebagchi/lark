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
		*err = _Raised(raised)
	}()

	*result, *err = fn()
}

// _Raised turns a recovered value into an error.
//
// A panic carrying an error is wrapped with %w, so errors.Is and errors.As keep
// working through the recovery. Anything else is wrapped with %v, because a
// panic value may be any type and there is nothing to unwrap.
//
// Revisions:
//   - 2026-09-20 12:00: initial creation, shared by WithRecover and Contained
func _Raised(raised any) error {
	failure, ok := raised.(error)
	if ok {
		return fmt.Errorf("recovered: %w", failure)
	}

	return fmt.Errorf("recovered: %v", raised)
}

// Contained runs fn and returns the panic it raised as an error, or nil.
//
// For work done for its effect, where there is no result to return but a
// failure still has to go somewhere. WithRecover is for a call whose result
// somebody is waiting for; this is for one that only has to happen.
//
// Without it, that case is written as a WithRecover over two throwaway
// variables, which reads as though a result mattered and then discards it. One
// primitive and two needs is how a guard becomes something each caller
// improvises a shape for.
//
// It returns the error rather than absorbing it, which is the whole difference
// between this and swallowing a panic: a caller decides what a failure means,
// and none of them may decide to ignore it silently.
//
// Revisions:
//   - 2026-09-20 11:58: initial creation, as Contain, which absorbed the panic
//   - 2026-09-20 12:00: returns the panic instead of swallowing it, so a
//     caller's bug is loud wherever it happens rather than only where a guard
//     happened to reach
func Contained(fn func()) error {
	var err error

	func() {
		defer func() {
			raised := recover()
			if raised == nil {
				return
			}

			err = _Raised(raised)
		}()

		fn()
	}()

	return err
}
