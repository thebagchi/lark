// These tests hold the word path to the big.Int path that it stands beside.
// A field of eight bytes or fewer is written and read through a uint64, and
// every wider field through big.Int. Two paths are two contracts unless a test
// says they are one, and the contract includes what each path refuses. So each
// test here runs the package against a copy of the big.Int code, as it was
// before the word path, on the edges of every width, one past each edge, and
// random values, and wants the same bytes, the same number, and the same error
// text.
package integer_test

import (
	"bytes"
	"errors"
	"fmt"
	"math/big"
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/plugin/codec/integer"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

const (
	// ROUNDS is how many random values each width and order gets, and SEED
	// fixes them, so a failure comes back on the next run.
	ROUNDS = 64
	SEED   = 16

	// BOGUS is an order that is neither big nor little, which both paths
	// must refuse alike.
	BOGUS = "middle"

	// FAR is a bit count past every width under test, for a number that no
	// width here holds.
	FAR = 100
)

var (
	// ORDERS are the orders each width is tried in: the two a script may name,
	// and one it may not.
	ORDERS = []string{
		integer.BIG,
		integer.LITTLE,
		BOGUS,
	}

	// ENCODERS pair each encoder with the big.Int code it replaced.
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
	SENTINELS = []error{
		unpack.ErrRange,
		integer.ErrOrder,
		scheduler.ErrMemory,
	}
)

// _Encoder is the shape that Encode and EncodeSigned share.
type _Encoder func(
	*scheduler.Budget,
	string,
	*integer.Field,
) ([]byte, error)

// _Decoder is the shape that Decode and DecodeSigned share.
type _Decoder func(
	string,
	[]byte,
	string,
) (*big.Int, error)

// _Encoders is an encoder of the package, which has the word path, and the
// big.Int code it must match.
type _Encoders struct {
	word _Encoder
	big  _Encoder
}

// _Decoders is a decoder of the package, which has the word path, and the
// big.Int code it must match.
type _Decoders struct {
	word _Decoder
	big  _Decoder
}

// TestInteger_EncodeWordMatchesBig writes each number with Encode and
// EncodeSigned, and with the big.Int code they replaced, and wants the same
// bytes or the same refusal.
//
// The widths run from below one, which _Charge refuses, through every width
// the word path takes, to the first that it leaves to big.Int.
//
// Revisions:
//   - 2026-10-29 00:40: initial creation
func TestInteger_EncodeWordMatchesBig(t *testing.T) {
	var (
		budget = scheduler.NewBudget(scheduler.CEILING)
		random = rand.New(rand.NewPCG(
			SEED,
			SEED,
		))
	)

	for width := -1; width <= integer.WORD+1; width++ {
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
						field := &integer.Field{
							Number: number,
							Width:  width,
							Order:  order,
						}

						for _, pair := range ENCODERS {
							_SameBytes(
								t,
								budget,
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
func TestInteger_DecodeWordMatchesBig(t *testing.T) {
	random := rand.New(rand.NewPCG(
		SEED,
		SEED,
	))

	for size := 0; size <= integer.WORD+1; size++ {
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
func _SameBytes(
	t *testing.T,
	budget *scheduler.Budget,
	field *integer.Field,
	pair *_Encoders,
) {
	t.Helper()

	var (
		got, err = pair.word(
			budget,
			WHO,
			field,
		)
		expected, refusal = pair.big(
			budget,
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
		same = (got == nil) == (expected == nil) &&
			(got == nil || got.Cmp(expected) == 0)
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
// A width below one is tried as one byte's numbers, since _Charge refuses it
// before any number counts.
//
// Revisions:
//   - 2026-10-29 00:40: initial creation
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
		bits  = uint(size * integer.BITS_PER_BYTE)
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
			integer.BIG,
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
		first[0] = integer.SIGN_BIT
		last[size-1] = integer.SIGN_BIT
		low[0] = ^byte(integer.SIGN_BIT)
		high[size-1] = ^byte(integer.SIGN_BIT)
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
func _BigEncode(
	budget *scheduler.Budget,
	who string,
	field *integer.Field,
) ([]byte, error) {
	err := _BigCharge(
		budget,
		who,
		field.Width,
	)
	if err != nil {
		return nil, err
	}

	defer budget.Credit(int64(field.Width))

	if field.Number.Sign() < 0 {
		return nil, fmt.Errorf(
			"%s got %s, which is negative: %w",
			who,
			field.Number,
			unpack.ErrRange,
		)
	}

	if field.Number.BitLen() > field.Width*integer.BITS_PER_BYTE {
		return nil, fmt.Errorf(
			"%s: %s does not fit %d bytes: %w",
			who,
			field.Number,
			field.Width,
			unpack.ErrRange,
		)
	}

	return _BigOrdered(
		who,
		field.Number.FillBytes(make(
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
func _BigEncodeSigned(
	budget *scheduler.Budget,
	who string,
	field *integer.Field,
) ([]byte, error) {
	err := _BigCharge(
		budget,
		who,
		field.Width,
	)
	if err != nil {
		return nil, err
	}

	defer budget.Credit(int64(field.Width))

	var (
		negative  = field.Number.Sign() < 0
		magnitude = field.Number
	)
	if negative {
		magnitude = new(big.Int).Not(field.Number)
	}

	if magnitude.BitLen() >= field.Width*integer.BITS_PER_BYTE {
		return nil, fmt.Errorf(
			"%s: %s does not fit %d bytes: %w",
			who,
			field.Number,
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

	negative := len(ordered) > 0 && ordered[0]&integer.SIGN_BIT != 0
	if !negative {
		return new(big.Int).SetBytes(ordered), nil
	}

	flipped := slices.Clone(ordered)
	_BigFlip(flipped)

	return new(big.Int).Not(new(big.Int).SetBytes(flipped)), nil
}

// _BigCharge is _Charge as it was before the word path, so a refused width is
// held to the same text.
//
// Revisions:
//   - 2026-10-29 00:40: initial creation, a copy of _Charge at 2026-10-08 21:08
func _BigCharge(
	budget *scheduler.Budget,
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

	err := budget.Charge(int64(width))
	if err != nil {
		return fmt.Errorf(
			"%s: %w",
			who,
			err,
		)
	}

	return nil
}

// _BigOrdered is _Ordered as it was before the word path.
//
// Revisions:
//   - 2026-10-29 00:40: initial creation, a copy of _Ordered at 2026-10-08 21:08
func _BigOrdered(
	who string,
	data []byte,
	order string,
) ([]byte, error) {
	switch order {
	case integer.BIG:
		return data, nil

	case integer.LITTLE:
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
func _BigFlip(data []byte) {
	for idx := range data {
		data[idx] = ^data[idx]
	}
}
