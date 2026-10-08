package integer

import (
	"errors"
	"fmt"
	"math/big"
	"slices"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// ERR_ORDER is returned for a byte order that is neither big nor little.
var ERR_ORDER = errors.New("not a byte order")

const (
	// BIG and LITTLE are the byte orders a script may name, BIG when it names
	// none.
	BIG    = "big"
	LITTLE = "little"

	// BITS_PER_BYTE is how many bits each byte of a width holds, and SIGN_BIT
	// the bit of a two's complement number's first byte that marks it negative.
	BITS_PER_BYTE = 8
	SIGN_BIT      = 0x80
)

// Field is an integer, how many bytes it is written in, and their order.
type Field struct {
	Number *big.Int
	Width  int
	Order  string
}

// Unpack reads a builtin's arguments as a Field: an integer, a width, and a
// byte order, big unless a third argument names one.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
func Unpack(fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) (*Field, error) {
	var (
		number starlark.Int
		width  int
		order  = BIG
	)

	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 2, &number, &width, &order)
	if err != nil {
		return nil, err
	}

	return &Field{Number: number.BigInt(), Width: width, Order: order}, nil
}

// Encode is field's number as unsigned bytes, as many as its width, in its
// order.
//
// Returns unpack.ERR_RANGE for a width below one, a negative number, or a
// number too wide for the width, ERR_ORDER for an order that is neither big nor
// little, and scheduler.ERR_MEMORY for a width the run cannot afford.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as the body of codec's int2bytes
//   - 2026-10-08 21:08: lifted out of to_bytes, taking a byte order and charging
//     the width, so buf writes integers through it too
func Encode(budget *scheduler.Budget, who string, field *Field) ([]byte, error) {
	err := _Charge(budget, who, field.Width)
	if err != nil {
		return nil, err
	}

	defer budget.Credit(int64(field.Width))

	if field.Number.Sign() < 0 {
		return nil, fmt.Errorf(
			"%s got %s, which is negative: %w",
			who,
			field.Number,
			unpack.ERR_RANGE,
		)
	}

	if field.Number.BitLen() > field.Width*BITS_PER_BYTE {
		return nil, fmt.Errorf("%s: %s does not fit %d bytes: %w",
			who, field.Number, field.Width, unpack.ERR_RANGE)
	}

	return _Ordered(who, field.Number.FillBytes(make([]byte, field.Width)), field.Order)
}

// EncodeSigned is field's number in two's complement, as many bytes as its
// width, in its order.
//
// A negative number is written as its complement, -n-1, with every bit
// flipped. That is two's complement without building 2**(8*width), a number as
// large as the width a script chose.
//
// Returns as Encode does, but for a negative number, which it writes.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
func EncodeSigned(budget *scheduler.Budget, who string, field *Field) ([]byte, error) {
	err := _Charge(budget, who, field.Width)
	if err != nil {
		return nil, err
	}

	defer budget.Credit(int64(field.Width))

	negative := field.Number.Sign() < 0

	magnitude := field.Number
	if negative {
		magnitude = new(big.Int).Not(field.Number)
	}

	// The first bit is the sign's, so the number has one bit fewer than the
	// width holds.
	if magnitude.BitLen() >= field.Width*BITS_PER_BYTE {
		return nil, fmt.Errorf("%s: %s does not fit %d bytes: %w",
			who, field.Number, field.Width, unpack.ERR_RANGE)
	}

	data := magnitude.FillBytes(make([]byte, field.Width))
	if negative {
		_Flip(data)
	}

	return _Ordered(who, data, field.Order)
}

// Decode reads data as an unsigned integer, its bytes in order.
//
// Returns ERR_ORDER for an order that is neither big nor little.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as the body of codec's bytes2int
//   - 2026-10-08 21:08: lifted out of from_bytes, taking a byte order, so buf
//     reads integers through it too
func Decode(who string, data []byte, order string) (*big.Int, error) {
	ordered, err := _Ordered(who, data, order)
	if err != nil {
		return nil, err
	}

	return new(big.Int).SetBytes(ordered), nil
}

// DecodeSigned reads data as an integer in two's complement, its bytes in
// order.
//
// A negative number's bits are flipped back, which gives its complement, -n-1,
// and the complement of that is the number.
//
// Returns ERR_ORDER for an order that is neither big nor little.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
func DecodeSigned(who string, data []byte, order string) (*big.Int, error) {
	ordered, err := _Ordered(who, data, order)
	if err != nil {
		return nil, err
	}

	negative := len(ordered) > 0 && ordered[0]&SIGN_BIT != 0
	if !negative {
		return new(big.Int).SetBytes(ordered), nil
	}

	flipped := slices.Clone(ordered)
	_Flip(flipped)

	return new(big.Int).Not(new(big.Int).SetBytes(flipped)), nil
}

// _Charge reserves width bytes of the run's memory for an integer about to be
// written, or refuses a width below one or one the run cannot afford. The caller
// credits the bytes back once they are made.
//
// A script names the width, so one asking for more than the run may hold is
// refused here rather than handed to the allocator, whose refusal ends the
// process rather than the script.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
func _Charge(budget *scheduler.Budget, who string, width int) error {
	if width < 1 {
		return fmt.Errorf("%s got a width of %d: %w", who, width, unpack.ERR_RANGE)
	}

	err := budget.Charge(int64(width))
	if err != nil {
		return fmt.Errorf("%s: %w", who, err)
	}

	return nil
}

// _Ordered is data as big-endian bytes, read in order: as it is when the order
// is big, and reversed in a copy when it is little.
//
// Reversing is its own inverse, so one function serves writing and reading.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
func _Ordered(who string, data []byte, order string) ([]byte, error) {
	switch order {
	case BIG:
		return data, nil

	case LITTLE:
		reversed := slices.Clone(data)
		slices.Reverse(reversed)

		return reversed, nil
	}

	return nil, fmt.Errorf("%s got %q: %w", who, order, ERR_ORDER)
}

// _Flip inverts every bit of data, in place.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
func _Flip(data []byte) {
	for idx := range data {
		data[idx] = ^data[idx]
	}
}
