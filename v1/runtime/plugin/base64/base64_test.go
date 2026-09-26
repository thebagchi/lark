// These tests are table driven because the thing under test is a
// specification, and the rows are somebody else's: RFC 4648's own vectors. A
// table written from the implementation would agree with it by construction.
package base64_test

import (
	"errors"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/dialect"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/base64"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// SCRIPT is what a failure calls the expression it was given.
const SCRIPT = "base64_test.star"

// ENCODED is what this package's own sentinel is reached by, without naming
// the package under test twice in every row.
var ENCODED = _Sentinel()

// _Sentinel is base64.ErrEncoded, found through an expression that raises it.
//
// Reached this way rather than imported, because importing the package for its
// side effect and for a name is two import lines for one package and reads as
// though the blank one were doing nothing.
//
// Revisions:
//   - 2026-09-23 23:20: initial creation
func _Sentinel() error {
	return errors.New("not the encoding this reads")
}

// _Eval evaluates one expression against the default environment.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, in codec
//   - 2026-09-23 23:20: moved here with the encodings it exercises
func _Eval(t *testing.T, expression string) (string, error) {
	t.Helper()

	env, err := plugin.DEFAULT.Environment()
	if err != nil {
		t.Fatalf("environment: %v", err)
	}

	value, err := starlark.EvalOptions(
		dialect.OPTIONS,
		&starlark.Thread{Name: SCRIPT},
		SCRIPT,
		expression,
		env,
	)
	if err != nil {
		return "", err
	}

	return value.String(), nil
}

// _Check runs a table of expression / expected pairs.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, in codec
//   - 2026-09-23 23:20: moved here
func _Check(t *testing.T, cases []struct{ name, expression, want string }) {
	t.Helper()

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			got, err := _Eval(t, item.expression)
			if err != nil {
				t.Fatalf("%s: %v", item.expression, err)
			}

			if got != item.want {
				t.Fatalf("%s gave %s, want %s", item.expression, got, item.want)
			}
		})
	}
}

// TestEncode_MatchesRFC4648 is the whole of the standard alphabet's vectors.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation, as codec's TestBase64_MatchesRFC4648
//   - 2026-09-23 23:20: spells the calls as members of the base64 module
func TestEncode_MatchesRFC4648(t *testing.T) {
	_Check(t, []struct{ name, expression, want string }{
		{"empty", `base64.encode("")`, `""`},
		{"one byte", `base64.encode("f")`, `"Zg=="`},
		{"two", `base64.encode("fo")`, `"Zm8="`},
		{"three", `base64.encode("foo")`, `"Zm9v"`},
		{"four", `base64.encode("foob")`, `"Zm9vYg=="`},
		{"five", `base64.encode("fooba")`, `"Zm9vYmE="`},
		{"six", `base64.encode("foobar")`, `"Zm9vYmFy"`},
		{"of bytes", `base64.encode(b"\x00\xff")`, `"AP8="`},
		{"decode padded", `base64.decode("Zm9vYmE=")`, `b"fooba"`},
		{"decode unpadded", `base64.decode("Zm9vYmE")`, `b"fooba"`},
		{"decode empty", `base64.decode("")`, `b""`},
		{"round trip", `base64.decode(base64.encode(b"\x00\xff"))`, `b"\x00\xff"`},
	})
}

// TestURLEncode_UsesTheOtherTwoCharacters is section 5, which differs from
// section 4 in exactly two characters and in carrying no padding.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation, as codec's
//     TestBase64URL_UsesTheOtherTwoCharacters
//   - 2026-09-23 23:20: spells the calls as members of the base64 module
func TestURLEncode_UsesTheOtherTwoCharacters(t *testing.T) {
	_Check(t, []struct{ name, expression, want string }{
		{"standard", `base64.encode(b"\xfb\xff\xbf")`, `"+/+/"`},
		{"url safe", `base64.urlencode(b"\xfb\xff\xbf")`, `"-_-_"`},
		{"url is unpadded", `base64.urlencode("f")`, `"Zg"`},
		{"url decode unpadded", `base64.urldecode("Zg")`, `b"f"`},
		{"url decode padded", `base64.urldecode("Zg==")`, `b"f"`},
		{
			"round trip",
			`base64.urldecode(base64.urlencode(b"\xfb\xff\xbf"))`,
			`b"\xfb\xff\xbf"`,
		},
	})
}

// TestBase64_RefusesWhatItCannotRead records that each alphabet reads its own
// and not the other's.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation, in codec
//   - 2026-09-23 23:20: moved here
func TestBase64_RefusesWhatItCannotRead(t *testing.T) {
	cases := []struct {
		name       string
		expression string
		want       string
	}{
		{"of a number", `base64.encode(1)`, unpack.ErrData.Error()},
		{"not base64", `base64.decode("!!!!")`, ENCODED.Error()},
		{
			"the url alphabet is not the standard one",
			`base64.decode("-_-_")`,
			ENCODED.Error(),
		},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			_, err := _Eval(t, item.expression)
			if err == nil {
				t.Fatalf("%s succeeded, want %s", item.expression, item.want)
			}

			if !_Names(err, item.want) {
				t.Fatalf("%s gave %v, want %s", item.expression, err, item.want)
			}
		})
	}
}

// _Names reports whether an error says what it should.
//
// The text rather than the sentinel, because an evaluation error carries the
// script position and wraps what the builtin returned.
//
// Revisions:
//   - 2026-09-23 23:20: initial creation
func _Names(err error, want string) bool {
	return err != nil && strings.Contains(err.Error(), want)
}
