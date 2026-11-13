package tests

import (
	"errors"
	"strings"
	"testing"

	"github.com/thebagchi/lark/cmd/lark/internal/command"
	"github.com/thebagchi/lark/v1/runtime"
)

const (
	// TAGGED is the hint that goes with the refusal of a flow of plain values (Q433).
	//
	// Revisions:
	//   - 2026-11-13 17:39: initial creation (C133, Q433)
	TAGGED = "(a value is an object named for its kind, " +
		"such as {\"string\": \"x\"} or {\"int\": \"3\"})"
)

// TestGenerated_RefusesAFlowOfPlainValuesBounded proves a flow written before its
// values were tagged is refused (Q422) with words that say what a value is now,
// and that the words stay short when the old value is long: the decoder quotes a
// token as long as the token is, and the refusal cuts it to 100 bytes (Q4).
//
// The check: go test -race -count=1 -run TestGenerated_RefusesAFlowOfPlainValuesBounded
// ./cmd/lark/tests
//
// Revisions:
//   - 2026-11-13 10:15: initial creation (C132, Q4, Q422)
//   - 2026-11-13 17:52: the hint is the local TAGGED, as runtime.Read holds the decoding
//     (C133, Q441)
func TestGenerated_RefusesAFlowOfPlainValuesBounded(t *testing.T) {
	for _, tc := range []struct {
		name string
		flow string
		hint bool
	}{
		{
			"a string",
			`{"main": {"statement": [{"call": {"function": "f", "args": ["west"]}}]}}`,
			true,
		},
		{
			"a number",
			`{"constants": {"X": {"value": 3}}, "text": "pass"}`,
			true,
		},
		{
			"a list",
			`{"constants": {"X": {"value": [1, 2]}}, "text": "pass"}`,
			true,
		},
		{
			"a string of five thousand bytes",
			`{"constants": {"X": {"value": "` + strings.Repeat(
				"a",
				5000,
			) + `"}}, "text": "pass"}`,
			true,
		},
		{
			"a field of five thousand bytes the schema does not know",
			`{"` + strings.Repeat(
				"q",
				5000,
			) + `": 1}`,
			false,
		},
		{
			"text that is no JSON",
			"not json\n",
			false,
		},
	} {
		t.Run(
			tc.name,
			func(t *testing.T) {
				_, err := command.Generated(
					"old.json",
					[]byte(tc.flow),
				)
				if err == nil {
					t.Fatal("want an error")
				}

				const BOUND = 400

				if len(err.Error()) > BOUND {
					t.Fatalf(
						"the error is %d bytes, want at most %d",
						len(err.Error()),
						BOUND,
					)
				}

				if got := strings.Contains(
					err.Error(),
					TAGGED,
				); got != tc.hint {
					t.Fatalf(
						"hint = %v, want %v: %s",
						got,
						tc.hint,
						err,
					)
				}
			},
		)
	}
}

// TestGenerated_WritesTheScriptOfATaggedFlow proves a flow whose values are
// tagged reads, and the script it generates holds each value as a script
// would write it: the largest int64, a float that is whole, and bytes that are
// not UTF-8 (Q407, Q421, Q422).
//
// The check: go test -race -count=1 -run TestGenerated_WritesTheScriptOfATaggedFlow
// ./cmd/lark/tests
//
// Revisions:
//   - 2026-11-13 10:30: initial creation (C132, Q407, Q421, Q422)
func TestGenerated_WritesTheScriptOfATaggedFlow(t *testing.T) {
	const FLOW = `
{
  "functions": [{"name": "f", "params": ["a", "b", "c"], "body": "pass"}],
  "main": {"statement": [{"call": {"function": "f", "args": [
    {"int": "9223372036854775807"},
    {"float": 1},
    {"bytes": "/w=="}
  ]}}]}
}
`

	out, err := command.Generated(
		"tagged.json",
		[]byte(FLOW),
	)
	if err != nil {
		t.Fatal(err)
	}

	const WANT = `f(9223372036854775807, 1.0, b"\xff")`

	if !strings.Contains(
		string(out),
		WANT,
	) {
		t.Fatalf(
			"want %s in\n%s",
			WANT,
			out,
		)
	}
}

// TestGenerated_RefusesAnIntOutsideInt64Bounded proves a flow with an int outside
// int64, or one that is no whole number, is refused by the decoder (Q438, Q440), with
// words that stay short when the int is long (Q4), no hint for a plain value, and an
// error that matches runtime.ErrForm.
//
// The check: go test -race -count=1 -run TestGenerated_RefusesAnIntOutsideInt64Bounded
// ./cmd/lark/tests
//
// Revisions:
//   - 2026-11-13 12:59: initial creation (C132, Q4, Q438, Q440)
//   - 2026-11-13 17:52: the hint is the local TAGGED, as runtime.Read holds the decoding
//     (C133, Q441)
func TestGenerated_RefusesAnIntOutsideInt64Bounded(t *testing.T) {
	for _, tc := range []struct{ name, value string }{
		{
			"one past the largest",
			`"9223372036854775808"`,
		},
		{
			"one past the smallest",
			`"-9223372036854775809"`,
		},
		{
			"2^64+1",
			`"18446744073709551617"`,
		},
		{
			"a fraction",
			`"1.5"`,
		},
		{
			"hex",
			`"0x10"`,
		},
		{
			"script text",
			`"1); import_os("`,
		},
		{
			"five thousand digits",
			`"` + strings.Repeat(
				"7",
				5000,
			) + `"`,
		},
		{
			"five thousand digits as a number",
			strings.Repeat(
				"7",
				5000,
			),
		},
	} {
		t.Run(
			tc.name,
			func(t *testing.T) {
				_, err := command.Generated(
					"big.json",
					[]byte(`{"constants": {"X": {"value": `+
						`{"int": `+tc.value+`}}}, "text": "pass"}`),
				)
				if err == nil {
					t.Fatal("want an error")
				}

				if len(err.Error()) > 400 {
					t.Fatalf(
						"the error is %d bytes, want at most 400",
						len(err.Error()),
					)
				}

				if !errors.Is(
					err,
					runtime.ErrForm,
				) {
					t.Fatalf(
						"%v does not match %v",
						err,
						runtime.ErrForm,
					)
				}

				if strings.Contains(
					err.Error(),
					TAGGED,
				) {
					t.Fatalf(
						"a bad int has a hint: %s",
						err,
					)
				}
			},
		)
	}
}

// TestGenerated_RefusesANullWhereAValueStands proves a JSON null where a value is
// expected is refused with the hint, whatever field holds the value, and that a null
// kind that holds anything but null is out of range (Q439). protojson reads such a
// null as no value, so an argument default that is null would otherwise read as no
// default. A null in a field that is no value, and the value {"null": null}, are read.
//
// The check: go test -race -count=1 -run TestGenerated_RefusesANullWhereAValueStands
// ./cmd/lark/tests
//
// Revisions:
//   - 2026-11-13 12:59: initial creation (C132, Q439)
func TestGenerated_RefusesANullWhereAValueStands(t *testing.T) {
	const FUNCTIONS = `"functions": [{"name": "f", "params": ["p"], "body": "pass"}], `

	for _, tc := range []struct {
		name string
		flow string
		want string
	}{
		{
			"the default of an argument",
			`{"args": {"a": {"name": "a", "default": null}}, "text": "pass"}`,
			"flow.args{}.default",
		},
		{
			"the value of a constant",
			`{"constants": {"K": {"value": null}}, "text": "pass"}`,
			"flow.constants{}.value",
		},
		{
			"a literal operand",
			`{` + FUNCTIONS + `"main": {"statement": [{"call": {"function": "f", ` +
				`"operands": [{"literal": null}]}}]}}`,
			".operands[0].literal",
		},
		{
			"the literal of a keyword",
			`{` + FUNCTIONS + `"main": {"statement": [{"call": {"function": "f", ` +
				`"keywords": [{"name": "p", "value": {"literal": null}}]}}]}}`,
			".keywords[0].value.literal",
		},
		{
			"the count of a loop",
			`{` + FUNCTIONS + `"main": {"statement": [{"loop": ` +
				`{"times": {"literal": null}, ` +
				`"body": {"call": {"function": "f"}}}}]}}`,
			".loop.times.literal",
		},
		{
			"an argument of a call",
			`{` + FUNCTIONS + `"main": {"statement": [{"call": ` +
				`{"function": "f", "args": [null]}}]}}`,
			"",
		},
		{
			"an item of a list",
			`{"constants": {"K": {"value": {"list": {"items": [null]}}}}, ` +
				`"text": "pass"}`,
			"",
		},
		{
			"an entry of a map",
			`{"constants": {"K": {"value": {"map": {"entries": {"k": null}}}}}, ` +
				`"text": "pass"}`,
			"",
		},
		{
			"a null kind that holds 5",
			`{"constants": {"K": {"value": {"null": 5}}}, "text": "pass"}`,
			"flow.constants{}.value.null",
		},
		{
			"a null kind that holds its own name",
			`{"constants": {"K": {"value": {"null": "NULL_VALUE"}}}, "text": "pass"}`,
			"flow.constants{}.value.null",
		},
		{
			"a null kind that holds 0",
			`{"constants": {"K": {"value": {"null": 0}}}, "text": "pass"}`,
			"flow.constants{}.value.null",
		},
		{
			"a null kind inside a list",
			`{"constants": {"K": {"value": {"list": {"items": [{"null": 1}]}}}}, ` +
				`"text": "pass"}`,
			".null",
		},
	} {
		t.Run(
			tc.name,
			func(t *testing.T) {
				_, err := command.Generated(
					"nulled.json",
					[]byte(tc.flow),
				)
				if err == nil {
					t.Fatal("want an error")
				}

				if !errors.Is(
					err,
					runtime.ErrForm,
				) {
					t.Fatalf(
						"%v does not match %v",
						err,
						runtime.ErrForm,
					)
				}

				if !strings.Contains(
					err.Error(),
					tc.want,
				) {
					t.Fatalf(
						"%q lacks %q",
						err,
						tc.want,
					)
				}

				if len(err.Error()) > 400 {
					t.Fatalf(
						"the error is %d bytes",
						len(err.Error()),
					)
				}
			},
		)
	}

	for _, ok := range []string{
		`{"constants": {"K": {"value": {"null": null}}}, "text": "pass"}`,
		`{"args": {"a": {"name": "a", "default": {"null": null}}}, "text": "pass"}`,
		`{"functions": [{"name": "f", "body": "pass", "params": null}], "text": "pass"}`,
		`{"text": "pass", "init": null}`,
	} {
		_, err := command.Generated(
			"fine.json",
			[]byte(ok),
		)
		if err != nil {
			t.Fatalf(
				"%s: %v",
				ok,
				err,
			)
		}
	}
}

// TestGenerated_MatchesTheDecoderError proves the error for a flow that does not
// read is the cut words, and that errors.Is names the flow runtime.ErrForm and the
// decoder's own error, so the cut text loses neither cause (CLAUDE.md 1.7).
//
// The check: go test -race -count=1 -run TestGenerated_MatchesTheDecoderError
// ./cmd/lark/tests
//
// Revisions:
//   - 2026-11-13 12:59: initial creation (C132, Q4, Q422)
func TestGenerated_MatchesTheDecoderError(t *testing.T) {
	_, err := command.Generated(
		"junk.json",
		[]byte("not json\n"),
	)
	if err == nil {
		t.Fatal("want an error")
	}

	if !errors.Is(
		err,
		runtime.ErrForm,
	) {
		t.Fatalf(
			"%v does not match %v",
			err,
			runtime.ErrForm,
		)
	}

	if !strings.HasPrefix(
		err.Error(),
		"junk.json: proto:",
	) || !strings.HasSuffix(
		err.Error(),
		"(line 1:1): invalid value not",
	) {
		t.Fatalf(
			"got %q",
			err,
		)
	}

	var syntax interface{ Unwrap() []error }

	if !errors.As(
		err,
		&syntax,
	) || len(syntax.Unwrap()) != 2 {
		t.Fatalf(
			"%v does not hold the decoder's error beside ErrForm",
			err,
		)
	}
}

// TestGenerated_NamesTheFileInEveryRefusal proves the words of every refusal of
// runtime.Read begin with the path: a flow that does not read, a null where a value
// stands, and a flow that reads and fails Check, one that wraps two errors included
// (the spawn or timeout in init) (C133, Q441, Q448).
//
// The check: go test -race -count=1 -run TestGenerated_NamesTheFileInEveryRefusal
// ./cmd/lark/tests
//
// Revisions:
//   - 2026-11-13 17:42: initial creation (C133, Q441)
func TestGenerated_NamesTheFileInEveryRefusal(t *testing.T) {
	for _, tc := range []struct {
		name string
		flow string
		is   error
		want string
	}{
		{
			"a flow that does not read",
			"not json\n",
			runtime.ErrForm,
			"flow.json: proto: syntax error (line 1:1): invalid value not",
		},
		{
			"a null where a value stands",
			`{"constants": {"K": {"value": null}}, "text": "pass"}`,
			runtime.ErrForm,
			"flow.json: flow.constants{}.value: a null where a value stands " +
				"(a value is an object named for its kind, " +
				"such as {\"null\": null})",
		},
		{
			"a flow that Check refuses for a name",
			`{"functions":[{"name":"f","body":"return 1"},` +
				`{"name":"f","body":"return 2"}],` +
				`"main":{"statement":[{"call":{"function":"f"}}]}}`,
			runtime.ErrDuplicate,
			`flow.json: "f": a name is declared twice`,
		},
		{
			"a flow that Check refuses for a form",
			`{"main": {}}`,
			runtime.ErrForm,
			"flow.json: main: function has no body: no form",
		},
		{
			"a timeout in init, which wraps two errors",
			`{"init":[{"text":"x = timeout(1, print)"}],"text":"pass"}`,
			runtime.ErrInit,
			"flow.json: init: no form (timeout cannot start a thread while the " +
				"module loads; call timeout from main or a function main calls)",
		},
		{
			"a spawn in init, which wraps two errors",
			`{"init":[{"call":{"function":"spawn"}}],"text":"pass"}`,
			runtime.ErrForm,
			"flow.json: init: no form (spawn cannot start a thread while the module " +
				"loads; call spawn from main or a function main calls)",
		},
	} {
		t.Run(
			tc.name,
			func(t *testing.T) {
				_, err := command.Generated(
					"flow.json",
					[]byte(tc.flow),
				)
				if err == nil {
					t.Fatal("want an error")
				}

				got := strings.ReplaceAll(
					err.Error(),
					"\u00a0",
					" ",
				)
				if got != tc.want {
					t.Fatalf(
						"got %q, want %q",
						got,
						tc.want,
					)
				}

				if !errors.Is(
					err,
					tc.is,
				) {
					t.Fatalf(
						"%v does not match %v",
						err,
						tc.is,
					)
				}
			},
		)
	}
}
