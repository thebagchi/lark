// Package hash gives a script the digests and the keyed digest. Importing it
// is what enables it.
//
// A module rather than flat names, for the reason base64 is one: sha256 says
// what it is anywhere, but a bare hmac does not, and a script reads better
// when the family is named once.
//
// Hex rather than bytes, because what a digest is compared against almost
// always arrives as text - a checksum file, a header, a column in a table. A
// script wanting the bytes decodes the hex, which is what codec is for.
//
// md5 and sha1 are here and are not to be used for anything a reader must not
// forge. They are here because a script meeting an old checksum still has to
// read it, and a runtime that refused would just send the author somewhere
// worse.
package hash

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	stdhash "hash"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/runtime/plugin"
	"github.com/thebagchi/lark/runtime/plugin/unpack"
)

// ErrAlgorithm is returned for a keyed digest asked for under a name this does
// not know.
var ErrAlgorithm = errors.New("no such algorithm")

const (
	// NAME is the module, and the names it holds.
	NAME   = "hash"
	MD5    = "md5"
	SHA1   = "sha1"
	SHA256 = "sha256"
	SHA512 = "sha512"
	HMAC   = "hmac"

	// KEY and DATA are what hmac calls its arguments, so a script may pass
	// them either way round, and ALGORITHM which digest to key.
	ALGORITHM = "algorithm"
	KEY       = "key"
	DATA      = "data"
)

// init registers this plugin, so that a host importing this package for its
// side effect is the whole of enabling it.
//
// Revisions:
//   - 2026-09-23 23:20: initial creation
func init() {
	plugin.Register(new(_Hash))
}

// _Hash is the plugin. Empty: a digest works on what it is given.
type _Hash struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-23 23:20: initial creation
func (h *_Hash) Name() string {
	return NAME
}

// Values returns the hash module.
//
// Revisions:
//   - 2026-09-23 23:20: initial creation
func (h *_Hash) Values() starlark.StringDict {
	return starlark.StringDict{
		NAME: &starlarkstruct.Module{
			Name: NAME,
			Members: starlark.StringDict{
				MD5:    _Digest(MD5),
				SHA1:   _Digest(SHA1),
				SHA256: _Digest(SHA256),
				SHA512: _Digest(SHA512),
				HMAC:   starlark.NewBuiltin(NAME+"."+HMAC, _HMAC),
			},
		},
	}
}

// _Digest is the builtin for one unkeyed digest.
//
// Built from the name rather than written five times: the four differ in which
// constructor they call and in nothing else, and five copies of one function
// is five places for one fix to be forgotten.
//
// Revisions:
//   - 2026-09-23 23:20: initial creation
func _Digest(name string) *starlark.Builtin {
	return starlark.NewBuiltin(NAME+"."+name, func(
		thread *starlark.Thread,
		fn *starlark.Builtin,
		args starlark.Tuple,
		kwargs []starlark.Tuple,
	) (starlark.Value, error) {
		data, err := unpack.Data(fn, args, kwargs)
		if err != nil {
			return nil, err
		}

		digest, err := _Maker(name)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", fn.Name(), err)
		}

		return _Sum(fn.Name(), digest(), data)
	})
}

// _HMAC is the keyed digest: hmac(algorithm, key, data).
//
// The algorithm is named rather than there being one builtin per digest,
// because a script choosing between them chooses a string and a table of five
// near-identical names would be read as five different things.
//
// Returns ErrAlgorithm for a name this does not know.
//
// Revisions:
//   - 2026-09-23 23:20: initial creation
func _HMAC(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		algorithm string
		key       starlark.Value
		data      starlark.Value
	)

	err := starlark.UnpackArgs(fn.Name(), args, kwargs,
		ALGORITHM, &algorithm, KEY, &key, DATA, &data)
	if err != nil {
		return nil, err
	}

	secret, err := unpack.Bytes(fn.Name(), key)
	if err != nil {
		return nil, err
	}

	message, err := unpack.Bytes(fn.Name(), data)
	if err != nil {
		return nil, err
	}

	digest, err := _Maker(algorithm)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", fn.Name(), err)
	}

	return _Sum(fn.Name(), hmac.New(digest, secret), message)
}

// _Maker is the constructor one name asks for.
//
// The constructor rather than a built digest, because hmac takes one and
// calls it twice - so handing back the function is what lets an unknown name
// be refused once, before anything is built, without a second lookup that
// cannot report what it found.
//
// Revisions:
//   - 2026-09-23 23:20: initial creation
func _Maker(name string) (func() stdhash.Hash, error) {
	switch name {
	case MD5:
		return md5.New, nil

	case SHA1:
		return sha1.New, nil

	case SHA256:
		return sha256.New, nil

	case SHA512:
		return sha512.New, nil
	}

	return nil, fmt.Errorf("%q: %w", name, ErrAlgorithm)
}

// _Sum is data through made, as hex.
//
// Writing to a digest never fails - the interface carries an error because
// io.Writer does, and every implementation here returns nil - but ignoring one
// is forbidden, so it is checked rather than dropped.
//
// Revisions:
//   - 2026-09-23 23:20: initial creation
func _Sum(who string, made stdhash.Hash, data []byte) (starlark.Value, error) {
	_, err := made.Write(data)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", who, err)
	}

	return starlark.String(hex.EncodeToString(made.Sum(nil))), nil
}
