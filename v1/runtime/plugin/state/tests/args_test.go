package tests

// This file tests that reading the arguments of the set, get and update
// builtins allocates nothing. It calls SetArgs, GetArgs and UpdateArgs
// directly, because a call through the module allocates for its frame, its run
// and its result, so only a call to the readers shows what reading the
// arguments costs.
//
// Revisions:
//   - 2026-11-08 22:46: states the reason in the first comment (C65b)
//   - 2026-11-10 01:37: moved to tests/ from state_internal_test.go and rewrote its header
//     (C120b, Q311, Q322)

import (
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/state/internal/spec"
	"github.com/thebagchi/lark/v1/runtime/plugin/state/internal/store"
)

const (
	// RUNS is how many calls each count averages over.
	//
	// Revisions:
	//   - 2026-10-29 06:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	RUNS = 100
)

// TestRegression_ReadingArgumentsAllocatesNothing keeps the argument readers
// of set, get and update off the heap.
//
// Each reader once unpacked through starlark.UnpackPositionalArgs. That takes
// a pointer to each local through ...any and can call a method through it, so
// the compiler moved every local to the heap: two allocations on each set, one
// on each get and two on each update, before any work. A reader now checks the
// count and the type itself, and its locals stay on the stack.
//
// Revisions:
//   - 2026-10-29 01:31: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestRegression_ReadingArgumentsAllocatesNothing(t *testing.T) {
	var (
		fn = starlark.NewBuiltin(
			spec.NAME,
			nil,
		)

		change = starlark.NewBuiltin(
			"change",
			nil,
		)

		cases = map[string]struct {
			read func(args starlark.Tuple) error
			args starlark.Tuple
		}{
			"a set": {
				read: func(args starlark.Tuple) error {
					_, _, err := store.SetArgs(
						fn,
						args,
						nil,
					)

					return err
				},
				args: starlark.Tuple{
					starlark.String("k"),
					starlark.MakeInt(1),
				},
			},
			"a get": {
				read: func(args starlark.Tuple) error {
					_, err := store.GetArgs(
						fn,
						args,
						nil,
					)

					return err
				},
				args: starlark.Tuple{
					starlark.String("k"),
				},
			},
			"an update": {
				read: func(args starlark.Tuple) error {
					_, _, err := store.UpdateArgs(
						fn,
						args,
						nil,
					)

					return err
				},
				args: starlark.Tuple{
					starlark.String("k"),
					change,
				},
			},
		}
	)

	for name, tc := range cases {
		t.Run(
			name,
			func(t *testing.T) {
				var err error

				made := testing.AllocsPerRun(
					RUNS,
					func() {
						err = tc.read(tc.args)
					},
				)

				if err != nil {
					t.Fatal(err)
				}

				if made != 0 {
					t.Fatalf(
						"%v allocations to read the arguments of %s",
						made,
						name,
					)
				}
			},
		)
	}
}
