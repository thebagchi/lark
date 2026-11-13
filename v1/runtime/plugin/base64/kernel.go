package base64

import (
	"encoding/base64"

	"github.com/thebagchi/lark/v1/runtime/plugin/base64/internal/codec"
	"github.com/thebagchi/lark/v1/runtime/plugin/base64/internal/spec"
	"github.com/thebagchi/lark/v1/runtime/plugin/internal/cpu"
)

// _NewCoding is the coding of a builtin: the CPU's level, and the sizes that
// ship.
//
// Revisions:
//   - 2026-11-03 22:40: initial creation (C51)
//   - 2026-11-08 00:59: names read from internal/spec (C91w12c, Q228)
//   - 2026-11-10 11:32: uses codec.Coding, which moved to internal/codec (C120b, Q315)
func _NewCoding(
	alphabet int,
	encoding *base64.Encoding,
) *codec.Coding {
	return &codec.Coding{
		Level:    cpu.LEVEL,
		Sizes:    &spec.SIZES,
		Alphabet: alphabet,
		Encoding: encoding,
	}
}
