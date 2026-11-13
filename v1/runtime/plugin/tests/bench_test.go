package tests

import (
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/codec/integer"
)

// BenchmarkBuiltinCall measures one call of integer.from_bytes taken from the
// environment, the path the check of C93 sits on.
//
// Revisions:
//   - 2026-11-04 22:35: initial creation (C93)
//   - 2026-11-04 22:35: takes the builtin from Environment alone, so it builds on the base
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func BenchmarkBuiltinCall(b *testing.B) {
	env, err := plugin.Environment(plugin.Registered())
	if err != nil {
		b.Fatal(err)
	}

	var (
		module = env["integer"].(*starlarkstruct.Module)
		from   = module.Members["from_bytes"].(*starlark.Builtin)
		thread = new(starlark.Thread)
		args   = starlark.Tuple{starlark.Bytes("\x01\x02")}
		kwargs = []starlark.Tuple{
			{starlark.String("order"), starlark.String("little")},
		}
	)

	b.ReportAllocs()

	for b.Loop() {
		_, err := from.CallInternal(
			thread,
			args,
			kwargs,
		)
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkEnvironment measures building the environment of every registered
// plugin, which a compile does once and which calls each plugin's Values.
//
// Revisions:
//   - 2026-11-04 22:35: initial creation (Q134)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func BenchmarkEnvironment(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		_, err := plugin.Environment(plugin.Registered())
		if err != nil {
			b.Fatal(err)
		}
	}
}

// BenchmarkValues measures one call of Values on every registered plugin, as a
// Go caller makes it.
//
// Revisions:
//   - 2026-11-04 22:35: initial creation (Q134)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func BenchmarkValues(b *testing.B) {
	b.ReportAllocs()

	for b.Loop() {
		for _, registered := range plugin.Registered() {
			_ = registered.Values()
		}
	}
}
