// These tests hold the word path to the big.Int path that it stands beside.
// A field of eight bytes or fewer is written and read through a uint64, and
// every wider field through big.Int. Two paths are two contracts unless a test
// says they are one, and the contract includes what each path refuses. So each
// test here runs the package against a copy of the big.Int code, as it was
// before the word path, on the edges of every width, one past each edge, and
// random values, and wants the same bytes, the same number, and the same error
// text.
//
// Revisions:
//   - 2026-10-29 06:17: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
package tests

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"math/rand/v2"
	"slices"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/integer"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

const (
	// ROUNDS is how many random values each width and order gets, and SEED
	// fixes them, so a failure comes back on the next run.
	//
	// Revisions:
	//   - 2026-10-29 06:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	ROUNDS = 64
	SEED   = 16

	// BOGUS is an order that is neither big nor little, which both paths
	// must refuse alike.
	//
	// Revisions:
	//   - 2026-10-29 06:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	BOGUS = "middle"

	// FAR is a bit count past every width under test, for a number that no
	// width here holds.
	//
	// Revisions:
	//   - 2026-10-29 06:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	FAR = 100
)

var (
	// ORDERS are the orders each width is tried in: the two a script may name,
	// and one it may not.
	//
	// Revisions:
	//   - 2026-10-29 06:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	ORDERS = []string{
		common.BIG,
		common.LITTLE,
		BOGUS,
	}

	// ENCODERS pair each encoder with the big.Int code it replaced.
	//
	// Revisions:
	//   - 2026-10-29 06:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	ENCODERS = []*_Encoders{
		{
			word: integer.Encode,
			big:  _BigEncode,
		},
		{
			word: integer.EncodeSigned,
			big:  _BigEncodeSigned,
		},
	}

	// DECODERS pair each decoder with the big.Int code it replaced.
	//
	// Revisions:
	//   - 2026-10-29 06:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	DECODERS = []*_Decoders{
		{
			word: integer.Decode,
			big:  _BigDecode,
		},
		{
			word: integer.DecodeSigned,
			big:  _BigDecodeSigned,
		},
	}

	// SENTINELS are the errors a caller may look for in a refusal.
	//
	// Revisions:
	//   - 2026-10-29 06:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	SENTINELS = []error{
		unpack.ErrRange,
		integer.ErrOrder,
	}
)

// _Encoder is the shape that Encode and EncodeSigned share.
//
// Revisions:
//   - 2026-10-29 06:17: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Encoder func(
	string,
	*common.Field,
) ([]byte, error)

// _Decoder is the shape that Decode and DecodeSigned share.
//
// Revisions:
//   - 2026-10-29 06:17: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Decoder func(
	string,
	[]byte,
	string,
) (starlark.Int, error)

// _BigDecoder is the shape of the big.Int code that Decode and DecodeSigned
// replaced.
//
// Revisions:
//   - 2026-11-04 06:52: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _BigDecoder func(
	string,
	[]byte,
	string,
) (*big.Int, error)

// _Encoders is an encoder of the package, which has the word path, and the
// big.Int code it must match.
//
// Revisions:
//   - 2026-10-29 06:17: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Encoders struct {
	word _Encoder
	big  _Encoder
}

// _Decoders is a decoder of the package, which has the word path, and the
// big.Int code it must match.
//
// Revisions:
//   - 2026-10-29 06:17: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Decoders struct {
	word _Decoder
	big  _BigDecoder
}

// TestInteger_EncodeWordMatchesBig writes each number with Encode and
// EncodeSigned, and with the big.Int code they replaced, and wants the same
// bytes or the same refusal.
//
// The widths run from below one, which _Width refuses, through every width
// the word path takes, to the first that it leaves to big.Int.
//
// Revisions:
//   - 2026-10-29 00:40: initial creation
//   - 2026-11-01 13:00: Encode takes no budget
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestInteger_EncodeWordMatchesBig(t *testing.T) {
	var (
		random = rand.New(rand.NewPCG(
			SEED,
			SEED,
		))
	)

	for width := -1; width <= 8+1; width++ {
		for _, order := range ORDERS {
			numbers := _Numbers(
				t,
				random,
				width,
			)

			t.Run(
				fmt.Sprintf(
					"%s/%d",
					order,
					width,
				),
				func(t *testing.T) {
					for _, number := range numbers {
						field := &common.Field{
							Number: starlark.MakeBigInt(number),
							Width:  width,
							Order:  order,
						}

						for _, pair := range ENCODERS {
							_SameBytes(
								t,
								field,
								pair,
							)
						}
					}
				},
			)
		}
	}
}

// TestInteger_DecodeWordMatchesBig reads each run of bytes with Decode and
// DecodeSigned, and with the big.Int code they replaced, and wants the same
// number or the same refusal, and the bytes left as they were.
//
// Revisions:
//   - 2026-10-29 00:40: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestInteger_DecodeWordMatchesBig(t *testing.T) {
	random := rand.New(rand.NewPCG(
		SEED,
		SEED,
	))

	for size := 0; size <= 8+1; size++ {
		for _, order := range ORDERS {
			runs := _Runs(
				random,
				size,
			)

			t.Run(
				fmt.Sprintf(
					"%s/%d",
					order,
					size,
				),
				func(t *testing.T) {
					for _, data := range runs {
						for _, pair := range DECODERS {
							_SameNumber(
								t,
								data,
								order,
								pair,
							)
						}
					}
				},
			)
		}
	}
}

// _SameBytes fails t unless both encoders of pair write field alike: the same
// bytes, or an error with the same text and the same sentinels.
//
// Revisions:
//   - 2026-10-29 00:40: initial creation
//   - 2026-11-01 13:00: Encode takes no budget
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _SameBytes(
	t *testing.T,
	field *common.Field,
	pair *_Encoders,
) {
	t.Helper()

	var (
		got, err = pair.word(
			WHO,
			field,
		)
		expected, refusal = pair.big(
			WHO,
			field,
		)
	)

	if !bytes.Equal(
		got,
		expected,
	) || !_SameError(
		err,
		refusal,
	) {
		t.Fatalf(
			"%s in %d bytes: got %x, %v, want %x, %v",
			field.Number,
			field.Width,
			got,
			err,
			expected,
			refusal,
		)
	}
}

// _SameNumber fails t unless both decoders of pair read data alike: the same
// number, or an error with the same text and the same sentinels. It also fails
// t when the word path changes data, which belongs to the caller.
//
// Revisions:
//   - 2026-10-29 00:40: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _SameNumber(
	t *testing.T,
	data []byte,
	order string,
	pair *_Decoders,
) {
	t.Helper()

	var (
		before   = slices.Clone(data)
		got, err = pair.word(
			WHO,
			data,
			order,
		)
	)
	if !bytes.Equal(
		data,
		before,
	) {
		t.Fatalf(
			"%x became %x",
			before,
			data,
		)
	}

	var (
		expected, refusal = pair.big(
			WHO,
			data,
			order,
		)
		same = (err == nil) == (expected != nil) &&
			(err != nil || got.BigInt().Cmp(expected) == 0)
	)
	if !same || !_SameError(
		err,
		refusal,
	) {
		t.Fatalf(
			"%x: got %v, %v, want %v, %v",
			data,
			got,
			err,
			expected,
			refusal,
		)
	}
}

// _SameError is whether got and want are both nil, or carry the same text and
// wrap the same sentinels, which is what a script and a caller can see.
//
// Revisions:
//   - 2026-10-29 00:40: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _SameError(
	got error,
	want error,
) bool {
	if got == nil || want == nil {
		return got == nil && want == nil
	}

	if got.Error() != want.Error() {
		return false
	}

	for _, sentinel := range SENTINELS {
		wraps := errors.Is(
			got,
			sentinel,
		)
		if wraps != errors.Is(
			want,
			sentinel,
		) {
			return false
		}
	}

	return true
}

// _Numbers are the numbers worth writing in width bytes: zero, one and minus
// one; the edges of the unsigned and the signed range, and one past each; one
// far past them all on either side; and random numbers that fill the width,
// read both unsigned and signed.
//
// A width below one is tried as one byte's numbers, since _Width refuses it
// before any number counts.
//
// Revisions:
//   - 2026-10-29 00:40: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Numbers(
	t *testing.T,
	random *rand.Rand,
	width int,
) []*big.Int {
	t.Helper()

	var (
		size = max(
			width,
			1,
		)
		bits  = uint(size * 8)
		top   = _Power(bits)
		half  = _Power(bits - 1)
		least = new(big.Int).Neg(half)
		far   = _Power(FAR)
		edges = []*big.Int{
			big.NewInt(0),
			big.NewInt(1),
			big.NewInt(-1),
			_Less(top),
			top,
			_Less(half),
			half,
			least,
			_Less(least),
			far,
			new(big.Int).Neg(far),
		}
		runs = _Runs(
			random,
			size,
		)
	)

	for _, data := range runs {
		signed, err := _BigDecodeSigned(
			WHO,
			data,
			common.BIG,
		)
		if err != nil {
			t.Fatal(err)
		}

		edges = append(
			edges,
			new(big.Int).SetBytes(data),
			signed,
		)
	}

	return edges
}

// _Runs are the runs of size bytes worth reading: all zero, all set, a sign bit
// alone at either end, every bit but a sign bit at either end, and random runs.
// Both ends, because the sign byte is first in big order and last in little.
//
// Revisions:
//   - 2026-10-29 00:40: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Runs(
	random *rand.Rand,
	size int,
) [][]byte {
	const (
		NONE = 0x00
		ALL  = 0xff
	)

	var (
		zero = bytes.Repeat(
			[]byte{
				NONE,
			},
			size,
		)
		full = bytes.Repeat(
			[]byte{
				ALL,
			},
			size,
		)
		first = slices.Clone(zero)
		last  = slices.Clone(zero)
		low   = slices.Clone(full)
		high  = slices.Clone(full)
		runs  = [][]byte{
			zero,
			full,
			first,
			last,
			low,
			high,
		}
	)

	if size > 0 {
		first[0] = 0x80
		last[size-1] = 0x80
		low[0] = ^byte(0x80)
		high[size-1] = ^byte(0x80)
	}

	for range ROUNDS {
		data := make(
			[]byte,
			size,
		)
		for idx := range data {
			data[idx] = byte(random.Uint())
		}

		runs = append(
			runs,
			data,
		)
	}

	return runs
}

// _Power is two to the power bits.
//
// Revisions:
//   - 2026-10-29 00:40: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Power(bits uint) *big.Int {
	return new(big.Int).Lsh(
		big.NewInt(1),
		bits,
	)
}

// _Less is number minus one, in a new big.Int.
//
// Revisions:
//   - 2026-10-29 00:40: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Less(number *big.Int) *big.Int {
	return new(big.Int).Sub(
		number,
		big.NewInt(1),
	)
}

// _BigEncode is Encode as it was before the word path, every width through
// big.Int. It is the reference the word path must match.
//
// Revisions:
//   - 2026-10-29 00:40: initial creation, a copy of Encode at 2026-10-08 21:08
//   - 2026-11-01 13:00: Encode takes no budget
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _BigEncode(
	who string,
	field *common.Field,
) ([]byte, error) {
	err := _BigWidth(
		who,
		field.Width,
	)
	if err != nil {
		return nil, err
	}

	number := field.Number.BigInt()

	if number.Sign() < 0 {
		return nil, fmt.Errorf(
			"%s got %s, which is negative: %w",
			who,
			number,
			unpack.ErrRange,
		)
	}

	if number.BitLen() > field.Width*8 {
		return nil, fmt.Errorf(
			"%s: %s does not fit %d bytes: %w",
			who,
			number,
			field.Width,
			unpack.ErrRange,
		)
	}

	return _BigOrdered(
		who,
		number.FillBytes(make(
			[]byte,
			field.Width,
		)),
		field.Order,
	)
}

// _BigEncodeSigned is EncodeSigned as it was before the word path. It is the
// reference the word path must match.
//
// Revisions:
//   - 2026-10-29 00:40: initial creation, a copy of EncodeSigned at
//     2026-10-08 21:08
//   - 2026-11-01 13:00: Encode takes no budget
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _BigEncodeSigned(
	who string,
	field *common.Field,
) ([]byte, error) {
	err := _BigWidth(
		who,
		field.Width,
	)
	if err != nil {
		return nil, err
	}

	var (
		number    = field.Number.BigInt()
		negative  = number.Sign() < 0
		magnitude = number
	)
	if negative {
		magnitude = new(big.Int).Not(number)
	}

	if magnitude.BitLen() >= field.Width*8 {
		return nil, fmt.Errorf(
			"%s: %s does not fit %d bytes: %w",
			who,
			number,
			field.Width,
			unpack.ErrRange,
		)
	}

	data := magnitude.FillBytes(make(
		[]byte,
		field.Width,
	))
	if negative {
		_BigFlip(data)
	}

	return _BigOrdered(
		who,
		data,
		field.Order,
	)
}

// _BigDecode is Decode as it was before the word path. It is the reference
// the word path must match.
//
// Revisions:
//   - 2026-10-29 00:40: initial creation, a copy of Decode at 2026-10-08 21:08
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _BigDecode(
	who string,
	data []byte,
	order string,
) (*big.Int, error) {
	ordered, err := _BigOrdered(
		who,
		data,
		order,
	)
	if err != nil {
		return nil, err
	}

	return new(big.Int).SetBytes(ordered), nil
}

// _BigDecodeSigned is DecodeSigned as it was before the word path. It is the
// reference the word path must match.
//
// Revisions:
//   - 2026-10-29 00:40: initial creation, a copy of DecodeSigned at
//     2026-10-08 21:08
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _BigDecodeSigned(
	who string,
	data []byte,
	order string,
) (*big.Int, error) {
	ordered, err := _BigOrdered(
		who,
		data,
		order,
	)
	if err != nil {
		return nil, err
	}

	negative := len(ordered) > 0 && ordered[0]&0x80 != 0
	if !negative {
		return new(big.Int).SetBytes(ordered), nil
	}

	flipped := slices.Clone(ordered)
	_BigFlip(flipped)

	return new(big.Int).Not(new(big.Int).SetBytes(flipped)), nil
}

// _BigWidth is _Width as it was before the word path, so a refused width is
// held to the same text.
//
// Revisions:
//   - 2026-10-29 00:40: initial creation, a copy of _Charge at 2026-10-08 21:08
//   - 2026-11-01 13:00: renamed _BigWidth and reserves nothing, no budget now
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _BigWidth(
	who string,
	width int,
) error {
	if width < 1 {
		return fmt.Errorf(
			"%s got a width of %d: %w",
			who,
			width,
			unpack.ErrRange,
		)
	}

	return nil
}

// _BigOrdered is _Ordered as it was before the word path.
//
// Revisions:
//   - 2026-10-29 00:40: initial creation, a copy of _Ordered at 2026-10-08 21:08
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _BigOrdered(
	who string,
	data []byte,
	order string,
) ([]byte, error) {
	switch order {
	case common.BIG:
		return data, nil

	case common.LITTLE:
		reversed := slices.Clone(data)
		slices.Reverse(reversed)

		return reversed, nil
	}

	return nil, fmt.Errorf(
		"%s got %q: %w",
		who,
		order,
		integer.ErrOrder,
	)
}

// _BigFlip is _Flip as it was before the word path.
//
// Revisions:
//   - 2026-10-29 00:40: initial creation, a copy of _Flip at 2026-10-08 21:08
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _BigFlip(data []byte) {
	for idx := range data {
		data[idx] = ^data[idx]
	}
}
