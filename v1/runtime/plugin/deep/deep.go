// Package deep reads a Starlark value and everything under it: copying one,
// and saying whether one is data.
//
// It supplies no names to a script and is not a plugin. It is here rather than
// inside state because two plugins need the same copy for the same reason:
// state hands a reader something it owns, and a JSON patch that copies a value
// must give the copy its own containers. Two implementations of "copy this and
// everything under it" would be two sets of cycle handling to keep in step.
package deep

import (
	"fmt"

	"go.starlark.net/starlark"
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
func Copy(value starlark.Value) (starlark.Value, error) {
	return Into(value, map[starlark.Value]starlark.Value{})
}

// Into copies value, reusing whatever has already been copied.
//
// The seen map is what makes a self-referential value safe. Starlark allows
// one - x = [1]; x.append(x) is legal - and a copy that did not remember what
// it had already made would follow that reference until the stack ran out.
//
// It also preserves sharing: a value reachable twice is copied once, so a
// script that reads back a structure with two paths to one list still has two
// paths to one list.
//
// Only the three mutable containers consult the map, each in its own copier.
// They are the only values that can alias or cycle, and they are pointers,
// which a Go map can hash. A tuple is a slice, which a Go map cannot: looking
// every value up here panicked on the first tuple a script stored.
//
// Revisions:
//   - 2026-09-20 00:43: initial creation, as state's own _CopyInto
//   - 2026-09-24 16:08: moved here
//   - 2026-09-21 08:09: consults seen only for a container, so a tuple never
//     reaches a map that cannot hash it
func Into(
	value starlark.Value,
	seen map[starlark.Value]starlark.Value,
) (starlark.Value, error) {
	switch original := value.(type) {
	case *starlark.List:
		return _List(original, seen)

	case *starlark.Dict:
		return _Dict(original, seen)

	case starlark.Tuple:
		return _Tuple(original, seen)

	case *starlark.Set:
		return _Set(original, seen)

	default:
		return value, nil
	}
}

// _CopyList copies a list, registering the copy before filling it so a list
// that contains itself terminates.
//
// Revisions:
//   - 2026-09-20 00:44: initial creation
//   - 2026-09-21 08:09: reuses a copy already made
func _List(
	original *starlark.List,
	seen map[starlark.Value]starlark.Value,
) (starlark.Value, error) {
	copied, found := seen[original]
	if found {
		return copied, nil
	}

	made := starlark.NewList(make([]starlark.Value, 0, original.Len()))

	seen[original] = made

	for index := range original.Len() {
		element, err := Into(original.Index(index), seen)
		if err != nil {
			return nil, err
		}

		err = made.Append(element)
		if err != nil {
			return nil, fmt.Errorf("copy element %d: %w", index, err)
		}
	}

	return made, nil
}

// _CopyDict copies a dict, registering the copy before filling it.
//
// Keys are copied too, because a tuple key may hold a mutable value.
//
// Revisions:
//   - 2026-09-20 00:45: initial creation
//   - 2026-09-21 08:09: reuses a copy already made
func _Dict(
	original *starlark.Dict,
	seen map[starlark.Value]starlark.Value,
) (starlark.Value, error) {
	copied, found := seen[original]
	if found {
		return copied, nil
	}

	made := starlark.NewDict(original.Len())

	seen[original] = made

	for _, item := range original.Items() {
		key, err := Into(item[0], seen)
		if err != nil {
			return nil, err
		}

		held, err := Into(item[1], seen)
		if err != nil {
			return nil, err
		}

		err = made.SetKey(key, held)
		if err != nil {
			return nil, fmt.Errorf("copy key %s: %w", item[0].String(), err)
		}
	}

	return made, nil
}

// _CopyTuple copies a tuple.
//
// A tuple cannot be changed, but what it holds can, so its elements are copied
// and the tuple itself is rebuilt. It is not registered in seen beforehand: a
// tuple cannot contain itself, because it is built complete.
//
// Revisions:
//   - 2026-09-20 00:46: initial creation
func _Tuple(
	original starlark.Tuple,
	seen map[starlark.Value]starlark.Value,
) (starlark.Value, error) {
	made := make(starlark.Tuple, 0, len(original))

	for _, element := range original {
		copied, err := Into(element, seen)
		if err != nil {
			return nil, err
		}

		made = append(made, copied)
	}

	return made, nil
}

// _CopySet copies a set.
//
// A set holds only hashable values, and nothing hashable is mutable, so the
// members are taken as they are and only the set itself is new.
//
// Revisions:
//   - 2026-09-20 00:47: initial creation
//   - 2026-09-21 08:09: reuses a copy already made
func _Set(original *starlark.Set, seen map[starlark.Value]starlark.Value) (starlark.Value, error) {
	copied, found := seen[original]
	if found {
		return copied, nil
	}

	made := starlark.NewSet(original.Len())

	seen[original] = made

	iterator := original.Iterate()
	defer iterator.Done()

	var member starlark.Value

	for iterator.Next(&member) {
		err := made.Insert(member)
		if err != nil {
			return nil, fmt.Errorf("copy member %s: %w", member.String(), err)
		}
	}

	return made, nil
}

// IsData reports whether value, and everything under it, is data.
//
// Data is what a value can be built from and read back as: None, a bool, a
// number, a string, bytes, and the containers of those. A function is code, a
// handle names a thread, a module is a plugin's own table - none of them mean
// anything to whoever did not make them, so a caller passing one across a
// boundary has made a mistake that otherwise reads as if it worked.
//
// The first return is what failed, so a caller can say which thing was wrong
// rather than naming the outermost container. It is nil when the answer is
// yes.
//
// Deep, because a list of functions is no more data than a function is. A
// value that contains itself terminates, which Starlark allows and the copier
// beside this already handles.
//
// Revisions:
//   - 2026-09-24 20:10: initial creation, as state's own _Data
//   - 2026-09-24 20:14: a question anything can ask, rather than one plugin's
func IsData(value starlark.Value) (starlark.Value, bool) {
	// Most values a script stores are one number or one word, and those
	// cannot contain anything - so the map the walk needs is built only once
	// something might be walked into.
	switch value.(type) {
	case *starlark.List, *starlark.Dict, starlark.Tuple, *starlark.Set:
		return _Walk(value, map[starlark.Value]bool{})
	}

	return _Walk(value, nil)
}

// _Walk is IsData, carrying what it has already seen.
//
// Revisions:
//   - 2026-09-24 20:14: initial creation
func _Walk(value starlark.Value, seen map[starlark.Value]bool) (starlark.Value, bool) {
	switch held := value.(type) {
	case starlark.NoneType, starlark.Bool, starlark.Int, starlark.Float,
		starlark.String, starlark.Bytes:
		return nil, true

	case *starlark.List:
		return _Every(held, seen)

	case *starlark.Dict:
		return _Pairs(held, seen)

	case starlark.Tuple:
		return _Items(held, seen)

	case *starlark.Set:
		return _Every(held, seen)
	}

	return value, false
}

// _Every walks a list or a set.
//
// Revisions:
//   - 2026-09-24 20:10: initial creation
func _Every(held starlark.Iterable, seen map[starlark.Value]bool) (starlark.Value, bool) {
	if seen[held] {
		return nil, true
	}

	seen[held] = true

	iter := held.Iterate()
	defer iter.Done()

	var value starlark.Value

	for iter.Next(&value) {
		bad, ok := _Walk(value, seen)
		if !ok {
			return bad, false
		}
	}

	return nil, true
}

// _Pairs walks a dict, keys as well as values.
//
// Revisions:
//   - 2026-09-24 20:10: initial creation
func _Pairs(held *starlark.Dict, seen map[starlark.Value]bool) (starlark.Value, bool) {
	if seen[held] {
		return nil, true
	}

	seen[held] = true

	for _, pair := range held.Items() {
		for _, value := range pair {
			bad, ok := _Walk(value, seen)
			if !ok {
				return bad, false
			}
		}
	}

	return nil, true
}

// _Items walks a tuple.
//
// A tuple is a slice rather than a pointer, so it cannot be a map key and is
// not recorded in seen. It cannot contain itself either, for the same reason:
// there is nothing to take the address of.
//
// Revisions:
//   - 2026-09-24 20:10: initial creation
func _Items(held starlark.Tuple, seen map[starlark.Value]bool) (starlark.Value, bool) {
	for _, value := range held {
		bad, ok := _Walk(value, seen)
		if !ok {
			return bad, false
		}
	}

	return nil, true
}
