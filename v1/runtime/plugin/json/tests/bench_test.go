package tests

import (
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin"
)

// BenchmarkEncode measures json.encode through the plugin on text with no DEL
// (the case that must not pay for the fix), with DEL, and with non-ASCII text.
//
// Revisions:
//   - 2026-11-04 23:49: initial creation (C92)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkEncode(b *testing.B) {
	env, err := plugin.Environment(plugin.Registered())
	if err != nil {
		b.Fatalf(
			"environment: %v",
			err,
		)
	}

	cases := []struct {
		name string
		text string
	}{
		{
			"ascii",
			strings.Repeat(
				"hello world ",
				16,
			),
		},
		{
			"backslash_x7f_text",
			strings.Repeat(
				`a\x7f `,
				16,
			),
		},
		{
			"del",
			strings.Repeat(
				"hello\x7f world ",
				16,
			),
		},
		{
			"unicode",
			strings.Repeat(
				"héllo wörld ",
				16,
			),
		},
	}

	var (
		module    = env["json"].(starlark.HasAttrs)
		encode, _ = module.Attr("encode")
	)

	for _, item := range cases {
		var (
			argument = starlark.Tuple{
				starlark.NewList([]starlark.Value{
					starlark.String(item.text),
				}),
			}
			thread = &starlark.Thread{
				Name: SCRIPT,
			}
		)

		b.Run(
			item.name,
			func(b *testing.B) {
				b.ReportAllocs()

				for range b.N {
					_, err := starlark.Call(
						thread,
						encode,
						argument,
						nil,
					)
					if err != nil {
						b.Fatal(err)
					}
				}
			},
		)
	}
}
