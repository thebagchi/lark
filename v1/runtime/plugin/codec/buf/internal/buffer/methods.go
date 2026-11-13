package buffer

import (
	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/integer"
)

var (
	// BUFFER_METHODS is what the attributes of a buffer call, each bound to the
	// buffer it is read from. Each is made with plugin.NewBuiltin, so a nil
	// argument is refused.
	//
	// Revisions:
	//   - 2026-11-04 22:35: made with plugin.NewBuiltin (Q134)
	//   - 2026-11-07 19:17: moved from package buf (C91w8a)
	//   - 2026-11-10 01:00: now uses WriteString, Number, Bytes, were _WriteString, _Number,
	//     _Bytes (C120a, Q314)
	BUFFER_METHODS = map[string]*starlark.Builtin{
		WRITE_BYTES: plugin.NewBuiltin(
			WRITE_BYTES,
			_WriteBytes,
		),
		WRITE_STRING: plugin.NewBuiltin(
			WRITE_STRING,
			WriteString,
		),
		WRITE_UINT: _Writer(
			WRITE_UINT,
			integer.Unpack,
			integer.Encode,
		),
		WRITE_INT: _Writer(
			WRITE_INT,
			integer.Unpack,
			integer.EncodeSigned,
		),
		WRITE_UINT8: _Writer(
			WRITE_UINT8,
			Number(WIDTH_8),
			integer.Encode,
		),
		WRITE_UINT16: _Writer(
			WRITE_UINT16,
			Number(WIDTH_16),
			integer.Encode,
		),
		WRITE_UINT32: _Writer(
			WRITE_UINT32,
			Number(WIDTH_32),
			integer.Encode,
		),
		WRITE_UINT64: _Writer(
			WRITE_UINT64,
			Number(WIDTH_64),
			integer.Encode,
		),
		WRITE_INT8: _Writer(
			WRITE_INT8,
			Number(WIDTH_8),
			integer.EncodeSigned,
		),
		WRITE_INT16: _Writer(
			WRITE_INT16,
			Number(WIDTH_16),
			integer.EncodeSigned,
		),
		WRITE_INT32: _Writer(
			WRITE_INT32,
			Number(WIDTH_32),
			integer.EncodeSigned,
		),
		WRITE_INT64: _Writer(
			WRITE_INT64,
			Number(WIDTH_64),
			integer.EncodeSigned,
		),
		BYTES: plugin.NewBuiltin(
			BYTES,
			Bytes,
		),
	}

	// READER_METHODS is what the attributes of a reader call, each bound to the reader
	// it is read from. Each is made with plugin.NewBuiltin.
	//
	// Revisions:
	//   - 2026-11-04 22:35: made with plugin.NewBuiltin (Q134)
	//   - 2026-11-07 19:17: moved from package buf (C91w8a)
	//   - 2026-11-10 01:00: now uses ReadBytes, Width, Order, were _ReadBytes, _Width, _Order
	//     (C120a, Q314)
	READER_METHODS = map[string]*starlark.Builtin{
		READ_BYTES: plugin.NewBuiltin(
			READ_BYTES,
			ReadBytes,
		),
		READ_STRING: plugin.NewBuiltin(
			READ_STRING,
			_ReadString,
		),
		READ_UINT: _Reading(
			READ_UINT,
			Width,
			integer.Decode,
		),
		READ_INT: _Reading(
			READ_INT,
			Width,
			integer.DecodeSigned,
		),
		READ_UINT8: _Reading(
			READ_UINT8,
			Order(WIDTH_8),
			integer.Decode,
		),
		READ_UINT16: _Reading(
			READ_UINT16,
			Order(WIDTH_16),
			integer.Decode,
		),
		READ_UINT32: _Reading(
			READ_UINT32,
			Order(WIDTH_32),
			integer.Decode,
		),
		READ_UINT64: _Reading(
			READ_UINT64,
			Order(WIDTH_64),
			integer.Decode,
		),
		READ_INT8: _Reading(
			READ_INT8,
			Order(WIDTH_8),
			integer.DecodeSigned,
		),
		READ_INT16: _Reading(
			READ_INT16,
			Order(WIDTH_16),
			integer.DecodeSigned,
		),
		READ_INT32: _Reading(
			READ_INT32,
			Order(WIDTH_32),
			integer.DecodeSigned,
		),
		READ_INT64: _Reading(
			READ_INT64,
			Order(WIDTH_64),
			integer.DecodeSigned,
		),
	}
)
