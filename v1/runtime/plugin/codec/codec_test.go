// These tests are table driven because the thing under test is a
// specification: the twelve conversions in section 10.6 of the brief, one row
// each, so a missing rule is visible as a missing row.
package codec_test

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/dialect"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

const SCRIPT = "codec_test.star"

// _Eval runs one Starlark expression against the plugin environment and
// returns what it produced, as its String form.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
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
//   - 2026-09-21 10:35: initial creation
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
//   - 2026-09-21 10:35: initial creation
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

// TestBytes_ConvertToAndFromHexAndBits is the first four rows of the table,
// plus the rule that a byte input may be a str.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
//   - 2026-10-03 16:59: bytes2bits of nothing
func TestBytes_ConvertToAndFromHexAndBits(t *testing.T) {
	_Check(t, []struct{ name, expression, want string }{
		{"bytes2hex of bytes", `bytes2hex(b"\x00\xffAB")`, `"00ff4142"`},
		{"bytes2hex of str", `bytes2hex("AB")`, `"4142"`},
		{"bytes2hex of nothing", `bytes2hex(b"")`, `""`},
		{"hex2bytes", `hex2bytes("00ff4142")`, `b"\x00\xffAB"`},
		{"hex2bytes upper case", `hex2bytes("4A")`, `b"J"`},
		{"bytes2bits", `bytes2bits(b"\x01\x80")`, `"0000000110000000"`},
		{"bytes2bits of str", `bytes2bits("A")`, `"01000001"`},
		{"bytes2bits of nothing", `bytes2bits(b"")`, `""`},
		{"bits2bytes", `bits2bytes("0000000110000000")`, `b"\x01\x80"`},
		{"bits2bytes of nothing", `bits2bytes("")`, `b""`},
	})

	_Refuse(t, []struct {
		name       string
		expression string
		want       error
	}{
		{"hex2bytes odd length", `hex2bytes("abc")`, codec.ERR_HEX},
		{"hex2bytes not hex", `hex2bytes("zz")`, codec.ERR_HEX},
		{"bits2bytes not a multiple of 8", `bits2bytes("0000000")`, codec.ERR_BITS},
		{"bits2bytes not bits", `bits2bytes("00000002")`, codec.ERR_BITS},
		{"bytes2hex of a number", `bytes2hex(1)`, unpack.ERR_DATA},
	})
}

// TestInts_ConvertToAndFromBytes is rows five and six: unsigned, big-endian,
// zero-padded, and refused when negative or too wide.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
func TestInts_ConvertToAndFromBytes(t *testing.T) {
	_Check(t, []struct{ name, expression, want string }{
		{"bytes2int", `bytes2int(b"\x01\x00")`, `256`},
		{"bytes2int of nothing", `bytes2int(b"")`, `0`},
		{
			"bytes2int past 64 bits",
			`bytes2int(b"\x01" + b"\x00" * 8)`,
			`18446744073709551616`,
		},
		{"int2bytes", `int2bytes(256, 2)`, `b"\x01\x00"`},
		{"int2bytes zero padded", `int2bytes(1, 4)`, `b"\x00\x00\x00\x01"`},
		{"int2bytes exact fit", `int2bytes(255, 1)`, `b"\xff"`},
		{
			"int2bytes past 64 bits",
			`int2bytes(18446744073709551616, 9)`,
			`b"\x01\x00\x00\x00\x00\x00\x00\x00\x00"`,
		},
	})

	_Refuse(t, []struct {
		name       string
		expression string
		want       error
	}{
		{"int2bytes negative", `int2bytes(-1, 2)`, codec.ERR_RANGE},
		{"int2bytes does not fit", `int2bytes(256, 1)`, codec.ERR_RANGE},
		{"int2bytes zero width", `int2bytes(1, 0)`, codec.ERR_RANGE},
	})
}

// TestBits_ConvertToAndFromHexAndInts is rows seven to ten: the padding rules
// are what each row is about.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
//   - 2026-10-03 16:59: an empty, an odd-length, an upper-case-prefixed and a signed
//     hex string, and a leading zero digit kept
//   - 2026-10-03 19:01: bits2hex across bytes, and of zero digits it has bits for
func TestBits_ConvertToAndFromHexAndInts(t *testing.T) {
	_Check(t, []struct{ name, expression, want string }{
		{"bits2hex", `bits2hex("10101111")`, `"AF"`},
		{"bits2hex pads to a nibble", `bits2hex("101")`, `"5"`},
		{"bits2hex pads across nibbles", `bits2hex("11010")`, `"1A"`},
		{"bits2hex of nothing", `bits2hex("")`, `""`},
		{"bits2hex keeps a leading zero digit", `bits2hex("00000001")`, `"01"`},
		{"bits2hex pads across bytes", `bits2hex("110101111")`, `"1AF"`},
		{"bits2hex keeps zero digits it has bits for", `bits2hex("000000001")`, `"001"`},
		{"hex2bits", `hex2bits("AF")`, `"10101111"`},
		{"hex2bits strips 0x", `hex2bits("0xAF")`, `"10101111"`},
		{"hex2bits strips 0X", `hex2bits("0XAF")`, `"10101111"`},
		{"hex2bits pads to a byte", `hex2bits("A")`, `"00001010"`},
		{"hex2bits pads odd digits to a byte", `hex2bits("ABC")`, `"0000101010111100"`},
		{"hex2bits of nothing", `hex2bits("")`, `""`},
		{"hex2bits lower case", `hex2bits("af")`, `"10101111"`},
		{"int2bits", `int2bits(5, 8)`, `"00000101"`},
		{"int2bits not truncated", `int2bits(300, 4)`, `"100101100"`},
		{"int2bits of zero", `int2bits(0, 3)`, `"000"`},
		{"bits2int", `bits2int("100101100")`, `300`},
		{"bits2int past 64 bits", `bits2int("1" + "0" * 64)`, `18446744073709551616`},
	})

	_Refuse(t, []struct {
		name       string
		expression string
		want       error
	}{
		{"bits2hex not bits", `bits2hex("12")`, codec.ERR_BITS},
		{"hex2bits not hex", `hex2bits("0xZZ")`, codec.ERR_HEX},
		{"hex2bits signed", `hex2bits("-1")`, codec.ERR_HEX},
		{"int2bits negative", `int2bits(-5, 8)`, codec.ERR_RANGE},
		{"bits2int not bits", `bits2int("")`, codec.ERR_BITS},
	})
}

// TestHexInts_ComposeTheOtherTwo is rows eleven and twelve, which the brief
// defines by composition.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
func TestHexInts_ComposeTheOtherTwo(t *testing.T) {
	_Check(t, []struct{ name, expression, want string }{
		{"int2hex", `int2hex(175, 8)`, `"AF"`},
		{"int2hex pads through bits", `int2hex(1, 8)`, `"01"`},
		{"int2hex not truncated", `int2hex(300, 4)`, `"12C"`},
		{"hex2int", `hex2int("AF")`, `175`},
		{"hex2int strips 0x", `hex2int("0x12C")`, `300`},
		{"round trip", `hex2int(int2hex(4096, 16))`, `4096`},
	})

	_Refuse(t, []struct {
		name       string
		expression string
		want       error
	}{
		{"int2hex negative", `int2hex(-1, 8)`, codec.ERR_RANGE},
		{"hex2int not hex", `hex2int("g")`, codec.ERR_HEX},
	})
}

// TestBits_AgreeForEveryByte converts every byte value through bytes2bits,
// bits2bytes and bits2hex, against answers reached another way: the bits a test
// of each one spells, the byte itself, and the digits encoding/hex writes.
//
// Every value, because a conversion that works on the bytes of a word side by
// side can be right for most values and wrong for one; and all of them in one
// input as well, so that whole words follow one another.
//
// Revisions:
//   - 2026-10-03 19:01: initial creation
func TestBits_AgreeForEveryByte(t *testing.T) {
	const EVERY = `[b for b in range(256) if not (
    bytes2bits(bytes([b])) == "".join(["1" if b & (128 >> i) else "0" for i in range(8)])
    and bits2bytes(bytes2bits(bytes([b]))) == bytes([b])
    and bits2hex(bytes2bits(bytes([b]))) == bytes2hex(bytes([b])).upper()
)]`

	_Check(t, []struct{ name, expression, want string }{
		{"every byte alone", EVERY, `[]`},
		{
			"every byte in one input",
			`bits2bytes(bytes2bits(bytes(range(256)))) == bytes(range(256))`,
			`True`,
		},
	})
}

// TestBits_RefusedWhereverTheWrongCharacterIs puts one character that is not a
// bit at every position of seventeen - two words of eight, and one character
// left over - and expects each refused.
//
// Each character differs from 0 in one bit, from the second bit to the seventh,
// so every bit a check of eight characters at once compares is tried; é is two
// bytes, each with the eighth bit set.
//
// Revisions:
//   - 2026-10-03 19:01: initial creation
func TestBits_RefusedWhereverTheWrongCharacterIs(t *testing.T) {
	const LENGTH = 17

	for _, wrong := range []string{"2", "4", "8", " ", "\x10", "p", "é"} {
		for at := range LENGTH {
			bits := strings.Repeat("1", at) + wrong + strings.Repeat("0", LENGTH-1-at)
			expression := fmt.Sprintf("bits2int(%q)", bits)

			_, err := _Eval(t, expression)
			if !errors.Is(err, codec.ERR_BITS) {
				t.Fatalf("%s gave %v, want %v", expression, err, codec.ERR_BITS)
			}
		}
	}
}
