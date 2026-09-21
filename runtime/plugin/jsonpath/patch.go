package jsonpath

import (
	"errors"
	"fmt"

	"go.starlark.net/starlark"
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
// The document is never mutated: each operation copies the containers along the
// path it touches and leaves everything else shared. That is what Python's
// jsonpatch does with in_place=False, and it is the only safe shape here, since
// a document may already be frozen or reachable from another thread.
//
// Revisions:
//   - 2026-09-20 00:53: initial creation
//   - 2026-09-21 08:09: one keyword per case
func _Patch(doc starlark.Value, op *starlark.Dict) (starlark.Value, error) {
	kind, err := _Field(op, OP)
	if err != nil {
		return nil, err
	}

	switch kind {
	case ADD:
		fallthrough
	case REPLACE:
		return _Write(doc, op, kind)

	case REMOVE:
		path, err := _Field(op, PATH)
		if err != nil {
			return nil, err
		}

		return _Remove(doc, path)

	case MOVE:
		fallthrough
	case COPY:
		return _Relocate(doc, op, kind)

	case TEST:
		return _Test(doc, op)

	default:
		return nil, fmt.Errorf("%q: %w", kind, ErrOperation)
	}
}

// _Field reads a required string member of an operation.
//
// Revisions:
//   - 2026-09-20 00:54: initial creation
func _Field(op *starlark.Dict, name string) (string, error) {
	value, found, err := op.Get(starlark.String(name))
	if err != nil || !found {
		return "", fmt.Errorf("%q: %w", name, ErrField)
	}

	text, ok := value.(starlark.String)
	if !ok {
		return "", fmt.Errorf("%q is %s: %w", name, value.Type(), ErrField)
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
		return nil, fmt.Errorf("%q: %w", VALUE, ErrField)
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
func _Write(doc starlark.Value, op *starlark.Dict, kind string) (starlark.Value, error) {
	path, err := _Field(op, PATH)
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

	if kind == ADD {
		return _Insert(doc, steps, value)
	}

	_, err = _Walk(doc, steps)
	if err != nil {
		return nil, err
	}

	return _Replace(doc, steps, value)
}

// _Remove deletes what path names.
//
// Revisions:
//   - 2026-09-20 00:57: initial creation
func _Remove(doc starlark.Value, path string) (starlark.Value, error) {
	steps, err := _Steps(path)
	if err != nil {
		return nil, err
	}

	if len(steps) == 0 {
		return nil, fmt.Errorf("the whole document: %w", ErrPointer)
	}

	_, err = _Walk(doc, steps)
	if err != nil {
		return nil, err
	}

	return _Delete(doc, steps)
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
func _Relocate(doc starlark.Value, op *starlark.Dict, kind string) (starlark.Value, error) {
	from, err := _Field(op, FROM)
	if err != nil {
		return nil, err
	}

	path, err := _Field(op, PATH)
	if err != nil {
		return nil, err
	}

	source, err := _Steps(from)
	if err != nil {
		return nil, err
	}

	target, err := _Steps(path)
	if err != nil {
		return nil, err
	}

	if kind == MOVE && _Inside(source, target) {
		return nil, fmt.Errorf("%q into %q: %w", from, path, ErrInto)
	}

	value, err := _Walk(doc, source)
	if err != nil {
		return nil, err
	}

	if kind == MOVE {
		doc, err = _Delete(doc, source)
		if err != nil {
			return nil, err
		}
	}

	return _Insert(doc, target, value)
}

// _Inside reports whether target is strictly below source.
//
// Strictly: RFC 6902 forbids a "from" that is a proper prefix of "path", and
// says nothing against the two being equal.
//
// Revisions:
//   - 2026-09-20 00:59: initial creation
//   - 2026-09-21 08:09: a path equal to its source is not inside it
func _Inside(source []string, target []string) bool {
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
func _Test(doc starlark.Value, op *starlark.Dict) (starlark.Value, error) {
	path, err := _Field(op, PATH)
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

	got, err := _Walk(doc, steps)
	if err != nil {
		return nil, err
	}

	same, err := _Equal(got, want)
	if err != nil {
		return nil, err
	}

	if !same {
		return nil, fmt.Errorf("%s is %s, not %s: %w", path, got.String(), want.String(), ErrTest)
	}

	return doc, nil
}
