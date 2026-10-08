// These tests are table driven because the thing under test is a
// specification: each conversion, the case it writes and reads, and the
// padding rule it keeps, one row each, so a missing rule is visible as a
// missing row.
package hex_test

import (
	"errors"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/dialect"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/binary"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/hex"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

const SCRIPT = "hex_test.star"

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

// TestHex_ConvertsToAndFromBytes is two digits a byte, written in upper case
// and read in either, plus the rule that a byte input may be a str.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as part of codec's
//     TestBytes_ConvertToAndFromHexAndBits
//   - 2026-10-08 17:52: spells the calls as members of the hex module, which writes
//     upper case and reads either
//   - 2026-10-08 18:06: to_bytes with a 0x, taken off once in either case
func TestHex_ConvertsToAndFromBytes(t *testing.T) {
	_Check(t, []struct{ name, expression, want string }{
		{"from_bytes", `hex.from_bytes(b"\x00\xffAB")`, `"00FF4142"`},
		{"from_bytes of str", `hex.from_bytes("AB")`, `"4142"`},
		{"from_bytes of nothing", `hex.from_bytes(b"")`, `""`},
		{"from_bytes writes upper case", `hex.from_bytes(b"\xab\xcd\xef")`, `"ABCDEF"`},
		{"to_bytes", `hex.to_bytes("00FF4142")`, `b"\x00\xffAB"`},
		{"to_bytes lower case", `hex.to_bytes("00ff4142")`, `b"\x00\xffAB"`},
		{"to_bytes mixed case", `hex.to_bytes("aBcD")`, `b"\xab\xcd"`},
		{"to_bytes strips 0x", `hex.to_bytes("0x4142")`, `b"AB"`},
		{"to_bytes strips 0X", `hex.to_bytes("0XaBcD")`, `b"\xab\xcd"`},
		{"to_bytes of the prefix alone", `hex.to_bytes("0x")`, `b""`},
	})

	_Refuse(t, []struct {
		name       string
		expression string
		want       error
	}{
		{"to_bytes odd length", `hex.to_bytes("abc")`, hex.ERR_HEX},
		{"to_bytes odd length after the prefix", `hex.to_bytes("0xabc")`, hex.ERR_HEX},
		{"to_bytes prefix taken off once", `hex.to_bytes("0x0x41")`, hex.ERR_HEX},
		{"to_bytes not hex", `hex.to_bytes("zz")`, hex.ERR_HEX},
		{"from_bytes of a number", `hex.from_bytes(1)`, unpack.ERR_DATA},
	})
}

// TestHex_ConvertsToAndFromInts is digits padded to the width, which counts
// digits, never cut when the number is wider, and refused when the width is
// past the run's memory.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as codec's TestHexInts_ComposeTheOtherTwo
//   - 2026-10-08 17:52: spells the calls as members of the hex module, whose width
//     counts digits, and refuses empty text
//   - 2026-10-08 21:37: a width past the run's memory
//   - 2026-10-08 22:19: a width past the million fmt pads to
func TestHex_ConvertsToAndFromInts(t *testing.T) {
	_Check(t, []struct{ name, expression, want string }{
		{"from_int", `hex.from_int(175, 2)`, `"AF"`},
		{
			"from_int past the million fmt pads to",
			`[len(hex.from_int(255, 1000001)), hex.from_int(255, 1000001)[-2:]]`,
			`[1000001, "FF"]`,
		},
		{"from_int pads to the width in digits", `hex.from_int(255, 4)`, `"00FF"`},
		{"from_int not truncated", `hex.from_int(300, 2)`, `"12C"`},
		{"from_int of zero", `hex.from_int(0, 1)`, `"0"`},
		{"to_int", `hex.to_int("AF")`, `175`},
		{"to_int lower case", `hex.to_int("af")`, `175`},
		{"to_int strips 0x", `hex.to_int("0x12C")`, `300`},
		{"to_int strips 0X", `hex.to_int("0X12c")`, `300`},
		{"to_int past 64 bits", `hex.to_int("1" + "0" * 16)`, `18446744073709551616`},
		{"round trip", `hex.to_int(hex.from_int(4096, 8))`, `4096`},
	})

	_Refuse(t, []struct {
		name       string
		expression string
		want       error
	}{
		{"from_int negative", `hex.from_int(-1, 2)`, unpack.ERR_RANGE},
		{"from_int zero width", `hex.from_int(1, 0)`, unpack.ERR_RANGE},
		{"from_int wider than the run", `hex.from_int(0, 1 << 40)`, scheduler.ERR_MEMORY},
		{"to_int not hex", `hex.to_int("g")`, hex.ERR_HEX},
		{"to_int signed", `hex.to_int("-1")`, hex.ERR_HEX},
		{"to_int of nothing", `hex.to_int("")`, hex.ERR_HEX},
		{"to_int of the prefix alone", `hex.to_int("0x")`, hex.ERR_HEX},
	})
}

// TestHex_ConvertsToAndFromBinary is the padding rules: bits to whole digits
// one way, digits to whole bytes the other.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as part of codec's
//     TestBits_ConvertToAndFromHexAndInts
//   - 2026-10-03 16:59: an empty, an odd-length, an upper-case-prefixed and a signed
//     hex string, and a leading zero digit kept
//   - 2026-10-03 19:01: bits2hex across bytes, and of zero digits it has bits for
//   - 2026-10-08 17:52: spells the calls as members of the hex module
//   - 2026-10-08 18:32: from_binary with a 0b, taken off before the bits are counted
func TestHex_ConvertsToAndFromBinary(t *testing.T) {
	_Check(t, []struct{ name, expression, want string }{
		{"from_binary", `hex.from_binary("10101111")`, `"AF"`},
		{"from_binary pads to a digit", `hex.from_binary("101")`, `"5"`},
		{"from_binary strips 0b", `hex.from_binary("0b11010")`, `"1A"`},
		{"from_binary counts the bits after 0B", `hex.from_binary("0B101")`, `"5"`},
		{"from_binary pads across digits", `hex.from_binary("11010")`, `"1A"`},
		{"from_binary of nothing", `hex.from_binary("")`, `""`},
		{"from_binary keeps a leading zero digit", `hex.from_binary("00000001")`, `"01"`},
		{"from_binary pads across bytes", `hex.from_binary("110101111")`, `"1AF"`},
		{
			"from_binary keeps zero digits it has bits for",
			`hex.from_binary("000000001")`,
			`"001"`,
		},
		{"to_binary", `hex.to_binary("AF")`, `"10101111"`},
		{"to_binary lower case", `hex.to_binary("af")`, `"10101111"`},
		{"to_binary strips 0x", `hex.to_binary("0xAF")`, `"10101111"`},
		{"to_binary strips 0X", `hex.to_binary("0XAF")`, `"10101111"`},
		{"to_binary pads to a byte", `hex.to_binary("A")`, `"00001010"`},
		{
			"to_binary pads odd digits to a byte",
			`hex.to_binary("ABC")`,
			`"0000101010111100"`,
		},
		{"to_binary of nothing", `hex.to_binary("")`, `""`},
	})

	_Refuse(t, []struct {
		name       string
		expression string
		want       error
	}{
		{"from_binary not bits", `hex.from_binary("12")`, binary.ERR_BITS},
		{
			"from_binary refuses the hex prefix",
			`hex.from_binary("0x101")`,
			binary.ERR_BITS,
		},
		{"to_binary not hex", `hex.to_binary("0xZZ")`, hex.ERR_HEX},
		{"to_binary signed", `hex.to_binary("-1")`, hex.ERR_HEX},
	})
}

// TestHex_AgreesWithBinaryForEveryByte writes every byte value as hex two
// ways - through its bits, and directly - and expects the same digits.
//
// Every value, because from_binary reads bits a word at a time and can be
// right for most values and wrong for one; and all of them in one input as
// well, so that whole words follow one another.
//
// Revisions:
//   - 2026-10-03 19:01: initial creation, as part of codec's
//     TestBits_AgreeForEveryByte
//   - 2026-10-08 17:52: spells the calls as members of the hex and binary modules
func TestHex_AgreesWithBinaryForEveryByte(t *testing.T) {
	const EVERY = `[b for b in range(256) if not (
    hex.from_binary(binary.from_bytes(bytes([b]))) == hex.from_bytes(bytes([b]))
)]`
	const WHOLE = `hex.from_binary(binary.from_bytes(bytes(range(256)))) == ` +
		`hex.from_bytes(bytes(range(256)))`

	_Check(t, []struct{ name, expression, want string }{
		{"every byte alone", EVERY, `[]`},
		{"every byte in one input", WHOLE, `True`},
	})
}
