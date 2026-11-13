package tests

import (
	"context"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/deep"
)

// _Pair is the key and the value of one dict member.
//
// Revisions:
//   - 2026-11-02 16:48: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
type _Pair struct {
	key   starlark.Value
	value starlark.Value
}

// _Ints is a tuple of the numbers given.
//
// Revisions:
//   - 2026-11-02 16:30: initial creation (C30)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Ints(numbers ...int) starlark.Tuple {
	tuple := make(
		starlark.Tuple,
		0,
		len(numbers),
	)

	for _, number := range numbers {
		tuple = append(
			tuple,
			starlark.MakeInt(number),
		)
	}

	return tuple
}

// _Dicted builds a dict from pairs in order.
//
// Revisions:
//   - 2026-11-02 16:30: initial creation (C30)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Dicted(
	t *testing.T,
	pairs ...*_Pair,
) *starlark.Dict {
	t.Helper()

	dict := starlark.NewDict(len(pairs))

	for _, pair := range pairs {
		err := dict.SetKey(
			pair.key,
			pair.value,
		)
		if err != nil {
			t.Fatal(err)
		}
	}

	return dict
}

// _Frozen freezes a value and returns it.
//
// Revisions:
//   - 2026-11-02 16:30: initial creation (C30)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Frozen(value starlark.Value) starlark.Value {
	value.Freeze()

	return value
}

// _Copied is deep.Copy of value, failing the test on an error.
//
// Revisions:
//   - 2026-11-02 16:30: initial creation (C30)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Copied(
	t *testing.T,
	value starlark.Value,
) starlark.Value {
	t.Helper()

	made, err := deep.Copy(
		context.Background(),
		value,
	)
	if err != nil {
		t.Fatal(err)
	}

	return made
}

// _Equal fails the test unless left and right are equal.
//
// Revisions:
//   - 2026-11-02 16:30: initial creation (C30)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Equal(
	t *testing.T,
	left starlark.Value,
	right starlark.Value,
) {
	t.Helper()

	equal, err := starlark.Equal(
		left,
		right,
	)
	if err != nil || !equal {
		t.Fatalf(
			"%s differs from %s (%v)",
			left,
			right,
			err,
		)
	}
}

// _Append adds a number to a list, failing the test on an error.
//
// Revisions:
//   - 2026-11-02 16:30: initial creation (C30)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Append(
	t *testing.T,
	list starlark.Value,
	number int,
) {
	t.Helper()

	err := list.(*starlark.List).Append(starlark.MakeInt(number))
	if err != nil {
		t.Fatal(err)
	}
}

// _Set stores a member in a dict, failing the test on an error.
//
// Revisions:
//   - 2026-11-02 16:30: initial creation (C30)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Set(
	t *testing.T,
	dict *starlark.Dict,
	key starlark.Value,
	value starlark.Value,
) {
	t.Helper()

	err := dict.SetKey(
		key,
		value,
	)
	if err != nil {
		t.Fatal(err)
	}
}

// TestCopy_KeepsEveryKindOfKey proves each dict made of hashable keys copies to
// an equal dict, and that each key is found again by its value.
//
// Revisions:
//   - 2026-11-02 16:30: initial creation (C30)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestCopy_KeepsEveryKindOfKey(t *testing.T) {
	cases := map[string]starlark.Value{
		"a string":  starlark.String("k"),
		"an int":    starlark.MakeInt(3),
		"a big int": starlark.MakeInt64(1 << 62),
		"a float":   starlark.Float(2.5),
		"a bool":    starlark.True,
		"none":      starlark.None,
		"bytes":     starlark.Bytes("k"),
		"a tuple of scalars": _Ints(
			1,
			2,
		),
		"an empty tuple": starlark.Tuple{},
		"a tuple of tuples": starlark.Tuple{
			_Ints(1),
			_Ints(
				2,
				3,
			),
			starlark.Tuple{},
		},
		"a tuple of mixed": starlark.Tuple{
			starlark.String("a"),
			starlark.None,
			_Ints(1),
		},
		"a frozen tuple": _Frozen(_Ints(
			4,
			5,
		)),
		"a frozen tuple of pairs": _Frozen(starlark.Tuple{
			_Ints(1),
			starlark.String("x"),
		}),
	}

	for name, key := range cases {
		t.Run(
			name,
			func(t *testing.T) {
				original := _Dicted(
					t,
					&_Pair{
						key,
						starlark.String("v"),
					},
					&_Pair{
						starlark.String("other"),
						starlark.NewList(
							[]starlark.Value{_Ints(9)},
						),
					},
				)

				var copied = _Copied(
					t,
					original,
				).(*starlark.Dict)

				_Equal(
					t,
					original,
					copied,
				)

				got, found, err := copied.Get(key)
				if err != nil || !found || got != starlark.String("v") {
					t.Fatalf(
						"key not found in the copy: %v %v %v",
						got,
						found,
						err,
					)
				}
			},
		)
	}
}

// TestCopy_KeysAreIndependent proves a mutation of the copy never reaches the
// original, and the reverse, while the keys are shared.
//
// Revisions:
//   - 2026-11-02 16:30: initial creation (C30)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestCopy_KeysAreIndependent(t *testing.T) {
	var (
		key = _Ints(
			1,
			2,
		)
		inner = starlark.NewList([]starlark.Value{starlark.MakeInt(1)})
		other = starlark.NewList(nil)
	)

	original := _Dicted(
		t,
		&_Pair{
			key,
			inner,
		},
		&_Pair{
			starlark.String("s"),
			other,
		},
	)

	var copied = _Copied(
		t,
		original,
	).(*starlark.Dict)

	t.Run(
		"the copy changes",
		func(t *testing.T) {
			held, _, _ := copied.Get(key)

			_Append(
				t,
				held,
				2,
			)

			_Set(
				t,
				copied,
				starlark.String("new"),
				starlark.None,
			)

			_, _, err := copied.Delete(key)
			if err != nil {
				t.Fatal(err)
			}

			if inner.Len() != 1 || original.Len() != 2 {
				t.Fatalf(
					"original changed: %s",
					original,
				)
			}

			_, found, _ := original.Get(key)
			if !found {
				t.Fatal("key gone from the original")
			}
		},
	)

	t.Run(
		"the original changes",
		func(t *testing.T) {
			_Append(
				t,
				inner,
				7,
			)

			_Set(
				t,
				original,
				_Ints(8),
				starlark.None,
			)

			if copied.Len() != 2 {
				t.Fatalf(
					"copy changed: %s",
					copied,
				)
			}

			held, _, _ := copied.Get(starlark.String("s"))
			if held.(*starlark.List).Len() != 0 {
				t.Fatalf(
					"copy list changed: %s",
					held,
				)
			}
		},
	)
}

// TestCopy_TupleIsRebuiltOnlyForSomethingMutable proves a tuple of scalars is
// returned as it is, and a tuple that holds, at any depth, a list, a dict or a
// set is new, with that part copied.
//
// Revisions:
//   - 2026-11-02 16:30: initial creation (C30)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestCopy_TupleIsRebuiltOnlyForSomethingMutable(t *testing.T) {
	var set = starlark.NewSet(1)

	err := set.Insert(starlark.MakeInt(1))
	if err != nil {
		t.Fatal(err)
	}

	list := func() starlark.Value {
		return starlark.NewList([]starlark.Value{starlark.MakeInt(1)})
	}

	var cases = []struct {
		name    string
		value   starlark.Tuple
		rebuilt bool
	}{
		{
			"scalars",
			_Ints(
				1,
				2,
				3,
			),
			false,
		},
		{
			"empty",
			starlark.Tuple{},
			false,
		},
		{
			"tuples of scalars",
			starlark.Tuple{_Ints(1), starlark.Tuple{_Ints(2)}},
			false,
		},
		{
			"mixed scalars",
			starlark.Tuple{starlark.None, starlark.String("a"), starlark.Float(1)},
			false,
		},
		{
			"a list",
			starlark.Tuple{starlark.MakeInt(1), list()},
			true,
		},
		{
			"a dict",
			starlark.Tuple{starlark.NewDict(0)},
			true,
		},
		{
			"a set",
			starlark.Tuple{set},
			true,
		},
		{
			"a list in a tuple",
			starlark.Tuple{starlark.Tuple{list()}},
			true,
		},
		{
			"a list after tuples",
			starlark.Tuple{_Ints(1), _Ints(2), list()},
			true,
		},
		{
			"a frozen list",
			starlark.Tuple{_Frozen(list())},
			true,
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				copied := _Copied(
					t,
					tc.value,
				).(starlark.Tuple)

				_Equal(
					t,
					tc.value,
					copied,
				)

				same := len(tc.value) == 0 || &copied[0] == &tc.value[0]
				if same == tc.rebuilt {
					t.Fatalf(
						"rebuilt is %v, want %v",
						!same,
						tc.rebuilt,
					)
				}
			},
		)
	}
}

// TestCopy_ListUnderATupleIsIndependent proves the list inside a rebuilt tuple
// is a new list, and a change on either side stays on that side.
//
// Revisions:
//   - 2026-11-02 16:30: initial creation (C30)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestCopy_ListUnderATupleIsIndependent(t *testing.T) {
	var (
		list  = starlark.NewList([]starlark.Value{starlark.MakeInt(1)})
		inner = starlark.NewDict(0)
	)

	original := starlark.Tuple{
		_Ints(
			1,
			2,
		),
		starlark.Tuple{list, _Ints(3)},
		inner,
	}

	var copied = _Copied(
		t,
		original,
	).(starlark.Tuple)

	var (
		copiedList = copied[1].(starlark.Tuple)[0].(*starlark.List)
		copiedDict = copied[2].(*starlark.Dict)
	)

	if copiedList == list || copiedDict == inner {
		t.Fatal("a mutable part is shared")
	}

	_Append(
		t,
		copiedList,
		2,
	)

	_Set(
		t,
		copiedDict,
		starlark.MakeInt(1),
		starlark.MakeInt(1),
	)

	if list.Len() != 1 || inner.Len() != 0 {
		t.Fatal("the original changed with the copy")
	}

	_Append(
		t,
		list,
		5,
	)

	if copiedList.Len() != 2 {
		t.Fatalf(
			"the copy changed with the original: %s",
			copiedList,
		)
	}
}

// TestCopy_SharedListUnderTuplesStaysShared proves a list reached through two
// tuples is still one list in the copy.
//
// Revisions:
//   - 2026-11-02 16:30: initial creation (C30)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestCopy_SharedListUnderTuplesStaysShared(t *testing.T) {
	var list = starlark.NewList(nil)

	original := starlark.NewList([]starlark.Value{
		starlark.Tuple{list},
		starlark.Tuple{list, _Ints(1)},
	})

	var copied = _Copied(
		t,
		original,
	).(*starlark.List)

	var (
		first  = copied.Index(0).(starlark.Tuple)[0]
		second = copied.Index(1).(starlark.Tuple)[0]
	)

	if first != second || first == starlark.Value(list) {
		t.Fatal("sharing was not kept, or the list was not copied")
	}
}

// TestCopy_TupleKeysAllocateLittle proves the two measured wins stay: a dict of
// 1000 tuple keys (81 allocations, was 2080) and a tuple of scalars (3, was 4).
//
// Revisions:
//   - 2026-11-02 16:30: initial creation (C30)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestCopy_TupleKeysAllocateLittle(t *testing.T) {
	dict := starlark.NewDict(1000)

	for idx := range 1000 {
		_Set(
			t,
			dict,
			_Ints(
				idx,
				idx+1,
			),
			starlark.MakeInt(idx),
		)
	}

	cases := []struct {
		name  string
		value starlark.Value
		most  float64
	}{
		{
			"dict of tuple keys",
			dict,
			200,
		},
		{
			"tuple of scalars",
			_Ints(
				1,
				2,
				3,
				4,
				5,
				6,
				7,
				8,
			),
			3,
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				made := testing.AllocsPerRun(
					RUNS,
					func() {
						_, _ = deep.Copy(
							context.Background(),
							tc.value,
						)
					},
				)

				if made > tc.most {
					t.Fatalf(
						"%v allocations, want at most %v",
						made,
						tc.most,
					)
				}
			},
		)
	}
}
