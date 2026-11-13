package tests

import (
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/codec/buf"
)

// BenchmarkMethodCall measures one write_bytes call on a buffer taken through
// Attr, the path the check of Q134 sits on.
//
// Revisions:
//   - 2026-11-04 22:35: initial creation (Q134)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func BenchmarkMethodCall(b *testing.B) {
	env, err := plugin.Environment(plugin.Registered())
	if err != nil {
		b.Fatal(err)
	}

	var (
		module = env["buf"].(*starlarkstruct.Module)
		thread = new(starlark.Thread)
		args   = starlark.Tuple{starlark.Bytes("ab")}
	)

	made, err := starlark.Call(
		thread,
		module.Members["new"],
		nil,
		nil,
	)
	if err != nil {
		b.Fatal(err)
	}

	method, err := made.(starlark.HasAttrs).Attr("write_bytes")
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		_, err := starlark.Call(
			thread,
			method,
			args,
			nil,
		)
		if err != nil {
			b.Fatal(err)
		}
	}
}
