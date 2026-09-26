// These tests are table driven because the thing under test is a
// specification, and the rows are somebody else's: the check value every CRC catalogue publishes.
// A table written
// from the implementation would agree with it by construction.
//
// The base encodings' vectors moved with them, to packages of their own.
package codec_test

import (
	"testing"

	"github.com/thebagchi/lark/v1/runtime/plugin/codec"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// TestCRC32_MatchesTheCheckValue uses the string every CRC catalogue
// publishes a check value for, so this agrees with something outside itself.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation
func TestCRC32_MatchesTheCheckValue(t *testing.T) {
	_Check(t, []struct{ name, expression, want string }{
		{"the check value", `crc32("123456789")`, `3421780262`},
		{"of nothing", `crc32("")`, `0`},
		{"of bytes", `crc32(b"\x00")`, `3523407757`},
		{"continued in two parts", `crc32("56789", crc32("1234"))`, `3421780262`},
	})
}

// TestEncodings_RefuseWhatTheyCannotRead covers what each reader turns away,
// and that each reaches a sentinel a host can act on.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation
func TestEncodings_RefuseWhatTheyCannotRead(t *testing.T) {
	_Refuse(t, []struct {
		name       string
		expression string
		want       error
	}{
		{"crc32 of a number", `crc32(1)`, unpack.ErrData},
		{"crc32 continuing a negative", `crc32("x", -1)`, codec.ErrRange},
	})
}

// TestCRC32_RefusesASeedTooWideToBeAChecksum is what cutting a seed to width
// hid: a checksum is thirty-two bits, and 2**32 silently became 0, so a script
// continuing from a number it had mangled got the checksum of starting over.
//
// Revisions:
//   - 2026-09-24 16:15: initial creation
func TestCRC32_RefusesASeedTooWideToBeAChecksum(t *testing.T) {
	_Refuse(t, []struct {
		name       string
		expression string
		want       error
	}{
		{"one past the width", `crc32(b"abc", 4294967296)`, codec.ErrRange},
		{"far past it", `crc32(b"abc", 18446744073709551616)`, codec.ErrRange},
		{"negative", `crc32(b"abc", -1)`, codec.ErrRange},
	})

	_Check(t, []struct{ name, expression, want string }{
		{
			"the widest seed that fits still works",
			`crc32(b"abc", 4294967295)`,
			"899311407",
		},
		{"and the plain checksum is unchanged", `crc32(b"abc")`, "891568578"},
		{"and a seed of nothing is the plain checksum", `crc32(b"abc", 0)`, "891568578"},
	})
}
