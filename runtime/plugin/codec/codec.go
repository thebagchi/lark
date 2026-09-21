// Package codec gives a script the conversions between bytes, hex, bits,
// integers and the base encodings. Importing it is what enables it.
//
// The twelve of section 10.6 of the brief are spelled x2y - bytes2hex,
// int2bits. The rest cover what Python's binascii covers and are spelled
// encode and decode, because a name ending in a digit cannot take the 2 infix
// without reading as a number: base642bytes is "base 642 bytes" to every
// reader who has not been told otherwise.
//
// Four of binascii's own are deliberately absent. Hex is already here under
// its x2y names; quoted-printable and uuencode are email-era formats, the
// second with nothing in the standard library to read it; crc_hqx is the
// checksum of one obsolete protocol.
//
// The names are flat rather than members of a module, because that is how the
// brief spells the twelve.
package codec

import (
	"errors"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/runtime/plugin"
)

var (
	// ErrData is returned when something that should be bytes or a string
	// is neither.
	ErrData = errors.New("wants bytes or a string")

	// ErrHex is returned for text that is not hexadecimal, or has an odd
	// number of digits where whole bytes are wanted.
	ErrHex = errors.New("not hexadecimal")

	// ErrBits is returned for text that is not made of 0 and 1, or whose
	// length is not the multiple of eight that whole bytes need.
	ErrBits = errors.New("not a bit string")

	// ErrRange is returned for a negative integer, a width below one, or an
	// integer that does not fit the width it was given.
	ErrRange = errors.New("out of range")

	// ErrEncoded is returned for text that is not the encoding it was handed
	// to.
	ErrEncoded = errors.New("not the encoding this reads")
)

const (
	NAME         = "codec"
	BYTES2HEX    = "bytes2hex"
	HEX2BYTES    = "hex2bytes"
	BYTES2BITS   = "bytes2bits"
	BITS2BYTES   = "bits2bytes"
	BYTES2INT    = "bytes2int"
	INT2BYTES    = "int2bytes"
	BITS2HEX     = "bits2hex"
	HEX2BITS     = "hex2bits"
	INT2BITS     = "int2bits"
	BITS2INT     = "bits2int"
	INT2HEX      = "int2hex"
	HEX2INT      = "hex2int"
	B64ENCODE    = "b64encode"
	B64DECODE    = "b64decode"
	B64URLENCODE = "b64urlencode"
	B64URLDECODE = "b64urldecode"
	B32ENCODE    = "b32encode"
	B32DECODE    = "b32decode"
	CRC32        = "crc32"

	// BITS_PER_BYTE and BITS_PER_NIBBLE are what the padding rules pad to.
	BITS_PER_BYTE   = 8
	BITS_PER_NIBBLE = 4

	// BINARY and HEXADECIMAL are the bases the text forms are read in.
	BINARY      = 2
	HEXADECIMAL = 16

	// PADDING is what the padded base encodings end with, and what a decoder
	// here takes off before reading, so padded and unpadded text both decode.
	PADDING = "="

	// NO_SEED is the checksum a chain starts from.
	NO_SEED = 0
)

// init registers this plugin, so that a host importing this package for its
// side effect is the whole of enabling it.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
func init() {
	plugin.Register(&_Codec{})
}

// _Codec is the plugin. Empty: every conversion works on what it is given.
type _Codec struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
func (c *_Codec) Name() string {
	return NAME
}

// Values returns the nineteen names this plugin supplies.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
//   - 2026-09-21 15:25: the base encodings and the checksum
func (c *_Codec) Values() starlark.StringDict {
	return starlark.StringDict{
		BYTES2HEX:    starlark.NewBuiltin(BYTES2HEX, _Bytes2Hex),
		HEX2BYTES:    starlark.NewBuiltin(HEX2BYTES, _Hex2Bytes),
		BYTES2BITS:   starlark.NewBuiltin(BYTES2BITS, _Bytes2Bits),
		BITS2BYTES:   starlark.NewBuiltin(BITS2BYTES, _Bits2Bytes),
		BYTES2INT:    starlark.NewBuiltin(BYTES2INT, _Bytes2Int),
		INT2BYTES:    starlark.NewBuiltin(INT2BYTES, _Int2Bytes),
		BITS2HEX:     starlark.NewBuiltin(BITS2HEX, _Bits2Hex),
		HEX2BITS:     starlark.NewBuiltin(HEX2BITS, _Hex2Bits),
		INT2BITS:     starlark.NewBuiltin(INT2BITS, _Int2Bits),
		BITS2INT:     starlark.NewBuiltin(BITS2INT, _Bits2Int),
		INT2HEX:      starlark.NewBuiltin(INT2HEX, _Int2Hex),
		HEX2INT:      starlark.NewBuiltin(HEX2INT, _Hex2Int),
		B64ENCODE:    starlark.NewBuiltin(B64ENCODE, _B64Encode),
		B64DECODE:    starlark.NewBuiltin(B64DECODE, _B64Decode),
		B64URLENCODE: starlark.NewBuiltin(B64URLENCODE, _B64URLEncode),
		B64URLDECODE: starlark.NewBuiltin(B64URLDECODE, _B64URLDecode),
		B32ENCODE:    starlark.NewBuiltin(B32ENCODE, _B32Encode),
		B32DECODE:    starlark.NewBuiltin(B32DECODE, _B32Decode),
		CRC32:        starlark.NewBuiltin(CRC32, _CRC32),
	}
}
