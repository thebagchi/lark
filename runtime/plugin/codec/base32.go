package codec

import (
	"encoding/base32"
	"fmt"
	"strings"

	"go.starlark.net/starlark"
)

// _B32Encode is RFC 4648 base32, padded.
//
// Not one of binascii's, and here anyway: it is what a one-time password
// secret and some DNS records are written in, and it is one call over the
// standard library.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation
func _B32Encode(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	data, err := _Data(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	return starlark.String(base32.StdEncoding.EncodeToString(data)), nil
}

// _B32Decode reads base32, padded or not, by the rule the base64 readers
// follow.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation
func _B32Decode(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	text, err := _Text(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	bare := base32.StdEncoding.WithPadding(base32.NoPadding)

	data, err := bare.DecodeString(strings.TrimRight(text, PADDING))
	if err != nil {
		return nil, fmt.Errorf("%s got %q: %w", fn.Name(), text, ErrEncoded)
	}

	return starlark.Bytes(data), nil
}
