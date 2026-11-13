package digest

import (
	"crypto/hmac"
	"crypto/md5"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"errors"
	"fmt"
	stdhash "hash"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/codec/hex"
	"github.com/thebagchi/lark/v1/runtime/plugin/hash/internal/spec"
	shared "github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// ErrAlgorithm is returned for a keyed digest asked for under a name this does
// not know.
//
// Revisions:
//   - 2026-09-24 17:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/hash (C120b, Q315)
var (
	ErrAlgorithm = errors.New("no such algorithm")
)

// HMAC is the keyed digest: hmac(algorithm, key, data).
//
// The algorithm is named rather than there being one builtin per digest,
// because a script choosing between them chooses a string and a table of five
// near-identical names would be read as five different things.
//
// Returns ErrAlgorithm for a name this does not know.
//
// Revisions:
//   - 2026-09-23 23:20: initial creation
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/hash and renamed _HMAC to HMAC, for the
//     tests in tests/ (C120b, Q315, Q314)
func HMAC(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	algorithm, key, data, err := HMACArgs(
		fn,
		args,
		kwargs,
	)
	if err != nil {
		return nil, err
	}

	secret, message, err := HMACBytes(
		fn,
		key,
		data,
	)
	if err != nil {
		return nil, err
	}

	digest, err := _Maker(algorithm)
	if err != nil {
		return nil, fmt.Errorf(
			"%s: %w",
			shared.Called(fn),
			err,
		)
	}

	return _Sum(
		shared.Called(fn),
		hmac.New(
			digest,
			secret,
		),
		message,
	)
}

// HMACArgs reads the algorithm, the key, and the message.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/hash and renamed _HMACArgs to HMACArgs,
//     for the tests in tests/ (C120b, Q315, Q314)
func HMACArgs(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (string, starlark.Value, starlark.Value, error) {
	var (
		algorithm string
		key       starlark.Value
		data      starlark.Value
	)

	err := starlark.UnpackArgs(
		shared.Called(fn),
		args,
		kwargs,
		spec.ALGORITHM,
		&algorithm,
		spec.KEY,
		&key,
		spec.DATA,
		&data,
	)
	if err != nil {
		return "", nil, nil, err
	}

	return algorithm, key, data, nil
}

// HMACBytes reads the key and the message as bytes.
//
// Each is a view of the script's value, not a copy. hmac.New copies the key
// into its pads, or hashes a long key first, and keeps only its length; the
// message goes to a digest, which keeps nothing of it. That is the contract of
// unpack.View.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-10-29 01:08: reads a view of the key and the message rather than a
//     copy
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/hash and renamed _HMACBytes to HMACBytes,
//     for the tests in tests/ (C120b, Q315, Q314)
func HMACBytes(
	fn *starlark.Builtin,
	key starlark.Value,
	data starlark.Value,
) ([]byte, []byte, error) {
	secret, err := unpack.View(
		shared.Called(fn),
		key,
	)
	if err != nil {
		return nil, nil, err
	}

	message, err := unpack.View(
		shared.Called(fn),
		data,
	)
	if err != nil {
		return nil, nil, err
	}

	return secret, message, nil
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
//   - 2026-11-02 10:58: bound the script text in the error (C21)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/hash (C120b, Q315)
func _Maker(name string) (func() stdhash.Hash, error) {
	switch name {
	case spec.MD5:
		return md5.New, nil

	case spec.SHA1:
		return sha1.New, nil

	case spec.SHA256:
		return sha256.New, nil

	case spec.SHA512:
		return sha512.New, nil
	}

	return nil, fmt.Errorf(
		"%s: %w",
		unpack.QuotePrefix(name),
		ErrAlgorithm,
	)
}

// _Sum is data through made, as upper-case hex.
//
// Writing to a digest never fails - the interface carries an error because
// io.Writer does, and every implementation here returns nil - but ignoring one
// is forbidden, so it is checked rather than dropped.
//
// Revisions:
//   - 2026-09-23 23:20: initial creation
//   - 2026-10-08 18:06: upper case, as every hex the runtime writes
//   - 2026-10-29 01:08: writes through hex.Encode rather than fmt's %X
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/hash (C120b, Q315)
func _Sum(
	who string,
	made stdhash.Hash,
	data []byte,
) (starlark.Value, error) {
	_, err := made.Write(data)
	if err != nil {
		return nil, fmt.Errorf(
			"%s: %w",
			who,
			err,
		)
	}

	return starlark.String(hex.Encode(made.Sum(nil))), nil
}
