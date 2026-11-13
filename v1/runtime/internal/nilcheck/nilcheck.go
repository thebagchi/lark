// Package nilcheck is internal, not public: it holds the one check for a nil
// value of any kind, which the packages that guard a value they were handed
// share. It imports nothing of lark, so every package of the runtime can
// import it without a cycle.
//
// Revisions:
//   - 2026-11-11 06:40: initial creation, moved here from the guard package (C122, Q377)
package nilcheck

import "reflect"

// Absent reports whether value is nothing: nil, or a nil of a kind that can be
// nil (a pointer, map, slice, function, channel, interface or unsafe pointer)
// held in an interface. A method called on the second kind may dereference it,
// and a plain nil check cannot see it, so reflect looks.
//
// One answer for every package that guards a value it was handed: a syntax
// node, a callable, a plugin and a plugin's value. A map, slice or function
// that is made but empty is there. Whether a nil of some kind is a value to
// refuse is the caller's to say; a nil starlark.Tuple, for one, is an empty
// tuple.
//
// Revisions:
//   - 2026-11-11 06:40: initial creation, from four copies of this check in artifact,
//     rewrite, run and registry (C122, B268, B267, Q377)
func Absent[T any](value T) bool {
	held := reflect.ValueOf(value)
	if !held.IsValid() {
		return true
	}

	switch held.Kind() {
	case reflect.Pointer,
		reflect.Map,
		reflect.Slice,
		reflect.Func,
		reflect.Chan,
		reflect.Interface,
		reflect.UnsafePointer:
		return held.IsNil()

	default:
		return false
	}
}
