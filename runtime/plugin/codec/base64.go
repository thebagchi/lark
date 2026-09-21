package codec

import (
	"encoding/base64"
	"fmt"
	"strings"

	"go.starlark.net/starlark"
)

// _B64Encode is standard base64, padded, which is RFC 4648 section 4.
//
// No trailing newline. Python's b2a_base64 appends one, and a script that did
// not want it would have to strip it before comparing anything.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation
func _B64Encode(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	data, err := _Data(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	return starlark.String(base64.StdEncoding.EncodeToString(data)), nil
}

// _B64Decode reads standard base64, padded or not.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation
func _B64Decode(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	text, err := _Text(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	return _Decoded(fn.Name(), text, base64.RawStdEncoding)
}

// _B64URLEncode is the URL and filename safe alphabet, unpadded, which is RFC
// 4648 section 5.
//
// Unpadded because what reaches for this alphabet is a JSON Web Token, whose
// segments carry no padding. A caller wanting padding has the standard form.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation
func _B64URLEncode(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	data, err := _Data(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	return starlark.String(base64.RawURLEncoding.EncodeToString(data)), nil
}

// _B64URLDecode reads the URL safe alphabet, padded or not.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation
func _B64URLDecode(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	text, err := _Text(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	return _Decoded(fn.Name(), text, base64.RawURLEncoding)
}

// _Decoded reads text through encoding, with any padding taken off first.
//
// Padding off rather than required, because both spellings of one value are in
// the wild: a JSON Web Token's segments carry none, and an encoder that pads is
// the commoner one. A decoder that accepted only its own output would refuse
// half of what a script is handed.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation
func _Decoded(who string, text string, encoding *base64.Encoding) (starlark.Value, error) {
	data, err := encoding.DecodeString(strings.TrimRight(text, PADDING))
	if err != nil {
		return nil, fmt.Errorf("%s got %q: %w", who, text, ErrEncoded)
	}

	return starlark.Bytes(data), nil
}
