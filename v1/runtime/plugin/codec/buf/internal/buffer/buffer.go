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

// Buffer is bytes a script writes a piece at a time and reads back whole.
//
// It changes until it is frozen, as a list does, and so it cannot be hashed, as
// a list cannot.
//
// Revisions:
//   - 2026-10-08 22:45: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
type Buffer struct {
	data   []byte
	frozen bool
}

// _Parse reads a writing method's arguments as the integer to write, its width
// and its byte order.
//
// Revisions:
//   - 2026-10-08 22:45: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
type _Parse func(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (*common.Field, error)

// _Encoder is integer's Encode or EncodeSigned.
//
// Revisions:
//   - 2026-10-08 22:45: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
type _Encoder func(
	who string,
	field *common.Field,
) ([]byte, error)

// String is what print shows: how many bytes are written, not the bytes, which
// may be many.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-11-04 19:30: a nil buffer is empty (C87)
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
func (b *Buffer) String() string {
	return fmt.Sprintf(
		"<%s length %d>",
		BUFFER_TYPE,
		b.Len(),
	)
}

// Type names this value's type, as a script's type() sees it.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
func (b *Buffer) Type() string {
	return BUFFER_TYPE
}

// Freeze stops every later write, which a spawn handing the buffer to another
// thread needs: two threads writing one buffer would race.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-11-04 19:30: a nil buffer is left as it is (C87)
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
func (b *Buffer) Freeze() {
	if b == nil {
		return
	}

	b.frozen = true
}

// Truth reports a buffer as true once anything is written, as bytes are.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-11-04 19:30: a nil buffer is empty (C87)
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
func (b *Buffer) Truth() starlark.Bool {
	return b.Len() > 0
}

// Hash refuses: a buffer changes, so it cannot be a key.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
func (b *Buffer) Hash() (uint32, error) {
	return 0, fmt.Errorf(
		"%s is unhashable",
		BUFFER_TYPE,
	)
}

// Len is how many bytes are written, which len() reads.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-11-04 19:30: a nil buffer is empty (C87)
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
func (b *Buffer) Len() int {
	if b == nil {
		return 0
	}

	return len(b.data)
}

// Index is the byte at idx, as bytes of length one, as indexing bytes gives.
//
// Here because len() reads a length only from a value that can be indexed. The
// interpreter checks idx against Len before it asks.
//
// An idx outside the buffer is None: there is no byte there, and the interpreter
// never asks.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-11-04 19:30: an idx out of range is None, not a panic (C87)
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
func (b *Buffer) Index(idx int) starlark.Value {
	if idx < 0 || idx >= b.Len() {
		return starlark.None
	}

	return starlark.Bytes(b.data[idx : idx+1])
}

// Attr is the method name calls, bound to this buffer, or nil when there is no
// such method.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
func (b *Buffer) Attr(name string) (starlark.Value, error) {
	method, ok := BUFFER_METHODS[name]
	if !ok {
		return nil, nil
	}

	return method.BindReceiver(b), nil
}

// AttrNames is the methods, as dir() lists them.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
func (b *Buffer) AttrNames() []string {
	return slices.Sorted(maps.Keys(BUFFER_METHODS))
}

// _WriteBytes appends data, bytes or the UTF-8 bytes of a str, as every
// function here that takes bytes also takes a str.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation, as write
//   - 2026-10-08 21:37: write_bytes, beside write_string
//   - 2026-11-03 23:00: reads a view of the data, with no copy: _Appended copies it
//     into the buffer and keeps no slice of it (C38b, plan D4)
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
//   - 2026-11-10 01:00: now uses Appended, were _Appended (C120a, Q314)
func _WriteBytes(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	data, err := unpack.DataView(
		fn,
		args,
		kwargs,
	)
	if err != nil {
		return nil, err
	}

	return Appended(
		fn,
		data,
	)
}

// WriteString appends the UTF-8 bytes of a str, and only of a str, as Go's
// WriteString takes only a string.
//
// Revisions:
//   - 2026-10-08 21:37: initial creation
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
//   - 2026-11-10 00:25: renamed from _WriteString, as a test in tests/ needs it (C120a, Q314)
func WriteString(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	text, err := unpack.Text(
		fn,
		args,
		kwargs,
	)
	if err != nil {
		return nil, err
	}

	return Appended(
		fn,
		[]byte(text),
	)
}

// _Writer is a method that appends an integer as encode writes it - unsigned
// for uint, two's complement for int - in the width and order parse reads from
// its arguments: one the script names for write_uint and write_int, and a fixed
// one for each of the rest.
//
// Built from the two rather than written ten times, because the methods differ
// in those and in nothing else.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-10-08 21:37: takes what parses the arguments, so a fixed width is a
//     method too
//   - 2026-11-01 13:00: hands encode no budget
//   - 2026-11-04 22:35: builds the method with plugin.NewBuiltin (Q134)
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
//   - 2026-11-10 01:00: now uses Appended, were _Appended (C120a, Q314)
func _Writer(
	name string,
	parse _Parse,
	encode _Encoder,
) *starlark.Builtin {
	return plugin.NewBuiltin(
		name,
		func(
			thread *starlark.Thread,
			fn *starlark.Builtin,
			args starlark.Tuple,
			kwargs []starlark.Tuple,
		) (starlark.Value, error) {
			field, err := parse(
				fn,
				args,
				kwargs,
			)
			if err != nil {
				return nil, err
			}

			data, err := encode(
				shared.Called(fn),
				field,
			)
			if err != nil {
				return nil, err
			}

			return Appended(
				fn,
				data,
			)
		},
	)
}

// Number reads the arguments of a method whose width is fixed - a number, and
// a byte order, big unless named - as the integer to write in width bytes.
//
// Revisions:
//   - 2026-10-08 21:37: initial creation
//   - 2026-11-04 00:51: keeps the number as a Starlark int (C38c)
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
//   - 2026-11-10 00:25: renamed from _Number, as a test in tests/ needs it (C120a, Q314)
func Number(width int) _Parse {
	return func(
		fn *starlark.Builtin,
		args starlark.Tuple,
		kwargs []starlark.Tuple,
	) (*common.Field, error) {
		var (
			number starlark.Int
			order  = common.BIG
		)

		err := starlark.UnpackPositionalArgs(
			shared.Called(fn),
			args,
			kwargs,
			1,
			&number,
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
}

// Bytes is everything written, as bytes.
//
// A copy, so the buffer goes on growing without changing what it handed out;
// and allowed once frozen, since it changes nothing.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
//   - 2026-11-10 00:25: renamed from _Bytes, as a test in tests/ needs it (C120a, Q314)
func Bytes(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	err := starlark.UnpackPositionalArgs(
		shared.Called(fn),
		args,
		kwargs,
		0,
	)
	if err != nil {
		return nil, err
	}

	return starlark.Bytes(_Bound(fn).data), nil
}

// Appended appends data to the buffer fn is bound to, or refuses with
// ErrFrozen once a spawn has frozen it.
//
// Every write ends here, so a frozen buffer is refused in one place.
//
// Revisions:
//   - 2026-10-08 21:37: initial creation, from _Writable, which each write called
//     first
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
//   - 2026-11-10 00:25: renamed from _Appended, as a test in tests/ needs it (C120a, Q314)
func Appended(
	fn *starlark.Builtin,
	data []byte,
) (starlark.Value, error) {
	buffer := _Bound(fn)
	if buffer.frozen {
		return nil, fmt.Errorf(
			"%s: %w",
			shared.Called(fn),
			ErrFrozen,
		)
	}

	buffer.data = append(
		buffer.data,
		data...,
	)

	return starlark.None, nil
}

// _Bound is the buffer fn is bound to.
//
// Attr is the one place that binds these methods, and it binds each to the
// buffer it was read from. A method taken from BUFFER_METHODS and called
// unbound, or bound to a nil buffer or to another type by a Go caller, has no
// buffer: it is a frozen empty one, so a write is refused with ErrFrozen and a
// read answers nothing. The empty buffer is made per call, on that path only.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-11-04 19:30: no buffer is a frozen empty one, not a panic (C87)
//   - 2026-11-06 21:38: a nil builtin has no buffer either (C102, Q214)
//   - 2026-11-07 19:17: moved to package buffer (C91w8a)
func _Bound(fn *starlark.Builtin) *Buffer {
	var buffer *Buffer
	if fn != nil {
		buffer, _ = fn.Receiver().(*Buffer)
	}

	if buffer == nil {
		return &Buffer{
			frozen: true,
		}
	}

	return buffer
}
