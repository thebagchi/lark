package hex

import (
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/hex/internal/codec"
)

// Encode calls codec.Encode.
//
// Revisions:
//   - 2026-11-10 01:37: initial creation (C120b, Q319)
func Encode(data []byte) string {
	return codec.Encode(data)
}
