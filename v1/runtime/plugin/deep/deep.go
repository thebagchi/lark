// Package deep reads a Starlark value and everything under it: copying one,
// and saying whether one is data - from the value when it runs, or from the
// source before it does, where the source shows a function.
//
// It supplies no names to a script and is not a plugin. It is here rather than
// inside state because two plugins need the same copy for the same reason:
// state hands a reader something it owns, and a JSON patch that copies a value
// must give the copy its own containers. Two implementations of "copy this and
// everything under it" would be two sets of cycle handling to keep in step.
//
// Revisions:
//   - 2026-09-20 00:39: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
package deep

import (
	"context"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/deep/internal/clone"
	"github.com/thebagchi/lark/v1/runtime/trail"
)

// Copy returns a deep, unfrozen copy of value.
//
// What is stored is frozen, so that several threads reading one name at once
// cannot be handed something one of them can change underneath the others. That
// is what makes the store safe, and it is also what would make it useless: a
// script that cannot change what it read back cannot build the next value from
// it.
//
// So the store keeps the frozen original and hands out copies. A script mutates
// its own copy freely, and nothing it does is visible to another thread until
// it stores the result.
//
// The cost is a copy per read, proportional to the size of the value. A script
// reading a large structure in a loop pays for it each time, and should read
// once outside the loop.
//
// Scalars are returned as they are: an int, a string, a bool, a float, bytes
// and None cannot be mutated, so a copy would be an allocation that changes
// nothing. Functions and builtins are returned as they are for the same reason
// - they hold no mutable state a script can reach.
//
// Revisions:
//   - 2026-09-20 00:42: initial creation, as state's own _Copy
//   - 2026-09-24 16:08: moved here, where jsonpath reaches it too
//   - 2026-10-29 01:22: builds the seen map only for a container, so a scalar
//     costs no allocation
//   - 2026-11-02 12:28: takes the context of the walk; a value nested past
//     common.DEPTH is trail.ErrTooDeep, which ends the run, and the walk looks
//     at ctx every common.POLL elements (C26)
//   - 2026-11-04 19:30: a nil container is returned as it is, not a panic (C87)
//   - 2026-11-10 11:32: uses clone.Into, which moved to internal/clone (C120b, Q315)
func Copy(
	ctx context.Context,
	value starlark.Value,
) (starlark.Value, error) {
	// A scalar comes back as it is and never reaches the map or the trail, so
	// both are built only for what can hold something. A store copies every
	// value a script reads, and most are one number or one word.
	switch value.(type) {
	case *starlark.List, *starlark.Dict, starlark.Tuple, *starlark.Set:
		return clone.Into(
			trail.Bounded(ctx),
			value,
			map[starlark.Value]starlark.Value{},
		)
	}

	return value, nil
}

// IsData reports whether value, and everything under it, is data.
//
// Data is what a value can be built from and read back as: None, a bool, a
// number, a string, bytes, and the containers of those. A function is code, a
// handle names a thread, a module is a plugin's own table - none of them mean
// anything to whoever did not make them, so a caller passing one across a
// boundary has made a mistake that otherwise reads as if it worked. A nil value,
// as an element or as the value itself, is not data either: a script cannot
// build one, a Go host can, and freezing the container that holds it panics.
//
// The first return is what failed, so a caller can say which thing was wrong
// rather than naming the outermost container. It is nil when the answer is
// yes.
//
// Deep, because a list of functions is no more data than a function is. A
// value that contains itself terminates, which Starlark allows and the copier
// beside this already handles: a cycle is data here, so the walk keeps a set
// of what it has seen and leaves the trail to count the depth.
//
// The error is not an answer about the value. It is trail.ErrDeep for a
// value nested past common.DEPTH, which ends the run, or the context's error
// when ctx is done. A caller returns it, wrapped with %w.
//
// Revisions:
//   - 2026-09-24 20:10: initial creation, as state's own _Data
//   - 2026-09-24 20:14: a question anything can ask, rather than one plugin's
//   - 2026-11-02 12:28: takes the context of the walk and returns an error as
//     a third result, for a value nested too deep (C26)
//   - 2026-11-04 19:30: a nil *List, *Dict or *Set is not data (C87)
//   - 2026-11-10 11:32: uses clone.Walk, which moved to internal/clone (C120b, Q315)
//   - 2026-11-10 17:15: uses ErrDeep, formerly ErrTooDeep (C121, Q349)
//   - 2026-11-13 09:37: a nil value, as an element or as the value, is not data (C131, B293, Q429)
func IsData(
	ctx context.Context,
	value starlark.Value,
) (starlark.Value, bool, error) {
	// Most values a script stores are one number or one word, and those
	// cannot contain anything - so the map and the trail the walk needs are
	// built only once something might be walked into.
	switch value.(type) {
	case *starlark.List, *starlark.Dict, starlark.Tuple, *starlark.Set:
		bad, err := clone.Walk(
			trail.Bounded(ctx),
			value,
			map[starlark.Value]bool{},
		)

		return bad, bad == nil && err == nil, err
	}

	bad, err := clone.Walk(
		nil,
		value,
		nil,
	)

	return bad, bad == nil && err == nil, err
}
