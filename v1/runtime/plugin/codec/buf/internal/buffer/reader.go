package buffer

import (
	"fmt"
	"maps"
	"slices"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	shared "github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// Reader is bytes a script takes apart a piece at a time, from the front.
//
// Each read moves it past what was read. A read that is refused does not move
// it, so a script that is told no still knows where it is.
//
// Revisions:
//   - 2026-10-08 22:45: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
type Reader struct {
	data   []byte
	at     int
	frozen bool
}

// _Shape reads a reading method's arguments as the width and the byte order of
// the integer to read.
//
// Revisions:
//   - 2026-10-08 22:45: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
type _Shape func(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (int, string, error)

// _Decoder is integer's Decode or DecodeSigned.
//
// Revisions:
//   - 2026-10-08 22:45: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
type _Decoder func(
	who string,
	data []byte,
	order string,
) (starlark.Int, error)

// String is what print shows: how many bytes are left, not the bytes, which
// may be many.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
func (r *Reader) String() string {
	return fmt.Sprintf(
		"<%s length %d>",
		READER_TYPE,
		r.Len(),
	)
}

// Type names this value's type, as a script's type() sees it.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
func (r *Reader) Type() string {
	return READER_TYPE
}

// Freeze stops every later read, which a spawn handing the reader to another
// thread needs: a read moves the reader, so two threads reading would race.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-11-04 19:30: a nil reader is left as it is (C87)
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
func (r *Reader) Freeze() {
	if r == nil {
		return
	}

	r.frozen = true
}

// Truth reports a reader as true while any bytes are left.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
func (r *Reader) Truth() starlark.Bool {
	return r.Len() > 0
}

// Hash refuses: a reader changes as it is read, so it cannot be a key.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
func (r *Reader) Hash() (uint32, error) {
	return 0, fmt.Errorf(
		"%s is unhashable",
		READER_TYPE,
	)
}

// Len is how many bytes are left, which len() reads.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-11-04 19:30: a nil reader has none left (C87)
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
func (r *Reader) Len() int {
	if r == nil {
		return 0
	}

	return len(r.data) - r.at
}

// Index is the byte idx places ahead, as bytes of length one, without moving
// the reader: r[0] is the next byte.
//
// Here because len() reads a length only from a value that can be indexed. The
// interpreter checks idx against Len before it asks.
//
// An idx outside what is left is None: there is no byte there, and the
// interpreter never asks.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-11-04 19:30: an idx out of range is None, not a panic (C87)
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
func (r *Reader) Index(idx int) starlark.Value {
	if idx < 0 || idx >= r.Len() {
		return starlark.None
	}

	at := r.at + idx

	return starlark.Bytes(r.data[at : at+1])
}

// Attr is the method name calls, bound to this reader, or nil when there is no
// such method.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
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
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
func (r *Reader) AttrNames() []string {
	return slices.Sorted(maps.Keys(READER_METHODS))
}

// ReadBytes is the next count bytes, as bytes.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation, as read
//   - 2026-10-08 21:37: read_bytes, beside read_string
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
//   - 2026-11-10 00:25: renamed from _ReadBytes, as a test in tests/ needs it (C120a, Q314)
func ReadBytes(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	data, err := Take(
		fn,
		args,
		kwargs,
	)
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
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
//   - 2026-11-10 01:00: now uses Take, were _Take (C120a, Q314)
func _ReadString(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	data, err := Take(
		fn,
		args,
		kwargs,
	)
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
//   - 2026-11-04 00:42: decode gives a Starlark int, which it returns as it is (C38c)
//   - 2026-11-04 22:35: builds the method with plugin.NewBuiltin (Q134)
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
//   - 2026-11-10 01:00: now uses Movable, were _Movable (C120a, Q314)
func _Reading(
	name string,
	shape _Shape,
	decode _Decoder,
) *starlark.Builtin {
	return plugin.NewBuiltin(
		name,
		func(
			thread *starlark.Thread,
			fn *starlark.Builtin,
			args starlark.Tuple,
			kwargs []starlark.Tuple,
		) (starlark.Value, error) {
			reader, err := Movable(fn)
			if err != nil {
				return nil, err
			}

			width, order, err := shape(
				fn,
				args,
				kwargs,
			)
			if err != nil {
				return nil, err
			}

			data, err := reader._Next(
				shared.Called(fn),
				width,
			)
			if err != nil {
				return nil, err
			}

			number, err := decode(
				shared.Called(fn),
				data,
				order,
			)
			if err != nil {
				return nil, err
			}

			reader.at += width

			return number, nil
		},
	)
}

// Width reads the arguments of read_uint and read_int: a width of at least
// one, and a byte order, big unless named.
//
// Revisions:
//   - 2026-10-08 21:37: initial creation, from _Reading, which read them itself
//   - 2026-11-04 21:49: refuses a width below one through common.Width (C59)
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
//   - 2026-11-10 00:25: renamed from _Width, as a test in tests/ needs it (C120a, Q314)
func Width(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (int, string, error) {
	var (
		width int
		order = common.BIG
	)

	err := starlark.UnpackPositionalArgs(
		shared.Called(fn),
		args,
		kwargs,
		1,
		&width,
		&order,
	)
	if err != nil {
		return 0, "", err
	}

	err = shared.Width(
		shared.Called(fn),
		width,
	)
	if err != nil {
		return 0, "", err
	}

	return width, order, nil
}

// Order reads the arguments of a method whose width is fixed: only a byte
// order, big unless named.
//
// Revisions:
//   - 2026-10-08 21:37: initial creation
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
//   - 2026-11-10 00:25: renamed from _Order, as a test in tests/ needs it (C120a, Q314)
func Order(width int) _Shape {
	return func(
		fn *starlark.Builtin,
		args starlark.Tuple,
		kwargs []starlark.Tuple,
	) (int, string, error) {
		var (
			order = common.BIG
			err   = starlark.UnpackPositionalArgs(
				shared.Called(fn),
				args,
				kwargs,
				0,
				&order,
			)
		)
		if err != nil {
			return 0, "", err
		}

		return width, order, nil
	}
}

// Take is the next count bytes, count read from the arguments, moving the
// reader past them.
//
// Revisions:
//   - 2026-10-08 21:37: initial creation, from read, so read_string shares it
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
//   - 2026-11-10 00:25: renamed from _Take, as a test in tests/ needs it (C120a, Q314)
func Take(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) ([]byte, error) {
	reader, err := Movable(fn)
	if err != nil {
		return nil, err
	}

	var count int

	err = starlark.UnpackPositionalArgs(
		shared.Called(fn),
		args,
		kwargs,
		1,
		&count,
	)
	if err != nil {
		return nil, err
	}

	data, err := reader._Next(
		shared.Called(fn),
		count,
	)
	if err != nil {
		return nil, err
	}

	reader.at += count

	return data, nil
}

// Movable is the reader fn is bound to, or ErrFrozen once a spawn has frozen
// it.
//
// Attr is the one place that binds these methods, and it binds each to the
// reader it was read from. A method called unbound, or bound to a nil reader or
// to another type by a Go caller, has no reader to move: it is refused with
// ErrFrozen, as a frozen one is.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-11-04 19:30: no reader is ErrFrozen, not a panic (C87)
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-06 21:38: a nil builtin has no reader either (C102, Q214)
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
//   - 2026-11-10 00:25: renamed from _Movable, as a test in tests/ needs it (C120a, Q314)
func Movable(fn *starlark.Builtin) (*Reader, error) {
	var reader *Reader
	if fn != nil {
		reader, _ = fn.Receiver().(*Reader)
	}

	if reader == nil || reader.frozen {
		return nil, fmt.Errorf(
			"%s: %w",
			shared.Called(fn),
			ErrFrozen,
		)
	}

	return reader, nil
}

// _Next is the next count bytes, without moving past them: refused with
// unpack.ErrRange for a negative count, and ErrShort for more than are left.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
func (r *Reader) _Next(
	who string,
	count int,
) ([]byte, error) {
	if count < 0 {
		return nil, fmt.Errorf(
			"%s got %d, which is negative: %w",
			who,
			count,
			unpack.ErrRange,
		)
	}

	left := r.Len()
	if count > left {
		return nil, fmt.Errorf(
			"%s wants %d bytes with %d left: %w",
			who,
			count,
			left,
			ErrShort,
		)
	}

	return r.data[r.at : r.at+count], nil
}
