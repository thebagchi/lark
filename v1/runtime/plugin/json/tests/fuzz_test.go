package tests

import (
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin"
)

// FuzzDecode feeds json.decode any text, through the plugin, as a script calls
// it. Refused text is an error and never a panic. Accepted text decodes to a
// value that json.encode writes, when it can, and json.decode reads back to an
// equal value (RFC 8259 section 2: a value has one structure). The seeds are
// the accepted and refused forms of the table tests, and the nesting the
// plugin bounds.
//
// Revisions:
//   - 2026-11-04 19:33: initial creation (C89, Q94 D2)
//   - 2026-11-04 23:49: the round trip covers text with DEL; B121 is fixed (C92)
//   - 2026-11-09 11:16: the corpus file becomes an f.Add call (C114, Q283)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func FuzzDecode(f *testing.F) {
	for _, seed := range []string{
		"", "null", "true", "false", "0", "-0", "1.5", "1e3", "1E+2", "[1.]",
		"[]", "{}", `[1,2,3]`, `{"a":1}`, `{"a":{"b":[null]}}`, `"a"`,
		"\"a\nb\"", `"é"`, `"😀"`, `"\ud800"`, `"\x"`,
		"[1,]", `{"a":1,}`, "{1:2}", "'a'", "NaN", "Infinity", "01", "+1",
		"[", "]", "[]]", "[[[[[[[[", `{"a"`, "\x00", "\xff", " [1] ",
		"[" + "[" + "]" + "]", `{"a":1,"a":2}`, "1e999", "-1e999",
	} {
		f.Add(seed)
	}

	// A finding of the fuzzer, once a corpus file: a string holding DEL.
	f.Add("\"\x7f\"")

	env, err := plugin.Environment(plugin.Registered())
	if err != nil {
		f.Fatalf(
			"environment: %v",
			err,
		)
	}

	var (
		module    = env["json"].(starlark.HasAttrs)
		decode, _ = module.Attr("decode")
		encode, _ = module.Attr("encode")
	)

	f.Fuzz(func(
		t *testing.T,
		text string,
	) {
		value, err := starlark.Call(
			&starlark.Thread{
				Name: SCRIPT,
			},
			decode,
			starlark.Tuple{starlark.String(text)},
			nil,
		)
		if err != nil {
			return
		}

		written, err := starlark.Call(
			&starlark.Thread{
				Name: SCRIPT,
			},
			encode,
			starlark.Tuple{value},
			nil,
		)
		if err != nil {
			return
		}

		again, err := starlark.Call(
			&starlark.Thread{
				Name: SCRIPT,
			},
			decode,
			starlark.Tuple{written},
			nil,
		)
		if err != nil {
			t.Fatalf(
				"decode(%q) gave %s, whose encoding %s is refused: %v",
				text,
				value,
				written,
				err,
			)
		}

		equal, err := starlark.Equal(
			value,
			again,
		)
		if err != nil || !equal {
			t.Fatalf(
				"decode(%q) gave %s, round trip gave %s (%v)",
				text,
				value,
				again,
				err,
			)
		}
	})
}
