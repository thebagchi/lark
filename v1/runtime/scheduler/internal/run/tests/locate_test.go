package tests

import (
	"errors"
	"fmt"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/scheduler/internal/run"
)

// ErrBoom is what the builtins of these tests fail with.
//
// Revisions:
//   - 2026-11-04 18:59: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 22:45: moved to the external package (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
var (
	ErrBoom = errors.New("boom")
)

// _Builtins is the names these tests' scripts use: boom fails, and wrap calls
// its argument and fails with that failure inside a text of its own, as a
// builtin of the runtime does.
//
// Revisions:
//   - 2026-11-04 06:39: initial creation (C69)
//   - 2026-11-08 22:45: moved to the external package (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Builtins() starlark.StringDict {
	return starlark.StringDict{
		"boom": starlark.NewBuiltin(
			"boom",
			func(
				_ *starlark.Thread,
				_ *starlark.Builtin,
				_ starlark.Tuple,
				_ []starlark.Tuple,
			) (starlark.Value, error) {
				return nil, ErrBoom
			},
		),
		"wrap": starlark.NewBuiltin(
			"wrap",
			func(
				thread *starlark.Thread,
				_ *starlark.Builtin,
				args starlark.Tuple,
				_ []starlark.Tuple,
			) (starlark.Value, error) {
				_, err := starlark.Call(
					thread,
					args[0],
					nil,
					nil,
				)

				return nil, fmt.Errorf(
					"wrap: %w",
					err,
				)
			},
		),
	}
}

// _Raised runs source and returns the error it ends with.
//
// Revisions:
//   - 2026-11-04 06:39: initial creation (C69)
//   - 2026-11-08 22:45: moved to the external package (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Raised(
	t *testing.T,
	source string,
) error {
	t.Helper()

	_, err := starlark.ExecFileOptions(
		new(syntax.FileOptions),
		new(starlark.Thread),
		"x.star",
		source,
		_Builtins(),
	)
	if err == nil {
		t.Fatalf(
			"%q passed, want a failure",
			source,
		)
	}

	return err
}

// TestLocate_Cases checks Locate against each shape of error it is handed.
//
// Revisions:
//   - 2026-11-04 06:39: initial creation (C69)
//   - 2026-11-04 18:12: the position is file:line, with no column (Q89)
//   - 2026-11-04 18:40: a wrapper's words come before the position (C69, fix round 2)
//   - 2026-11-08 22:45: moved to the external package (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestLocate_Cases(t *testing.T) {
	const (
		DEEP    = "def f():\n    boom()\nf()\n"
		WRAPPED = "def f():\n    boom()\n\ndef g():\n    wrap(f)\ng()\n"
	)

	for _, tc := range []struct {
		name   string
		source string
		thread *starlark.Thread
		want   string
	}{
		{
			"the innermost Starlark frame, not the builtin above it",
			DEEP,
			new(starlark.Thread),
			"x.star:2: boom",
		},
		{
			"the deeper of two interpreter errors in one chain",
			WRAPPED,
			nil,
			"wrap: x.star:2: boom",
		},
	} {
		t.Run(
			tc.name,
			func(t *testing.T) {
				got := run.Locate(
					tc.thread,
					_Raised(
						t,
						tc.source,
					),
				)

				if got.Error() != tc.want {
					t.Fatalf(
						"got %q, want %q",
						got,
						tc.want,
					)
				}

				if !errors.Is(
					got,
					ErrBoom,
				) {
					t.Fatalf(
						"got %v, want the cause kept",
						got,
					)
				}
			},
		)
	}
}

// TestLocate_NoPlace checks an error with no Starlark frame comes back as it
// was: a builtin that Go called is a stack of one builtin frame, and a nil
// error stays nil.
//
// Revisions:
//   - 2026-11-04 06:39: initial creation (C69)
//   - 2026-11-08 22:45: moved to the external package (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestLocate_NoPlace(t *testing.T) {
	_, direct := starlark.Call(
		new(starlark.Thread),
		_Builtins()["boom"],
		nil,
		nil,
	)

	var raised *starlark.EvalError
	if !errors.As(
		direct,
		&raised,
	) {
		t.Fatalf(
			"got %v, want an EvalError",
			direct,
		)
	}

	if len(raised.CallStack) != 1 {
		t.Fatalf(
			"got %d frames, want 1",
			len(raised.CallStack),
		)
	}

	if got := run.Locate(
		new(starlark.Thread),
		direct,
	); !errors.Is(
		got,
		ErrBoom,
	) || got.Error() != direct.Error() {
		t.Fatalf(
			"got %q, want the error as it was",
			got,
		)
	}

	if got := run.Locate(
		nil,
		nil,
	); got != nil {
		t.Fatalf(
			"got %v, want nil",
			got,
		)
	}

	if got := run.Locate(
		nil,
		ErrBoom,
	); !errors.Is(
		got,
		ErrBoom,
	) || got.Error() != "boom" {
		t.Fatalf(
			"got %v, want the error as it was",
			got,
		)
	}
}

// TestLocate_IsNotAddedTwice checks an error already located comes back as it
// is, wrapped or not, so a join keeps the place in the thread that failed.
//
// Revisions:
//   - 2026-11-04 06:39: initial creation (C69)
//   - 2026-11-04 18:12: the position is file:line, with no column (Q89)
//   - 2026-11-08 22:45: moved to the external package (C65c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestLocate_IsNotAddedTwice(t *testing.T) {
	var (
		once = run.Locate(
			nil,
			_Raised(
				t,
				"def f():\n    boom()\nf()\n",
			),
		)
		again = run.Locate(
			nil,
			fmt.Errorf(
				"w: %w",
				once,
			),
		)
	)

	if again.Error() != "w: x.star:2: boom" {
		t.Fatalf(
			"got %q, want the place once",
			again,
		)
	}

	if run.Locate(
		nil,
		once,
	).Error() != once.Error() {
		t.Fatal("a located error was located again")
	}
}
