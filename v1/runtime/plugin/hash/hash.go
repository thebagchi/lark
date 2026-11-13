// Package hash gives a script the digests, the keyed digest and a checksum.
// Importing it is what enables it.
//
// A module rather than flat names, for the reason base64 is one: sha256 says
// what it is anywhere, but a bare hmac does not, and a script reads better
// when the family is named once.
//
// Hex rather than bytes, because what a digest is compared against almost
// always arrives as text - a checksum file, a header, a column in a table. A
// script wanting the bytes decodes the hex, which is what hex.to_bytes is for.
//
// Upper case, as every hex the runtime writes, so a script compares two values
// it made without folding either. Text from outside is often lower case, and a
// script comparing a digest against it folds that side.
//
// The hex is written through the hex package, so importing this one enables
// hex as well.
//
// crc32 is the one sum given as an int rather than hex, because a script
// continues a checksum by handing the number back as the seed of the next
// call.
//
// md5 and sha1 are here and are not to be used for anything a reader must not
// forge. They are here because a script meeting an old checksum still has to
// read it, and a runtime that refused would just send the author somewhere
// worse.
//
// Revisions:
//   - 2026-09-24 17:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
package hash

import (
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/hex"
	"github.com/thebagchi/lark/v1/runtime/plugin/hash/internal/digest"
	"github.com/thebagchi/lark/v1/runtime/plugin/hash/internal/spec"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

var (
	// ErrAlgorithm is digest.ErrAlgorithm.
	//
	// Revisions:
	//   - 2026-11-10 01:37: initial creation (C120b, Q319)
	ErrAlgorithm = digest.ErrAlgorithm
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
//
// Revisions:
//   - 2026-09-24 17:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Hash struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-23 23:20: initial creation
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
func (h *_Hash) Name() string {
	return spec.NAME
}

// Values returns the hash module.
//
// Revisions:
//   - 2026-09-23 23:20: initial creation
//   - 2026-10-08 17:47: crc32, from codec
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 11:32: uses digest.HMAC and digest.CRC32, which moved to internal/digest
//     (C120b, Q315)
func (h *_Hash) Values() starlark.StringDict {
	return starlark.StringDict{
		spec.NAME: &starlarkstruct.Module{
			Name: spec.NAME,
			Members: starlark.StringDict{
				spec.MD5: _Digest(
					spec.MD5,
					_MD5Hex,
				),
				spec.SHA1: _Digest(
					spec.SHA1,
					_SHA1Hex,
				),
				spec.SHA256: _Digest(
					spec.SHA256,
					_SHA256Hex,
				),
				spec.SHA512: _Digest(
					spec.SHA512,
					_SHA512Hex,
				),
				spec.HMAC: starlark.NewBuiltin(
					spec.NAME+"."+spec.HMAC,
					digest.HMAC,
				),
				spec.CRC32: starlark.NewBuiltin(
					spec.NAME+"."+spec.CRC32,
					digest.CRC32,
				),
			},
		},
	}
}

// _Digest is the builtin for one unkeyed digest.
//
// Built from the name rather than written five times: the four differ in which
// sum they call and in nothing else, and five copies of one function is five
// places for one fix to be forgotten.
//
// The data is a view of the script's value, not a copy: a digest copies what
// it is given into a block of its own and keeps nothing of it, which is the
// contract of unpack.View.
//
// The sum is the one-shot function of the algorithm, not a digest that is
// built, written to, and asked for its sum. The one-shot form keeps its state
// on the stack, so the call allocates the text and the box around it, and
// nothing else. hmac stays streaming: it needs a digest to key.
//
// Revisions:
//   - 2026-09-23 23:20: initial creation
//   - 2026-10-29 01:08: reads a view of the data rather than a copy
//   - 2026-11-03 22:30: takes the one-shot sum of its algorithm, not a
//     constructor (C38a)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
func _Digest(
	name string,
	sum func([]byte) string,
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

			return starlark.String(sum(data)), nil
		},
	)
}

// _MD5Hex is the MD5 digest of data as upper-case hex.
//
// The digest is an array on the stack and hex.Encode only reads it, so the
// text is the one allocation. hex.Encode already hands its buffer over as the
// string, so unpack.Handover has nothing left to do here. The three functions
// that follow hold the same.
//
// Revisions:
//   - 2026-11-03 22:30: initial creation (C38a)
func _MD5Hex(data []byte) string {
	sum := md5.Sum(data)

	return hex.Encode(sum[:])
}

// _SHA1Hex is the SHA-1 digest of data as upper-case hex.
//
// Revisions:
//   - 2026-11-03 22:30: initial creation (C38a)
func _SHA1Hex(data []byte) string {
	sum := sha1.Sum(data)

	return hex.Encode(sum[:])
}

// _SHA256Hex is the SHA-256 digest of data as upper-case hex.
//
// Revisions:
//   - 2026-11-03 22:30: initial creation (C38a)
func _SHA256Hex(data []byte) string {
	sum := sha256.Sum256(data)

	return hex.Encode(sum[:])
}

// _SHA512Hex is the SHA-512 digest of data as upper-case hex.
//
// Revisions:
//   - 2026-11-03 22:30: initial creation (C38a)
func _SHA512Hex(data []byte) string {
	sum := sha512.Sum512(data)

	return hex.Encode(sum[:])
}
