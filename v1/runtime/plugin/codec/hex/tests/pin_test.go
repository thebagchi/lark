package tests

import (
	"math/big"
	"strconv"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/codec/binary"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/hex/internal/spec"
)

const (
	// MAX_ALLOCS is the most allocations one from_binary call may make at any
	// size: the text of the call, the bytes decoded, the hex written, and the
	// Starlark string around it.
	//
	// Revisions:
	//   - 2026-11-02 17:41: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	MAX_ALLOCS = 4
)

var (
	// PIN_WIDTHS is every kind of padding width: the least the API allows, widths
	// below, at and above the length of the numbers, and one past a million, which
	// fmt refuses.
	//
	// Revisions:
	//   - 2026-11-02 17:41: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	PIN_WIDTHS = []int{1, 2, 3, 4, 7, 8, 9, 16, 17, 32, 33, 64, 65, 130, 500, 1000001}

	// PIN_BITS are the lengths of bits that from_binary reads: none, each count
	// to a whole byte and past, and a few longer ones.
	//
	// Revisions:
	//   - 2026-11-02 17:41: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	PIN_BITS = []int{
		0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 11, 12, 13,
		15, 16, 17, 23, 24, 25, 31, 33, 63, 64, 65, 127, 129,
	}
)

// _PinNumbers are the numbers that from_int is pinned on: zero, one, each end
// of the digits, and the largest and smallest values of 8, 16, 32, 64 and 128
// bits.
//
// Revisions:
//   - 2026-11-02 16:55: initial creation (C31)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _PinNumbers() []*big.Int {
	numbers := []*big.Int{
		big.NewInt(0),
		big.NewInt(1),
		big.NewInt(9),
		big.NewInt(10),
		big.NewInt(15),
		big.NewInt(16),
		big.NewInt(0xABCDEF),
	}

	for _, bits := range []uint{8, 16, 32, 63, 64, 128, 200} {
		var (
			smallest = new(big.Int).Lsh(
				big.NewInt(1),
				bits-1,
			)
			largest = new(big.Int).Sub(
				new(big.Int).Lsh(
					big.NewInt(1),
					bits,
				),
				big.NewInt(1),
			)
		)

		numbers = append(
			numbers,
			smallest,
			largest,
		)
	}

	return numbers
}

// _OldDigits is the digits of number in base, upper case, zeros in front to
// width, as hex.from_int and binary.from_int wrote them before C31.
//
// Revisions:
//   - 2026-11-02 16:55: initial creation (C31)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _OldDigits(
	number *big.Int,
	base int,
	width int,
) string {
	digits := strings.ToUpper(number.Text(base))
	if len(digits) < width {
		digits = strings.Repeat(
			"0",
			width-len(digits),
		) + digits
	}

	return digits
}

// _OldHexOfBits is the hex that from_binary wrote before C31, found a nibble at
// a time and not through fmt: the bits left-padded to whole digits, then four to
// a digit.
//
// Revisions:
//   - 2026-11-02 16:55: initial creation (C31)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _OldHexOfBits(bits string) string {
	if len(bits)%spec.BITS_PER_DIGIT != 0 {
		bits = strings.Repeat(
			"0",
			spec.BITS_PER_DIGIT-len(bits)%spec.BITS_PER_DIGIT,
		) + bits
	}

	var text strings.Builder

	for idx := 0; idx < len(bits); idx += spec.BITS_PER_DIGIT {
		value, err := strconv.ParseUint(
			bits[idx:idx+spec.BITS_PER_DIGIT],
			2,
			8,
		)
		if err != nil {
			panic(err)
		}

		text.WriteByte(spec.UPPER[value])
	}

	return text.String()
}

// _Run calls the builtin the way a script's call reaches it.
//
// Revisions:
//   - 2026-11-02 16:55: initial creation (C31)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Run(
	tb testing.TB,
	name string,
	args ...starlark.Value,
) (string, error) {
	tb.Helper()

	value, err := _Builtin(
		tb,
		name,
	).CallInternal(
		&starlark.Thread{
			Name: BENCH,
		},
		args,
		nil,
	)
	if err != nil {
		return "", err
	}

	text, ok := starlark.AsString(value)
	if !ok {
		tb.Fatalf(
			"%s gave %v, not a string",
			name,
			value,
		)
	}

	return text, nil
}

// TestFromInt_PinsItsOutput holds from_int to the digits, the case and the
// padding it wrote before C31, for every number against every width the API
// allows, and to the literal rows below.
//
// Revisions:
//   - 2026-11-02 16:55: initial creation (C31)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestFromInt_PinsItsOutput(t *testing.T) {
	for _, number := range _PinNumbers() {
		for _, width := range PIN_WIDTHS {
			t.Run(
				number.String()+"/"+strconv.Itoa(width),
				func(t *testing.T) {
					got, err := _Run(
						t,
						spec.FROM_INT,
						starlark.MakeBigInt(number),
						starlark.MakeInt(width),
					)
					if err != nil {
						t.Fatal(err)
					}

					want := _OldDigits(
						number,
						16,
						width,
					)
					if got != want {
						t.Fatalf(
							"got %d chars, want %d, or other digits",
							len(got),
							len(want),
						)
					}
				},
			)
		}
	}

	for _, item := range []struct {
		name   string
		number int64
		width  int
		want   string
	}{
		{
			"zero",
			0,
			1,
			"0",
		},
		{
			"zero wide",
			0,
			4,
			"0000",
		},
		{
			"one",
			1,
			1,
			"1",
		},
		{
			"all letters",
			0xABCDEF,
			6,
			"ABCDEF",
		},
		{
			"letters padded",
			0xABCDEF,
			8,
			"00ABCDEF",
		},
		{
			"not truncated",
			0xABCDEF,
			2,
			"ABCDEF",
		},
		{
			"largest int64",
			1<<63 - 1,
			16,
			"7FFFFFFFFFFFFFFF",
		},
		{
			"digit then letter",
			0x1F,
			3,
			"01F",
		},
	} {
		t.Run(
			"literal "+item.name,
			func(t *testing.T) {
				got, err := _Run(
					t,
					spec.FROM_INT,
					starlark.MakeInt64(item.number),
					starlark.MakeInt(item.width),
				)
				if err != nil {
					t.Fatal(err)
				}

				if got != item.want {
					t.Fatalf(
						"got %q, want %q",
						got,
						item.want,
					)
				}
			},
		)
	}

	for _, item := range []struct {
		name string
		args []starlark.Value
		want string
	}{
		{
			"negative",
			[]starlark.Value{starlark.MakeInt(-1), starlark.MakeInt(2)},
			"hex.from_int got -1, which is negative: out of range",
		},
		{
			"zero width",
			[]starlark.Value{starlark.MakeInt(1), starlark.MakeInt(0)},
			"hex.from_int got a width of 0: out of range",
		},
		{
			"negative width",
			[]starlark.Value{starlark.MakeInt(1), starlark.MakeInt(-3)},
			"hex.from_int got a width of -3: out of range",
		},
	} {
		t.Run(
			"refuses "+item.name,
			func(t *testing.T) {
				_, err := _Run(
					t,
					spec.FROM_INT,
					item.args...,
				)
				if err == nil || err.Error() != item.want {
					t.Fatalf(
						"got %v, want %q",
						err,
						item.want,
					)
				}
			},
		)
	}
}

// TestFromBinary_PinsItsOutput holds from_binary to the digits, the case and
// the padding it wrote before C31: for bits of every length near a digit and a
// byte, in four patterns, with no prefix and with 0b and 0B, and for the
// refusals.
//
// Revisions:
//   - 2026-11-02 16:55: initial creation (C31)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestFromBinary_PinsItsOutput(t *testing.T) {
	patterns := map[string]func(int) string{
		"ones": func(n int) string {
			return strings.Repeat(
				"1",
				n,
			)
		},
		"zeros": func(n int) string {
			return strings.Repeat(
				"0",
				n,
			)
		},
		"alt": func(n int) string {
			return strings.Repeat(
				"10",
				n,
			)[:n]
		},
		"last": func(n int) string {
			if n == 0 {
				return ""
			}

			return strings.Repeat(
				"0",
				n-1,
			) + "1"
		},
	}

	for name, pattern := range patterns {
		for _, count := range PIN_BITS {
			for _, prefix := range []string{"", "0b", "0B"} {
				bits := pattern(count)

				t.Run(
					name+"/"+strconv.Itoa(count)+"/"+prefix,
					func(t *testing.T) {
						got, err := _Run(
							t,
							spec.FROM_BINARY,
							starlark.String(prefix+bits),
						)
						if err != nil {
							t.Fatal(err)
						}

						if want := _OldHexOfBits(bits); got != want {
							t.Fatalf(
								"got %q, want %q",
								got,
								want,
							)
						}
					},
				)
			}
		}
	}

	for _, item := range []struct {
		name string
		bits string
		want string
	}{
		{
			"empty",
			"",
			"",
		},
		{
			"prefix alone",
			"0b",
			"",
		},
		{
			"one bit",
			"1",
			"1",
		},
		{
			"one zero bit",
			"0",
			"0",
		},
		{
			"a byte",
			"11111111",
			"FF",
		},
		{
			"leading zero digit",
			"00000001",
			"01",
		},
		{
			"nine bits",
			"111111111",
			"1FF",
		},
		{
			"nine zero bits",
			"000000000",
			"000",
		},
		{
			"letters",
			"10101011",
			"AB",
		},
	} {
		t.Run(
			"literal "+item.name,
			func(t *testing.T) {
				got, err := _Run(
					t,
					spec.FROM_BINARY,
					starlark.String(item.bits),
				)
				if err != nil {
					t.Fatal(err)
				}

				if got != item.want {
					t.Fatalf(
						"got %q, want %q",
						got,
						item.want,
					)
				}
			},
		)
	}

	for _, item := range []struct {
		name string
		bits string
	}{
		{
			"digit two",
			"12",
		},
		{
			"hex prefix",
			"0x101",
		},
		{
			"space",
			"1 0",
		},
		{
			"sign",
			"-1",
		},
		{
			"wrong char in a long run",
			strings.Repeat(
				"1",
				70,
			) + "2",
		},
	} {
		t.Run(
			"refuses "+item.name,
			func(t *testing.T) {
				_, err := _Run(
					t,
					spec.FROM_BINARY,
					starlark.String(item.bits),
				)
				if err == nil || !strings.Contains(
					err.Error(),
					binary.ErrBits.Error(),
				) {
					t.Fatalf(
						"got %v, want %v",
						err,
						binary.ErrBits,
					)
				}
			},
		)
	}
}

// TestFromBinary_MakesFewAllocationsAtAnySize holds from_binary to a count of
// allocations that does not grow with the input. fmt's %X grew its buffer as it
// wrote: 28 allocations at 64 KiB.
//
// Revisions:
//   - 2026-11-02 16:55: initial creation (C31)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestFromBinary_MakesFewAllocationsAtAnySize(t *testing.T) {
	for _, size := range SIZES {
		t.Run(
			strconv.Itoa(size),
			func(t *testing.T) {
				var (
					fn = _Builtin(
						t,
						spec.FROM_BINARY,
					)
					args = starlark.Tuple{
						starlark.String(binary.Encode(_Data(size))),
					}
					thread = &starlark.Thread{
						Name: BENCH,
					}
				)

				got := testing.AllocsPerRun(
					20,
					func() {
						_, err := fn.CallInternal(
							thread,
							args,
							nil,
						)
						if err != nil {
							t.Fatal(err)
						}
					},
				)
				if got > MAX_ALLOCS {
					t.Fatalf(
						"%d bytes: %v allocations, want at most %d",
						size,
						got,
						MAX_ALLOCS,
					)
				}
			},
		)
	}
}
