package tests

import (
	stdjson "encoding/json"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/json"
)

// TestJSON_EncodeWritesDELAsAnEscape pins bug B121 (Q111). RFC 8259 section 7
// has no \x escape: a string is quoted with \", \\, \/, \b, \f, \n, \r, \t or
// \uXXXX. The library wrote DEL (0x7f) as \x7f. The plugin writes \u007f.
// Each row must be valid JSON for Go's encoding/json, decode back to the
// input, and contain no \x escape. A text of a backslash and "x7f" is a row
// too: its backslash is written as \\ and must stay.
//
// Revisions:
//   - 2026-11-04 23:49: initial creation (C92, B121, Q111)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestJSON_EncodeWritesDELAsAnEscape(t *testing.T) {
	cases := []struct {
		name       string
		expression string
		want       string
	}{
		{
			"DEL alone",
			`json.encode("\x7f")`,
			`"\u007f"`,
		},
		{
			"DEL inside text",
			`json.encode("a\x7fb\x7f")`,
			`"a\u007fb\u007f"`,
		},
		{
			"DEL in a dict key",
			`json.encode({"k\x7f": 1})`,
			`{"k\u007f":1}`,
		},
		{
			"DEL in nested lists",
			`json.encode([["\x7f", [{"a": "\x7f"}]], 1])`,
			`[["\u007f",[{"a":"\u007f"}]],1]`,
		},
		{
			"DEL next to a literal backslash and x7f",
			`json.encode("\\x7f\x7f\\")`,
			`"\\x7f\u007f\\"`,
		},
		{
			"literal backslash x7f is left alone",
			`json.encode("\\x7f")`,
			`"\\x7f"`,
		},
		{
			// A string with a non-ASCII rune takes the library's other path,
			// encoding/json, which leaves DEL raw. Section 7 allows that.
			"DEL beside a non-ASCII rune is raw",
			`json.encode("é\x7f")`,
			"\"é\x7f\"",
		},
		{
			"no DEL is unchanged",
			`json.encode("plain \"q\" \\ \n")`,
			`"plain \"q\" \\ \n"`,
		},
		{
			"indent mode",
			`json.encode_indent({"k\x7f": ["\x7f"]}, indent = " ")`,
			"{\n \"k\\u007f\": [\n  \"\\u007f\"\n ]\n}",
		},
		{
			"indent mode with a prefix",
			`json.encode_indent(["\x7f"], prefix = " ", indent = "  ")`,
			"[\n   \"\\u007f\"\n ]",
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

				written := string(value.(starlark.String))
				if written != item.want {
					t.Fatalf(
						"%s gave %q, want %q",
						item.expression,
						written,
						item.want,
					)
				}

				if !stdjson.Valid([]byte(written)) {
					t.Fatalf(
						"%s gave %q, which encoding/json refuses",
						item.expression,
						written,
					)
				}

				// The output must decode, and decode to what encoding/json
				// reads from the same text.
				decoded, err := starlark.EvalOptions(
					common.OPTIONS,
					&starlark.Thread{
						Name: SCRIPT,
					},
					SCRIPT,
					"json.decode(text)",
					_MergeText(
						env,
						written,
					),
				)
				if err != nil {
					t.Fatalf(
						"json.decode(%q): %v",
						written,
						err,
					)
				}

				var reference any

				err = stdjson.Unmarshal(
					[]byte(written),
					&reference,
				)
				if err != nil {
					t.Fatalf(
						"encoding/json: %v",
						err,
					)
				}

				again, err := starlark.EvalOptions(
					common.OPTIONS,
					&starlark.Thread{
						Name: SCRIPT,
					},
					SCRIPT,
					"json.encode(value)",
					_MergeValue(
						env,
						decoded,
					),
				)
				if err != nil {
					t.Fatalf(
						"re-encode: %v",
						err,
					)
				}

				var back any

				err = stdjson.Unmarshal(
					[]byte(again.(starlark.String)),
					&back,
				)
				if err != nil {
					t.Fatalf(
						"re-encode %q: %v",
						string(again.(starlark.String)),
						err,
					)
				}

				var (
					first, _  = stdjson.Marshal(reference)
					second, _ = stdjson.Marshal(back)
				)

				if string(first) != string(second) {
					t.Fatalf(
						"round trip gave %s, want %s",
						second,
						first,
					)
				}
			},
		)
	}
}

// _MergeText returns env with the name text bound to s.
//
// Revisions:
//   - 2026-11-04 23:49: initial creation (C92)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _MergeText(
	env starlark.StringDict,
	s string,
) starlark.StringDict {
	return _MergeValue(
		env,
		starlark.String(s),
	)
}

// _MergeValue returns a copy of env with the names text and value bound to v.
//
// Revisions:
//   - 2026-11-04 23:49: initial creation (C92)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _MergeValue(
	env starlark.StringDict,
	v starlark.Value,
) starlark.StringDict {
	merged := make(
		starlark.StringDict,
		len(env)+2,
	)
	for name, member := range env {
		merged[name] = member
	}

	merged["text"] = v
	merged["value"] = v

	return merged
}
