// Package buf gives a script the module buf: a buffer to write bytes into a
// piece at a time, and a reader to take bytes apart the same way. Importing it
// is what enables it.
//
// A buffer, because a script cannot otherwise build bytes in pieces: Starlark's
// bytes has no join, and += copies everything written so far on every write.
//
// The methods are named as Go names them: every write starts with write_ and
// every read with read_, and an integer is written or read in 8, 16, 32 or 64
// bits, or in any width through write_uint, write_int, read_uint and read_int.
// Integers follow the integer module's rules, through its Encode and Decode, so
// the two never disagree; importing this enables integer as well. uint is
// unsigned and int two's complement, and the byte order is big unless "little"
// is named.
//
// A buffer and a reader are not data, so a store or an event refuses one as it
// refuses a handle. A spawn freezes either, and from then on each refuses what
// would change it with ErrFrozen - for a reader, every read, since a read
// moves it.
//
// Revisions:
//   - 2026-10-08 22:45: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:17: Buffer, Reader and module moved to internal/buffer (C91w8a)
package buf

import (
	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/buf/internal/buffer"
)

var (
	// ErrFrozen is returned for a change to a buffer or a reader that a spawn
	// has frozen.
	//
	// Revisions:
	//   - 2026-10-08 22:45: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-07 19:17: an alias of buffer.ErrFrozen (C91w8a)
	ErrFrozen = buffer.ErrFrozen

	// ErrShort is returned for a read of more bytes than a reader has left.
	//
	// Revisions:
	//   - 2026-10-08 22:45: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-07 19:17: an alias of buffer.ErrShort (C91w8a)
	ErrShort = buffer.ErrShort
)

// init registers this plugin, so that a host importing this package for its
// side effect is the whole of enabling it.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
func init() {
	plugin.Register(new(_Buf))
}

// _Buf is the plugin. Empty: each buffer and reader holds its own bytes.
//
// Revisions:
//   - 2026-10-08 22:45: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Buf struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-11-07 19:17: the name is buffer.NAME (C91w8a)
func (b *_Buf) Name() string {
	return buffer.NAME
}

// Values returns the buf module.
//
// Revisions:
//   - 2026-10-08 21:08: initial creation
//   - 2026-11-07 19:17: built by buffer.Values (C91w8a)
func (b *_Buf) Values() starlark.StringDict {
	return buffer.Values()
}
