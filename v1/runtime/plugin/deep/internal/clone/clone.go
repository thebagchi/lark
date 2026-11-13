// Package clone holds the walk of the deep plugin: it copies a Starlark value to
// a plain one, finds the calls in a syntax tree, and walks a value with a map of
// what it has seen. The Copy, IsData, ShowsCode and Seek entry points stay in
// package deep.
//
// Revisions:
//   - 2026-11-10 01:37: initial creation (C120b, Q315)
package clone

import (
	"fmt"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/trail"
)

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
//   - 2026-09-21 08:09: consults seen only for a container, so a tuple never
//     reaches a map that cannot hash it
//   - 2026-09-24 16:08: moved here
//   - 2026-10-03 08:26: unexported, Copy being how anything outside copies
//   - 2026-11-02 12:28: carries the trail, which counts the depth and looks at
//     the context (C26)
//   - 2026-11-04 19:30: a nil *List, *Dict or *Set is returned as it is, as a
//     scalar is: it holds nothing to copy (C87)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/deep and renamed _Into to Into, for the
//     tests in tests/ (C120b, Q315, Q314)
func Into(
	path *common.Trail,
	value starlark.Value,
	seen map[starlark.Value]starlark.Value,
) (starlark.Value, error) {
	switch original := value.(type) {
	case *starlark.List:
		if original == nil {
			return value, nil
		}

		return _List(
			path,
			original,
			seen,
		)

	case *starlark.Dict:
		if original == nil {
			return value, nil
		}

		return _Dict(
			path,
			original,
			seen,
		)

	case starlark.Tuple:
		return _Tuple(
			path,
			original,
			seen,
		)

	case *starlark.Set:
		if original == nil {
			return value, nil
		}

		return _Set(
			path,
			original,
			seen,
		)

	default:
		return value, trail.Tick(path)
	}
}

// _List copies a list, registering the copy before filling it so a list
// that contains itself terminates.
//
// Revisions:
//   - 2026-09-20 00:44: initial creation
//   - 2026-09-21 08:09: reuses a copy already made
//   - 2026-11-02 12:28: enters the trail, so a list nested past common.DEPTH is
//     an error (C26)
//   - 2026-11-08 21:23: doc names the function as renamed in 48ad095 (C113, B233)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/deep (C120b, Q315)
func _List(
	path *common.Trail,
	original *starlark.List,
	seen map[starlark.Value]starlark.Value,
) (starlark.Value, error) {
	copied, found := seen[original]
	if found {
		return copied, nil
	}

	err := trail.Enter(
		path,
		original,
	)
	if err != nil {
		return nil, err
	}

	defer trail.Leave(path)

	made := starlark.NewList(make(
		[]starlark.Value,
		0,
		original.Len(),
	))

	seen[original] = made

	for index := range original.Len() {
		element, err := Into(
			path,
			original.Index(index),
			seen,
		)
		if err != nil {
			return nil, err
		}

		err = made.Append(element)
		if err != nil {
			return nil, fmt.Errorf(
				"copy element %d: %w",
				index,
				err,
			)
		}
	}

	return made, nil
}

// _Dict copies a dict, registering the copy before filling it.
//
// A key is taken as it is. A key must hash, and nothing that hashes is mutable:
// a list, a dict and a set refuse to hash, frozen or not, and a tuple hashes
// only when everything in it does. The copy and the original then share the
// key, and neither side can reach the other through it. The trail still counts
// each key.
//
// The pairs are walked through Entries rather than Items, which builds a slice
// of every pair first: on a dict of 4096 members, a third of what the copy
// allocated.
//
// Revisions:
//   - 2026-09-20 00:45: initial creation
//   - 2026-09-21 08:09: reuses a copy already made
//   - 2026-10-03 17:08: walks the pairs without first building a slice of them
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-02 12:28: enters the trail (C26)
//   - 2026-11-02 16:34: keeps each key as it is, not a copy of it (C30)
//   - 2026-11-08 21:23: doc names the function as renamed in 48ad095 (C113, B233)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/deep (C120b, Q315)
func _Dict(
	path *common.Trail,
	original *starlark.Dict,
	seen map[starlark.Value]starlark.Value,
) (starlark.Value, error) {
	copied, found := seen[original]
	if found {
		return copied, nil
	}

	err := trail.Enter(
		path,
		original,
	)
	if err != nil {
		return nil, err
	}

	defer trail.Leave(path)

	made := starlark.NewDict(original.Len())

	seen[original] = made

	for name, value := range original.Entries() {
		// err is declared here, not assigned from outside: the body of a range
		// over Entries is a closure, and an outer err it assigns moves to the
		// heap, once for each dict.
		err := trail.Tick(path)
		if err != nil {
			return nil, err
		}

		held, err := Into(
			path,
			value,
			seen,
		)
		if err != nil {
			return nil, err
		}

		err = made.SetKey(
			name,
			held,
		)
		if err != nil {
			return nil, fmt.Errorf(
				"copy key %s: %w",
				unpack.ShortText(name.String()),
				err,
			)
		}
	}

	return made, nil
}

// _Tuple copies a tuple.
//
// A tuple cannot be changed, but what it holds can. So a tuple that holds a
// list, a dict or a set, at any depth, is rebuilt with those parts copied. A
// tuple that holds none is returned as it is: it is immutable and so is
// everything under it, and a copy would be an allocation that changes nothing.
//
// The rebuilt tuple is made only at the first element that changed, so a tuple
// of scalars allocates nothing here. Every element still passes through Into,
// so the trail counts it as before.
//
// It is not registered in seen beforehand: a tuple cannot contain itself,
// because it is built complete.
//
// Revisions:
//   - 2026-09-20 00:46: initial creation
//   - 2026-11-02 12:28: enters the trail with nil, a tuple not being comparable (C26)
//   - 2026-11-02 16:34: returns the original when nothing under it is mutable (C30)
//   - 2026-11-08 21:23: doc names the function as renamed in 48ad095 (C113, B233)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/deep (C120b, Q315)
func _Tuple(
	path *common.Trail,
	original starlark.Tuple,
	seen map[starlark.Value]starlark.Value,
) (starlark.Value, error) {
	err := trail.Enter(
		path,
		nil,
	)
	if err != nil {
		return nil, err
	}

	defer trail.Leave(path)

	var made starlark.Tuple

	for index, element := range original {
		copied, err := Into(
			path,
			element,
			seen,
		)
		if err != nil {
			return nil, err
		}

		if made == nil && !_Changed(
			element,
			copied,
		) {
			continue
		}

		if made == nil {
			made = make(
				starlark.Tuple,
				index,
				len(original),
			)

			copy(
				made,
				original[:index],
			)
		}

		made = append(
			made,
			copied,
		)
	}

	if made == nil {
		return original, nil
	}

	return made, nil
}

// _Changed reports whether copying element gave something other than element.
//
// A list, a dict and a set are always new. A tuple is the same when it is
// the same slice: it is not comparable with ==, which panics, so the first
// element's address is compared. Anything else is a scalar, a function or a
// plugin value, and Into returns it as it is.
//
// Revisions:
//   - 2026-11-02 16:34: initial creation (C30)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/deep (C120b, Q315)
func _Changed(
	element starlark.Value,
	copied starlark.Value,
) bool {
	switch held := element.(type) {
	case *starlark.List, *starlark.Dict, *starlark.Set:
		return true

	case starlark.Tuple:
		now, ok := copied.(starlark.Tuple)

		return !ok || len(now) != len(held) ||
			len(held) > 0 && &now[0] != &held[0]
	}

	return false
}

// _Set copies a set.
//
// A set holds only hashable values, and nothing hashable is mutable, so the
// members are taken as they are and only the set itself is new.
//
// Revisions:
//   - 2026-09-20 00:47: initial creation
//   - 2026-09-21 08:09: reuses a copy already made
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-02 12:28: enters the trail and counts each member (C26)
//   - 2026-11-08 21:23: doc names the function as renamed in 48ad095 (C113, B233)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/deep (C120b, Q315)
func _Set(
	path *common.Trail,
	original *starlark.Set,
	seen map[starlark.Value]starlark.Value,
) (starlark.Value, error) {
	copied, found := seen[original]
	if found {
		return copied, nil
	}

	err := trail.Enter(
		path,
		original,
	)
	if err != nil {
		return nil, err
	}

	defer trail.Leave(path)

	made := starlark.NewSet(original.Len())

	seen[original] = made

	iterator := original.Iterate()
	defer iterator.Done()

	var member starlark.Value

	for iterator.Next(&member) {
		err = trail.Tick(path)
		if err != nil {
			return nil, err
		}

		err = made.Insert(member)
		if err != nil {
			return nil, fmt.Errorf(
				"copy member %s: %w",
				unpack.ShortText(member.String()),
				err,
			)
		}
	}

	return made, nil
}

// Walk is IsData, carrying what it has already seen. It returns what is not
// data, or an error, and nothing when value is data.
//
// Two results, not three: a third result on a function that returns from
// inside a range over Entries moved every result to the heap, once for each
// dict (measured in the research, q2_nocrash.md section 4).
//
// A nil path is a scalar's: it holds nothing, so there is nothing to count.
//
// Revisions:
//   - 2026-09-24 20:14: initial creation
//   - 2026-11-02 12:28: carries the trail and returns an error in place of
//     the boolean (C26)
//   - 2026-11-04 19:30: a nil *List, *Dict or *Set is what is not data (C87)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/deep and renamed _Walk to Walk, for the
//     tests in tests/ (C120b, Q315, Q314)
//   - 2026-11-13 09:37: a nil value, as an element or as the root, is what is not data, and comes
//     back as Missing, tested after the cases of the known types so that they pay nothing;
//     Freeze of a container holding one panicked (C131, B293, Q429)
func Walk(
	path *common.Trail,
	value starlark.Value,
	seen map[starlark.Value]bool,
) (starlark.Value, error) {
	switch held := value.(type) {
	case starlark.NoneType, starlark.Bool, starlark.Int, starlark.Float,
		starlark.String, starlark.Bytes:
		if path == nil {
			return nil, nil
		}

		return nil, trail.Tick(path)

	case *starlark.List:
		if held == nil {
			return value, nil
		}

		return _Every(
			path,
			held,
			seen,
		)

	case *starlark.Dict:
		if held == nil {
			return value, nil
		}

		return _Pairs(
			path,
			held,
			seen,
		)

	case starlark.Tuple:
		return _Items(
			path,
			held,
			seen,
		)

	case *starlark.Set:
		if held == nil {
			return value, nil
		}

		return _Every(
			path,
			held,
			seen,
		)
	}

	// Tested here, after the cases for the types that are data, so that a value
	// of a known type pays nothing for it: a nil interface matches no case.
	if value == nil {
		return &Missing{}, nil
	}

	return value, nil
}

// _Every walks a list or a set.
//
// Revisions:
//   - 2026-09-24 20:10: initial creation
//   - 2026-11-02 12:28: enters the trail (C26)
//   - 2026-11-06 00:08: a nil seen map is a new one, made here and not in _Walk, since
//     IsData passes nil for a scalar and must not pay for a map (C100a, B173)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/deep (C120b, Q315)
func _Every(
	path *common.Trail,
	held starlark.Iterable,
	seen map[starlark.Value]bool,
) (starlark.Value, error) {
	if seen == nil {
		seen = make(map[starlark.Value]bool)
	}

	if seen[held] {
		return nil, nil
	}

	seen[held] = true

	err := trail.Enter(
		path,
		held,
	)
	if err != nil {
		return nil, err
	}

	defer trail.Leave(path)

	iter := held.Iterate()
	defer iter.Done()

	var value starlark.Value

	for iter.Next(&value) {
		bad, err := Walk(
			path,
			value,
			seen,
		)
		if bad != nil || err != nil {
			return bad, err
		}
	}

	return nil, nil
}

// _Pairs walks a dict, keys as well as values.
//
// Through Entries rather than Items, which builds a slice of every pair first,
// and on a dict of 4096 members that slice was all the walk allocated.
//
// Revisions:
//   - 2026-09-24 20:10: initial creation
//   - 2026-10-03 17:08: walks the pairs without first building a slice of them
//   - 2026-11-02 12:28: enters the trail (C26)
//   - 2026-11-06 00:08: a nil seen map is a new one, as in _Every (C100a, B173)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/deep (C120b, Q315)
func _Pairs(
	path *common.Trail,
	held *starlark.Dict,
	seen map[starlark.Value]bool,
) (starlark.Value, error) {
	if seen == nil {
		seen = make(map[starlark.Value]bool)
	}

	if seen[held] {
		return nil, nil
	}

	seen[held] = true

	err := trail.Enter(
		path,
		held,
	)
	if err != nil {
		return nil, err
	}

	defer trail.Leave(path)

	for key, value := range held.Entries() {
		bad, err := Walk(
			path,
			key,
			seen,
		)
		if bad != nil || err != nil {
			return bad, err
		}

		bad, err = Walk(
			path,
			value,
			seen,
		)
		if bad != nil || err != nil {
			return bad, err
		}
	}

	return nil, nil
}

// _Items walks a tuple.
//
// A tuple is a slice rather than a pointer, so it cannot be a map key and is
// not recorded in seen. It cannot contain itself either, for the same reason:
// there is nothing to take the address of. It enters the trail as nil, so the
// depth counts it.
//
// Revisions:
//   - 2026-09-24 20:10: initial creation
//   - 2026-11-02 12:28: enters the trail (C26)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/deep (C120b, Q315)
func _Items(
	path *common.Trail,
	held starlark.Tuple,
	seen map[starlark.Value]bool,
) (starlark.Value, error) {
	err := trail.Enter(
		path,
		nil,
	)
	if err != nil {
		return nil, err
	}

	defer trail.Leave(path)

	for _, value := range held {
		bad, err := Walk(
			path,
			value,
			seen,
		)
		if bad != nil || err != nil {
			return bad, err
		}
	}

	return nil, nil
}
