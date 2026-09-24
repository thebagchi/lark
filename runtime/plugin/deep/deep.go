// Package deep copies a Starlark value and everything under it.
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
func _List(original *starlark.List, seen map[starlark.Value]starlark.Value) (starlark.Value, error) {
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
func _Dict(original *starlark.Dict, seen map[starlark.Value]starlark.Value) (starlark.Value, error) {
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
func _Tuple(original starlark.Tuple, seen map[starlark.Value]starlark.Value) (starlark.Value, error) {
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
