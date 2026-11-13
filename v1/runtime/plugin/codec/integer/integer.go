// Package integer gives a script the module integer: an integer converted to
// and from its bytes. Importing it is what enables it.
//
// integer rather than int or bytes, because Starlark already gives those names
// to its own builtins, and a module of either name would hide one. The two
// functions read as Python's int.from_bytes and int.to_bytes, and take what
// those take: a byte order, big unless "little" is named, and signed = True for
// two's complement, unsigned otherwise. Any argument may be named.
//
// Encode, EncodeSigned, Decode and DecodeSigned are the conversions themselves,
// exported so buf writes and reads integers by these rules rather than by a
// second copy of them. The behavior lives in integer/internal/field; this
// package forwards to it. The data, common.Field, and the byte orders,
// common.BIG and common.LITTLE, are in package common.
//
// Revisions:
//   - 2026-10-08 22:45: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:17: thin forwards to integer/internal/field (C91w8b)
package integer

import (
	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/integer/internal/field"
)

// ErrOrder is returned for a byte order that is neither big nor little.
//
// Revisions:
//   - 2026-10-08 22:45: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:17: an alias of field.ErrOrder (C91w8b)
var (
	ErrOrder = field.ErrOrder
)

// init registers this plugin, so that a host importing this package for its
// side effect is the whole of enabling it.
//
// Revisions:
//   - 2026-10-08 18:42: initial creation
//   - 2026-11-07 19:17: registers field.New() (C91w8b)
func init() {
	plugin.Register(field.New())
}

// Unpack reads a builtin's arguments as a common.Field: an integer, a width, and
// a byte order, big unless a third argument names one.
//
// A nil fn is named "builtin" in an error.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-11-04 19:30: a nil builtin is named ANONYMOUS, not dereferenced (C87)
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-07 19:17: forwards to field.Unpack; the earlier revisions are there (C91w8b)
func Unpack(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (*common.Field, error) {
	return field.Unpack(
		fn,
		args,
		kwargs,
	)
}

// Encode is the field's number as unsigned bytes, as many as its width, in its
// order.
//
// Returns unpack.ErrRange for a width below one, a negative number, or a number
// too wide for the width, ErrOrder for an order that is neither big nor little.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as the body of codec's int2bytes
//   - 2026-11-07 19:17: forwards to field.Encode; the earlier revisions are there (C91w8b)
func Encode(
	who string,
	data *common.Field,
) ([]byte, error) {
	return field.Encode(
		who,
		data,
	)
}

// EncodeSigned is the field's number in two's complement, as many bytes as its
// width, in its order.
//
// Returns as Encode does, but for a negative number, which it writes.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-11-07 19:17: forwards to field.EncodeSigned; the earlier revisions are there (C91w8b)
func EncodeSigned(
	who string,
	data *common.Field,
) ([]byte, error) {
	return field.EncodeSigned(
		who,
		data,
	)
}

// Decode reads data as an unsigned integer, its bytes in order.
//
// Returns ErrOrder for an order that is neither big nor little.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, as the body of codec's bytes2int
//   - 2026-11-07 19:17: forwards to field.Decode; the earlier revisions are there (C91w8b)
func Decode(
	who string,
	data []byte,
	order string,
) (starlark.Int, error) {
	return field.Decode(
		who,
		data,
		order,
	)
}

// DecodeSigned reads data as an integer in two's complement, its bytes in
// order.
//
// Returns ErrOrder for an order that is neither big nor little.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-11-07 19:17: forwards to field.DecodeSigned; the earlier revisions are there (C91w8b)
func DecodeSigned(
	who string,
	data []byte,
	order string,
) (starlark.Int, error) {
	return field.DecodeSigned(
		who,
		data,
		order,
	)
}
