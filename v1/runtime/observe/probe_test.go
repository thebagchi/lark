package observe_test

import (
	"runtime"

	"go.starlark.net/starlark"
)

const (
	// PROBE is the name of the plugin a test compiles in, EXPLODE the builtin
	// of it that panics with PANIC_TEXT, and WHERE the builtin of it that
	// returns the stack of the goroutine it was called on.
	PROBE      = "probe"
	EXPLODE    = "explode"
	WHERE      = "where"
	PANIC_TEXT = "a plugin blew up"

	// STACK is room for the stack WHERE returns, enough that the frame of a
	// test is not cut off below the interpreter's frames.
	STACK = 1 << 20
)

// _Probe is a plugin of two builtins: one that panics, and one that tells which
// goroutine a script's call runs on.
type _Probe struct{}

// Name is what this plugin is called.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation
func (p *_Probe) Name() string {
	return PROBE
}

// Values returns the builtin that panics and the builtin that returns its own
// stack.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation
func (p *_Probe) Values() starlark.StringDict {
	return starlark.StringDict{
		EXPLODE: starlark.NewBuiltin(
			EXPLODE,
			_Explode,
		),
		WHERE: starlark.NewBuiltin(
			WHERE,
			_Where,
		),
	}
}

// _Explode panics, as a plugin with a bug does.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation
func _Explode(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	panic(PANIC_TEXT)
}

// _Where is the stack of the goroutine the script called it on, so a test can
// read which function that goroutine started from.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation
func _Where(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		buf = make(
			[]byte,
			STACK,
		)
		size = runtime.Stack(
			buf,
			false,
		)
	)

	return starlark.String(buf[:size]), nil
}
