// Package buf gives a script the module buf: a buffer to write bytes into a
// piece at a time, and a reader to take bytes apart the same way. Importing it
// is what enables it.
//
// A buffer, because a script cannot otherwise build bytes in pieces: Starlark's
// bytes has no join, and += copies everything written so far on every write.
//
// The methods are named as Go names them: every write starts with write_ and
// every read with read_, and an integer is written or read in 8, 16, 32 or 64
// bits, or in any width through write_uint, write_int, read_uint and read_int.
// Integers follow the integer module's rules, through its Encode and Decode, so
// the two never disagree; importing this enables integer as well. uint is
// unsigned and int two's complement, and the byte order is big unless "little"
// is named.
//
// A buffer and a reader are not data, so a store or an event refuses one as it
// refuses a handle. A spawn freezes either, and from then on each refuses what
// would change it with ERR_FROZEN - for a reader, every read, since a read
// moves it.
package buf

import (
	"errors"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/integer"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

var (
	// ERR_FROZEN is returned for a change to a buffer or a reader that a spawn
	// has frozen.
	ERR_FROZEN = errors.New("frozen")

	// ERR_SHORT is returned for a read of more bytes than a reader has left.
	ERR_SHORT = errors.New("fewer bytes left than asked for")

	// BUFFER_METHODS and READER_METHODS are what the attributes of a buffer and
	// of a reader call, each bound to the value it is read from.
	BUFFER_METHODS = map[string]*starlark.Builtin{
		WRITE_BYTES:  starlark.NewBuiltin(WRITE_BYTES, _WriteBytes),
		WRITE_STRING: starlark.NewBuiltin(WRITE_STRING, _WriteString),
		WRITE_UINT:   _Writer(WRITE_UINT, integer.Unpack, integer.Encode),
		WRITE_INT:    _Writer(WRITE_INT, integer.Unpack, integer.EncodeSigned),
		WRITE_UINT8:  _Writer(WRITE_UINT8, _Number(WIDTH_8), integer.Encode),
		WRITE_UINT16: _Writer(WRITE_UINT16, _Number(WIDTH_16), integer.Encode),
		WRITE_UINT32: _Writer(WRITE_UINT32, _Number(WIDTH_32), integer.Encode),
		WRITE_UINT64: _Writer(WRITE_UINT64, _Number(WIDTH_64), integer.Encode),
		WRITE_INT8:   _Writer(WRITE_INT8, _Number(WIDTH_8), integer.EncodeSigned),
		WRITE_INT16:  _Writer(WRITE_INT16, _Number(WIDTH_16), integer.EncodeSigned),
		WRITE_INT32:  _Writer(WRITE_INT32, _Number(WIDTH_32), integer.EncodeSigned),
		WRITE_INT64:  _Writer(WRITE_INT64, _Number(WIDTH_64), integer.EncodeSigned),
		BYTES:        starlark.NewBuiltin(BYTES, _Bytes),
	}
	READER_METHODS = map[string]*starlark.Builtin{
		READ_BYTES:  starlark.NewBuiltin(READ_BYTES, _ReadBytes),
		READ_STRING: starlark.NewBuiltin(READ_STRING, _ReadString),
		READ_UINT:   _Reading(READ_UINT, _Width, integer.Decode),
		READ_INT:    _Reading(READ_INT, _Width, integer.DecodeSigned),
		READ_UINT8:  _Reading(READ_UINT8, _Order(WIDTH_8), integer.Decode),
		READ_UINT16: _Reading(READ_UINT16, _Order(WIDTH_16), integer.Decode),
		READ_UINT32: _Reading(READ_UINT32, _Order(WIDTH_32), integer.Decode),
		READ_UINT64: _Reading(READ_UINT64, _Order(WIDTH_64), integer.Decode),
		READ_INT8:   _Reading(READ_INT8, _Order(WIDTH_8), integer.DecodeSigned),
		READ_INT16:  _Reading(READ_INT16, _Order(WIDTH_16), integer.DecodeSigned),
		READ_INT32:  _Reading(READ_INT32, _Order(WIDTH_32), integer.DecodeSigned),
		READ_INT64:  _Reading(READ_INT64, _Order(WIDTH_64), integer.DecodeSigned),
	}
)

const (
	// NAME is the module, and the two names it holds.
	NAME   = "buf"
	NEW    = "new"
	READER = "reader"

	// WRITE_BYTES to BYTES are the methods of a buffer.
	WRITE_BYTES  = "write_bytes"
	WRITE_STRING = "write_string"
	WRITE_UINT   = "write_uint"
	WRITE_INT    = "write_int"
	WRITE_UINT8  = "write_uint8"
	WRITE_UINT16 = "write_uint16"
	WRITE_UINT32 = "write_uint32"
	WRITE_UINT64 = "write_uint64"
	WRITE_INT8   = "write_int8"
	WRITE_INT16  = "write_int16"
	WRITE_INT32  = "write_int32"
	WRITE_INT64  = "write_int64"
	BYTES        = "bytes"

	// READ_BYTES to READ_INT64 are the methods of a reader.
	READ_BYTES  = "read_bytes"
	READ_STRING = "read_string"
	READ_UINT   = "read_uint"
	READ_INT    = "read_int"
	READ_UINT8  = "read_uint8"
	READ_UINT16 = "read_uint16"
	READ_UINT32 = "read_uint32"
	READ_UINT64 = "read_uint64"
	READ_INT8   = "read_int8"
	READ_INT16  = "read_int16"
	READ_INT32  = "read_int32"
	READ_INT64  = "read_int64"

	// WIDTH_8 to WIDTH_64 are how many bytes an integer of 8 to 64 bits takes.
	WIDTH_8  = 1
	WIDTH_16 = 2
	WIDTH_32 = 4
	WIDTH_64 = 8

	// BUFFER_TYPE and READER_TYPE are what type() calls the two values.
	BUFFER_TYPE = "buffer"
	READER_TYPE = "reader"
)

// init registers this plugin, so that a host importing this package for its
// side effect is the whole of enabling it.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
func init() {
	plugin.Register(new(_Buf))
}

// _Buf is the plugin. Empty: each buffer and reader holds its own bytes.
type _Buf struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
func (b *_Buf) Name() string {
	return NAME
}

// Values returns the buf module.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
func (b *_Buf) Values() starlark.StringDict {
	return starlark.StringDict{
		NAME: &starlarkstruct.Module{
			Name: NAME,
			Members: starlark.StringDict{
				NEW:    starlark.NewBuiltin(NAME+"."+NEW, _NewBuffer),
				READER: starlark.NewBuiltin(NAME+"."+READER, _NewReader),
			},
		},
	}
}

// _NewBuffer is an empty buffer.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
func _NewBuffer(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 0)
	if err != nil {
		return nil, err
	}

	return &Buffer{}, nil
}

// _NewReader is a reader over data, bytes or the UTF-8 bytes of a str, from
// its first byte.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
func _NewReader(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	data, err := unpack.Data(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	return &Reader{data: data}, nil
}
