// These tests are table driven because the thing under test is a
// specification, and the rows are somebody else's: RFC 4648's own vectors for
// base64 and base32, and the check value every CRC catalogue publishes. A
// table written from the implementation would agree with it by construction.
package codec_test

import (
	"testing"

	"github.com/thebagchi/lark/runtime/plugin/codec"
)

// TestBase64_MatchesRFC4648 is section 10 of the specification, which is the
// whole of the standard alphabet.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation
func TestBase64_MatchesRFC4648(t *testing.T) {
	_Check(t, []struct{ name, expression, want string }{
		{"empty", `b64encode("")`, `""`},
		{"one byte", `b64encode("f")`, `"Zg=="`},
		{"two", `b64encode("fo")`, `"Zm8="`},
		{"three", `b64encode("foo")`, `"Zm9v"`},
		{"four", `b64encode("foob")`, `"Zm9vYg=="`},
		{"five", `b64encode("fooba")`, `"Zm9vYmE="`},
		{"six", `b64encode("foobar")`, `"Zm9vYmFy"`},
		{"of bytes", `b64encode(b"\x00\xff")`, `"AP8="`},
		{"decode padded", `b64decode("Zm9vYmE=")`, `b"fooba"`},
		{"decode unpadded", `b64decode("Zm9vYmE")`, `b"fooba"`},
		{"decode empty", `b64decode("")`, `b""`},
		{"round trip", `b64decode(b64encode(b"\x00\xff"))`, `b"\x00\xff"`},
	})
}

// TestBase64URL_UsesTheOtherTwoCharacters is what separates the two
// alphabets, in the one input that shows both of them.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation
func TestBase64URL_UsesTheOtherTwoCharacters(t *testing.T) {
	_Check(t, []struct{ name, expression, want string }{
		{"standard", `b64encode(b"\xfb\xff\xbf")`, `"+/+/"`},
		{"url safe", `b64urlencode(b"\xfb\xff\xbf")`, `"-_-_"`},
		{"url is unpadded", `b64urlencode("f")`, `"Zg"`},
		{"url decode unpadded", `b64urldecode("Zg")`, `b"f"`},
		{"url decode padded", `b64urldecode("Zg==")`, `b"f"`},
		{"round trip", `b64urldecode(b64urlencode(b"\xfb\xff\xbf"))`, `b"\xfb\xff\xbf"`},
	})
}

// TestBase32_MatchesRFC4648 is the same section's base32 vectors.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation
func TestBase32_MatchesRFC4648(t *testing.T) {
	_Check(t, []struct{ name, expression, want string }{
		{"empty", `b32encode("")`, `""`},
		{"one byte", `b32encode("f")`, `"MY======"`},
		{"two", `b32encode("fo")`, `"MZXQ===="`},
		{"six", `b32encode("foobar")`, `"MZXW6YTBOI======"`},
		{"decode padded", `b32decode("MZXW6YTBOI======")`, `b"foobar"`},
		{"decode unpadded", `b32decode("MZXW6YTBOI")`, `b"foobar"`},
		{"round trip", `b32decode(b32encode(b"\x00\xff"))`, `b"\x00\xff"`},
	})
}

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
		{"base64 of a number", `b64encode(1)`, codec.ErrData},
		{"not base64", `b64decode("!!!!")`, codec.ErrEncoded},
		{"the url alphabet is not the standard one", `b64decode("-_-_")`, codec.ErrEncoded},
		{"not base32", `b32decode("1111")`, codec.ErrEncoded},
		{"crc32 of a number", `crc32(1)`, codec.ErrData},
		{"crc32 continuing a negative", `crc32("x", -1)`, codec.ErrRange},
	})
}
