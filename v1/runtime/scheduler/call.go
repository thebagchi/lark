package scheduler

import (
	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/scheduler/internal/run"
)

// Call is the hidden function a compiled statement call of a script function
// goes through, under the name no script can write.
//
// A new builtin on every call, so the package keeps no exported variable: the
// builtin holds no state, and the one caller puts it in an environment once per
// compile.
//
// Revisions:
//   - 2026-10-04 01:25: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:15: a func in place of the CALL var, so the top package has
//     no exported var (C91w4, Q225)
//   - 2026-11-07 20:10: forwards to run.Call (C91w5)
func Call() *starlark.Builtin {
	return run.Call()
}

// Hidden is the value of the hidden keyword name among kwargs, and kwargs
// without it.
//
// The dialect passes a few facts only the source knows - a spawn's binding, the
// function a lambda calls, the branch a call belongs to - as keywords a script
// does not write by name, on the real call. The builtin reading one takes it
// out with this before it reads its own arguments, so a script's own keywords
// are judged as they always were. Empty when the keyword is absent.
//
// Every pair of the name is taken out, and the first is the one read. A script
// can still hand a builtin the name through **, whose pairs Starlark puts after
// a call's own and does not check against them, so the dialect's pair, written
// on the call, comes first, and any other is a script's.
//
// A tuple that is not exactly two elements is not a pair, so it does not match
// the name and stays in what is handed back. The interpreter always passes
// pairs; only a Go caller can pass anything else, and Hidden never panics on it.
//
// What is handed back is kwargs itself when the name is absent, and a slice of
// it when the one pair stands at either end. A call of a script function passes
// through here, so copying the keywords allocated on every call that had one;
// only a second pair, which a script has to write, costs a copy now.
//
// Revisions:
//   - 2026-10-02 00:42: initial creation
//   - 2026-10-03 20:53: hands back kwargs, or a slice of it, rather than a copy,
//     unless the keyword stands between two others
//   - 2026-10-03 23:37: takes out every pair of the name again, reading the first,
//     since a script can add one through **
//   - 2026-11-03 00:31: a tuple of other than two elements does not match, so
//     Hidden never panics on one (bug B056)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 20:10: forwards to run.Hidden (C91w5)
func Hidden(
	kwargs []starlark.Tuple,
	name string,
) (string, []starlark.Tuple) {
	return run.Hidden(
		kwargs,
		name,
	)
}
