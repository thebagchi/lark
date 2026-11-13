package tests

import (
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/codec/buf"
	"github.com/thebagchi/lark/v1/runtime/plugin/codec/buf/internal/buffer"
)

const (
	// READ_SIZE is the bytes of a reader in BenchmarkReadInt: enough for many
	// reads of the widest field, so a new reader is rare.
	//
	// Revisions:
	//   - 2026-11-04 06:52: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	READ_SIZE = 1 << 16
)

var (
	// INT_METHODS are the writing methods that BenchmarkWriteInt calls, with
	// the arguments of each: a fixed width, and a width the script names.
	//
	// Revisions:
	//   - 2026-11-04 06:52: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-07 19:17: literals, internal/buffer types (C91w8a)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	INT_METHODS = []struct {
		name string
		args starlark.Tuple
	}{
		{
			name: "write_uint8",
			args: starlark.Tuple{
				starlark.MakeInt(200),
			},
		},
		{
			name: "write_uint64",
			args: starlark.Tuple{
				starlark.MakeInt(300),
			},
		},
		{
			name: "write_int16",
			args: starlark.Tuple{
				starlark.MakeInt(-300),
			},
		},
		{
			name: "write_uint",
			args: starlark.Tuple{
				starlark.MakeInt(300),
				starlark.MakeInt(4),
			},
		},
	}

	// READ_METHODS are the reading methods that BenchmarkReadInt calls, with
	// the arguments of each.
	//
	// Revisions:
	//   - 2026-11-04 06:52: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-07 19:17: literals, internal/buffer types (C91w8a)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	READ_METHODS = []struct {
		name string
		args starlark.Tuple
	}{
		{
			name: "read_uint8",
		},
		{
			name: "read_uint64",
		},
		{
			name: "read_int16",
		},
		{
			name: "read_uint",
			args: starlark.Tuple{
				starlark.MakeInt(4),
			},
		},
	}
)

// BenchmarkWriteInt measures the writing methods on one number, through the
// method, so the reading of its arguments shows in the allocations. All calls
// write to one buffer, which grows by doubling, so its growth is spread thin
// over the calls.
//
// Revisions:
//   - 2026-11-04 00:51: initial creation (C38c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func BenchmarkWriteInt(b *testing.B) {
	for _, method := range INT_METHODS {
		b.Run(
			method.name,
			func(b *testing.B) {
				write, _ := _Method(
					b,
					method.name,
				)

				var (
					thread = &starlark.Thread{
						Name: THREAD,
					}
				)

				b.ReportAllocs()

				for b.Loop() {
					_, err := write.CallInternal(
						thread,
						method.args,
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

// BenchmarkReadInt measures the reading methods on a reader of READ_SIZE
// bytes, through the method. A new reader replaces one that has run out, with
// the timer stopped.
//
// Revisions:
//   - 2026-11-04 00:51: initial creation (C38c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func BenchmarkReadInt(b *testing.B) {
	for _, method := range READ_METHODS {
		b.Run(
			method.name,
			func(b *testing.B) {
				var (
					reader = _Reader(b)
					thread = &starlark.Thread{
						Name: THREAD,
					}
					read = _Bound(
						b,
						reader,
						method.name,
					)
				)

				b.ReportAllocs()

				for b.Loop() {
					_, err := read.CallInternal(
						thread,
						method.args,
						nil,
					)
					if err != nil {
						b.StopTimer()

						reader = _Reader(b)
						read = _Bound(
							b,
							reader,
							method.name,
						)

						b.StartTimer()
					}
				}
			},
		)
	}
}

// _Reader is a new reader over READ_SIZE bytes, as a script makes one with
// buf.reader.
//
// Revisions:
//   - 2026-11-04 00:51: initial creation (C38c)
//   - 2026-11-07 19:17: literals, internal/buffer types (C91w8a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Reader(tb testing.TB) *buffer.Reader {
	tb.Helper()

	env, err := plugin.Environment(plugin.Registered())
	if err != nil {
		tb.Fatalf(
			"environment: %v",
			err,
		)
	}

	module, ok := env["buf"].(*starlarkstruct.Module)
	if !ok {
		tb.Fatalf(
			"%s is not a module",
			"buf",
		)
	}

	maker, ok := module.Members["reader"].(*starlark.Builtin)
	if !ok {
		tb.Fatalf(
			"%s.%s is not a builtin",
			"buf",
			"reader",
		)
	}

	made, err := maker.CallInternal(
		&starlark.Thread{
			Name: THREAD,
		},
		starlark.Tuple{
			starlark.Bytes(_Data(READ_SIZE)),
		},
		nil,
	)
	if err != nil {
		tb.Fatal(err)
	}

	reader, ok := made.(*buffer.Reader)
	if !ok {
		tb.Fatalf(
			"%s is not a reader",
			made.Type(),
		)
	}

	return reader
}
