// Package unpack reads a builtin's arguments, for the plugins that take bytes,
// text, or an integer and a width, and nothing more complicated.
//
// It supplies no names to a script and is not a plugin. It exists because
// several plugins ask the same question - "is this argument bytes?" - and an
// answer in each would be one error message per plugin for one mistake.
//
// Revisions:
//   - 2026-09-24 17:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
package unpack

import (
	"errors"
	"fmt"
	"math/big"
	"strings"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/memwatch"
)

var (
	// ErrData is returned when something that should be bytes or a string is
	// neither.
	//
	// Revisions:
	//   - 2026-09-21 23:56: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
	ErrData = errors.New("wants bytes or a string")

	// ErrRange is returned for an integer a builtin cannot take: a negative
	// one, one too wide for the width it must fit, a width below one, a
	// random range whose end is below its start, and a count or an attempt
	// limit below one. The wrap names which.
	//
	// Revisions:
	//   - 2026-09-21 23:56: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
	ErrRange = errors.New("out of range")

	// ErrEncoded is returned for text that is not the encoding a decoder
	// reads: base32 text handed to base32.decode, base64 text handed to either
	// base64 decoder. The wrap names the plugin, once.
	//
	// Revisions:
	//   - 2026-11-08 13:25: initial creation, merged from base32 and base64 (C98, Q251)
	ErrEncoded = errors.New("not the encoding this reads")
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
//   - 2026-11-04 23:50: names a nil builtin ANONYMOUS, and refuses a missing argument or a nil
//     value (C86 fix round 3)
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
		_NameOf(fn),
		args[0],
	)
}

// Bytes is one value as bytes, for a builtin that takes more than one
// argument and unpacks them itself.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation, as codec's own _Bytes
//   - 2026-09-23 23:40: moved here
//   - 2026-11-04 23:50: refuses a nil value with ErrData (C86 fix round 3)
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
			_TypeOf(given),
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
//   - 2026-11-04 23:50: names a nil builtin ANONYMOUS, and refuses a missing argument or a nil
//     value (C86 fix round 3)
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
// The width is the size of the buffer the caller makes, so it is refused, with
// memwatch.ErrRoom, when it does not fit the free memory. The check follows
// the range checks and comes before the caller allocates.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as codec's own _Counted
//   - 2026-10-08 17:47: moved here, where hex and binary reach it too
//   - 2026-10-24 16:05: stop prepending fn.Name(), which
//     UnpackPositionalArgs already includes
//   - 2026-10-29 23:58: checks the call with Positional, so no local goes to
//     the heap
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-03 00:35: refuses a width that does not fit the free memory (C46)
//   - 2026-11-04 23:50: names a nil builtin ANONYMOUS, and refuses a missing argument or a nil
//     value (C86 fix round 3)
//   - 2026-11-10 17:15: uses ErrRoom, formerly ErrNoRoom (C121, Q349)
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
			_NameOf(fn),
			ShortText(value.String()),
			ErrRange,
		)
	}

	if width < 1 {
		return nil, 0, fmt.Errorf(
			"%s got a width of %d: %w",
			_NameOf(fn),
			width,
			ErrRange,
		)
	}

	err = memwatch.Room(
		_NameOf(fn),
		uint64(width),
	)
	if err != nil {
		return nil, 0, err
	}

	return value, width, nil
}

// Unprefixed is text with an optional prefix taken off once, in either case.
//
// Either case, because text from outside writes 0x and 0X both, and a reader
// that refused one would refuse what something else wrote.
//
// A prefix is outside RFC 4648 section 8, which defines the hex text as digits
// only. RFC 4648 section 3.3 lets a reader accept more "unless the specification
// referring to this document explicitly states otherwise"; the readers that
// call this state the prefix in their own documentation.
//
// Revisions:
//   - 2026-10-08 18:06: initial creation, as hex's own _Unprefixed
//   - 2026-10-08 18:32: moved here and given the prefix, so binary takes 0b off too
//   - 2026-11-02 16:45: names the clause that allows the prefix (C22)
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
