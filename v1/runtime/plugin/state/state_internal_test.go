package state

// This test is internal because its subjects, _SetArgs, _GetArgs and
// _UpdateArgs, are unexported. A call through the module allocates for its
// frame, its run and its result, so only a call to the readers themselves
// shows what reading the arguments costs.

import (
	"testing"

	"go.starlark.net/starlark"
)

const (
	// RUNS is how many calls each count averages over.
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
func TestRegression_ReadingArgumentsAllocatesNothing(t *testing.T) {
	var (
		fn = starlark.NewBuiltin(
			NAME,
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
					_, _, err := _SetArgs(
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
					_, err := _GetArgs(
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
					_, _, err := _UpdateArgs(
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
