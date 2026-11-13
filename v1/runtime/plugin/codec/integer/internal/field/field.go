package field

import (
	"encoding/binary"
	"errors"
	"fmt"
	"math/big"
	"slices"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/memwatch"
	shared "github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// ErrOrder is returned for a byte order that is neither big nor little.
//
// Revisions:
//   - 2026-10-08 22:45: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
var (
	ErrOrder = errors.New("not a byte order")
)

// BITS_PER_BYTE and the other constants of the conversions.
//
// Revisions:
//   - 2026-09-21 23:56: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:16: BIG and LITTLE moved to package common (Q124, C91w8b)
const (
	// BITS_PER_BYTE is how many bits each byte of a width holds, and SIGN_BIT
	// the bit of a two's complement number's first byte that marks it negative.
	//
	// Revisions:
	//   - 2026-09-21 23:56: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	BITS_PER_BYTE = 8
	SIGN_BIT      = 0x80

	// WORD is the bytes in a uint64: the widest field written and read as a
	// machine word, with no big.Int between the number and its bytes.
	//
	// Revisions:
	//   - 2026-10-29 06:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	WORD = 8

	// ANONYMOUS is the name an error gives a builtin that is nil, which only a
	// Go caller can pass. It is the text core and unpack give the same case.
	//
	// Revisions:
	//   - 2026-11-04 19:51: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	ANONYMOUS = "builtin"
)

// Unpack reads a builtin's arguments as a common.Field: an integer, a width, and a
// byte order, big unless a third argument names one.
//
// A nil fn is named ANONYMOUS in an error.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-11-04 19:30: a nil builtin is named ANONYMOUS, not dereferenced (C87)
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-07 19:17: moved to integer/internal/field; uses package common (C91w8b)
func Unpack(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (*common.Field, error) {
	var (
		number starlark.Int
		width  int
		order  = common.BIG
		who    = ANONYMOUS
	)

	const NEEDED = 2

	if fn != nil {
		who = shared.Called(fn)
	}

	err := starlark.UnpackPositionalArgs(
		who,
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

	return &common.Field{
		Number: number,
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
// little.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as the body of codec's int2bytes
//   - 2026-10-08 21:08: lifted out of to_bytes, taking a byte order and charging
//     the width, so buf writes integers through it too
//   - 2026-10-29 00:40: writes a width of eight bytes or fewer through a uint64
//   - 2026-11-01 13:00: takes no budget; the width is checked, not charged
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-04 00:50: reads a width of eight bytes or fewer from the Starlark
//     int, with no big.Int (C38c)
//   - 2026-11-04 19:30: a nil field is unpack.ErrRange (C87)
//   - 2026-11-07 19:17: moved to integer/internal/field; uses package common (C91w8b)
func Encode(
	who string,
	field *common.Field,
) ([]byte, error) {
	if field == nil {
		return nil, _NoField(who)
	}

	err := _Width(
		who,
		field.Width,
	)
	if err != nil {
		return nil, err
	}

	if field.Number.Sign() < 0 {
		return nil, fmt.Errorf(
			"%s got %s, which is negative: %w",
			who,
			unpack.ShortText(field.Number.String()),
			unpack.ErrRange,
		)
	}

	if field.Width <= WORD {
		return _EncodeWord(
			who,
			field,
		)
	}

	number := field.Number.BigInt()
	if number.BitLen() > field.Width*BITS_PER_BYTE {
		return nil, _RefuseFit(
			who,
			field,
		)
	}

	return _Ordered(
		who,
		number.FillBytes(make(
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
//   - 2026-11-01 13:00: takes no budget; the width is checked, not charged
//   - 2026-11-04 00:50: reads the Starlark int, and makes a big.Int only past a
//     word (C38c)
//   - 2026-11-04 19:30: a nil field is unpack.ErrRange (C87)
//   - 2026-11-07 19:17: moved to integer/internal/field; uses package common (C91w8b)
func EncodeSigned(
	who string,
	field *common.Field,
) ([]byte, error) {
	if field == nil {
		return nil, _NoField(who)
	}

	err := _Width(
		who,
		field.Width,
	)
	if err != nil {
		return nil, err
	}

	value, fits := field.Number.Int64()
	if field.Width <= WORD && fits {
		return _PutSigned(
			who,
			field,
			value,
		)
	}

	var (
		number    = field.Number.BigInt()
		negative  = number.Sign() < 0
		magnitude = number
	)
	if negative {
		magnitude = new(big.Int).Not(number)
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
// Eight bytes or fewer are read as a machine word, as Encode writes them, and
// become a Starlark int with no big.Int between.
//
// Returns ErrOrder for an order that is neither big nor little.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as the body of codec's bytes2int
//   - 2026-10-08 21:08: lifted out of from_bytes, taking a byte order, so buf
//     reads integers through it too
//   - 2026-10-29 00:40: reads eight bytes or fewer through a uint64
//   - 2026-11-04 00:40: returns a Starlark int, so a word makes no big.Int (C38c)
func Decode(
	who string,
	data []byte,
	order string,
) (starlark.Int, error) {
	if len(data) <= WORD {
		value, err := _ReadWord(
			who,
			data,
			order,
		)
		if err != nil {
			return starlark.Int{}, err
		}

		return starlark.MakeUint64(value), nil
	}

	ordered, err := _Ordered(
		who,
		data,
		order,
	)
	if err != nil {
		return starlark.Int{}, err
	}

	return starlark.MakeBigInt(new(big.Int).SetBytes(ordered)), nil
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
//   - 2026-11-04 00:40: returns a Starlark int, so a word makes no big.Int (C38c)
func DecodeSigned(
	who string,
	data []byte,
	order string,
) (starlark.Int, error) {
	if len(data) <= WORD {
		value, err := _ReadWord(
			who,
			data,
			order,
		)
		if err != nil {
			return starlark.Int{}, err
		}

		spare := (WORD - len(data)) * BITS_PER_BYTE

		return starlark.MakeInt64(int64(value<<spare) >> spare), nil
	}

	ordered, err := _Ordered(
		who,
		data,
		order,
	)
	if err != nil {
		return starlark.Int{}, err
	}

	negative := len(ordered) > 0 && ordered[0]&SIGN_BIT != 0
	if !negative {
		return starlark.MakeBigInt(new(big.Int).SetBytes(ordered)), nil
	}

	flipped := slices.Clone(ordered)
	_Flip(flipped)

	return starlark.MakeBigInt(new(big.Int).Not(new(big.Int).SetBytes(flipped))), nil
}

// _NoField is the refusal for a nil Field: no number and no width, the
// argument a width of zero also lacks.
//
// Revisions:
//   - 2026-11-04 19:30: initial creation (C87)
func _NoField(who string) error {
	return fmt.Errorf(
		"%s: no field to write: %w",
		who,
		unpack.ErrRange,
	)
}

// _Width refuses a width below one, for an integer about to be written, and a
// width that does not fit the free memory (memwatch.ErrRoom). Every writer of
// a script-given width ends here, so to_bytes, write_uint and write_int are
// guarded in one place, before the buffer is made.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation, as _Charge
//   - 2026-11-01 13:00: renamed _Width and reserves nothing, the budget being
//     gone
//   - 2026-11-03 00:35: refuses a width that does not fit the free memory (C46)
//   - 2026-11-04 21:49: refuses a width below one through common.Width (C59)
//   - 2026-11-07 19:17: moved to integer/internal/field; uses package common (C91w8b)
//   - 2026-11-10 17:15: uses ErrRoom, formerly ErrNoRoom (C121, Q349)
func _Width(
	who string,
	width int,
) error {
	err := shared.Width(
		who,
		width,
	)
	if err != nil {
		return err
	}

	return memwatch.Room(
		who,
		uint64(width),
	)
}

// _Ordered is data as big-endian bytes, read in order: as it is when the order
// is big, and reversed in a copy when it is little.
//
// Reversing is its own inverse, so one function serves writing and reading.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-10-29 00:40: refuses through _RefuseOrder, which the word path shares
//   - 2026-11-07 19:17: moved to integer/internal/field; uses package common (C91w8b)
func _Ordered(
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

	return nil, shared.Refused(
		who,
		order,
		ErrOrder,
	)
}

// _EncodeWord is field's number, not negative, as unsigned bytes, for a width
// of one to WORD. A number that has a bit above the width, or does not fit a
// uint64, is refused as _RefuseFit refuses it.
//
// Revisions:
//   - 2026-11-04 00:50: initial creation, out of Encode (C38c)
//   - 2026-11-07 19:17: moved to integer/internal/field; uses package common (C91w8b)
func _EncodeWord(
	who string,
	field *common.Field,
) ([]byte, error) {
	value, fits := field.Number.Uint64()
	if !fits || value>>(field.Width*BITS_PER_BYTE) != 0 {
		return nil, _RefuseFit(
			who,
			field,
		)
	}

	return _PutWord(
		who,
		value,
		field.Width,
		field.Order,
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
//   - 2026-11-07 19:17: moved to integer/internal/field; uses package common (C91w8b)
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
	case common.BIG:
		binary.BigEndian.PutUint64(
			word[:],
			value,
		)
		used = word[WORD-width:]

	case common.LITTLE:
		binary.LittleEndian.PutUint64(
			word[:],
			value,
		)
		used = word[:width]

	default:
		return nil, shared.Refused(
			who,
			order,
			ErrOrder,
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

// _PutSigned is field's number, which is value, in two's complement, as
// EncodeSigned writes it, for a width of one to WORD and a number that fits an
// int64.
//
// A number fits the width when every bit above the width's sign bit matches
// that sign bit: all clear, or all set. The low bytes of its uint64 are then
// its two's complement.
//
// Revisions:
//   - 2026-10-29 00:40: initial creation
//   - 2026-11-04 00:50: takes the number as the int64 the caller read (C38c)
//   - 2026-11-07 19:17: moved to integer/internal/field; uses package common (C91w8b)
func _PutSigned(
	who string,
	field *common.Field,
	value int64,
) ([]byte, error) {
	above := value >> (field.Width*BITS_PER_BYTE - 1)
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
//   - 2026-11-07 19:17: moved to integer/internal/field; uses package common (C91w8b)
func _ReadWord(
	who string,
	data []byte,
	order string,
) (uint64, error) {
	var word [WORD]byte

	switch order {
	case common.BIG:
		copy(
			word[WORD-len(data):],
			data,
		)

		return binary.BigEndian.Uint64(word[:]), nil

	case common.LITTLE:
		copy(
			word[:],
			data,
		)

		return binary.LittleEndian.Uint64(word[:]), nil
	}

	return 0, shared.Refused(
		who,
		order,
		ErrOrder,
	)
}

// _RefuseFit is the refusal of a number too wide for its field. Both paths of
// Encode and EncodeSigned refuse with it, so a script sees one text whichever
// path its width takes.
//
// Revisions:
//   - 2026-10-29 00:40: initial creation, out of Encode and EncodeSigned
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-07 19:17: moved to integer/internal/field; uses package common (C91w8b)
func _RefuseFit(
	who string,
	field *common.Field,
) error {
	return fmt.Errorf(
		"%s: %s does not fit %d bytes: %w",
		who,
		unpack.ShortText(field.Number.String()),
		field.Width,
		unpack.ErrRange,
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
