package tests

import (
	"fmt"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/jsonpath/internal/spec"
)

const (
	// BENCH is the name of the thread that each benchmark calls a builtin on.
	//
	// Revisions:
	//   - 2026-11-02 00:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	BENCH = "bench"

	// LISTED is the count of numbers in the list of the benchmark document.
	//
	// Revisions:
	//   - 2026-11-02 00:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	LISTED = 1000

	// ADDED is the count of operations in the many-add patch benchmark.
	//
	// Revisions:
	//   - 2026-11-02 11:37: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	ADDED = 100

	// SEGMENTS is the count of steps in the pointer of the long-path benchmark.
	//
	// Revisions:
	//   - 2026-11-02 20:56: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	SEGMENTS = 15

	// KEYS is the count of members in the dict of the replace benchmark.
	//
	// Revisions:
	//   - 2026-11-02 20:56: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	KEYS = 4096

	// REPLACED is the count of operations in the many-replace patch case.
	//
	// Revisions:
	//   - 2026-11-02 20:56: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	REPLACED = 32

	// SPACING is the distance between the keys that the many-replace case
	// replaces, so the operations touch different parts of the dict.
	//
	// Revisions:
	//   - 2026-11-02 20:56: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	SPACING = 100
)

// BenchmarkExtract_Hit measures extract_json on a path that is in the document,
// through the builtin, so the allocations include the argument reading and the
// pointer. The document is the one of the research scratch benchmark.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation, from BenchmarkExtractHit of
//     poc/research/plugins_data_bench
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkExtract_Hit(b *testing.B) {
	_Call(
		b,
		_Builtin(
			b,
			spec.EXTRACT,
		),
		starlark.Tuple{
			_Document(b),
			starlark.String("/list/500"),
		},
	)
}

// BenchmarkExtract_Miss measures extract_json on a path that is not in the
// document, so the allocations include the error a miss builds.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation, from BenchmarkExtractMiss of
//     poc/research/plugins_data_bench
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkExtract_Miss(b *testing.B) {
	_Call(
		b,
		_Builtin(
			b,
			spec.EXTRACT,
		),
		starlark.Tuple{
			_Document(b),
			starlark.String("/nothing/here"),
		},
	)
}

// BenchmarkExtract_HitDeep measures extract_json on a hit three steps down, the
// depth at which the step slice of the old walk cost the most.
//
// Revisions:
//   - 2026-11-02 17:12: initial creation (C36)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkExtract_HitDeep(b *testing.B) {
	_Call(
		b,
		_Builtin(
			b,
			spec.EXTRACT,
		),
		starlark.Tuple{
			_Chain(b),
			starlark.String("/a/b/c"),
		},
	)
}

// BenchmarkExtract_HitEscaped measures extract_json on a hit whose tokens hold
// both escapes, so the allocations include the decoded tokens.
//
// Revisions:
//   - 2026-11-02 17:12: initial creation (C36)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkExtract_HitEscaped(b *testing.B) {
	_Call(
		b,
		_Builtin(
			b,
			spec.EXTRACT,
		),
		starlark.Tuple{
			_Chain(b),
			starlark.String("/e~1f/g~0h"),
		},
	)
}

// BenchmarkExtract_MissDeep measures extract_json on a miss three steps down.
//
// Revisions:
//   - 2026-11-02 17:12: initial creation (C36)
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkExtract_MissDeep(b *testing.B) {
	_Call(
		b,
		_Builtin(
			b,
			spec.EXTRACT,
		),
		starlark.Tuple{
			_Chain(b),
			starlark.String("/a/b/nothing"),
		},
	)
}

// BenchmarkExtract_HitLong measures extract_json on a hit SEGMENTS steps down,
// where the pointer is about 200 bytes long. The allocations show whether the
// cost grows with the depth of the path: one box for each dict step, so
// SEGMENTS, plus two for the argument reading. A hit with one allocation needs
// the unapplied P4 change of the research.
//
// Revisions:
//   - 2026-11-02 17:49: initial creation (C36c), from BenchmarkExtractHit_Depth15 of
//     poc/research/simd_json
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkExtract_HitLong(b *testing.B) {
	var (
		names = make(
			[]string,
			SEGMENTS,
		)
		document starlark.Value
	)

	for idx := range names {
		names[idx] = "segment_name"
	}

	document = starlark.MakeInt(1)

	for _, name := range names {
		var (
			parent = starlark.NewDict(1)
			err    = parent.SetKey(
				starlark.String(name),
				document,
			)
		)
		if err != nil {
			b.Fatal(err)
		}

		document = parent
	}

	_Call(
		b,
		_Builtin(
			b,
			spec.EXTRACT,
		),
		starlark.Tuple{
			document,
			starlark.String("/" + strings.Join(
				names,
				"/",
			)),
		},
	)
}

// _Chain is a document with a chain of dicts and a key of each escape.
//
// Revisions:
//   - 2026-11-02 17:12: initial creation (C36)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Chain(b *testing.B) *starlark.Dict {
	b.Helper()

	var (
		leaf   = starlark.NewDict(1)
		middle = starlark.NewDict(1)
		escape = starlark.NewDict(1)
		root   = starlark.NewDict(2)
	)

	for _, step := range []struct {
		holder *starlark.Dict
		key    string
		value  starlark.Value
	}{
		{
			leaf,
			"c",
			starlark.MakeInt(1),
		},
		{
			middle,
			"b",
			leaf,
		},
		{
			escape,
			"g~h",
			starlark.MakeInt(2),
		},
		{
			root,
			"a",
			middle,
		},
		{
			root,
			"e/f",
			escape,
		},
	} {
		err := step.holder.SetKey(
			starlark.String(step.key),
			step.value,
		)
		if err != nil {
			b.Fatal(err)
		}
	}

	return root
}

// BenchmarkPatch_AddInList measures patch_json with one add in the middle of
// the list of LISTED numbers. The patch copies the document, so the allocations
// include the copy.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation, from BenchmarkPatchAddMiddleOfList1000
//     of poc/research/plugins_data_bench
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkPatch_AddInList(b *testing.B) {
	operation := starlark.NewDict(3)

	for key, value := range map[string]starlark.Value{
		"op":    starlark.String("add"),
		"path":  starlark.String("/list/500"),
		"value": starlark.MakeInt(7),
	} {
		err := operation.SetKey(
			starlark.String(key),
			value,
		)
		if err != nil {
			b.Fatal(err)
		}
	}

	_Call(
		b,
		_Builtin(
			b,
			spec.PATCH,
		),
		starlark.Tuple{
			_Document(b),
			starlark.NewList([]starlark.Value{
				operation,
			}),
		},
	)
}

// BenchmarkPatch_RemoveInList measures patch_json with one remove in the middle
// of the list of LISTED numbers. The patch copies the document, so the
// allocations include the copy.
//
// Revisions:
//   - 2026-11-02 00:58: initial creation, from BenchmarkPatch_AddInList
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkPatch_RemoveInList(b *testing.B) {
	operation := starlark.NewDict(2)

	for key, value := range map[string]starlark.Value{
		"op":   starlark.String("remove"),
		"path": starlark.String("/list/500"),
	} {
		err := operation.SetKey(
			starlark.String(key),
			value,
		)
		if err != nil {
			b.Fatal(err)
		}
	}

	_Call(
		b,
		_Builtin(
			b,
			spec.PATCH,
		),
		starlark.Tuple{
			_Document(b),
			starlark.NewList([]starlark.Value{
				operation,
			}),
		},
	)
}

// BenchmarkPatch_AddMany measures patch_json with ADDED adds of one member each
// to a dict three levels down. The patch copies the three dicts on the path
// once, then sets the members in place, so the allocations show the cost of
// each operation after the first.
//
// Revisions:
//   - 2026-11-02 11:30: initial creation, from BenchmarkPatch_100Add_Deep of
//     poc/research/simd_json
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkPatch_AddMany(b *testing.B) {
	var (
		inner      = starlark.NewDict(0)
		operations = make(
			[]starlark.Value,
			ADDED,
		)
		document starlark.Value
	)

	document = inner

	for _, name := range []string{"c", "b", "a"} {
		var (
			parent = starlark.NewDict(1)
			err    = parent.SetKey(
				starlark.String(name),
				document,
			)
		)
		if err != nil {
			b.Fatal(err)
		}

		document = parent
	}

	for idx := range operations {
		operation := starlark.NewDict(3)

		for key, value := range map[string]starlark.Value{
			"op": starlark.String("add"),
			"path": starlark.String(fmt.Sprintf(
				"/a/b/c/member_%d",
				idx,
			)),
			"value": starlark.MakeInt(idx),
		} {
			err := operation.SetKey(
				starlark.String(key),
				value,
			)
			if err != nil {
				b.Fatal(err)
			}
		}

		operations[idx] = operation
	}

	_Call(
		b,
		_Builtin(
			b,
			spec.PATCH,
		),
		starlark.Tuple{
			document,
			starlark.NewList(operations),
		},
	)
}

// BenchmarkPatch_Replace measures patch_json on a dict of KEYS numbers with
// one replace and with REPLACED replaces. The patch copies the dict once, so
// the allocations of the two cases show the cost of the copy and the cost of
// each operation after the first.
//
// Revisions:
//   - 2026-11-02 17:30: initial creation (C36c), from BenchmarkPatch_1Replace_Dict4096 and
//     BenchmarkPatch_32Replace_Dict4096 of poc/research/simd_json
//   - 2026-11-08 00:26: read the names from internal/spec (C91w12b)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkPatch_Replace(b *testing.B) {
	cases := []struct {
		name  string
		count int
	}{
		{
			name:  "one",
			count: 1,
		},
		{
			name:  "many",
			count: REPLACED,
		},
	}

	for _, tc := range cases {
		b.Run(
			tc.name,
			func(b *testing.B) {
				operations := make(
					[]starlark.Value,
					tc.count,
				)

				for idx := range operations {
					operation := starlark.NewDict(3)

					for key, value := range map[string]starlark.Value{
						"op": starlark.String("replace"),
						"path": starlark.String(fmt.Sprintf(
							"/key_%d",
							idx*SPACING,
						)),
						"value": starlark.MakeInt(-1),
					} {
						err := operation.SetKey(
							starlark.String(key),
							value,
						)
						if err != nil {
							b.Fatal(err)
						}
					}

					operations[idx] = operation
				}

				_Call(
					b,
					_Builtin(
						b,
						spec.PATCH,
					),
					starlark.Tuple{
						_Keyed(b),
						starlark.NewList(operations),
					},
				)
			},
		)
	}
}

// _Keyed is a dict of KEYS members named key_0 up, whose values are numbers.
//
// Revisions:
//   - 2026-11-02 17:30: initial creation (C36c)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Keyed(b *testing.B) *starlark.Dict {
	b.Helper()

	dict := starlark.NewDict(KEYS)

	for idx := range KEYS {
		err := dict.SetKey(
			starlark.String(fmt.Sprintf(
				"key_%d",
				idx,
			)),
			starlark.MakeInt(idx),
		)
		if err != nil {
			b.Fatal(err)
		}
	}

	return dict
}

// _Document is a dict with a string and a list of LISTED numbers.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Document(b *testing.B) *starlark.Dict {
	b.Helper()

	list := starlark.NewList(nil)

	for idx := range LISTED {
		err := list.Append(starlark.MakeInt(idx))
		if err != nil {
			b.Fatal(err)
		}
	}

	var (
		document = starlark.NewDict(2)
		err      error
	)

	err = document.SetKey(
		starlark.String("a"),
		starlark.String("x"),
	)
	if err != nil {
		b.Fatal(err)
	}

	err = document.SetKey(
		starlark.String("list"),
		list,
	)
	if err != nil {
		b.Fatal(err)
	}

	return document
}

// _Builtin is the named function of the jsonpath plugin, found in the
// environment a script gets, so the benchmark calls what a script calls.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Builtin(
	b *testing.B,
	name string,
) *starlark.Builtin {
	b.Helper()

	env, err := plugin.Environment(plugin.Registered())
	if err != nil {
		b.Fatalf(
			"environment: %v",
			err,
		)
	}

	fn, ok := env[name].(*starlark.Builtin)
	if !ok {
		b.Fatalf(
			"%s is not a builtin",
			name,
		)
	}

	return fn
}

// _Call calls fn on args once per iteration, as a script's call reaches it,
// and ends the benchmark at the first error.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Call(
	b *testing.B,
	fn *starlark.Builtin,
	args starlark.Tuple,
) {
	b.Helper()

	thread := &starlark.Thread{
		Name: BENCH,
	}

	b.ReportAllocs()

	for b.Loop() {
		_, err := fn.CallInternal(
			thread,
			args,
			nil,
		)
		if err != nil {
			b.Fatal(err)
		}
	}
}
