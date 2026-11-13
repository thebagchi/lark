package unpack

import (
	"math/big"
	"unsafe"

	"github.com/thebagchi/lark/v1/runtime/memwatch"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack/internal/spec"
)

// Padded is number as digits in base, left-padded with zeros to width digits,
// and not truncated when the number is wider. The base is 2 to 62. For a base up
// to 36 the letters are upper case, as every hex the runtime writes is; base 2
// has none. For a base from 37 to 62 the digits are 0-9, then a-z, then A-Z, as
// big.Int.Text writes them, so every digit stays distinct. The number must not
// be negative.
//
// One buffer, written once. The digits go to the front of a buffer that has
// room for the width, are made upper case where they lie, and then slide right
// to make room for the zeros. The old way made the digits, a second string to
// upper-case them, a third of zeros, and a fourth to join the zeros to the
// digits: each as long as the width.
//
// The text is the buffer itself, as a string over the same bytes, as
// hex.Encode returns it: nothing else holds the buffer and nothing writes it
// after the return, which is the one condition unsafe.String asks.
//
// The width is the size of the buffer, so Padded asks memwatch.Room before it
// allocates, and returns an error wrapping memwatch.ErrRoom for a width that
// does not fit the free memory. A script's width has passed the same check in
// Counted; this one guards a Go caller. Where the caller gives nothing to print
// - a nil number, or a base big.Int refuses - the text is empty with no error,
// and a width below zero is zero.
//
// Revisions:
//   - 2026-11-02 16:55: initial creation, from hex's and binary's _FromInt (C31)
//   - 2026-11-04 23:50: answers empty for a nil number or a base big.Int refuses, and takes a
//     negative width as zero (C86 fix round 3)
//   - 2026-11-05 16:50: returns an error, and refuses a width that does not fit the free
//     memory with memwatch.ErrNoRoom, where it ran out of memory (B122, C96)
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
//   - 2026-11-10 11:54: a base above 36 keeps the case of its digits, so no two
//     digits are the same; the doc names the base range (C117, B245)
//   - 2026-11-10 17:15: uses ErrRoom, formerly ErrNoRoom (C121, Q349)
func Padded(
	number *big.Int,
	base int,
	width int,
) (string, error) {
	if number == nil || base < 2 || base > big.MaxBase {
		return "", nil
	}

	width = max(
		width,
		0,
	)

	err := memwatch.Room(
		"Padded",
		uint64(width),
	)
	if err != nil {
		return "", err
	}

	text := number.Append(
		make(
			[]byte,
			0,
			width,
		),
		base,
	)

	// LETTERS is the largest base whose digits are 0-9 and one case of a-z.
	// Above it big.Int.Text uses both cases, and the case carries the digit.
	const LETTERS = 36

	if base <= LETTERS {
		for idx, char := range text {
			if 'a' <= char && char <= 'z' {
				text[idx] = char - ('a' - 'A')
			}
		}
	}

	used := len(text)

	if pad := width - used; pad > 0 {
		text = text[:width]

		copy(
			text[pad:],
			text[:used],
		)

		for idx := range pad {
			text[idx] = spec.ZERO
		}
	}

	return unsafe.String(
		unsafe.SliceData(text),
		len(text),
	), nil
}
