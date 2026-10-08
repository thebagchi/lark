// These tests are table driven because the thing under test is a
// specification, and the rows are somebody else's: the check value every CRC
// catalogue publishes. A table written from the implementation would agree with
// it by construction.
package hash_test

import (
	"testing"

	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// TestCRC32_MatchesTheCheckValue uses the string every CRC catalogue
// publishes a check value for, so this agrees with something outside itself.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation, in codec
//   - 2026-10-08 17:52: spells the calls as members of the hash module
func TestCRC32_MatchesTheCheckValue(t *testing.T) {
	_CheckInt(t, []struct{ name, expression, want string }{
		{"the check value", `hash.crc32("123456789")`, `3421780262`},
		{"of nothing", `hash.crc32("")`, `0`},
		{"of bytes", `hash.crc32(b"\x00")`, `3523407757`},
		{
			"continued in two parts",
			`hash.crc32("56789", hash.crc32("1234"))`,
			`3421780262`,
		},
	})
}

// TestCRC32_RefusesWhatItCannotRead covers what crc32 turns away, and that
// each reaches a sentinel a host can act on.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation, as codec's
//     TestEncodings_RefuseWhatTheyCannotRead
//   - 2026-10-08 17:52: the crc32 rows alone, spelt as members of the hash module
func TestCRC32_RefusesWhatItCannotRead(t *testing.T) {
	_Refuse(t, []struct {
		name       string
		expression string
		want       error
	}{
		{"of a number", `hash.crc32(1)`, unpack.ERR_DATA},
		{"continuing a negative", `hash.crc32("x", -1)`, unpack.ERR_RANGE},
	})
}

// TestCRC32_RefusesASeedTooWideToBeAChecksum is what cutting a seed to width
// hid: a checksum is thirty-two bits, and 2**32 silently became 0, so a script
// continuing from a number it had mangled got the checksum of starting over.
//
// Revisions:
//   - 2026-09-24 16:15: initial creation, in codec
//   - 2026-10-08 17:52: spells the calls as members of the hash module
func TestCRC32_RefusesASeedTooWideToBeAChecksum(t *testing.T) {
	_Refuse(t, []struct {
		name       string
		expression string
		want       error
	}{
		{"one past the width", `hash.crc32(b"abc", 4294967296)`, unpack.ERR_RANGE},
		{"far past it", `hash.crc32(b"abc", 18446744073709551616)`, unpack.ERR_RANGE},
		{"negative", `hash.crc32(b"abc", -1)`, unpack.ERR_RANGE},
	})

	_CheckInt(t, []struct{ name, expression, want string }{
		{
			"the widest seed that fits still works",
			`hash.crc32(b"abc", 4294967295)`,
			"899311407",
		},
		{"and the plain checksum is unchanged", `hash.crc32(b"abc")`, "891568578"},
		{
			"and a seed of nothing is the plain checksum",
			`hash.crc32(b"abc", 0)`,
			"891568578",
		},
	})
}
