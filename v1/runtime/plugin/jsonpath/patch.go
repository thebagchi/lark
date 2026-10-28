package jsonpath

import (
	"errors"
	"fmt"
	"slices"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/deep"
)

var (
	// ErrOperation is returned for an op this does not implement.
	ErrOperation = errors.New("unknown patch operation")

	// ErrField is returned when an op is missing something it requires.
	ErrField = errors.New("patch operation is missing a field")

	// ErrTest is returned when a test op does not match.
	ErrTest = errors.New("test failed")

	// ErrInto is returned for a move whose destination is inside its source.
	ErrInto = errors.New("cannot move a value into itself")
)

// OP is the field a patch names its operation with, and the names that follow are the operations.
const (
	OP      = "op"
	PATH    = "path"
	VALUE   = "value"
	FROM    = "from"
	ADD     = "add"
	REMOVE  = "remove"
	REPLACE = "replace"
	MOVE    = "move"
	COPY    = "copy"
	TEST    = "test"
)

// _Patch applies one RFC 6902 operation to doc and returns the result.
//
// The document handed to the patch is never mutated: an operation copies a
// container of it along the path it touches, the first time any operation does,
// and leaves everything else shared. That is what Python's jsonpatch does with
// in_place=False, and it is the only safe shape here, since a document may
// already be frozen or reachable from another thread.
//
// Revisions:
//   - 2026-09-20 00:53: initial creation
//   - 2026-09-21 08:09: one keyword per case
//   - 2026-10-03 16:53: a method of _Edit, so a later operation changes in place what
//     an earlier one copied
func (e *_Edit) _Patch(
	doc starlark.Value,
	op *starlark.Dict,
) (starlark.Value, error) {
	kind, err := _Field(
		op,
		OP,
	)
	if err != nil {
		return nil, err
	}

	switch kind {
	case ADD:
		fallthrough
	case REPLACE:
		return e._Write(
			doc,
			op,
			kind,
		)

	case REMOVE:
		return e._Removed(
			doc,
			op,
		)

	case MOVE:
		fallthrough
	case COPY:
		return e._Relocate(
			doc,
			op,
			kind,
		)

	case TEST:
		return _Test(
			doc,
			op,
		)

	default:
		return nil, fmt.Errorf(
			"%q: %w",
			kind,
			ErrOperation,
		)
	}
}

// _Removed deletes the path an operation names.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
func (e *_Edit) _Removed(
	doc starlark.Value,
	op *starlark.Dict,
) (starlark.Value, error) {
	path, err := _Field(
		op,
		PATH,
	)
	if err != nil {
		return nil, err
	}

	return e._Remove(
		doc,
		path,
	)
}

// _Field reads a required string member of an operation.
//
// Revisions:
//   - 2026-09-20 00:54: initial creation
//   - 2026-09-24 16:08: a copy is given its own containers, where it used to
//     share them with the document it came from
func _Field(
	op *starlark.Dict,
	name string,
) (string, error) {
	value, found, err := op.Get(starlark.String(name))
	if err != nil || !found {
		return "", fmt.Errorf(
			"%q: %w",
			name,
			ErrField,
		)
	}

	text, ok := value.(starlark.String)
	if !ok {
		return "", fmt.Errorf(
			"%q is %s: %w",
			name,
			value.Type(),
			ErrField,
		)
	}

	return string(text), nil
}

// _Value reads an operation's value member, which add, replace and test
// require.
//
// Revisions:
//   - 2026-09-20 00:55: initial creation
func _Value(op *starlark.Dict) (starlark.Value, error) {
	value, found, err := op.Get(starlark.String(VALUE))
	if err != nil || !found {
		return nil, fmt.Errorf(
			"%q: %w",
			VALUE,
			ErrField,
		)
	}

	return value, nil
}

// _Write applies add or replace.
//
// They differ in one rule: replace requires the path to exist already, and add
// does not. Add to a list index inserts; add to "-" appends.
//
// Revisions:
//   - 2026-09-20 00:56: initial creation
//   - 2026-09-21 08:09: inserts or replaces through two functions rather than
//     one with a flag
//   - 2026-10-03 16:53: a method of _Edit, which copies a container once per patch
func (e *_Edit) _Write(
	doc starlark.Value,
	op *starlark.Dict,
	kind string,
) (starlark.Value, error) {
	path, err := _Field(
		op,
		PATH,
	)
	if err != nil {
		return nil, err
	}

	value, err := _Value(op)
	if err != nil {
		return nil, err
	}

	steps, err := _Steps(path)
	if err != nil {
		return nil, err
	}

	return e._Written(
		doc,
		steps,
		value,
		kind,
	)
}

// _Written copies value and inserts or replaces it at steps.
//
// The copy is the leaf. _Insert copies every container along the path, and
// this stops a caller who still holds value from changing the result.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
func (e *_Edit) _Written(
	doc starlark.Value,
	steps []string,
	value starlark.Value,
	kind string,
) (starlark.Value, error) {
	own, err := deep.Copy(value)
	if err != nil {
		return nil, err
	}

	if kind == ADD {
		return e._Insert(
			doc,
			steps,
			own,
		)
	}

	_, err = _Walk(
		doc,
		steps,
	)
	if err != nil {
		return nil, err
	}

	return e._Replace(
		doc,
		steps,
		own,
	)
}

// _Remove deletes what path names.
//
// Revisions:
//   - 2026-09-20 00:57: initial creation
//   - 2026-10-03 16:53: a method of _Edit, which copies a container once per patch
func (e *_Edit) _Remove(
	doc starlark.Value,
	path string,
) (starlark.Value, error) {
	steps, err := _Steps(path)
	if err != nil {
		return nil, err
	}

	if len(steps) == 0 {
		return nil, fmt.Errorf(
			"the whole document: %w",
			ErrPointer,
		)
	}

	_, err = _Walk(
		doc,
		steps,
	)
	if err != nil {
		return nil, err
	}

	return e._Delete(
		doc,
		steps,
	)
}

// _Relocate applies move or copy.
//
// A move out of a path into its own child is refused: the source would have to
// exist inside the value being moved, and RFC 6902 names this as an error
// rather than leaving the result to an implementation. A move onto itself is
// allowed, as the specification allows it, and changes nothing.
//
// Revisions:
//   - 2026-09-20 00:58: initial creation
//   - 2026-09-21 08:09: inserts through _Insert; a move onto itself is a no-op
//     rather than a refusal
//   - 2026-10-03 16:53: a method of _Edit, which copies a container once per patch
//   - 2026-10-03 19:28: a move onto itself returns the document as it was, the whole
//     document included, rather than deleting the value and putting it back
func (e *_Edit) _Relocate(
	doc starlark.Value,
	op *starlark.Dict,
	kind string,
) (starlark.Value, error) {
	from, path, source, target, err := _Route(op)
	if err != nil {
		return nil, err
	}

	if kind == MOVE && _Inside(
		source,
		target,
	) {
		return nil, fmt.Errorf(
			"%q into %q: %w",
			from,
			path,
			ErrInto,
		)
	}

	value, err := _Walk(
		doc,
		source,
	)
	if err != nil {
		return nil, err
	}

	return e._Placed(
		&_Place{
			doc:    doc,
			kind:   kind,
			source: source,
			target: target,
			value:  value,
		},
	)
}

// _Route reads from and path, and the steps each one walks.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
func _Route(
	op *starlark.Dict,
) (string, string, []string, []string, error) {
	from, err := _Field(
		op,
		FROM,
	)
	if err != nil {
		return "", "", nil, nil, err
	}

	path, err := _Field(
		op,
		PATH,
	)
	if err != nil {
		return "", "", nil, nil, err
	}

	source, err := _Steps(from)
	if err != nil {
		return "", "", nil, nil, err
	}

	target, err := _Steps(path)
	if err != nil {
		return "", "", nil, nil, err
	}

	return from, path, source, target, nil
}

// _Place is a value on its way from one path to another.
type _Place struct {
	doc    starlark.Value
	kind   string
	source []string
	target []string
	value  starlark.Value
}

// _Placed moves or copies the value onto its target.
//
// A move onto itself returns the document. Deleting the value and putting
// it back would move a member to the end of its dict. A copy gets its own
// value, so a later edit at the target cannot change the source.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
func (e *_Edit) _Placed(place *_Place) (starlark.Value, error) {
	stays := place.kind == MOVE && slices.Equal(
		place.source,
		place.target,
	)
	if stays {
		return place.doc, nil
	}

	if place.kind == MOVE {
		doc, err := e._Delete(
			place.doc,
			place.source,
		)
		if err != nil {
			return nil, err
		}

		return e._Insert(
			doc,
			place.target,
			place.value,
		)
	}

	own, err := deep.Copy(place.value)
	if err != nil {
		return nil, err
	}

	return e._Insert(
		place.doc,
		place.target,
		own,
	)
}

// _Inside reports whether target is strictly below source.
//
// Strictly: RFC 6902 forbids a "from" that is a proper prefix of "path", and
// says nothing against the two being equal.
//
// Revisions:
//   - 2026-09-20 00:59: initial creation
//   - 2026-09-21 08:09: a path equal to its source is not inside it
func _Inside(
	source []string,
	target []string,
) bool {
	if len(target) <= len(source) {
		return false
	}

	for index, step := range source {
		if target[index] != step {
			return false
		}
	}

	return true
}

// _Test compares what path names against a value, and returns the document
// unchanged when they match.
//
// Revisions:
//   - 2026-09-20 01:00: initial creation
func _Test(
	doc starlark.Value,
	op *starlark.Dict,
) (starlark.Value, error) {
	path, err := _Field(
		op,
		PATH,
	)
	if err != nil {
		return nil, err
	}

	want, err := _Value(op)
	if err != nil {
		return nil, err
	}

	steps, err := _Steps(path)
	if err != nil {
		return nil, err
	}

	got, err := _Walk(
		doc,
		steps,
	)
	if err != nil {
		return nil, err
	}

	same, err := _Equal(
		got,
		want,
	)
	if err != nil {
		return nil, err
	}

	if !same {
		return nil, fmt.Errorf(
			"%s is %s, not %s: %w",
			path,
			got.String(),
			want.String(),
			ErrTest,
		)
	}

	return doc, nil
}
