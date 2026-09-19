package jsonpath

import (
	"fmt"

	"go.starlark.net/starlark"
)

// _Set returns doc with the value at steps replaced, or inserted when insert is
// true and the last step names a list position.
//
// Nothing is mutated. Every container along the path is copied and the rest is
// shared, so a document that is frozen, or reachable from another thread, is
// safe to patch.
//
// Revisions:
//   - 2026-09-20 01:03: initial creation
func _Set(doc starlark.Value, steps []string, value starlark.Value, insert bool) (starlark.Value, error) {
	if len(steps) == 0 {
		return value, nil
	}

	step := steps[0]

	if len(steps) == 1 {
		return _Place(doc, step, value, insert)
	}

	child, err := _Step(doc, step)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", step, err)
	}

	replaced, err := _Set(child, steps[1:], value, insert)
	if err != nil {
		return nil, err
	}

	return _Place(doc, step, replaced, false)
}

// _Place returns a copy of holder with step set to value.
//
// Revisions:
//   - 2026-09-20 01:04: initial creation
func _Place(holder starlark.Value, step string, value starlark.Value, insert bool) (starlark.Value, error) {
	switch container := holder.(type) {
	case *starlark.Dict:
		return _Put(container, step, value)

	case *starlark.List:
		return _Splice(container, step, value, insert)

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

// _Splice returns a copy of the list with one element replaced, or with value
// inserted at that position when insert is true.
//
// The step "-" names the position after the last element, which is how RFC 6901
// spells "append" - and it is only meaningful when inserting, because there is
// nothing there to replace.
//
// Revisions:
//   - 2026-09-20 01:06: initial creation
func _Splice(holder *starlark.List, step string, value starlark.Value, insert bool) (starlark.Value, error) {
	elements := make([]starlark.Value, 0, holder.Len()+1)

	for index := range holder.Len() {
		elements = append(elements, holder.Index(index))
	}

	if step == APPEND {
		if !insert {
			return nil, fmt.Errorf("%q names no element: %w", APPEND, ErrMissing)
		}

		return starlark.NewList(append(elements, value)), nil
	}

	length := holder.Len()
	if insert {
		length++
	}

	index, err := _Index(step, length)
	if err != nil {
		return nil, err
	}

	if !insert {
		elements[index] = value

		return starlark.NewList(elements), nil
	}

	elements = append(elements, nil)
	copy(elements[index+1:], elements[index:])
	elements[index] = value

	return starlark.NewList(elements), nil
}

// _Delete returns doc without whatever steps names.
//
// Revisions:
//   - 2026-09-20 01:07: initial creation
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

	return _Place(doc, step, shortened, false)
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

// _Equal reports whether two values are deeply equal, comparing an int and a
// float of the same magnitude as equal.
//
// RFC 6902's test operation compares JSON values, where 1 and 1.0 are one
// number. Starlark keeps them apart, so a document decoded from JSON and a
// literal written in a script would otherwise fail to match for a reason that
// has nothing to do with the script.
//
// Revisions:
//   - 2026-09-20 01:09: initial creation
func _Equal(left starlark.Value, right starlark.Value) (bool, error) {
	_, leftNumber := _Float(left)
	_, rightNumber := _Float(right)

	if leftNumber && rightNumber {
		first, _ := _Float(left)
		second, _ := _Float(right)

		return first == second, nil
	}

	return starlark.EqualDepth(left, right, starlark.CompareLimit)
}

// _Float returns a value as a float when it is a number.
//
// Revisions:
//   - 2026-09-20 01:10: initial creation
func _Float(value starlark.Value) (float64, bool) {
	switch number := value.(type) {
	case starlark.Int:
		got, _ := starlark.AsFloat(number)

		return got, true

	case starlark.Float:
		return float64(number), true

	default:
		return 0, false
	}
}
