// These tests are table driven because the thing under test is a
// specification: bytes as an integer and back, in either byte order, one rule a
// row, so a missing rule is visible as a missing row.
//
// Revisions:
//   - 2026-10-08 22:45: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
package tests

import (
	"bytes"
	"encoding/binary"
	"errors"
	"math"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/integer"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

const (
	SCRIPT = "integer_test.star"
)

// _Eval runs one Starlark expression against the plugin environment and
// returns what it produced, as its String form.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, in codec
//   - 2026-10-08 18:42: moved here with the conversions it exercises
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Eval(
	t *testing.T,
	expression string,
) (string, error) {
	t.Helper()

	env, err := plugin.Environment(plugin.Registered())
	if err != nil {
		t.Fatalf(
			"environment: %v",
			err,
		)
	}

	value, err := starlark.EvalOptions(
		common.OPTIONS,
		&starlark.Thread{
			Name: SCRIPT,
		},
		SCRIPT,
		expression,
		env,
	)
	if err != nil {
		return "", err
	}

	return value.String(), nil
}

// _Check runs a table of expression / expected pairs.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, in codec
//   - 2026-10-08 18:42: moved here
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Check(
	t *testing.T,
	cases []struct{ name, expression, want string },
) {
	t.Helper()

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				got, err := _Eval(
					t,
					item.expression,
				)
				if err != nil {
					t.Fatalf(
						"%s: %v",
						item.expression,
						err,
					)
				}

				if got != item.want {
					t.Fatalf(
						"%s gave %s, want %s",
						item.expression,
						got,
						item.want,
					)
				}
			},
		)
	}
}

// _Refuse runs a table of expressions that must fail with the sentinel given.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, in codec
//   - 2026-10-08 18:42: moved here
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Refuse(
	t *testing.T,
	cases []struct {
		name       string
		expression string
		want       error
	},
) {
	t.Helper()

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				_, err := _Eval(
					t,
					item.expression,
				)
				if !errors.Is(
					err,
					item.want,
				) {
					t.Fatalf(
						"%s gave %v, want %v",
						item.expression,
						err,
						item.want,
					)
				}
			},
		)
	}
}

// TestInteger_ConvertsToAndFromBytes is unsigned unless signed is True,
// big-endian unless an order is named, zero-padded, and refused when a number
// does not fit or a width is below one.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as codec's TestInts_ConvertToAndFromBytes
//   - 2026-10-08 17:52: refused with unpack's ErrRange
//   - 2026-10-08 18:42: spells the calls as members of the integer module
//   - 2026-10-08 21:08: a byte order, and a width past the run's memory
//   - 2026-10-08 21:37: signed, and every argument named
//   - 2026-11-01 13:00: drops the wider-than-the-run row, the budget being gone
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestInteger_ConvertsToAndFromBytes(t *testing.T) {
	_Check(
		t,
		[]struct{ name, expression, want string }{
			{
				"from_bytes",
				`integer.from_bytes(b"\x01\x00")`,
				`256`,
			},
			{
				"from_bytes of nothing",
				`integer.from_bytes(b"")`,
				`0`,
			},
			{
				"from_bytes past 64 bits",
				`integer.from_bytes(b"\x01" + b"\x00" * 8)`,
				`18446744073709551616`,
			},
			{
				"from_bytes little-endian",
				`integer.from_bytes(b"\x02\x01", "little")`,
				`258`,
			},
			{
				"to_bytes",
				`integer.to_bytes(256, 2)`,
				`b"\x01\x00"`,
			},
			{
				"to_bytes zero padded",
				`integer.to_bytes(1, 4)`,
				`b"\x00\x00\x00\x01"`,
			},
			{
				"to_bytes exact fit",
				`integer.to_bytes(255, 1)`,
				`b"\xff"`,
			},
			{
				"to_bytes past 64 bits",
				`integer.to_bytes(18446744073709551616, 9)`,
				`b"\x01\x00\x00\x00\x00\x00\x00\x00\x00"`,
			},
			{
				"to_bytes big-endian named",
				`integer.to_bytes(258, 2, "big")`,
				`b"\x01\x02"`,
			},
			{
				"to_bytes little-endian",
				`integer.to_bytes(258, 2, "little")`,
				`b"\x02\x01"`,
			},
			{
				"to_bytes signed",
				`integer.to_bytes(-1, 2, signed = True)`,
				`b"\xff\xff"`,
			},
			{
				"to_bytes signed, little-endian",
				`integer.to_bytes(-2, 2, "little", signed = True)`,
				`b"\xfe\xff"`,
			},
			{
				"from_bytes unsigned unless told",
				`integer.from_bytes(b"\xff\xff")`,
				`65535`,
			},
			{
				"from_bytes signed",
				`integer.from_bytes(b"\xff\xff", signed = True)`,
				`-1`,
			},
			{
				"from_bytes signed, little-endian",
				`integer.from_bytes(b"\xfe\xff", "little", signed = True)`,
				`-2`,
			},
			{
				"every argument named",
				`integer.to_bytes(n` +
					` = 258, width = 2, order = "little", signed ` +
					`= False)`,
				`b"\x02\x01"`,
			},
		},
	)

	_Refuse(
		t,
		[]struct {
			name       string
			expression string
			want       error
		}{
			{
				"to_bytes negative",
				`integer.to_bytes(-1, 2)`,
				unpack.ErrRange,
			},
			{
				"to_bytes does not fit",
				`integer.to_bytes(256, 1)`,
				unpack.ErrRange,
			},
			{
				"to_bytes zero width",
				`integer.to_bytes(1, 0)`,
				unpack.ErrRange,
			},
			{
				"to_bytes signed past a byte",
				`integer.to_bytes(128, 1, signed = True)`,
				unpack.ErrRange,
			},
			{
				"to_bytes signed under a byte",
				`integer.to_bytes(-129, 1, signed = True)`,
				unpack.ErrRange,
			},
			{
				"to_bytes in an order nobody has",
				`integer.to_bytes(1, 2, "middle")`,
				integer.ErrOrder,
			},
			{
				"from_bytes in another case",
				`integer.from_bytes(b"\x01", "Little")`,
				integer.ErrOrder,
			},
		},
	)
}

// TestInteger_SignedMatchesGo writes every value of two bytes in two's
// complement, in both byte orders, against what encoding/binary writes for an
// int16, and reads each back.
//
// Every value, because two's complement is right for most numbers and wrong at
// the edges when it is wrong at all: the most negative number has no positive
// twin, and -1 is every bit set.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-11-01 13:00: Encode takes no budget
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestInteger_SignedMatchesGo(t *testing.T) {
	// WIDTH is the two bytes of an int16.
	const WIDTH = 2

	var (
		orders = map[string]binary.AppendByteOrder{
			common.BIG:    binary.BigEndian,
			common.LITTLE: binary.LittleEndian,
		}
	)

	for name, order := range orders {
		for value := math.MinInt16; value <= math.MaxInt16; value++ {
			var (
				want = order.AppendUint16(
					nil,
					uint16(int16(value)),
				)
				field = &common.Field{
					Number: starlark.MakeInt64(int64(value)),
					Width:  WIDTH,
					Order:  name,
				}
				got, err = integer.EncodeSigned(
					SCRIPT,
					field,
				)
			)
			if err != nil || !bytes.Equal(
				got,
				want,
			) {
				t.Fatalf(
					"%d %s wrote %x, %v, want %x",
					value,
					name,
					got,
					err,
					want,
				)
			}

			back, err := integer.DecodeSigned(
				SCRIPT,
				got,
				name,
			)
			if err != nil {
				t.Fatalf(
					"%d %s read back with %v",
					value,
					name,
					err,
				)
			}

			wide, fits := back.Int64()
			if !fits || wide != int64(value) {
				t.Fatalf(
					"%d %s read back %v",
					value,
					name,
					back,
				)
			}
		}
	}
}
