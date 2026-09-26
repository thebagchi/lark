// These tests are table driven because the thing under test is a
// specification, and the rows are somebody else's: the digest values RFC 1321
// and RFC 6234 publish for "abc" and the empty string, and RFC 4231's own HMAC
// vectors. A table written from the implementation would agree with it by
// construction.
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

// TestDigest_MatchesThePublishedValues is the empty string and "abc" for each
// digest, which every specification publishes and every implementation agrees
// on.
//
// Revisions:
//   - 2026-09-23 23:20: initial creation
func TestDigest_MatchesThePublishedValues(t *testing.T) {
	_Check(t, []struct{ name, expression, want string }{
		{"md5 of nothing", `hash.md5("")`, "d41d8cd98f00b204e9800998ecf8427e"},
		{"md5 of abc", `hash.md5("abc")`, "900150983cd24fb0d6963f7d28e17f72"},
		{"sha1 of nothing", `hash.sha1("")`, "da39a3ee5e6b4b0d3255bfef95601890afd80709"},
		{"sha1 of abc", `hash.sha1("abc")`, "a9993e364706816aba3e25717850c26c9cd0d89d"},
		{
			"sha256 of abc",
			`hash.sha256("abc")`,
			"ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
		},
		{
			"sha512 of abc",
			`hash.sha512("abc")`,
			"ddaf35a193617abacc417349ae20413112e6fa4e89a97ea20a9eeee64b55d39a" +
				"2192992a274fc1a836ba3c23a3feebbd454d4423643ce80e2a9ac94fa54ca49f",
		},
		{
			"of bytes",
			`hash.sha256(b"abc")`,
			"ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad",
		},
	})
}

// TestHMAC_MatchesRFC4231 is that document's first test case, which is the one
// every library is checked against.
//
// Revisions:
//   - 2026-09-23 23:20: initial creation
func TestHMAC_MatchesRFC4231(t *testing.T) {
	_Check(t, []struct{ name, expression, want string }{
		{
			"sha256, case 1",
			`hash.hmac("sha256", b"\x0b" * 20, "Hi There")`,
			"b0344c61d8db38535ca8afceaf0bf12b881dc200c9833da726e9376c2e32cff7",
		},
		{
			"sha256, case 2",
			`hash.hmac("sha256", "Jefe", "what do ya want for nothing?")`,
			"5bdcc146bf60754e6a042426089575c75a003f089d2739839dec58b964ec3843",
		},
		{
			"named either way round",
			`hash.hmac(algorithm = "sha256", key = "Jefe", data = "what do ya want for nothing?")`,
			"5bdcc146bf60754e6a042426089575c75a003f089d2739839dec58b964ec3843",
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
		{"a digest of a number", `hash.sha256(1)`, unpack.ErrData},
		{"an algorithm nobody has", `hash.hmac("sha3", "k", "d")`, hash.ErrAlgorithm},
		{"a key that is a number", `hash.hmac("sha256", 1, "d")`, unpack.ErrData},
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
