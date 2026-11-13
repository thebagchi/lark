package tests

// These tests pin what json.decode accepts beyond RFC 8259. The module is
// go.starlark.net's, unchanged, and lark documents the extensions rather than
// wrapping it in a second parser: RFC 8259 section 9 says "A JSON parser MAY
// accept non-JSON forms or extensions". A change in the library that closes one
// of these shows here, and the reader decides again.
//
// Revisions:
//   - 2026-11-02 16:29: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)

import (
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/json"
)

const (
	SCRIPT = "json_test.star"
)

// TestJSON_DecodeKeepsTheLibrarysExtensions lists each accepted form with its
// clause. Every row passed before C22; a failure is a change in the library.
//
// Revisions:
//   - 2026-11-02 16:45: initial creation (C22, RFC 8259 sections 4, 6, 7, 8.2 and 9)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestJSON_DecodeKeepsTheLibrarysExtensions(t *testing.T) {
	cases := []struct {
		name       string
		expression string
		want       string
	}{
		{
			// Section 7: control characters MUST be escaped; section 9 lets a
			// parser accept the raw form. The library does so in a string of
			// ASCII only.
			"raw newline in an ascii string",
			`json.decode('"a\nb"')`,
			`"a\nb"`,
		},
		{
			// Section 6: frac = decimal-point 1*DIGIT, so "1." is not a number.
			"number with a bare point",
			`json.decode("[1.]")`,
			`[1.0]`,
		},
		{
			"number with a bare point and exponent",
			`json.decode("[1.e3]")`,
			`[1000.0]`,
		},
		{
			// Section 4: names SHOULD be unique; many parsers keep the last.
			"duplicate keys keep the last",
			`json.decode('{"a": 1, "a": 2}')`,
			`{"a": 2}`,
		},
		{
			// Section 8.2: the grammar allows an unpaired surrogate and the
			// receiver's behavior is unpredictable. The library writes U+FFFD.
			"lone surrogate",
			`json.decode('"\\ud800"')`,
			"\"�\"",
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				env, err := plugin.Environment(plugin.Registered())
				if err != nil {
					t.Fatalf(
						"environment: %v",
						err,
					)
				}

				value, err := starlark.EvalOptions(
					common.OPTIONS,
					&starlark.Thread{
						Name: SCRIPT,
					},
					SCRIPT,
					item.expression,
					env,
				)
				if err != nil {
					t.Fatalf(
						"%s: %v",
						item.expression,
						err,
					)
				}

				if value.String() != item.want {
					t.Fatalf(
						"%s gave %s, want %s",
						item.expression,
						value.String(),
						item.want,
					)
				}
			},
		)
	}
}

// TestJSON_DecodeLeavesOddCallsToTheLibrary proves that a call with no text, or
// with something that is not text, passes the depth check and gets the
// library's own refusal.
//
// Revisions:
//   - 2026-11-04 21:59: initial creation (C59)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestJSON_DecodeLeavesOddCallsToTheLibrary(t *testing.T) {
	cases := []struct {
		name       string
		expression string
		want       string
	}{
		{
			"no argument",
			`json.decode()`,
			"json.decode: missing argument for x",
		},
		{
			"an int",
			`json.decode(1)`,
			"json.decode: for parameter x: got int, want string",
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				env, err := plugin.Environment(plugin.Registered())
				if err != nil {
					t.Fatalf(
						"environment: %v",
						err,
					)
				}

				_, err = starlark.EvalOptions(
					common.OPTIONS,
					&starlark.Thread{
						Name: SCRIPT,
					},
					SCRIPT,
					item.expression,
					env,
				)
				if err == nil || err.Error() != item.want {
					t.Fatalf(
						"%s gave %v, want %q",
						item.expression,
						err,
						item.want,
					)
				}
			},
		)
	}
}
