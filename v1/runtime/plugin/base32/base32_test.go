// These tests are table driven because the thing under test is a
// specification, and the rows are RFC 4648's own vectors. A table written from
// the implementation would agree with it by construction.
package base32_test

import (
	"errors"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/dialect"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/base32"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// SCRIPT is what a failure calls the expression it was given.
const SCRIPT = "base32_test.star"

// _Eval evaluates one expression against the default environment.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
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

// TestEncode_MatchesRFC4648 is that document's base32 vectors.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation, in codec
//   - 2026-09-24 00:53: moved here, spelled as members of the base32 module
func TestEncode_MatchesRFC4648(t *testing.T) {
	cases := []struct{ name, expression, want string }{
		{"empty", `base32.encode("")`, `""`},
		{"one", `base32.encode("f")`, `"MY======"`},
		{"two", `base32.encode("fo")`, `"MZXQ===="`},
		{"three", `base32.encode("foo")`, `"MZXW6==="`},
		{"four", `base32.encode("foob")`, `"MZXW6YQ="`},
		{"five", `base32.encode("fooba")`, `"MZXW6YTB"`},
		{"six", `base32.encode("foobar")`, `"MZXW6YTBOI======"`},
		{"decode padded", `base32.decode("MZXW6YTBOI======")`, `b"foobar"`},
		{"decode unpadded", `base32.decode("MZXW6YTBOI")`, `b"foobar"`},
		{"round trip", `base32.decode(base32.encode(b"\x00\xff"))`, `b"\x00\xff"`},
	}

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

// TestBase32_RefusesWhatItCannotRead records the two mistakes a caller can
// make.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func TestBase32_RefusesWhatItCannotRead(t *testing.T) {
	cases := []struct {
		name       string
		expression string
		want       error
	}{
		{"of a number", `base32.encode(1)`, unpack.ErrData},
		{"not base32", `base32.decode("1111")`, base32.ErrEncoded},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			_, err := _Eval(t, item.expression)
			if !errors.Is(err, item.want) {
				t.Fatalf("%s gave %v, want %v", item.expression, err, item.want)
			}
		})
	}
}
