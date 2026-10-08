package buf

import (
	"fmt"
	"maps"
	"math/big"
	"slices"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/codec/integer"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// Reader is bytes a script takes apart a piece at a time, from the front.
//
// Each read moves it past what was read. A read that is refused does not move
// it, so a script that is told no still knows where it is.
type Reader struct {
	data   []byte
	at     int
	frozen bool
}

// _Shape reads a reading method's arguments as the width and the byte order of
// the integer to read.
type _Shape func(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (int, string, error)

// _Decoder is integer's Decode or DecodeSigned.
type _Decoder func(who string, data []byte, order string) (*big.Int, error)

// String is what print shows: how many bytes are left, not the bytes, which
// may be many.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
func (r *Reader) String() string {
	return fmt.Sprintf("<%s length %d>", READER_TYPE, r.Len())
}

// Type names this value's type, as a script's type() sees it.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
func (r *Reader) Type() string {
	return READER_TYPE
}

// Freeze stops every later read, which a spawn handing the reader to another
// thread needs: a read moves the reader, so two threads reading would race.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
func (r *Reader) Freeze() {
	r.frozen = true
}

// Truth reports a reader as true while any bytes are left.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
func (r *Reader) Truth() starlark.Bool {
	return r.Len() > 0
}

// Hash refuses: a reader changes as it is read, so it cannot be a key.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
func (r *Reader) Hash() (uint32, error) {
	return 0, fmt.Errorf("%s is unhashable", READER_TYPE)
}

// Len is how many bytes are left, which len() reads.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
func (r *Reader) Len() int {
	return len(r.data) - r.at
}

// Index is the byte idx places ahead, as bytes of length one, without moving
// the reader: r[0] is the next byte.
//
// Here because len() reads a length only from a value that can be indexed. The
// interpreter checks idx against Len before it asks.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
func (r *Reader) Index(idx int) starlark.Value {
	at := r.at + idx

	return starlark.Bytes(r.data[at : at+1])
}

// Attr is the method name calls, bound to this reader, or nil when there is no
// such method.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
func (r *Reader) Attr(name string) (starlark.Value, error) {
	method, ok := READER_METHODS[name]
	if !ok {
		return nil, nil
	}

	return method.BindReceiver(r), nil
}

// AttrNames is the methods, as dir() lists them.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
func (r *Reader) AttrNames() []string {
	return slices.Sorted(maps.Keys(READER_METHODS))
}

// _ReadBytes is the next count bytes, as bytes.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation, as read
//   - 2026-10-08 21:37: read_bytes, beside read_string
func _ReadBytes(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	data, err := _Take(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	return starlark.Bytes(data), nil
}

// _ReadString is the next count bytes, as a str.
//
// The bytes are not checked for UTF-8, as a Starlark str is not: count may end
// inside a character, and the str holds what the bytes were.
//
// Revisions:
//   - 2026-10-08 21:37: initial creation
func _ReadString(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	data, err := _Take(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	return starlark.String(data), nil
}

// _Reading is a method that reads an integer as decode reads it - unsigned for
// uint, two's complement for int - in the width and order shape reads from its
// arguments: one the script names for read_uint and read_int, and a fixed one
// for each of the rest.
//
// Built from the two rather than written ten times, because the methods differ
// in those and in nothing else. The reader moves only once the bytes are read
// as a number, so an order that is refused leaves it where it was.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-10-08 21:37: takes what reads the width and order, so a fixed width is
//     a method too
func _Reading(name string, shape _Shape, decode _Decoder) *starlark.Builtin {
	return starlark.NewBuiltin(name, func(
		thread *starlark.Thread,
		fn *starlark.Builtin,
		args starlark.Tuple,
		kwargs []starlark.Tuple,
	) (starlark.Value, error) {
		reader, err := _Movable(fn)
		if err != nil {
			return nil, err
		}

		width, order, err := shape(fn, args, kwargs)
		if err != nil {
			return nil, err
		}

		data, err := reader._Next(fn.Name(), width)
		if err != nil {
			return nil, err
		}

		number, err := decode(fn.Name(), data, order)
		if err != nil {
			return nil, err
		}

		reader.at += width

		return starlark.MakeBigInt(number), nil
	})
}

// _Width reads the arguments of read_uint and read_int: a width of at least
// one, and a byte order, big unless named.
//
// Revisions:
//   - 2026-10-08 21:37: initial creation, from _Reading, which read them itself
func _Width(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (int, string, error) {
	var (
		width int
		order = integer.BIG
	)

	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 1, &width, &order)
	if err != nil {
		return 0, "", err
	}

	if width < 1 {
		return 0, "", fmt.Errorf(
			"%s got a width of %d: %w",
			fn.Name(),
			width,
			unpack.ERR_RANGE,
		)
	}

	return width, order, nil
}

// _Order reads the arguments of a method whose width is fixed: only a byte
// order, big unless named.
//
// Revisions:
//   - 2026-10-08 21:37: initial creation
func _Order(width int) _Shape {
	return func(
		fn *starlark.Builtin,
		args starlark.Tuple,
		kwargs []starlark.Tuple,
	) (int, string, error) {
		order := integer.BIG

		err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 0, &order)
		if err != nil {
			return 0, "", err
		}

		return width, order, nil
	}
}

// _Take is the next count bytes, count read from the arguments, moving the
// reader past them.
//
// Revisions:
//   - 2026-10-08 21:37: initial creation, from read, so read_string shares it
func _Take(fn *starlark.Builtin, args starlark.Tuple, kwargs []starlark.Tuple) ([]byte, error) {
	reader, err := _Movable(fn)
	if err != nil {
		return nil, err
	}

	var count int

	err = starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 1, &count)
	if err != nil {
		return nil, err
	}

	data, err := reader._Next(fn.Name(), count)
	if err != nil {
		return nil, err
	}

	reader.at += count

	return data, nil
}

// _Movable is the reader fn is bound to, or ERR_FROZEN once a spawn has frozen
// it.
//
// The receiver is asserted rather than asked: Attr is the one place that binds
// these methods, and it binds each to the reader it was read from.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
func _Movable(fn *starlark.Builtin) (*Reader, error) {
	reader := fn.Receiver().(*Reader)
	if reader.frozen {
		return nil, fmt.Errorf("%s: %w", fn.Name(), ERR_FROZEN)
	}

	return reader, nil
}

// _Next is the next count bytes, without moving past them: refused with
// unpack.ERR_RANGE for a negative count, and ERR_SHORT for more than are left.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
func (r *Reader) _Next(who string, count int) ([]byte, error) {
	if count < 0 {
		return nil, fmt.Errorf(
			"%s got %d, which is negative: %w",
			who,
			count,
			unpack.ERR_RANGE,
		)
	}

	left := r.Len()
	if count > left {
		return nil, fmt.Errorf(
			"%s wants %d bytes with %d left: %w",
			who,
			count,
			left,
			ERR_SHORT,
		)
	}

	return r.data[r.at : r.at+count], nil
}
