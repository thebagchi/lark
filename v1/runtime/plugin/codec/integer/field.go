package integer

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"slices"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// ErrOrder is returned for a byte order that is neither big nor little.
var (
	ErrOrder = errors.New("not a byte order")
)

// BIG and LITTLE are the byte orders a script may name, BIG when it names
// none.
const (
	BIG    = "big"
	LITTLE = "little"

	// BITS_PER_BYTE is how many bits each byte of a width holds, and SIGN_BIT
	// the bit of a two's complement number's first byte that marks it negative.
	BITS_PER_BYTE = 8
	SIGN_BIT      = 0x80

	// WORD is the bytes in a uint64: the widest field written and read as a
	// machine word, with no big.Int between the number and its bytes.
	WORD = 8
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
func Unpack(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (*Field, error) {
	var (
		number starlark.Int
		width  int
		order  = BIG
	)

	const NEEDED = 2

	err := starlark.UnpackPositionalArgs(
		fn.Name(),
		args,
		kwargs,
		NEEDED,
		&number,
		&width,
		&order,
	)
	if err != nil {
		return nil, err
	}

	return &Field{
		Number: number.BigInt(),
		Width:  width,
		Order:  order,
	}, nil
}

// Encode is field's number as unsigned bytes, as many as its width, in its
// order.
//
// A width of eight bytes or fewer is written as a machine word. Most fields are
// that narrow, and big.Int would cost them allocations and arithmetic.
//
// Returns unpack.ErrRange for a width below one, a negative number, or a
// number too wide for the width, ErrOrder for an order that is neither big nor
// little, and scheduler.ErrMemory for a width the run cannot afford.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as the body of codec's int2bytes
//   - 2026-10-08 21:08: lifted out of to_bytes, taking a byte order and charging
//     the width, so buf writes integers through it too
//   - 2026-10-29 00:40: writes a width of eight bytes or fewer through a uint64
func Encode(
	budget *scheduler.Budget,
	who string,
	field *Field,
) ([]byte, error) {
	err := _Charge(
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

	if field.Number.BitLen() > field.Width*BITS_PER_BYTE {
		return nil, _RefuseFit(
			who,
			field,
		)
	}

	if field.Width <= WORD {
		return _PutWord(
			who,
			field.Number.Uint64(),
			field.Width,
			field.Order,
		)
	}

	return _Ordered(
		who,
		field.Number.FillBytes(make(
			[]byte,
			field.Width,
		)),
		field.Order,
	)
}

// EncodeSigned is field's number in two's complement, as many bytes as its
// width, in its order.
//
// A negative number is written as its complement, -n-1, with every bit
// flipped. That is two's complement without building 2**(8*width), a number as
// large as the width a script chose.
//
// A width of eight bytes or fewer, with a number that fits an int64, is written
// as a machine word, as Encode writes it.
//
// Returns as Encode does, but for a negative number, which it writes.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-10-29 00:40: writes a width of eight bytes or fewer through a uint64
func EncodeSigned(
	budget *scheduler.Budget,
	who string,
	field *Field,
) ([]byte, error) {
	err := _Charge(
		budget,
		who,
		field.Width,
	)
	if err != nil {
		return nil, err
	}

	defer budget.Credit(int64(field.Width))

	if field.Width <= WORD && field.Number.IsInt64() {
		return _PutSigned(
			who,
			field,
		)
	}

	var (
		negative  = field.Number.Sign() < 0
		magnitude = field.Number
	)
	if negative {
		magnitude = new(big.Int).Not(field.Number)
	}

	// The first bit is the sign's, so the number has one bit fewer than the
	// width holds.
	if magnitude.BitLen() >= field.Width*BITS_PER_BYTE {
		return nil, _RefuseFit(
			who,
			field,
		)
	}

	data := magnitude.FillBytes(make(
		[]byte,
		field.Width,
	))
	if negative {
		_Flip(data)
	}

	return _Ordered(
		who,
		data,
		field.Order,
	)
}

// Decode reads data as an unsigned integer, its bytes in order.
//
// Eight bytes or fewer are read as a machine word, as Encode writes them.
//
// Returns ErrOrder for an order that is neither big nor little.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as the body of codec's bytes2int
//   - 2026-10-08 21:08: lifted out of from_bytes, taking a byte order, so buf
//     reads integers through it too
//   - 2026-10-29 00:40: reads eight bytes or fewer through a uint64
func Decode(
	who string,
	data []byte,
	order string,
) (*big.Int, error) {
	if len(data) <= WORD {
		value, err := _ReadWord(
			who,
			data,
			order,
		)
		if err != nil {
			return nil, err
		}

		return new(big.Int).SetUint64(value), nil
	}

	ordered, err := _Ordered(
		who,
		data,
		order,
	)
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
// Eight bytes or fewer are read as a machine word instead. The field's sign
// bit is shifted to the top of the word and back, and an arithmetic shift
// copies it into every bit above the field, which is the int64 of the number.
//
// Returns ErrOrder for an order that is neither big nor little.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-10-29 00:40: reads eight bytes or fewer through a uint64
func DecodeSigned(
	who string,
	data []byte,
	order string,
) (*big.Int, error) {
	if len(data) <= WORD {
		value, err := _ReadWord(
			who,
			data,
			order,
		)
		if err != nil {
			return nil, err
		}

		spare := (WORD - len(data)) * BITS_PER_BYTE

		return big.NewInt(int64(value<<spare) >> spare), nil
	}

	ordered, err := _Ordered(
		who,
		data,
		order,
	)
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
func _Charge(
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

// _Ordered is data as big-endian bytes, read in order: as it is when the order
// is big, and reversed in a copy when it is little.
//
// Reversing is its own inverse, so one function serves writing and reading.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-10-29 00:40: refuses through _RefuseOrder, which the word path shares
func _Ordered(
	who string,
	data []byte,
	order string,
) ([]byte, error) {
	switch order {
	case BIG:
		return data, nil

	case LITTLE:
		reversed := slices.Clone(data)
		slices.Reverse(reversed)

		return reversed, nil
	}

	return nil, _RefuseOrder(
		who,
		order,
	)
}

// _PutWord is value's last width bytes, in order. The caller has checked that
// value fits width bytes, and that width is one to WORD.
//
// The word is laid out in an array on the stack, so the bytes returned are the
// one allocation.
//
// Revisions:
//   - 2026-10-29 00:40: initial creation
func _PutWord(
	who string,
	value uint64,
	width int,
	order string,
) ([]byte, error) {
	var (
		word [WORD]byte
		used []byte
	)

	switch order {
	case BIG:
		binary.BigEndian.PutUint64(
			word[:],
			value,
		)
		used = word[WORD-width:]

	case LITTLE:
		binary.LittleEndian.PutUint64(
			word[:],
			value,
		)
		used = word[:width]

	default:
		return nil, _RefuseOrder(
			who,
			order,
		)
	}

	data := make(
		[]byte,
		width,
	)
	copy(
		data,
		used,
	)

	return data, nil
}

// _PutSigned is field's number in two's complement, as EncodeSigned writes it,
// for a width of one to WORD and a number that fits an int64.
//
// A number fits the width when every bit above the width's sign bit matches
// that sign bit: all clear, or all set. The low bytes of its uint64 are then
// its two's complement.
//
// Revisions:
//   - 2026-10-29 00:40: initial creation
func _PutSigned(
	who string,
	field *Field,
) ([]byte, error) {
	var (
		value = field.Number.Int64()
		above = value >> (field.Width*BITS_PER_BYTE - 1)
	)

	if above != 0 && above != -1 {
		return nil, _RefuseFit(
			who,
			field,
		)
	}

	return _PutWord(
		who,
		uint64(value),
		field.Width,
		field.Order,
	)
}

// _ReadWord is data, at most WORD bytes in order, as the low bytes of a uint64.
//
// The bytes are copied into an array on the stack at the end that the order
// reads as low, so the read allocates nothing.
//
// Revisions:
//   - 2026-10-29 00:40: initial creation
func _ReadWord(
	who string,
	data []byte,
	order string,
) (uint64, error) {
	var word [WORD]byte

	switch order {
	case BIG:
		copy(
			word[WORD-len(data):],
			data,
		)

		return binary.BigEndian.Uint64(word[:]), nil

	case LITTLE:
		copy(
			word[:],
			data,
		)

		return binary.LittleEndian.Uint64(word[:]), nil
	}

	return 0, _RefuseOrder(
		who,
		order,
	)
}

// _RefuseFit is the refusal of a number too wide for its field. Both paths of
// Encode and EncodeSigned refuse with it, so a script sees one text whichever
// path its width takes.
//
// Revisions:
//   - 2026-10-29 00:40: initial creation, out of Encode and EncodeSigned
func _RefuseFit(
	who string,
	field *Field,
) error {
	return fmt.Errorf(
		"%s: %s does not fit %d bytes: %w",
		who,
		field.Number,
		field.Width,
		unpack.ErrRange,
	)
}

// _RefuseOrder is the refusal of an order that is neither big nor little. Both
// paths refuse with it, so a script sees one text whichever path its width
// takes.
//
// Revisions:
//   - 2026-10-29 00:40: initial creation, out of _Ordered
func _RefuseOrder(
	who string,
	order string,
) error {
	return fmt.Errorf(
		"%s got %q: %w",
		who,
		order,
		ErrOrder,
	)
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
