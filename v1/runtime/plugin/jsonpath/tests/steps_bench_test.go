package tests

// These benchmarks measure Steps and Walk. Patch and the located-error path
// call both, and no builtin call measures them alone.
//
// Revisions:
//   - 2026-11-02 20:56: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ from bench_internal_test.go and rewrote its header
//     (C120b, Q311, Q322)

import (
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/jsonpath/internal/patch"
)

const (
	// SHORT is a pointer of three steps.
	//
	// Revisions:
	//   - 2026-11-02 20:56: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	SHORT = "/a/b/c"

	// ESCAPED is a pointer whose steps hold both escapes.
	//
	// Revisions:
	//   - 2026-11-02 20:56: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	ESCAPED = "/a~1b/c~0d/e~01f"

	// LONG is the count of repeats of the step in the long pointers.
	//
	// Revisions:
	//   - 2026-11-02 20:56: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	LONG = 15

	// HUGE is the count of repeats of the step in the very long pointer.
	//
	// Revisions:
	//   - 2026-11-02 20:56: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	HUGE = 150
)

// SUNK keeps the result of a call, so the compiler cannot remove the call.
//
// Revisions:
//   - 2026-11-02 20:56: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
var (
	SUNK any
)

// BenchmarkSteps measures Steps alone on pointers of four shapes. The
// allocations include the one that boxes the result into SUNK.
//
// Revisions:
//   - 2026-11-02 17:46: initial creation (C36c), from the Steps_* benchmarks of
//     poc/research/simd_json
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkSteps(b *testing.B) {
	cases := []struct {
		name    string
		pointer string
	}{
		{
			name:    "short",
			pointer: SHORT,
		},
		{
			name:    "escaped",
			pointer: ESCAPED,
		},
		{
			name: "long 200 bytes",
			pointer: "/" + strings.Repeat(
				"segment_name/",
				LONG,
			) + "x",
		},
		{
			name: "long 4 KiB",
			pointer: "/" + strings.Repeat(
				"segment_name_of_some_length/",
				HUGE,
			) + "x",
		},
	}

	for _, tc := range cases {
		b.Run(
			tc.name,
			func(b *testing.B) {
				b.ReportAllocs()
				b.SetBytes(int64(len(tc.pointer)))

				for b.Loop() {
					var err error

					SUNK, err = patch.Steps(tc.pointer)
					if err != nil {
						b.Fatal(err)
					}
				}
			},
		)
	}
}

// BenchmarkWalk measures Walk alone on steps that are already split, three
// dicts down, so the allocations are the walk and not the pointer.
//
// Revisions:
//   - 2026-11-02 17:46: initial creation (C36c), from BenchmarkWalk_Depth3_Preparsed of
//     poc/research/simd_json
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkWalk(b *testing.B) {
	var (
		steps    = []string{"alpha", "beta", "gamma"}
		document starlark.Value
	)

	document = starlark.MakeInt(1)

	for idx := len(steps) - 1; idx >= 0; idx-- {
		var (
			parent = starlark.NewDict(1)
			err    = parent.SetKey(
				starlark.String(steps[idx]),
				document,
			)
		)
		if err != nil {
			b.Fatal(err)
		}

		document = parent
	}

	b.ReportAllocs()

	for b.Loop() {
		var err error

		SUNK, err = patch.Walk(
			document,
			steps,
		)
		if err != nil {
			b.Fatal(err)
		}
	}
}
