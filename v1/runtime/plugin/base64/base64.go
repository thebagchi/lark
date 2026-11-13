// Package base64 gives a script the base64 encodings. Importing it is what
// enables it.
//
// A module rather than flat names, as every conversion is: encode and decode
// are words a script uses for several things, so they are said behind the
// encoding they belong to. base64.encode reads as what it is; a global encode
// would not.
//
// Two alphabets, because both are in the wild. The standard one of RFC 4648
// section 4 is padded, and the URL and filename safe one of section 5 is not -
// what reaches for the second is a JSON Web Token, whose segments carry no
// padding.
//
// Revisions:
//   - 2026-09-24 17:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
package base64

import (
	"encoding/base64"
	"unsafe"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/base64/internal/codec"
	"github.com/thebagchi/lark/v1/runtime/plugin/base64/internal/spec"
	"github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// init registers this plugin, so that a host importing this package for its
// side effect is the whole of enabling it.
//
// Revisions:
//   - 2026-09-23 23:20: initial creation
func init() {
	plugin.Register(new(_Base64))
}

// _Base64 is the plugin. Empty: every conversion works on what it is given.
//
// Revisions:
//   - 2026-09-24 17:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Base64 struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-23 23:20: initial creation
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
func (b *_Base64) Name() string {
	return spec.NAME
}

// Values returns the base64 module.
//
// Revisions:
//   - 2026-09-23 23:20: initial creation
//   - 2026-11-01 23:00: builds the builtins with _Encoder and _Decoder
//   - 2026-11-02 16:45: the decoders read in Strict mode (C22)
//   - 2026-11-03 22:45: gives each builtin its coding, for the kernels (C51)
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
func (b *_Base64) Values() starlark.StringDict {
	return starlark.StringDict{
		spec.NAME: &starlarkstruct.Module{
			Name: spec.NAME,
			Members: starlark.StringDict{
				spec.ENCODE: _Encoder(
					spec.ENCODE,
					_NewCoding(
						spec.ALPHABET_STD,
						base64.StdEncoding,
					),
				),
				spec.DECODE: _Decoder(
					spec.DECODE,
					_NewCoding(
						spec.ALPHABET_STD,
						base64.RawStdEncoding.Strict(),
					),
				),
				spec.URLENCODE: _Encoder(
					spec.URLENCODE,
					_NewCoding(
						spec.ALPHABET_URL,
						base64.RawURLEncoding,
					),
				),
				spec.URLDECODE: _Decoder(
					spec.URLDECODE,
					_NewCoding(
						spec.ALPHABET_URL,
						base64.RawURLEncoding.Strict(),
					),
				),
			},
		},
	}
}

// _Encoder is the builtin that writes data in one encoding.
//
// Built from the name and the coding rather than written twice: the two
// writers differ in the alphabet and in whether they pad, and in nothing else,
// and two copies of one function is two places for one fix to be forgotten.
// The encodings are these, and the reason for each:
//
//   - base64.StdEncoding is RFC 4648 section 4, padded. It has no trailing
//     newline. Python's b2a_base64 appends one, and a script that did not want
//     it would have to strip it before comparing anything.
//   - base64.RawURLEncoding is the URL and filename safe alphabet of section 5,
//     unpadded. What reaches for this alphabet is a JSON Web Token, whose
//     segments carry no padding. A caller wanting padding has the standard form.
//
// The data is a view of the script's value, not a copy: the encoder only reads
// it, into a buffer of its own, which is the contract of unpack.View. On amd64
// the encoder is a kernel in assembly for the large part of the data, chosen by
// the CPU (kernel.go); encoding/base64 writes the rest and the padding.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation, as codec's b64encode and b64urlencode
//   - 2026-09-23 23:20: members of the base64 module
//   - 2026-10-29 01:08: reads a view of the data rather than a copy
//   - 2026-11-01 23:00: one factory in place of _Encode and _URLEncode
//   - 2026-11-03 00:30: hands the output buffer over, in place of EncodeToString's copy of it
//     (C37)
//   - 2026-11-03 22:45: encodes through _Encode, with its coding (C51)
//   - 2026-11-04 18:14: doc comments name the coding, not the encoding (C51 fix 1)
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
//   - 2026-11-10 11:32: uses codec.Coding and codec.Encode, which moved to internal/codec
//     (C120b, Q315)
func _Encoder(
	name string,
	coding *codec.Coding,
) *starlark.Builtin {
	return starlark.NewBuiltin(
		spec.NAME+"."+name,
		func(
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

			text := make(
				[]byte,
				coding.Encoding.EncodedLen(len(data)),
			)
			codec.Encode(
				coding,
				text,
				data,
			)

			return starlark.String(unpack.Handover(text)), nil
		},
	)
}

// _Decoder is the builtin that reads text in one alphabet, padded or not.
//
// Built from the name and the coding for the reason _Encoder is. The
// coding holds the unpadded encoding of the alphabet in Strict mode, as _Read
// checks and takes off the padding before it reads.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation, as codec's b64decode and b64urldecode
//   - 2026-09-23 23:20: members of the base64 module
//   - 2026-11-01 23:00: one factory in place of _Decode and _URLDecode
//   - 2026-11-02 16:45: the encoding is Strict (C22)
//   - 2026-11-03 22:45: reads with its coding, for the kernels (C51)
//   - 2026-11-04 06:44: doc comments name the coding, not the encoding (C51 fix 1)
//   - 2026-11-04 18:48: rewrapped the doc comment (C51 fix 3)
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
//   - 2026-11-10 11:32: uses codec.Coding, which moved to internal/codec (C120b, Q315)
func _Decoder(
	name string,
	coding *codec.Coding,
) *starlark.Builtin {
	return starlark.NewBuiltin(
		spec.NAME+"."+name,
		func(
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

			return _Read(
				common.Called(fn),
				text,
				coding,
			)
		},
	)
}

// _Read decodes text through the coding, with any padding taken off first.
//
// Padding off rather than required, because both spellings of one value are in
// the wild: a JSON Web Token's segments carry none, and an encoder that pads is
// the commoner one. A decoder that accepted only its own output would refuse
// half of what a script is handed. RFC 4648 section 3.2 lets a specification
// omit padding, and RFC 7515 section 2 does.
//
// Two groups of refusals, each a clause of RFC 4648:
//
//   - Excess or wrong-count "=" and any CR or LF: section 3.3, "Implementations
//     MUST reject the encoded data if it contains characters outside the base
//     alphabet ... unless the specification referring to this document
//     explicitly states otherwise". lark states no such rule. unpack.CheckPadding
//     scans for them, as Go skips CR and LF even in Strict mode.
//   - Pad bits that are not zero: section 3.5, "decoders MAY chose to reject an
//     encoding if the pad bits have not been set to zero". lark does, because
//     otherwise several texts name one value (section 12). The Strict mode of
//     the coding's encoding does this.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation, as codec's _Decoded
//   - 2026-09-23 23:20: reads for the base64 module
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-02 16:45: refuse excess padding, non-zero pad bits and line breaks (C22)
//   - 2026-11-03 00:30: hands the decoded bytes over, with no copy of them (C37)
//   - 2026-11-03 22:45: decodes through _Decode, over a view of the text (C51)
//   - 2026-11-04 06:44: doc comments name the coding, not the encoding (C51 fix 1)
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 11:32: uses codec.Coding, which moved to internal/codec (C120b, Q315)
func _Read(
	who string,
	text string,
	coding *codec.Coding,
) (starlark.Value, error) {
	bare, ok := unpack.CheckPadding(
		text,
		spec.GROUP,
	)

	var (
		data  []byte
		valid bool
	)

	if ok {
		data, valid = _DecodeText(
			coding,
			bare,
		)
	}

	if !ok || !valid {
		return nil, common.Refused(
			who,
			text,
			unpack.ErrEncoded,
		)
	}

	return starlark.Bytes(unpack.Handover(data)), nil
}

// _DecodeText is the bytes that bare stands for, and whether it was text of the
// encoding. bare has no padding. The error text of a caller names the whole
// text and no position, so a false says all that a caller needs.
//
// The text is read through a view, not a copy: Decode only reads it.
// encoding.DecodeString copied the text first, and then decoded into a buffer
// of its own; this makes the one buffer, the result.
//
// Revisions:
//   - 2026-11-03 22:45: initial creation, in place of DecodeString (C51)
//   - 2026-11-10 11:32: uses codec.Coding and codec.Decode, which moved to internal/codec
//     (C120b, Q315)
func _DecodeText(
	coding *codec.Coding,
	bare string,
) ([]byte, bool) {
	var (
		data = make(
			[]byte,
			coding.Encoding.DecodedLen(len(bare)),
		)
		n, valid = codec.Decode(
			coding,
			data,
			unsafe.Slice(
				unsafe.StringData(bare),
				len(bare),
			),
		)
	)

	return data[:n], valid
}
