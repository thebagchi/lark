package scheduler

import (
	"errors"
	"fmt"
	"slices"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/spelling"
)

// ABSENT is the index slices.IndexFunc answers when nothing matches.
const ABSENT = -1

// ERR_NO_CALLEE is returned when the hidden call is handed nothing to call. Only
// the dialect writes a call of it, and always with a function first, so this
// is an invariant broken rather than a script's mistake.
var ERR_NO_CALLEE = errors.New("nothing to call")

// CALL is the hidden function a compiled statement call of a script function
// goes through, under the name no script can write.
var CALL = starlark.NewBuiltin(spelling.CALL, _Calling)

// _Calling calls the function it is handed first with the rest of its
// arguments, and reports the call as a line of the thread that made it.
//
// The interpreter has no hook on a call, so the dialect compiles once() as a
// call of this with once first: it reports the line's start, calls once, and
// reports its end, which is how a CALL node gets both. A branch's call passes
// the builtin it belongs to - if or match - under BUILTIN, and that keyword is
// removed before the function sees its arguments.
//
// The dialect writes a call of this only where the callee is a function the
// script defines at its top level or loads, read off the source. So the check
// here is that the callee is a script function with a name, and nothing more:
// asking whether it is one of its module's globals copies those globals on
// every call, measured at 1.2µs a call in a module of fifty. A loaded name can
// still hold a lambda, which no flow lists. A callee that turns out to be that,
// or anything else, is called and not reported.
//
// Returns whatever the call returns, unwrapped, and ERR_NO_CALLEE when handed
// nothing to call.
//
// Revisions:
//   - 2026-10-02 00:42: initial creation
//   - 2026-10-02 01:20: does not report a lambda, which a loaded name can hold
//   - 2026-10-03 08:26: unexported, reached only as CALL
func _Calling(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("%s: %w", fn.Name(), ERR_NO_CALLEE)
	}

	builtin, rest := Hidden(kwargs, spelling.BUILTIN)
	target := args[0]

	script, ok := target.(*starlark.Function)

	reported := ok && script.Name() != LAMBDA
	if reported {
		Open(thread, &Line{Name: script.Name(), Builtin: builtin})
	}

	value, err := starlark.Call(thread, target, args[1:], rest)

	if reported {
		Close(thread, script.Name(), err)
	}

	return value, err
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
func Hidden(kwargs []starlark.Tuple, name string) (string, []starlark.Tuple) {
	named := func(pair starlark.Tuple) bool {
		key, ok := starlark.AsString(pair[0])

		return ok && key == name
	}

	at := slices.IndexFunc(kwargs, named)
	if at == ABSENT {
		return "", kwargs
	}

	// Not a string is no value, as an absent keyword is.
	found, _ := starlark.AsString(kwargs[at][1])

	again := slices.ContainsFunc(kwargs[at+1:], named)
	if again {
		return found, slices.DeleteFunc(slices.Clone(kwargs), named)
	}

	switch at {
	case 0:
		return found, kwargs[1:]

	case len(kwargs) - 1:
		return found, kwargs[:at:at]

	default:
		return found, slices.Concat(kwargs[:at], kwargs[at+1:])
	}
}
