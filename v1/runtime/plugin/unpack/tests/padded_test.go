package tests

import (
	"errors"
	"math"
	"math/big"
	"strconv"
	"strings"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/memwatch"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// TestPadded_AgreesWithText reads Padded against the old way: the digits from
// Text, upper-cased, with a string of zeros in front. The rows run over both
// bases the runtime uses, widths from below the length of the number to well
// above it, and numbers of all letters, all digits and zero.
//
// Revisions:
//   - 2026-11-02 16:55: initial creation (C31)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestPadded_AgreesWithText(t *testing.T) {
	numbers := []*big.Int{
		big.NewInt(0),
		big.NewInt(1),
		big.NewInt(9),
		big.NewInt(10),
		big.NewInt(15),
		big.NewInt(16),
		big.NewInt(0xABCDEF),
		big.NewInt(1234567890),
		new(big.Int).Sub(
			new(big.Int).Lsh(
				big.NewInt(1),
				130,
			),
			big.NewInt(1),
		),
	}

	for _, number := range numbers {
		for _, base := range []int{2, 16} {
			for _, width := range []int{1, 2, 3, 7, 8, 9, 31, 32, 33, 130, 131, 500} {
				t.Run(
					strings.Join(
						[]string{
							number.String(),
							strconv.Itoa(base),
							strconv.Itoa(width),
						},
						"/",
					),
					func(t *testing.T) {
						want := strings.ToUpper(number.Text(base))
						if len(want) < width {
							want = strings.Repeat(
								"0",
								width-len(want),
							) + want
						}

						if got, err := unpack.Padded(
							number,
							base,
							width,
						); err != nil || got != want {
							t.Fatalf(
								"Padded = %q, %v, want %q",
								got,
								err,
								want,
							)
						}
					},
				)
			}
		}
	}
}

// TestPadded_LeavesTheNumberAlone makes sure that padding does not change the
// number it was given, which a caller still holds as a script's value.
//
// Revisions:
//   - 2026-11-02 16:55: initial creation (C31)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestPadded_LeavesTheNumberAlone(t *testing.T) {
	number := big.NewInt(0xDEADBEEF)

	_, _ = unpack.Padded(
		number,
		16,
		64,
	)

	if number.Cmp(big.NewInt(0xDEADBEEF)) != 0 {
		t.Fatalf(
			"number is %v after Padded",
			number,
		)
	}
}

// TestPadded_RefusesAWidthThatDoesNotFit proves B122: a width of math.MaxInt
// is refused with memwatch.ErrRoom, and no buffer is made.
//
// Revisions:
//   - 2026-11-05 16:50: initial creation (B122, C96)
//   - 2026-11-05 19:07: the width is math.MaxInt, for 32 bits (C58, B119)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrRoom, formerly ErrNoRoom (C121, Q349)
func TestPadded_RefusesAWidthThatDoesNotFit(t *testing.T) {
	got, err := unpack.Padded(
		big.NewInt(255),
		16,
		math.MaxInt,
	)
	if !errors.Is(
		err,
		memwatch.ErrRoom,
	) {
		t.Fatalf(
			"err = %v, want memwatch.ErrRoom",
			err,
		)
	}

	if got != "" {
		t.Fatalf(
			"text has %d bytes, want none",
			len(got),
		)
	}
}

// TestPadded_EveryDigitIsDistinct proves that in each base from 2 to 62 every
// digit value prints as a different text, so Padded is a number as digits in
// base for the whole range, and that a base up to 36 is upper case (B245).
//
// Success: 62 digits of base 62 are 62 different one-character texts, a-z
// before A-Z as big.Int.Text writes them; a base up to 36 has no lower case.
//
// Revisions:
//   - 2026-11-10 11:54: initial creation (C117, B245)
func TestPadded_EveryDigitIsDistinct(t *testing.T) {
	for base := 2; base <= big.MaxBase; base++ {
		seen := make(
			map[string]int,
			base,
		)

		for digit := range base {
			got, err := unpack.Padded(
				big.NewInt(int64(digit)),
				base,
				0,
			)
			if err != nil {
				t.Fatal(err)
			}

			if got != big.NewInt(int64(digit)).Text(base) &&
				base > 36 {
				t.Fatalf(
					"base %d digit %d = %q, want %q",
					base,
					digit,
					got,
					big.NewInt(int64(digit)).Text(base),
				)
			}

			if other, ok := seen[got]; ok {
				t.Fatalf(
					"base %d: digits %d and %d both print %q",
					base,
					other,
					digit,
					got,
				)
			}

			seen[got] = digit

			if base <= 36 && got != strings.ToUpper(got) {
				t.Fatalf(
					"base %d digit %d = %q, want upper case",
					base,
					digit,
					got,
				)
			}
		}
	}
}
