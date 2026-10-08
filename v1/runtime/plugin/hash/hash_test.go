// These tests are table driven because the thing under test is a
// specification, and the rows are somebody else's: the digest values RFC 1321
// and RFC 6234 publish for "abc" and the empty string, and RFC 4231's own HMAC
// vectors. A table written from the implementation would agree with it by
// construction. The documents print the digits in lower case; they are written
// here in upper case, as hash writes them.
package hash_test

import (
	"errors"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/dialect"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/hash"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// SCRIPT is what a failure calls the expression it was given.
const SCRIPT = "hash_test.star"

// _Eval evaluates one expression against the default environment.
//
// Revisions:
//   - 2026-09-23 23:20: initial creation
func _Eval(t *testing.T, expression string) (string, error) {
	t.Helper()

	env, err := plugin.Environment(plugin.DEFAULT)
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
//   - 2026-09-23 23:20: initial creation
func _Check(t *testing.T, cases []struct{ name, expression, want string }) {
	t.Helper()

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			got, err := _Eval(t, item.expression)
			if err != nil {
				t.Fatalf("%s: %v", item.expression, err)
			}

			if got != `"`+item.want+`"` {
				t.Fatalf("%s gave %s, want %q", item.expression, got, item.want)
			}
		})
	}
}

// _CheckInt runs a table of expression / expected pairs whose value is an int,
// as a checksum is, rather than the string a digest is.
//
// Revisions:
//   - 2026-10-08 17:52: initial creation
func _CheckInt(t *testing.T, cases []struct{ name, expression, want string }) {
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

// _Refuse runs a table of expressions that must fail with the sentinel given.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation, in codec
//   - 2026-10-08 17:52: moved here with crc32
func _Refuse(t *testing.T, cases []struct {
	name       string
	expression string
	want       error
}) {
	t.Helper()

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			_, err := _Eval(t, item.expression)
			if !errors.Is(err, item.want) {
				t.Fatalf("%s gave %v, want %v", item.expression, err, item.want)
			}
		})
	}
}

// TestDigest_MatchesThePublishedValues is the empty string and "abc" for each
// digest, which every specification publishes and every implementation agrees
// on.
//
// Revisions:
//   - 2026-09-23 23:20: initial creation
//   - 2026-10-08 18:06: the values in upper case, which hash now writes
func TestDigest_MatchesThePublishedValues(t *testing.T) {
	_Check(t, []struct{ name, expression, want string }{
		{"md5 of nothing", `hash.md5("")`, "D41D8CD98F00B204E9800998ECF8427E"},
		{"md5 of abc", `hash.md5("abc")`, "900150983CD24FB0D6963F7D28E17F72"},
		{"sha1 of nothing", `hash.sha1("")`, "DA39A3EE5E6B4B0D3255BFEF95601890AFD80709"},
		{"sha1 of abc", `hash.sha1("abc")`, "A9993E364706816ABA3E25717850C26C9CD0D89D"},
		{
			"sha256 of abc",
			`hash.sha256("abc")`,
			"BA7816BF8F01CFEA414140DE5DAE2223B00361A396177A9CB410FF61F20015AD",
		},
		{
			"sha512 of abc",
			`hash.sha512("abc")`,
			"DDAF35A193617ABACC417349AE20413112E6FA4E89A97EA20A9EEEE64B55D39A" +
				"2192992A274FC1A836BA3C23A3FEEBBD454D4423643CE80E2A9AC94FA54CA49F",
		},
		{
			"of bytes",
			`hash.sha256(b"abc")`,
			"BA7816BF8F01CFEA414140DE5DAE2223B00361A396177A9CB410FF61F20015AD",
		},
	})
}

// TestHMAC_MatchesRFC4231 is that document's first test case, which is the one
// every library is checked against.
//
// Revisions:
//   - 2026-09-23 23:20: initial creation
//   - 2026-10-08 18:06: the values in upper case, which hash now writes
func TestHMAC_MatchesRFC4231(t *testing.T) {
	_Check(t, []struct{ name, expression, want string }{
		{
			"sha256, case 1",
			`hash.hmac("sha256", b"\x0b" * 20, "Hi There")`,
			"B0344C61D8DB38535CA8AFCEAF0BF12B881DC200C9833DA726E9376C2E32CFF7",
		},
		{
			"sha256, case 2",
			`hash.hmac("sha256", "Jefe", "what do ya want for nothing?")`,
			"5BDCC146BF60754E6A042426089575C75A003F089D2739839DEC58B964EC3843",
		},
		{
			"named either way round",
			`hash.hmac(algorithm = "sha256", key = "Jefe", data = "what do ya want ` +
				`for nothing?")`,
			"5BDCC146BF60754E6A042426089575C75A003F089D2739839DEC58B964EC3843",
		},
	})
}

// TestHash_RefusesWhatItCannotDo records the two mistakes a caller can make.
//
// Revisions:
//   - 2026-09-23 23:20: initial creation
func TestHash_RefusesWhatItCannotDo(t *testing.T) {
	cases := []struct {
		name       string
		expression string
		want       error
	}{
		{"a digest of a number", `hash.sha256(1)`, unpack.ERR_DATA},
		{"an algorithm nobody has", `hash.hmac("sha3", "k", "d")`, hash.ERR_ALGORITHM},
		{"a key that is a number", `hash.hmac("sha256", 1, "d")`, unpack.ERR_DATA},
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
