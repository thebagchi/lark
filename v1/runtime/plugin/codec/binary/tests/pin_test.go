package tests

import (
	"math/big"
	"strconv"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/codec/binary/internal/spec"
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
)

// TestFromInt_PinsItsOutput holds binary.from_int to the digits and the padding
// it wrote before C31, for the numbers at each end of 8 to 200 bits against
// every width the API allows, and to the literal rows and refusals below.
//
// Revisions:
//   - 2026-11-02 16:55: initial creation (C31)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestFromInt_PinsItsOutput(t *testing.T) {
	numbers := []*big.Int{
		big.NewInt(0),
		big.NewInt(1),
		big.NewInt(2),
		big.NewInt(5),
		big.NewInt(300),
	}

	for _, bits := range []uint{8, 16, 32, 63, 64, 128, 200} {
		numbers = append(
			numbers,
			new(big.Int).Lsh(
				big.NewInt(1),
				bits-1,
			),
			new(big.Int).Sub(
				new(big.Int).Lsh(
					big.NewInt(1),
					bits,
				),
				big.NewInt(1),
			),
		)
	}

	fn := _Builtin(
		t,
		spec.FROM_INT,
	)

	for _, number := range numbers {
		for _, width := range PIN_WIDTHS {
			t.Run(
				number.String()+"/"+strconv.Itoa(width),
				func(t *testing.T) {
					value, err := fn.CallInternal(
						&starlark.Thread{
							Name: BENCH,
						},
						starlark.Tuple{
							starlark.MakeBigInt(number),
							starlark.MakeInt(width),
						},
						nil,
					)
					if err != nil {
						t.Fatal(err)
					}

					want := number.Text(2)
					if len(want) < width {
						want = strings.Repeat(
							"0",
							width-len(want),
						) + want
					}

					if got, _ := starlark.AsString(value); got != want {
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
			3,
			"000",
		},
		{
			"one",
			1,
			1,
			"1",
		},
		{
			"padded",
			5,
			8,
			"00000101",
		},
		{
			"not truncated",
			300,
			4,
			"100101100",
		},
	} {
		t.Run(
			"literal "+item.name,
			func(t *testing.T) {
				value, err := fn.CallInternal(
					&starlark.Thread{
						Name: BENCH,
					},
					starlark.Tuple{
						starlark.MakeInt64(item.number),
						starlark.MakeInt(item.width),
					},
					nil,
				)
				if err != nil {
					t.Fatal(err)
				}

				if got, _ := starlark.AsString(value); got != item.want {
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
		args starlark.Tuple
		want string
	}{
		{
			"negative",
			starlark.Tuple{starlark.MakeInt(-1), starlark.MakeInt(2)},
			"binary.from_int got -1, which is negative: out of range",
		},
		{
			"zero width",
			starlark.Tuple{starlark.MakeInt(1), starlark.MakeInt(0)},
			"binary.from_int got a width of 0: out of range",
		},
		{
			"negative width",
			starlark.Tuple{starlark.MakeInt(1), starlark.MakeInt(-3)},
			"binary.from_int got a width of -3: out of range",
		},
	} {
		t.Run(
			"refuses "+item.name,
			func(t *testing.T) {
				_, err := fn.CallInternal(
					&starlark.Thread{
						Name: BENCH,
					},
					item.args,
					nil,
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
