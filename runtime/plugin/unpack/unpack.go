// Package unpack reads a builtin's arguments, for the plugins that take bytes
// or text and nothing more complicated.
//
// It supplies no names to a script and is not a plugin. It exists because
// three plugins ask the same question - "is this argument bytes?" - and three
// answers to it would be three error messages for one mistake.
package unpack

import (
	"errors"
	"fmt"

	"go.starlark.net/starlark"
)

// ErrData is returned when something that should be bytes or a string is
// neither.
var ErrData = errors.New("wants bytes or a string")

// Data reads a builtin's one argument as bytes.
//
// A str is accepted where bytes are wanted, because a script encoding a word
// writes it as a word and Starlark has no literal for bytes that reads like
// one.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as codec's own _Data
//   - 2026-09-23 23:40: moved here, where base64 and hash reach it too
func Data(fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) ([]byte, error) {
	var given starlark.Value

	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 1, &given)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	return Bytes(fn.Name(), given)
}

// Bytes is one value as bytes, for a builtin that takes more than one
// argument and unpacks them itself.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation, as codec's own _Bytes
//   - 2026-09-23 23:40: moved here
func Bytes(who string, given starlark.Value) ([]byte, error) {
	switch held := given.(type) {
	case starlark.Bytes:
		return []byte(held), nil

	case starlark.String:
		return []byte(held), nil

	default:
		return nil, fmt.Errorf("%s got %s: %w", who, given.Type(), ErrData)
	}
}

// Text reads a builtin's one argument as a string.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as codec's own _Text
//   - 2026-09-23 23:40: moved here
func Text(fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (string, error) {
	var text string

	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 1, &text)
	if err != nil {
		return "", fmt.Errorf("%s: %w", fn.Name(), err)
	}

	return text, nil
}
