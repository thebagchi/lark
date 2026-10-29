// Package unpack reads a builtin's arguments, for the plugins that take bytes,
// text, or an integer and a width, and nothing more complicated.
//
// It supplies no names to a script and is not a plugin. It exists because
// several plugins ask the same question - "is this argument bytes?" - and an
// answer in each would be one error message per plugin for one mistake.
package unpack

import (
	"errors"
	"fmt"
	"math/big"
	"strings"

	"go.starlark.net/starlark"
)

var (
	// ErrData is returned when something that should be bytes or a string is
	// neither.
	ErrData = errors.New("wants bytes or a string")

	// ErrRange is returned for an integer a builtin cannot take: a negative
	// one, one too wide for the width it must fit, or a width below one.
	ErrRange = errors.New("out of range")
)

// Data reads a builtin's one argument as bytes.
//
// A str is accepted where bytes are wanted, because a script encoding a word
// writes it as a word and Starlark has no literal for bytes that reads like
// one.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as codec's own _Data
//   - 2026-09-23 23:40: moved here, where base64 and hash reach it too
//   - 2026-10-24 16:05: stop prepending fn.Name(), which
//     UnpackPositionalArgs already includes
//   - 2026-10-29 23:58: checks the call with Positional, so no local goes to
//     the heap
func Data(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) ([]byte, error) {
	err := Positional(
		fn,
		args,
		kwargs,
		1,
	)
	if err != nil {
		return nil, err
	}

	return Bytes(
		fn.Name(),
		args[0],
	)
}

// Bytes is one value as bytes, for a builtin that takes more than one
// argument and unpacks them itself.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation, as codec's own _Bytes
//   - 2026-09-23 23:40: moved here
func Bytes(
	who string,
	given starlark.Value,
) ([]byte, error) {
	switch held := given.(type) {
	case starlark.Bytes:
		return []byte(held), nil

	case starlark.String:
		return []byte(held), nil

	default:
		return nil, fmt.Errorf(
			"%s got %s: %w",
			who,
			given.Type(),
			ErrData,
		)
	}
}

// Text reads a builtin's one argument as a string.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as codec's own _Text
//   - 2026-09-23 23:40: moved here
//   - 2026-10-24 16:05: stop prepending fn.Name(), which
//     UnpackPositionalArgs already includes
//   - 2026-10-29 23:58: checks the call with Positional, so no local goes to
//     the heap
func Text(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (string, error) {
	err := Positional(
		fn,
		args,
		kwargs,
		1,
	)
	if err != nil {
		return "", err
	}

	return StringAt(
		fn,
		args,
		0,
	)
}

// Counted reads a builtin's two arguments as a non-negative integer and a
// width of at least one.
//
// The integer is a big.Int, because nothing bounds it and a Starlark int is
// not bounded either.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as codec's own _Counted
//   - 2026-10-08 17:47: moved here, where hex and binary reach it too
//   - 2026-10-24 16:05: stop prepending fn.Name(), which
//     UnpackPositionalArgs already includes
//   - 2026-10-29 23:58: checks the call with Positional, so no local goes to
//     the heap
func Counted(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (*big.Int, int, error) {
	const NEEDED = 2

	err := Positional(
		fn,
		args,
		kwargs,
		NEEDED,
	)
	if err != nil {
		return nil, 0, err
	}

	number, err := _IntAt(
		fn,
		args,
		0,
	)
	if err != nil {
		return nil, 0, err
	}

	width, err := _SizeAt(
		fn,
		args,
		1,
	)
	if err != nil {
		return nil, 0, err
	}

	value := number.BigInt()

	if value.Sign() < 0 {
		return nil, 0, fmt.Errorf(
			"%s got %s, which is negative: %w",
			fn.Name(),
			value,
			ErrRange,
		)
	}

	if width < 1 {
		return nil, 0, fmt.Errorf(
			"%s got a width of %d: %w",
			fn.Name(),
			width,
			ErrRange,
		)
	}

	return value, width, nil
}

// Unprefixed is text with an optional prefix taken off once, in either case.
//
// Either case, because text from outside writes 0x and 0X both, and a reader
// that refused one would refuse what something else wrote.
//
// Revisions:
//   - 2026-10-08 18:06: initial creation, as hex's own _Unprefixed
//   - 2026-10-08 18:32: moved here and given the prefix, so binary takes 0b off too
func Unprefixed(
	text string,
	prefix string,
) string {
	prefixed := len(text) >= len(prefix) && strings.EqualFold(
		text[:len(prefix)],
		prefix,
	)
	if prefixed {
		return text[len(prefix):]
	}

	return text
}
