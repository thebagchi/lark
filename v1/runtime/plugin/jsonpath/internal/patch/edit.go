// Package patch holds the jsonpath plugin: the builtins that find, extract,
// match, measure and patch a JSON value, the JSON pointers they read, and the
// edit that applies a patch. Package jsonpath keeps the registration and the
// sentinels.
//
// Revisions:
//   - 2026-11-10 01:37: initial creation (C120b, Q315)
package patch

import (
	"fmt"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/jsonpath/internal/spec"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// _Edit is one patch_json call, and the containers it has made itself.
//
// A container this edit made is held by the document it is building and by
// nothing else, so it is changed in place. Any other container may be held
// elsewhere - it came in with the document, or with a value an operation
// carries - so it is copied the first time this edit writes under it, and the
// copy is this edit's from then on. A patch of many operations then copies a
// container once rather than once an operation, and the document handed in is
// still never modified.
//
// made holds them by identity: a dict and a list are pointers, which a Go map
// hashes.
//
// thread is the thread the patch runs on, which a copy reads its context from.
//
// Revisions:
//   - 2026-10-04 01:25: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
type _Edit struct {
	made   map[starlark.Value]bool
	thread *starlark.Thread
}

// _NewEdit returns an edit that has made nothing.
//
// Revisions:
//   - 2026-10-03 16:48: initial creation
//   - 2026-11-02 12:28: remembers the thread, for the context of a copy (C26)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func _NewEdit(thread *starlark.Thread) *_Edit {
	return &_Edit{
		made:   map[starlark.Value]bool{},
		thread: thread,
	}
}

// _Insert returns doc with value added at steps: a new member of a dict, or
// an element inserted before a list position, "-" meaning after the last.
//
// The caller's containers are never mutated: each along the path is copied the
// first time this edit writes under it, and the rest is shared, so a document
// that is frozen, or reachable from another thread, is safe to patch.
//
// Revisions:
//   - 2026-09-20 01:03: initial creation, as _Set with a flag
//   - 2026-09-21 08:09: one of two functions where a flag chose between them
//   - 2026-10-03 16:48: a method of _Edit, which copies a container once per patch
//   - 2026-11-02 00:30: passes _SpliceIn to _Descend, with no _PlaceNew
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func (e *_Edit) _Insert(
	doc starlark.Value,
	steps []string,
	value starlark.Value,
) (starlark.Value, error) {
	return e._Descend(
		doc,
		steps,
		value,
		e._SpliceIn,
	)
}

// _Replace returns doc with the value at steps replaced. The path must exist,
// which the caller has checked.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
//   - 2026-10-03 16:48: a method of _Edit, which copies a container once per patch
//   - 2026-11-02 00:30: passes _SpliceOver to _Descend, with no _PlaceOver
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func (e *_Edit) _Replace(
	doc starlark.Value,
	steps []string,
	value starlark.Value,
) (starlark.Value, error) {
	return e._Descend(
		doc,
		steps,
		value,
		e._SpliceOver,
	)
}

// _Descend makes the containers along steps this edit's and lets splice decide
// what the last one does with value when it is a list: insert, or overwrite.
// Every container above it has its child set again, overwriting.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
//   - 2026-10-03 16:48: a method of _Edit, owning the containers along steps rather
//     than copying each one every time
//   - 2026-11-02 00:30: takes the list difference, splice, in place of a place func
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func (e *_Edit) _Descend(
	doc starlark.Value,
	steps []string,
	value starlark.Value,
	splice func(
		*starlark.List,
		string,
		starlark.Value,
	) (starlark.Value, error),
) (starlark.Value, error) {
	if len(steps) == 0 {
		return value, nil
	}

	step := steps[0]

	if len(steps) == 1 {
		return e._Place(
			doc,
			step,
			value,
			splice,
		)
	}

	child, err := _Step(
		doc,
		step,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"%s: %w",
			unpack.QuotePrefix(step),
			err,
		)
	}

	replaced, err := e._Descend(
		child,
		steps[1:],
		value,
		splice,
	)
	if err != nil {
		return nil, err
	}

	return e._Place(
		doc,
		step,
		replaced,
		e._SpliceOver,
	)
}

// _Place returns holder, or this edit's copy of it, with value written under
// step. A dict sets the member either way; splice is the one difference, and
// says what a list does with the step: insert before it, or overwrite it.
//
// Revisions:
//   - 2026-11-02 00:30: initial creation, from the shared body of _PlaceNew and
//     _PlaceOver, which it replaces
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func (e *_Edit) _Place(
	holder starlark.Value,
	step string,
	value starlark.Value,
	splice func(
		*starlark.List,
		string,
		starlark.Value,
	) (starlark.Value, error),
) (starlark.Value, error) {
	switch container := holder.(type) {
	case *starlark.Dict:
		return e._Put(
			container,
			step,
			value,
		)

	case *starlark.List:
		return splice(
			container,
			step,
			value,
		)

	default:
		return nil, fmt.Errorf(
			"%s %w",
			holder.Type(),
			ErrKind,
		)
	}
}

// _Put returns holder, or this edit's copy of it, with one member set.
//
// Revisions:
//   - 2026-09-20 01:05: initial creation
//   - 2026-10-03 16:48: a method of _Edit, setting the member in place on a dict this
//     edit made rather than copying every member again
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func (e *_Edit) _Put(
	holder *starlark.Dict,
	step string,
	value starlark.Value,
) (starlark.Value, error) {
	dict, err := e._Dict(holder)
	if err != nil {
		return nil, fmt.Errorf(
			"%s: %w",
			unpack.QuotePrefix(step),
			err,
		)
	}

	err = dict.SetKey(
		starlark.String(step),
		value,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"%s: %w",
			unpack.QuotePrefix(step),
			err,
		)
	}

	return dict, nil
}

// _SpliceIn returns holder, or this edit's copy of it, with value inserted at
// step, which may be "-": RFC 6901's spelling of the position after the last
// element.
//
// Revisions:
//   - 2026-09-20 01:06: initial creation, as _Splice with a flag
//   - 2026-09-21 08:09: the inserting half
//   - 2026-10-03 16:48: a method of _Edit, inserting in place into a list this edit
//     made
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func (e *_Edit) _SpliceIn(
	holder *starlark.List,
	step string,
	value starlark.Value,
) (starlark.Value, error) {
	index := holder.Len()

	if step != spec.APPEND {
		at, err := _Index(
			step,
			holder.Len()+1,
		)
		if err != nil {
			return nil, err
		}

		index = at
	}

	var (
		list = e._List(holder)
		err  = list.Append(value)
	)
	if err != nil {
		return nil, fmt.Errorf(
			"%s: %w",
			unpack.QuotePrefix(step),
			err,
		)
	}

	err = _Shift(
		list,
		index,
		value,
		step,
	)
	if err != nil {
		return nil, err
	}

	return list, nil
}

// _Shift moves each element from index on one place, then writes value.
//
// The move starts at the end, so the appended element is overwritten rather
// than any that were there before.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func _Shift(
	list *starlark.List,
	index int,
	value starlark.Value,
	step string,
) error {
	for at := list.Len() - 1; at > index; at-- {
		err := list.SetIndex(
			at,
			list.Index(at-1),
		)
		if err != nil {
			return fmt.Errorf(
				"%s: %w",
				unpack.QuotePrefix(step),
				err,
			)
		}
	}

	err := list.SetIndex(
		index,
		value,
	)
	if err != nil {
		return fmt.Errorf(
			"%s: %w",
			unpack.QuotePrefix(step),
			err,
		)
	}

	return nil
}

// _SpliceOver returns holder, or this edit's copy of it, with the element at
// step replaced.
//
// "-" names no element, so it is refused: there is nothing there to replace.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
//   - 2026-10-03 16:48: a method of _Edit, replacing in place in a list this edit made
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func (e *_Edit) _SpliceOver(
	holder *starlark.List,
	step string,
	value starlark.Value,
) (starlark.Value, error) {
	if step == spec.APPEND {
		return nil, fmt.Errorf(
			"%q names no element: %w",
			spec.APPEND,
			ErrMissing,
		)
	}

	index, err := _Index(
		step,
		holder.Len(),
	)
	if err != nil {
		return nil, err
	}

	list := e._List(holder)

	err = list.SetIndex(
		index,
		value,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"%s: %w",
			unpack.QuotePrefix(step),
			err,
		)
	}

	return list, nil
}

// _Delete returns doc without whatever steps names.
//
// Revisions:
//   - 2026-09-20 01:07: initial creation
//   - 2026-09-21 08:09: replaces the shortened child through _PlaceOver
//   - 2026-10-03 16:48: a method of _Edit, which copies a container once per patch
//   - 2026-11-02 00:30: replaces it through _Place, with _SpliceOver
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func (e *_Edit) _Delete(
	doc starlark.Value,
	steps []string,
) (starlark.Value, error) {
	step := steps[0]

	if len(steps) == 1 {
		return e._Drop(
			doc,
			step,
		)
	}

	child, err := _Step(
		doc,
		step,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"%s: %w",
			unpack.QuotePrefix(step),
			err,
		)
	}

	shortened, err := e._Delete(
		child,
		steps[1:],
	)
	if err != nil {
		return nil, err
	}

	return e._Place(
		doc,
		step,
		shortened,
		e._SpliceOver,
	)
}

// _Drop returns holder without step, which the caller has checked is there: a
// dict this edit made loses the member in place, a dict of the caller's is
// copied first, and a list is always made again, since a list cannot be
// shortened in place.
//
// Revisions:
//   - 2026-09-20 01:08: initial creation
//   - 2026-10-03 16:48: a method of _Edit, deleting in place from a dict this edit
//     made
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func (e *_Edit) _Drop(
	holder starlark.Value,
	step string,
) (starlark.Value, error) {
	switch container := holder.(type) {
	case *starlark.Dict:
		dict, err := e._Dict(container)
		if err != nil {
			return nil, fmt.Errorf(
				"%s: %w",
				unpack.QuotePrefix(step),
				err,
			)
		}

		_, _, err = dict.Delete(starlark.String(step))
		if err != nil {
			return nil, fmt.Errorf(
				"%s: %w",
				unpack.QuotePrefix(step),
				err,
			)
		}

		return dict, nil

	case *starlark.List:
		return e._DropList(
			container,
			step,
		)

	default:
		return nil, fmt.Errorf(
			"%s %w",
			holder.Type(),
			ErrKind,
		)
	}
}

// _DropList returns the list without the element step names.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func (e *_Edit) _DropList(
	container *starlark.List,
	step string,
) (starlark.Value, error) {
	index, err := _Index(
		step,
		container.Len(),
	)
	if err != nil {
		return nil, err
	}

	elements := make(
		[]starlark.Value,
		0,
		container.Len()-1,
	)

	for at := range container.Len() {
		if at == index {
			continue
		}

		elements = append(
			elements,
			container.Index(at),
		)
	}

	list := starlark.NewList(elements)
	e.made[list] = true

	return list, nil
}

// _Dict is holder when this edit made it, and otherwise a copy of it, which
// this edit has made from then on.
//
// The copy is _CopyDict's: the loop over Entries is a closure, and a closure
// in this function moved its results to the heap on every call, including the
// call that copies nothing.
//
// Revisions:
//   - 2026-10-03 16:48: initial creation, from _Put's copy
//   - 2026-11-02 11:35: copies through _CopyDict, so the made path allocates nothing
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func (e *_Edit) _Dict(holder *starlark.Dict) (*starlark.Dict, error) {
	if e.made[holder] {
		return holder, nil
	}

	dict, err := _CopyDict(holder)
	if err != nil {
		return nil, err
	}

	e.made[dict] = true

	return dict, nil
}

// _CopyDict returns a copy of holder, with room for one more member.
//
// Revisions:
//   - 2026-11-02 11:35: initial creation, from the copy in _Dict
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func _CopyDict(holder *starlark.Dict) (*starlark.Dict, error) {
	dict := starlark.NewDict(holder.Len() + 1)

	// Entries rather than Items: Items builds a slice of every pair first,
	// which on a large dict is a third of what the copy allocates.
	for key, value := range holder.Entries() {
		err := dict.SetKey(
			key,
			value,
		)
		if err != nil {
			return nil, err
		}
	}

	return dict, nil
}

// _List is holder when this edit made it, and otherwise a copy of it, which
// this edit has made from then on.
//
// Revisions:
//   - 2026-10-03 16:48: initial creation, from _SpliceIn's and _SpliceOver's copies
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func (e *_Edit) _List(holder *starlark.List) *starlark.List {
	if e.made[holder] {
		return holder
	}

	list := starlark.NewList(_Elements(holder))
	e.made[list] = true

	return list
}

// _Elements is a list's elements in a slice with room for one more.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func _Elements(holder *starlark.List) []starlark.Value {
	elements := make(
		[]starlark.Value,
		0,
		holder.Len()+1,
	)

	for index := range holder.Len() {
		elements = append(
			elements,
			holder.Index(index),
		)
	}

	return elements
}

// _Equal reports whether two values are deeply equal.
//
// The interpreter's own equality, which already treats 1 and 1.0 as one
// number at any depth - measured 2026-09-21: 1 == 1.0, [1] == [1.0] and
// {"a": 1} == {"a": 1.0} are all True. A special case for numbers used to sit
// here and duplicated that.
//
// Revisions:
//   - 2026-09-20 01:09: initial creation
//   - 2026-09-21 08:09: the interpreter's equality alone
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/jsonpath (C120b, Q315)
func _Equal(
	left starlark.Value,
	right starlark.Value,
) (bool, error) {
	return starlark.EqualDepth(
		left,
		right,
		starlark.CompareLimit,
	)
}
