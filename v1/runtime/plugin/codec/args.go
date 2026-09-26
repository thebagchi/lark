package codec

import (
	"fmt"
	"math/big"
	"strings"

	"go.starlark.net/starlark"
)

// _Counted reads a builtin's two arguments as a non-negative integer and a
// width of at least one.
//
// The integer is a big.Int, because nothing in the brief bounds it and a
// Starlark int is not bounded either.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
func _Counted(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (*big.Int, int, error) {
	var (
		number starlark.Int
		width  int
	)

	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 2, &number, &width)
	if err != nil {
		return nil, 0, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	value := number.BigInt()

	if value.Sign() < 0 {
		return nil, 0, fmt.Errorf("%s got %s, which is negative: %w",
			fn.Name(), value, ErrRange)
	}

	if width < 1 {
		return nil, 0, fmt.Errorf("%s got a width of %d: %w", fn.Name(), width, ErrRange)
	}

	return value, width, nil
}

// _Bits checks that text is made only of 0 and 1.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
func _Bits(who string, text string) error {
	for _, char := range text {
		if char != '0' && char != '1' {
			return fmt.Errorf("%s got %q: %w", who, text, ErrBits)
		}
	}

	return nil
}

// _Padded returns text left-padded with zeros to the next multiple of width.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
func _Padded(text string, width int) string {
	short := len(text) % width
	if short == 0 {
		return text
	}

	return strings.Repeat("0", width-short) + text
}
