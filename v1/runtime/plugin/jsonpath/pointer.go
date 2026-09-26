// Package jsonpath gives a script RFC 6901 pointers and RFC 6902 patch over
// Starlark values. Importing it is what enables it.
//
// It works on Starlark dicts and lists directly rather than on decoded JSON, so
// nothing round-trips through text on the way.
package jsonpath

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"go.starlark.net/starlark"
)

var (
	// ErrPointer is returned for a pointer that is not RFC 6901.
	ErrPointer = errors.New("not a json pointer")

	// ErrMissing is returned when a pointer names something that is not there.
	ErrMissing = errors.New("no such path")

	// ErrKind is returned when a step asks for a member of something that has
	// no members, or an index of something that is not a list.
	ErrKind = errors.New("cannot be indexed")
)

const (
	SEPARATOR = "/"
	APPEND    = "-"
	ESCAPED_1 = "~1"
	LITERAL_1 = "/"
	ESCAPED_0 = "~0"
	LITERAL_0 = "~"
)

// _Steps splits an RFC 6901 pointer into its unescaped reference tokens.
//
// The empty pointer is the whole document and yields no steps. Anything else
// must start with a slash: RFC 6901 has no relative pointers, and accepting one
// would silently treat "a/b" as the two steps a caller probably meant rather
// than the error they made.
//
// Unescaping is ~1 before ~0, and the order is the specification's. Doing it
// the other way turns "~01" into "/" instead of "~1".
//
// Revisions:
//   - 2026-09-20 00:46: initial creation
func _Steps(pointer string) ([]string, error) {
	if pointer == "" {
		return nil, nil
	}

	if !strings.HasPrefix(pointer, SEPARATOR) {
		return nil, fmt.Errorf("%q does not start with %q: %w",
			pointer, SEPARATOR, ErrPointer)
	}

	raw := strings.Split(strings.TrimPrefix(pointer, SEPARATOR), SEPARATOR)
	steps := make([]string, 0, len(raw))

	for _, token := range raw {
		token = strings.ReplaceAll(token, ESCAPED_1, LITERAL_1)
		token = strings.ReplaceAll(token, ESCAPED_0, LITERAL_0)

		steps = append(steps, token)
	}

	return steps, nil
}

// _Walk returns the value at the given steps, starting from doc.
//
// Returns ErrMissing when a step names nothing, and ErrKind when a step asks a
// value for something it cannot hold - a member of a list, or an index of a
// dict.
//
// Revisions:
//   - 2026-09-20 00:48: initial creation
func _Walk(doc starlark.Value, steps []string) (starlark.Value, error) {
	at := doc

	for index, step := range steps {
		next, err := _Step(at, step)
		if err != nil {
			return nil, fmt.Errorf("%s: %w",
				SEPARATOR+strings.Join(steps[:index+1], SEPARATOR), err)
		}

		at = next
	}

	return at, nil
}

// _Step returns the member or element of at named by step.
//
// Revisions:
//   - 2026-09-20 00:49: initial creation
func _Step(at starlark.Value, step string) (starlark.Value, error) {
	switch holder := at.(type) {
	case *starlark.Dict:
		value, found, err := holder.Get(starlark.String(step))
		if err != nil {
			return nil, fmt.Errorf("%q: %w", step, err)
		}

		if !found {
			return nil, ErrMissing
		}

		return value, nil

	case *starlark.List:
		index, err := _Index(step, holder.Len())
		if err != nil {
			return nil, err
		}

		return holder.Index(index), nil

	default:
		return nil, fmt.Errorf("%s %w", at.Type(), ErrKind)
	}
}

// _Index reads step as a list index, refusing anything RFC 6901 does not allow.
//
// Leading zeros are refused because the specification spells an index as a
// digit sequence with no leading zero, so "01" is not 1 - it is not an index at
// all, and treating it as one would let two spellings name one element. A sign
// is refused for the same reason: strconv accepts "+1", and the specification
// does not.
//
// Revisions:
//   - 2026-09-20 00:50: initial creation
//   - 2026-09-21 08:09: digits only, so a signed step is not an index
func _Index(step string, length int) (int, error) {
	if !_Digits(step) || (len(step) > 1 && strings.HasPrefix(step, "0")) {
		return 0, fmt.Errorf("%q is not an index: %w", step, ErrPointer)
	}

	index, err := strconv.Atoi(step)
	if err != nil {
		return 0, fmt.Errorf("%q is not an index: %w", step, ErrPointer)
	}

	if index >= length {
		return 0, fmt.Errorf("%d is outside a list of %d: %w", index, length, ErrMissing)
	}

	return index, nil
}

// _Digits reports whether step is one or more decimal digits.
//
// Revisions:
//   - 2026-09-21 08:09: initial creation
func _Digits(step string) bool {
	if step == "" {
		return false
	}

	for _, char := range step {
		if char < '0' || char > '9' {
			return false
		}
	}

	return true
}
