package buf

import (
	"fmt"
	"maps"
	"slices"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/codec/integer"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// Buffer is bytes a script writes a piece at a time and reads back whole.
//
// It changes until it is frozen, as a list does, and so it cannot be hashed, as
// a list cannot.
type Buffer struct {
	data   []byte
	frozen bool
}

// _Parse reads a writing method's arguments as the integer to write, its width
// and its byte order.
type _Parse func(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (*integer.Field, error)

// _Encoder is integer's Encode or EncodeSigned.
type _Encoder func(budget *scheduler.Budget, who string, field *integer.Field) ([]byte, error)

// String is what print shows: how many bytes are written, not the bytes, which
// may be many.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
func (b *Buffer) String() string {
	return fmt.Sprintf("<%s length %d>", BUFFER_TYPE, len(b.data))
}

// Type names this value's type, as a script's type() sees it.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
func (b *Buffer) Type() string {
	return BUFFER_TYPE
}

// Freeze stops every later write, which a spawn handing the buffer to another
// thread needs: two threads writing one buffer would race.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
func (b *Buffer) Freeze() {
	b.frozen = true
}

// Truth reports a buffer as true once anything is written, as bytes are.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
func (b *Buffer) Truth() starlark.Bool {
	return len(b.data) > 0
}

// Hash refuses: a buffer changes, so it cannot be a key.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
func (b *Buffer) Hash() (uint32, error) {
	return 0, fmt.Errorf("%s is unhashable", BUFFER_TYPE)
}

// Len is how many bytes are written, which len() reads.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
func (b *Buffer) Len() int {
	return len(b.data)
}

// Index is the byte at idx, as bytes of length one, as indexing bytes gives.
//
// Here because len() reads a length only from a value that can be indexed. The
// interpreter checks idx against Len before it asks.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
func (b *Buffer) Index(idx int) starlark.Value {
	return starlark.Bytes(b.data[idx : idx+1])
}

// Attr is the method name calls, bound to this buffer, or nil when there is no
// such method.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
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
func (b *Buffer) AttrNames() []string {
	return slices.Sorted(maps.Keys(BUFFER_METHODS))
}

// _WriteBytes appends data, bytes or the UTF-8 bytes of a str, as every
// function here that takes bytes also takes a str.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation, as write
//   - 2026-10-08 21:37: write_bytes, beside write_string
func _WriteBytes(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	data, err := unpack.Data(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	return _Appended(fn, data)
}

// _WriteString appends the UTF-8 bytes of a str, and only of a str, as Go's
// WriteString takes only a string.
//
// Revisions:
//   - 2026-10-08 21:37: initial creation
func _WriteString(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	text, err := unpack.Text(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	return _Appended(fn, []byte(text))
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
func _Writer(name string, parse _Parse, encode _Encoder) *starlark.Builtin {
	return starlark.NewBuiltin(name, func(
		thread *starlark.Thread,
		fn *starlark.Builtin,
		args starlark.Tuple,
		kwargs []starlark.Tuple,
	) (starlark.Value, error) {
		field, err := parse(fn, args, kwargs)
		if err != nil {
			return nil, err
		}

		data, err := encode(scheduler.Allowance(thread), fn.Name(), field)
		if err != nil {
			return nil, err
		}

		return _Appended(fn, data)
	})
}

// _Number reads the arguments of a method whose width is fixed - a number, and
// a byte order, big unless named - as the integer to write in width bytes.
//
// Revisions:
//   - 2026-10-08 21:37: initial creation
func _Number(width int) _Parse {
	return func(
		fn *starlark.Builtin,
		args starlark.Tuple,
		kwargs []starlark.Tuple,
	) (*integer.Field, error) {
		var (
			number starlark.Int
			order  = integer.BIG
		)

		err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 1, &number, &order)
		if err != nil {
			return nil, err
		}

		return &integer.Field{Number: number.BigInt(), Width: width, Order: order}, nil
	}
}

// _Bytes is everything written, as bytes.
//
// A copy, so the buffer goes on growing without changing what it handed out;
// and allowed once frozen, since it changes nothing.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
func _Bytes(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 0)
	if err != nil {
		return nil, err
	}

	return starlark.Bytes(_Bound(fn).data), nil
}

// _Appended appends data to the buffer fn is bound to, or refuses with
// ERR_FROZEN once a spawn has frozen it.
//
// Every write ends here, so a frozen buffer is refused in one place.
//
// Revisions:
//   - 2026-10-08 21:37: initial creation, from _Writable, which each write called
//     first
func _Appended(fn *starlark.Builtin, data []byte) (starlark.Value, error) {
	buffer := _Bound(fn)
	if buffer.frozen {
		return nil, fmt.Errorf("%s: %w", fn.Name(), ERR_FROZEN)
	}

	buffer.data = append(buffer.data, data...)

	return starlark.None, nil
}

// _Bound is the buffer fn is bound to.
//
// Asserted rather than asked: Attr is the one place that binds these methods,
// and it binds each to the buffer it was read from, so the receiver is a buffer
// whenever one of them runs.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
func _Bound(fn *starlark.Builtin) *Buffer {
	return fn.Receiver().(*Buffer)
}
