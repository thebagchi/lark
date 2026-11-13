package tests

import (
	"strconv"
	"strings"
	"testing"
	"unsafe"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
)

// TestHandover_BytesNeverWritesAHandedOverBuffer proves, for random.bytes, the
// rule that makes its handover safe: nothing writes the buffer after the call.
// A script makes many values and keeps them all. Each must be a buffer of its
// own, and the later draws must leave the earlier ones as they were. A source
// that reused one buffer would give two values the same start address.
//
// Revisions:
//   - 2026-11-03 00:30: initial creation (C37)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestHandover_BytesNeverWritesAHandedOverBuffer(t *testing.T) {
	const DRAWS = 6

	var (
		body = "return [" + strings.Repeat(
			"random.bytes(64), ",
			DRAWS,
		) + "]"
		got, err = _Expression(
			t,
			body,
		)
	)
	if err != nil {
		t.Fatal(err)
	}

	list, ok := got.(*starlark.List)
	if !ok || list.Len() != DRAWS {
		t.Fatalf(
			"got %s, want a list of %d",
			got,
			DRAWS,
		)
	}

	seen := make(map[*byte]int)

	for idx := range DRAWS {
		held, ok := list.Index(idx).(starlark.Bytes)
		if !ok || len(held) != 64 {
			t.Fatalf(
				"draw %d is %s, want 64 bytes",
				idx,
				list.Index(idx),
			)
		}

		start := unsafe.StringData(string(held))

		if other, dup := seen[start]; dup {
			t.Fatalf(
				"draws %d and %d share one buffer",
				other,
				idx,
			)
		}

		seen[start] = idx

		for earlier := range idx {
			if held == list.Index(earlier).(starlark.Bytes) {
				t.Fatalf(
					"draws %d and %d hold the same 64 bytes",
					earlier,
					idx,
				)
			}
		}
	}
}

// BenchmarkBytes is random.bytes at three sizes, called 100 times in a run, as
// BenchmarkChoice calls its builtin. The allocations are those of the run as a
// whole; compare two builds on the same benchmark.
//
// Revisions:
//   - 2026-11-03 00:30: initial creation (C37)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkBytes(b *testing.B) {
	for _, size := range []string{"16", "1024", "65536"} {
		b.Run(
			size,
			func(b *testing.B) {
				_BenchScript(
					b,
					"random.bytes("+size+")",
				)
			},
		)
	}
}

// _BenchScript runs a script that makes the call CALLS times, once for each
// pass of the benchmark loop.
//
// Revisions:
//   - 2026-11-03 00:30: initial creation (C37)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _BenchScript(
	b *testing.B,
	call string,
) {
	b.Helper()

	var (
		text = strings.Join(
			[]string{
				"def main():",
				"    for i in range(" + strconv.Itoa(CALLS) + "):",
				"        " + call,
				"",
			},
			"\n",
		)
		art, err = runtime.Compile(&common.Source{
			Entry: "bytes_bench.star",
			Text:  []byte(text),
		})
	)
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		run, err := runtime.Start(
			b.Context(),
			art,
		)
		if err != nil {
			b.Fatal(err)
		}

		_, err = run.Wait()
		if err != nil {
			b.Fatal(err)
		}
	}
}
