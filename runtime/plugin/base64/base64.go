// Package base64 gives a script the base64 encodings. Importing it is what
// enables it.
//
// A module rather than flat names, unlike codec's own conversions: encode and
// decode are words a script uses for several things, so they are said behind
// the encoding they belong to. base64.encode reads as what it is; a global
// encode would not.
//
// Two alphabets, because both are in the wild. The standard one of RFC 4648
// section 4 is padded, and the URL and filename safe one of section 5 is not -
// what reaches for the second is a JSON Web Token, whose segments carry no
// padding.
package base64

import (
	"encoding/base64"
	"errors"
	"fmt"
	"strings"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/runtime/plugin"
	"github.com/thebagchi/lark/runtime/plugin/unpack"
)

// ErrEncoded is returned for text that is not the encoding it was handed to.
var ErrEncoded = errors.New("not the encoding this reads")

const (
	// NAME is the module, and the four names it holds.
	NAME      = "base64"
	ENCODE    = "encode"
	DECODE    = "decode"
	URLENCODE = "urlencode"
	URLDECODE = "urldecode"

	// PADDING is what the padded alphabet ends with, and what a decoder takes
	// off before reading.
	PADDING = "="
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
type _Base64 struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-23 23:20: initial creation
func (b *_Base64) Name() string {
	return NAME
}

// Values returns the base64 module.
//
// Revisions:
//   - 2026-09-23 23:20: initial creation
func (b *_Base64) Values() starlark.StringDict {
	return starlark.StringDict{
		NAME: &starlarkstruct.Module{
			Name: NAME,
			Members: starlark.StringDict{
				ENCODE:    starlark.NewBuiltin(NAME+"."+ENCODE, _Encode),
				DECODE:    starlark.NewBuiltin(NAME+"."+DECODE, _Decode),
				URLENCODE: starlark.NewBuiltin(NAME+"."+URLENCODE, _URLEncode),
				URLDECODE: starlark.NewBuiltin(NAME+"."+URLDECODE, _URLDecode),
			},
		},
	}
}

// _Encode is standard base64, padded, which is RFC 4648 section 4.
//
// No trailing newline. Python's b2a_base64 appends one, and a script that did
// not want it would have to strip it before comparing anything.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation, as codec's b64encode
//   - 2026-09-23 23:20: a member of the base64 module
func _Encode(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	data, err := unpack.Data(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	return starlark.String(base64.StdEncoding.EncodeToString(data)), nil
}

// _Decode reads standard base64, padded or not.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation, as codec's b64decode
//   - 2026-09-23 23:20: a member of the base64 module
func _Decode(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	text, err := unpack.Text(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	return _Read(fn.Name(), text, base64.RawStdEncoding)
}

// _URLEncode is the URL and filename safe alphabet, unpadded, which is RFC
// 4648 section 5.
//
// Unpadded because what reaches for this alphabet is a JSON Web Token, whose
// segments carry no padding. A caller wanting padding has the standard form.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation, as codec's b64urlencode
//   - 2026-09-23 23:20: a member of the base64 module
func _URLEncode(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	data, err := unpack.Data(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	return starlark.String(base64.RawURLEncoding.EncodeToString(data)), nil
}

// _URLDecode reads the URL safe alphabet, padded or not.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation, as codec's b64urldecode
//   - 2026-09-23 23:20: a member of the base64 module
func _URLDecode(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	text, err := unpack.Text(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	return _Read(fn.Name(), text, base64.RawURLEncoding)
}

// _Read decodes text through encoding, with any padding taken off first.
//
// Padding off rather than required, because both spellings of one value are in
// the wild: a JSON Web Token's segments carry none, and an encoder that pads is
// the commoner one. A decoder that accepted only its own output would refuse
// half of what a script is handed.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation, as codec's _Decoded
//   - 2026-09-23 23:20: reads for the base64 module
func _Read(who string, text string, encoding *base64.Encoding) (starlark.Value, error) {
	data, err := encoding.DecodeString(strings.TrimRight(text, PADDING))
	if err != nil {
		return nil, fmt.Errorf("%s got %q: %w", who, text, ErrEncoded)
	}

	return starlark.Bytes(data), nil
}
