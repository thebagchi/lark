package jsonpath

import (
	"fmt"

	"go.starlark.net/starlark"
)

// _Insert returns doc with value added at steps: a new member of a dict, or
// an element inserted before a list position, "-" meaning after the last.
//
// Nothing is mutated. Every container along the path is copied and the rest is
// shared, so a document that is frozen, or reachable from another thread, is
// safe to patch.
//
// Revisions:
//   - 2026-09-20 01:03: initial creation, as _Set with a flag
//   - 2026-09-21 08:09: one of two functions where a flag chose between them
func _Insert(doc starlark.Value, steps []string, value starlark.Value) (starlark.Value, error) {
	return _Descend(doc, steps, value, _PlaceNew)
}

// _Replace returns doc with the value at steps replaced. The path must exist,
// which the caller has checked.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
func _Replace(doc starlark.Value, steps []string, value starlark.Value) (starlark.Value, error) {
	return _Descend(doc, steps, value, _PlaceOver)
}

// _Descend copies the containers along steps and lets place decide what the
// last one does with value; every container above it has its child replaced.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
func _Descend(
	doc starlark.Value,
	steps []string,
	value starlark.Value,
	place func(starlark.Value, string, starlark.Value) (starlark.Value, error),
) (starlark.Value, error) {
	if len(steps) == 0 {
		return value, nil
	}

	step := steps[0]

	if len(steps) == 1 {
		return place(doc, step, value)
	}

	child, err := _Step(doc, step)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", step, err)
	}

	replaced, err := _Descend(child, steps[1:], value, place)
	if err != nil {
		return nil, err
	}

	return _PlaceOver(doc, step, replaced)
}

// _PlaceNew returns a copy of holder with value added under step: a member
// set, or an element inserted.
//
// Revisions:
//   - 2026-09-20 01:04: initial creation, as _Place with a flag
//   - 2026-09-21 08:09: the inserting half
func _PlaceNew(holder starlark.Value, step string, value starlark.Value) (starlark.Value, error) {
	switch container := holder.(type) {
	case *starlark.Dict:
		return _Put(container, step, value)

	case *starlark.List:
		return _SpliceIn(container, step, value)

	default:
		return nil, fmt.Errorf("%s %w", holder.Type(), ErrKind)
	}
}

// _PlaceOver returns a copy of holder with what step names replaced by value.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
func _PlaceOver(holder starlark.Value, step string, value starlark.Value) (starlark.Value, error) {
	switch container := holder.(type) {
	case *starlark.Dict:
		return _Put(container, step, value)

	case *starlark.List:
		return _SpliceOver(container, step, value)

	default:
		return nil, fmt.Errorf("%s %w", holder.Type(), ErrKind)
	}
}

// _Put returns a copy of the dict with one member set.
//
// Revisions:
//   - 2026-09-20 01:05: initial creation
func _Put(holder *starlark.Dict, step string, value starlark.Value) (starlark.Value, error) {
	made := starlark.NewDict(holder.Len() + 1)

	for _, item := range holder.Items() {
		err := made.SetKey(item[0], item[1])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", step, err)
		}
	}

	err := made.SetKey(starlark.String(step), value)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", step, err)
	}

	return made, nil
}

// _Elements is a list's elements in a slice with room for one more.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
func _Elements(holder *starlark.List) []starlark.Value {
	elements := make([]starlark.Value, 0, holder.Len()+1)

	for index := range holder.Len() {
		elements = append(elements, holder.Index(index))
	}

	return elements
}

// _SpliceIn returns a copy of the list with value inserted at step, which may
// be "-": RFC 6901's spelling of the position after the last element.
//
// Revisions:
//   - 2026-09-20 01:06: initial creation, as _Splice with a flag
//   - 2026-09-21 08:09: the inserting half
func _SpliceIn(holder *starlark.List, step string, value starlark.Value) (starlark.Value, error) {
	elements := _Elements(holder)

	if step == APPEND {
		return starlark.NewList(append(elements, value)), nil
	}

	index, err := _Index(step, holder.Len()+1)
	if err != nil {
		return nil, err
	}

	elements = append(elements, nil)
	copy(elements[index+1:], elements[index:])
	elements[index] = value

	return starlark.NewList(elements), nil
}

// _SpliceOver returns a copy of the list with the element at step replaced.
//
// "-" names no element, so it is refused: there is nothing there to replace.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
func _SpliceOver(holder *starlark.List, step string, value starlark.Value) (starlark.Value, error) {
	if step == APPEND {
		return nil, fmt.Errorf("%q names no element: %w", APPEND, ErrMissing)
	}

	index, err := _Index(step, holder.Len())
	if err != nil {
		return nil, err
	}

	elements := _Elements(holder)
	elements[index] = value

	return starlark.NewList(elements), nil
}

// _Delete returns doc without whatever steps names.
//
// Revisions:
//   - 2026-09-20 01:07: initial creation
//   - 2026-09-21 08:09: replaces the shortened child through _PlaceOver
func _Delete(doc starlark.Value, steps []string) (starlark.Value, error) {
	step := steps[0]

	if len(steps) == 1 {
		return _Drop(doc, step)
	}

	child, err := _Step(doc, step)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", step, err)
	}

	shortened, err := _Delete(child, steps[1:])
	if err != nil {
		return nil, err
	}

	return _PlaceOver(doc, step, shortened)
}

// _Drop returns a copy of holder without step.
//
// Revisions:
//   - 2026-09-20 01:08: initial creation
func _Drop(holder starlark.Value, step string) (starlark.Value, error) {
	switch container := holder.(type) {
	case *starlark.Dict:
		made := starlark.NewDict(container.Len())

		for _, item := range container.Items() {
			if item[0] == starlark.String(step) {
				continue
			}

			err := made.SetKey(item[0], item[1])
			if err != nil {
				return nil, fmt.Errorf("%s: %w", step, err)
			}
		}

		return made, nil

	case *starlark.List:
		index, err := _Index(step, container.Len())
		if err != nil {
			return nil, err
		}

		elements := make([]starlark.Value, 0, container.Len()-1)

		for at := range container.Len() {
			if at == index {
				continue
			}

			elements = append(elements, container.Index(at))
		}

		return starlark.NewList(elements), nil

	default:
		return nil, fmt.Errorf("%s %w", holder.Type(), ErrKind)
	}
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
func _Equal(left starlark.Value, right starlark.Value) (bool, error) {
	return starlark.EqualDepth(left, right, starlark.CompareLimit)
}
