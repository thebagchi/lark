// These tests are table driven because the thing under test is a
// specification: each conversion and the padding rule it keeps, one row each,
// so a missing rule is visible as a missing row.
package binary_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/dialect"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/binary"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

const SCRIPT = "binary_test.star"

// _Eval runs one Starlark expression against the plugin environment and
// returns what it produced, as its String form.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, in codec
//   - 2026-10-08 17:52: moved here with the conversions it exercises
func _Eval(t *testing.T, expression string) (string, error) {
	t.Helper()

	env, err := plugin.Environment(plugin.DEFAULT)
	if err != nil {
		t.Fatalf("environment: %v", err)
	}

	value, err := starlark.EvalOptions(
		dialect.OPTIONS,
		&starlark.Thread{Name: SCRIPT},
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
//   - 2026-10-08 17:52: moved here
func _Check(t *testing.T, cases []struct{ name, expression, want string }) {
	t.Helper()

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			got, err := _Eval(t, item.expression)
			if err != nil {
				t.Fatalf("%s: %v", item.expression, err)
			}

			if got != item.want {
				t.Fatalf("%s gave %s, want %s", item.expression, got, item.want)
			}
		})
	}
}

// _Refuse runs a table of expressions that must fail with the sentinel given.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, in codec
//   - 2026-10-08 17:52: moved here
func _Refuse(t *testing.T, cases []struct {
	name       string
	expression string
	want       error
}) {
	t.Helper()

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			_, err := _Eval(t, item.expression)
			if !errors.Is(err, item.want) {
				t.Fatalf("%s gave %v, want %v", item.expression, err, item.want)
			}
		})
	}
}

// TestBinary_ConvertsToAndFromBytes is eight bits a byte, the most significant
// first, plus the rule that a byte input may be a str.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as part of codec's
//     TestBytes_ConvertToAndFromHexAndBits
//   - 2026-10-03 16:59: bytes2bits of nothing
//   - 2026-10-08 17:52: spells the calls as members of the binary module
//   - 2026-10-08 18:32: to_bytes with a 0b, taken off once in either case
func TestBinary_ConvertsToAndFromBytes(t *testing.T) {
	_Check(t, []struct{ name, expression, want string }{
		{"from_bytes", `binary.from_bytes(b"\x01\x80")`, `"0000000110000000"`},
		{"from_bytes of str", `binary.from_bytes("A")`, `"01000001"`},
		{"from_bytes of nothing", `binary.from_bytes(b"")`, `""`},
		{"to_bytes", `binary.to_bytes("0000000110000000")`, `b"\x01\x80"`},
		{"to_bytes of nothing", `binary.to_bytes("")`, `b""`},
		{"to_bytes strips 0b", `binary.to_bytes("0b01000001")`, `b"A"`},
		{"to_bytes strips 0B", `binary.to_bytes("0B01000001")`, `b"A"`},
		{"to_bytes of the prefix alone", `binary.to_bytes("0b")`, `b""`},
	})

	_Refuse(t, []struct {
		name       string
		expression string
		want       error
	}{
		{"to_bytes not a multiple of 8", `binary.to_bytes("0000000")`, binary.ERR_BITS},
		{
			"to_bytes short of a byte after the prefix",
			`binary.to_bytes("0b0100")`,
			binary.ERR_BITS,
		},
		{"to_bytes not bits", `binary.to_bytes("00000002")`, binary.ERR_BITS},
		{"from_bytes of a number", `binary.from_bytes(1)`, unpack.ERR_DATA},
	})
}

// TestBinary_ConvertsToAndFromInts is digits padded to the width, which counts
// digits, never cut when the number is wider, and refused when the width is
// past the run's memory.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as part of codec's
//     TestBits_ConvertToAndFromHexAndInts
//   - 2026-10-08 17:52: spells the calls as members of the binary module, and refuses
//     a width of zero
//   - 2026-10-08 18:32: to_int with a 0b, taken off once in either case
//   - 2026-10-08 21:37: a width past the run's memory
//   - 2026-10-08 22:19: a width past the million fmt pads to
func TestBinary_ConvertsToAndFromInts(t *testing.T) {
	_Check(t, []struct{ name, expression, want string }{
		{"from_int", `binary.from_int(5, 8)`, `"00000101"`},
		{
			"from_int past the million fmt pads to",
			`[len(binary.from_int(5, 1000001)), binary.from_int(5, 1000001)[-3:]]`,
			`[1000001, "101"]`,
		},
		{"from_int not truncated", `binary.from_int(300, 4)`, `"100101100"`},
		{"from_int of zero", `binary.from_int(0, 3)`, `"000"`},
		{"to_int", `binary.to_int("100101100")`, `300`},
		{"to_int past 64 bits", `binary.to_int("1" + "0" * 64)`, `18446744073709551616`},
		{"to_int strips 0b", `binary.to_int("0b101")`, `5`},
		{"to_int strips 0B", `binary.to_int("0B101")`, `5`},
	})

	_Refuse(t, []struct {
		name       string
		expression string
		want       error
	}{
		{"from_int negative", `binary.from_int(-5, 8)`, unpack.ERR_RANGE},
		{"from_int zero width", `binary.from_int(5, 0)`, unpack.ERR_RANGE},
		{
			"from_int wider than the run",
			`binary.from_int(0, 1 << 40)`,
			scheduler.ERR_MEMORY,
		},
		{"to_int of nothing", `binary.to_int("")`, binary.ERR_BITS},
		{"to_int of the prefix alone", `binary.to_int("0b")`, binary.ERR_BITS},
		{"to_int prefix taken off once", `binary.to_int("0b0b1")`, binary.ERR_BITS},
	})
}

// TestBinary_AgreesForEveryByte converts every byte value through from_bytes
// and to_bytes, against answers reached another way: the bits a test of each
// one spells, and the byte itself.
//
// Every value, because a conversion that works on the bytes of a word side by
// side can be right for most values and wrong for one; and all of them in one
// input as well, so that whole words follow one another.
//
// Revisions:
//   - 2026-10-03 19:01: initial creation, as codec's TestBits_AgreeForEveryByte
//   - 2026-10-08 17:52: spells the calls as members of the binary module, leaving the
//     hex agreement to hex's own test
func TestBinary_AgreesForEveryByte(t *testing.T) {
	const EVERY = `[b for b in range(256) if not (
    binary.from_bytes(bytes([b])) == "".join(["1" if b & (128 >> i) else "0" for i in range(8)])
    and binary.to_bytes(binary.from_bytes(bytes([b]))) == bytes([b])
)]`
	const WHOLE = `binary.to_bytes(binary.from_bytes(bytes(range(256)))) == bytes(range(256))`

	_Check(t, []struct{ name, expression, want string }{
		{"every byte alone", EVERY, `[]`},
		{"every byte in one input", WHOLE, `True`},
	})
}

// TestBinary_RefusedWhereverTheWrongCharacterIs puts one character that is not
// a bit at every position of seventeen - two words of eight, and one character
// left over - and expects each refused.
//
// Each character differs from 0 in one bit, from the second bit to the seventh,
// so every bit a check of eight characters at once compares is tried; é is two
// bytes, each with the eighth bit set.
//
// Revisions:
//   - 2026-10-03 19:01: initial creation, as codec's
//     TestBits_RefusedWhereverTheWrongCharacterIs
//   - 2026-10-08 17:52: spells the call as a member of the binary module
func TestBinary_RefusedWhereverTheWrongCharacterIs(t *testing.T) {
	const LENGTH = 17

	for _, wrong := range []string{"2", "4", "8", " ", "\x10", "p", "é"} {
		for at := range LENGTH {
			bits := strings.Repeat("1", at) + wrong + strings.Repeat("0", LENGTH-1-at)
			expression := fmt.Sprintf("binary.to_int(%q)", bits)

			_, err := _Eval(t, expression)
			if !errors.Is(err, binary.ERR_BITS) {
				t.Fatalf("%s gave %v, want %v", expression, err, binary.ERR_BITS)
			}
		}
	}
}
