// A refusal of the arguments of a builtin names the builtin once.
//
// Revisions:
//   - 2026-11-02 10:48: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
package tests

import (
	"errors"
	"strings"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/flow"
)

// TestBuiltins_ArgumentRefusalsNameTheBuiltinOnce calls each builtin with the
// wrong arguments and pins the whole text of the refusal.
//
// starlark.UnpackArgs and the unpack helpers write the builtin name at the
// head of their errors. A builtin that wrapped such an error with its name
// again said it twice, for example "utils.datetime: utils.datetime: got 1
// arguments, want 0". Each row here would show the second name.
//
// The thread is bare. Every refusal comes before the builtin looks for a run.
//
// Revisions:
//   - 2026-11-02 08:41: initial creation
//   - 2026-11-02 10:41: rows for the sentinels join, spawn, assert and n
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrCondition and ErrHandle and ErrFunction, formerly ErrNotACondition
//     and ErrNotAHandle and ErrNotAName (C121, Q348)
func TestBuiltins_ArgumentRefusalsNameTheBuiltinOnce(t *testing.T) {
	var (
		none = starlark.None
		text = starlark.String("x")
		num  = starlark.MakeInt(-1)

		keyword = []starlark.Tuple{
			{
				starlark.String("zz"),
				starlark.MakeInt(1),
			},
		}

		tests = []struct {
			name    string
			builtin string
			args    starlark.Tuple
			kwargs  []starlark.Tuple
			want    string
			is      error
		}{
			{
				name:    "utils.datetime with an argument",
				builtin: "utils.datetime",
				args:    starlark.Tuple{none},
				want:    "utils.datetime: got 1 arguments, want 0",
			},
			{
				name:    "utils.datetime with a keyword",
				builtin: "utils.datetime",
				kwargs:  keyword,
				want:    "utils.datetime: unexpected keyword arguments",
			},
			{
				name:    "n with an argument",
				builtin: "n",
				args:    starlark.Tuple{none},
				want:    "n: got 1 arguments, want 0",
			},
			{
				name:    "assert with three arguments",
				builtin: "assert",
				args:    starlark.Tuple{none, none, none},
				want:    "assert: got 3 arguments, want at most 2",
			},
			{
				name:    "assert with a keyword",
				builtin: "assert",
				kwargs:  keyword,
				want:    `assert: unexpected keyword argument "zz"`,
			},
			{
				name:    "sleep with nothing",
				builtin: "sleep",
				want:    "sleep: got 0 arguments, want 1",
			},
			{
				name:    "sleep with a keyword",
				builtin: "sleep",
				kwargs:  keyword,
				want:    "sleep: unexpected keyword arguments",
			},
			{
				name:    "repeat with one argument",
				builtin: "repeat",
				args:    starlark.Tuple{num},
				want:    "repeat: got 1 arguments, want at least 2",
			},
			{
				name:    "repeat with a count that is a string",
				builtin: "repeat",
				args:    starlark.Tuple{text, none},
				want:    "repeat: for parameter 1: got string, want int",
			},
			{
				name:    "retry with one argument",
				builtin: "retry",
				args:    starlark.Tuple{num},
				want:    "retry: got 1 arguments, want at least 2",
			},
			{
				name:    "retry with a count that is a string",
				builtin: "retry",
				args:    starlark.Tuple{text, none},
				want:    "retry: for parameter 1: got string, want int",
			},
			{
				name:    "timeout with one argument",
				builtin: "timeout",
				args:    starlark.Tuple{num},
				want:    "timeout: got 1 arguments, want 2",
			},
			{
				name:    "timeout with a target that is None",
				builtin: "timeout",
				args:    starlark.Tuple{num, none},
				want:    "timeout: for parameter 2: got NoneType, want function",
			},
			{
				name:    "event.post with one argument",
				builtin: "event.post",
				args:    starlark.Tuple{text},
				want:    "event.post: got 1 arguments, want 2",
			},
			{
				name:    "event.post with a name that is None",
				builtin: "event.post",
				args:    starlark.Tuple{none, none},
				want:    "event.post: for parameter 1: got NoneType, want string",
			},
			{
				name:    "event.wait with one argument",
				builtin: "event.wait",
				args:    starlark.Tuple{text},
				want:    "event.wait: got 1 arguments, want 2",
			},
			{
				name:    "event.wait with a name that is None",
				builtin: "event.wait",
				args:    starlark.Tuple{none, none},
				want:    "event.wait: for parameter 1: got NoneType, want string",
			},
			{
				name:    "extract_json with one argument",
				builtin: "extract_json",
				args:    starlark.Tuple{none},
				want:    "extract_json: got 1 arguments, want 2",
			},
			{
				name:    "extract_json with a None pointer",
				builtin: "extract_json",
				args:    starlark.Tuple{none, none},
				want: "extract_json: for parameter 2: " +
					"got NoneType, want string",
			},
			{
				name:    "find_key with one argument",
				builtin: "find_key",
				args:    starlark.Tuple{none},
				want:    "find_key: got 1 arguments, want 2",
			},
			{
				name:    "find_key with a key that is an int",
				builtin: "find_key",
				args:    starlark.Tuple{text, num},
				want:    "find_key: for parameter 2: got int, want string",
			},
			{
				name:    "len_json with one argument",
				builtin: "len_json",
				args:    starlark.Tuple{none},
				want:    "len_json: got 1 arguments, want 2",
			},
			{
				name:    "len_json with a None pointer",
				builtin: "len_json",
				args:    starlark.Tuple{none, none},
				want:    "len_json: for parameter 2: got NoneType, want string",
			},
			{
				name:    "match_json with two arguments",
				builtin: "match_json",
				args:    starlark.Tuple{none, none},
				want:    "match_json: got 2 arguments, want 3",
			},
			{
				name:    "match_json with a None pointer",
				builtin: "match_json",
				args:    starlark.Tuple{none, none, none},
				want:    "match_json: for parameter 2: got NoneType, want string",
			},
			{
				name:    "patch_json with one argument",
				builtin: "patch_json",
				args:    starlark.Tuple{none},
				want:    "patch_json: got 1 arguments, want 2",
			},
			{
				name:    "patch_json with a patch that is a string",
				builtin: "patch_json",
				args:    starlark.Tuple{none, text},
				want:    "patch_json: for parameter 2: got string, want list",
			},
			{
				name:    "hash.crc32 with nothing",
				builtin: "hash.crc32",
				want:    "hash.crc32: got 0 arguments, want at least 1",
			},
			{
				name:    "hash.crc32 with a seed that is None",
				builtin: "hash.crc32",
				args:    starlark.Tuple{text, none},
				want:    "hash.crc32: for parameter 2: got NoneType, want int",
			},
			{
				name:    "state.get with nothing",
				builtin: "state.get",
				want:    "state.get: got 0 arguments, want 1",
			},
			{
				name:    "state.set with one argument",
				builtin: "state.set",
				args:    starlark.Tuple{text},
				want:    "state.set: got 1 arguments, want 2",
			},
			{
				name:    "state.set with a name that is None",
				builtin: "state.set",
				args:    starlark.Tuple{none, none},
				want:    "state.set: for parameter 1: got NoneType, want string",
			},
			{
				name:    "state.update with a change that is an int",
				builtin: "state.update",
				args:    starlark.Tuple{text, num},
				want:    "state.update: for parameter 2: got int, want callable",
			},
			{
				name:    "n outside a wrapper",
				builtin: "n",
				want:    "n: only meaningful inside repeat or retry",
				is:      flow.ErrAttempt,
			},
			{
				name:    "join of a None",
				builtin: "join",
				args:    starlark.Tuple{none},
				want:    "join got NoneType: wants handles",
				is:      runtime.ErrHandle,
			},
			{
				name:    "join with a keyword",
				builtin: "join",
				kwargs:  keyword,
				want:    "join takes no keyword arguments: wants handles",
				is:      runtime.ErrHandle,
			},
			{
				name:    "spawn of a None",
				builtin: "spawn",
				args:    starlark.Tuple{none},
				want:    "spawn got NoneType: wants a function",
				is:      runtime.ErrFunction,
			},
			{
				name:    "spawn of nothing",
				builtin: "spawn",
				want:    "spawn takes one function: wants a function",
				is:      runtime.ErrFunction,
			},
			{
				name:    "assert of a string alone",
				builtin: "assert",
				args:    starlark.Tuple{text},
				want: `assert got only the message "x": ` +
					`write assert(False, msg) to fail, ` +
					`or assert(msg = ...): wants a condition`,
				is: runtime.ErrCondition,
			},
		}
	)

	env, err := plugin.Environment(plugin.Registered())
	if err != nil {
		t.Fatalf(
			"the environment is refused: %v",
			err,
		)
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				var (
					fn = _Builtin(
						t,
						env,
						tt.builtin,
					)
					_, err = starlark.Call(
						new(starlark.Thread),
						fn,
						tt.args,
						tt.kwargs,
					)
				)
				if err == nil {
					t.Fatalf(
						"got no error, want %q",
						tt.want,
					)
				}

				if tt.is != nil && !errors.Is(
					err,
					tt.is,
				) {
					t.Errorf(
						"got %v, want it to match %v",
						err,
						tt.is,
					)
				}

				if err.Error() != tt.want {
					t.Errorf(
						"got %q, want %q",
						err.Error(),
						tt.want,
					)
				}
			},
		)
	}
}

// _Builtin finds the builtin a dotted name points to, in the environment.
//
// Revisions:
//   - 2026-11-02 08:41: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Builtin(
	t *testing.T,
	env starlark.StringDict,
	dotted string,
) *starlark.Builtin {
	t.Helper()

	var (
		parts = strings.Split(
			dotted,
			".",
		)
		found = env[parts[0]]
	)

	if len(parts) == 2 {
		module, ok := found.(*starlarkstruct.Module)
		if !ok {
			t.Fatalf(
				"%s is %v, want a module",
				parts[0],
				found,
			)
		}

		found = module.Members[parts[1]]
	}

	fn, ok := found.(*starlark.Builtin)
	if !ok {
		t.Fatalf(
			"%s is %v, want a builtin",
			dotted,
			found,
		)
	}

	return fn
}
