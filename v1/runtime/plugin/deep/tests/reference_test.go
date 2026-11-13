package tests

// This file tests Copy against what it did before it skipped the map for a
// scalar: Copy was Into with a fresh map. The test keeps that older form to
// compare with.
//
// Revisions:
//   - 2026-11-08 22:46: states the reason in the first comment (C65b)
//   - 2026-11-10 01:37: moved to tests/ from reference_internal_test.go and rewrote its
//     header (C120b, Q311, Q322)

import (
	"context"
	"fmt"
	"math/big"
	"slices"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/deep"
	"github.com/thebagchi/lark/v1/runtime/plugin/deep/internal/clone"
	"github.com/thebagchi/lark/v1/runtime/trail"
)

const (
	// BIG_SHIFT makes an integer too large for a machine word.
	//
	// Revisions:
	//   - 2026-10-29 06:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	BIG_SHIFT = 100
)

// TestCopy_SameAsBefore is Copy against Into with a fresh map, which is what
// Copy was. The two copies must have one shape, sharing and cycles included,
// and the new copy must own every container it holds.
//
// Revisions:
//   - 2026-10-29 01:22: initial creation
//   - 2026-11-02 12:28: Copy and _Into take a context and a trail (C26)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestCopy_SameAsBefore(t *testing.T) {
	for name, value := range _Cases(t) {
		t.Run(
			name,
			func(t *testing.T) {
				var (
					want, wanted = clone.Into(
						trail.Bounded(context.Background()),
						value,
						map[starlark.Value]starlark.Value{},
					)
					got, err = deep.Copy(
						context.Background(),
						value,
					)
				)

				if fmt.Sprint(err) != fmt.Sprint(wanted) {
					t.Fatalf(
						"error %v, before it was %v",
						err,
						wanted,
					)
				}

				var (
					before = _Shape(
						want,
						map[starlark.Value]int{},
						nil,
					)
					after = _Shape(
						got,
						map[starlark.Value]int{},
						nil,
					)
				)

				if !slices.Equal(
					after,
					before,
				) {
					t.Fatalf(
						"copy is %v, before it was %v",
						after,
						before,
					)
				}

				_Owned(
					t,
					value,
					got,
				)
			},
		)
	}
}

// _Cases is every shape the comparison runs over. The containers are frozen,
// as a store freezes what it holds, so a copy that returned one would show.
//
// Revisions:
//   - 2026-10-29 01:22: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Cases(t *testing.T) map[string]starlark.Value {
	t.Helper()

	var (
		huge = starlark.MakeBigInt(new(big.Int).Lsh(
			big.NewInt(1),
			BIG_SHIFT,
		))
		inner = starlark.NewList([]starlark.Value{
			starlark.MakeInt(1),
			starlark.MakeInt(2),
		})
		loop  = starlark.NewList([]starlark.Value{starlark.MakeInt(1)})
		round = starlark.NewList(nil)
		self  = starlark.NewDict(1)
		hop   = starlark.NewDict(1)
	)

	_Must(
		t,
		loop.Append(loop),
	)
	_Must(
		t,
		round.Append(starlark.Tuple{starlark.String("a"), round}),
	)
	_Must(
		t,
		self.SetKey(
			starlark.String("self"),
			self,
		),
	)
	_Must(
		t,
		hop.SetKey(
			starlark.String("back"),
			starlark.NewList([]starlark.Value{hop}),
		),
	)

	cases := map[string]starlark.Value{
		"none":        starlark.None,
		"a bool":      starlark.True,
		"a small int": starlark.MakeInt(7),
		"a big int":   huge,
		"a float":     starlark.Float(1.5),
		"a string":    starlark.String("word"),
		"bytes":       starlark.Bytes("raw"),
		"empty list":  starlark.NewList(nil),
		"empty dict":  starlark.NewDict(0),
		"empty tuple": starlark.Tuple{},
		"empty set":   starlark.NewSet(0),
		"nested list": starlark.NewList([]starlark.Value{
			starlark.MakeInt(1),
			starlark.NewList([]starlark.Value{
				starlark.String("a"),
				starlark.Tuple{starlark.MakeInt(2), starlark.Float(3)},
			}),
			_Mapping(
				t,
				starlark.String("k"),
				starlark.NewList([]starlark.Value{starlark.None}),
			),
		}),
		"nested dict": _Mapping(
			t,
			starlark.String("a"),
			starlark.NewList([]starlark.Value{huge, starlark.MakeInt(2)}),
			starlark.Tuple{starlark.String("t"), starlark.MakeInt(1)},
			_Mapping(
				t,
				starlark.String("b"),
				starlark.Bytes("x"),
			),
		),
		"nested tuple": starlark.Tuple{
			starlark.MakeInt(1),
			starlark.NewList([]starlark.Value{
				starlark.MakeInt(2),
				starlark.Tuple{starlark.MakeInt(3)},
			}),
			_Mapping(
				t,
				starlark.String("k"),
				starlark.String("v"),
			),
		},
		"a set": _Bag(
			t,
			starlark.MakeInt(1),
			starlark.String("a"),
			starlark.Tuple{starlark.String("t"), starlark.MakeInt(2)},
		),
		"a shared list": starlark.NewList([]starlark.Value{
			inner,
			inner,
			starlark.Tuple{inner},
			_Mapping(
				t,
				starlark.String("x"),
				inner,
			),
		}),
		"a list that holds itself":      loop,
		"a list that a tuple loops to":  round,
		"a dict that holds itself":      self,
		"a dict that a list loops back": hop,
	}

	for _, value := range cases {
		value.Freeze()
	}

	return cases
}

// _Mapping is a dict of the pairs given, key then value, in that order.
//
// Revisions:
//   - 2026-10-29 01:22: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Mapping(
	t *testing.T,
	pairs ...starlark.Value,
) *starlark.Dict {
	t.Helper()

	made := starlark.NewDict(len(pairs) / 2)

	for index := 0; index < len(pairs); index += 2 {
		_Must(
			t,
			made.SetKey(
				pairs[index],
				pairs[index+1],
			),
		)
	}

	return made
}

// _Bag is a set of the members given.
//
// Revisions:
//   - 2026-10-29 01:22: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Bag(
	t *testing.T,
	members ...starlark.Value,
) *starlark.Set {
	t.Helper()

	made := starlark.NewSet(len(members))

	for _, member := range members {
		_Must(
			t,
			made.Insert(member),
		)
	}

	return made
}

// _Must ends the test at a fixture that cannot be built.
//
// Revisions:
//   - 2026-10-29 01:22: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Must(
	t *testing.T,
	err error,
) {
	t.Helper()

	if err != nil {
		t.Fatal(err)
	}
}

// _Shape is value written out as tokens, with each list, dict, and set
// numbered at its first visit and named by that number after. Two values with
// one shape share in the same places and loop in the same places.
//
// Revisions:
//   - 2026-10-29 01:22: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Shape(
	value starlark.Value,
	ids map[starlark.Value]int,
	out []string,
) []string {
	switch held := value.(type) {
	case *starlark.List:
		tag, found := _Tag(
			held,
			ids,
		)
		if found {
			return append(
				out,
				tag,
			)
		}

		out = append(
			out,
			"list"+tag+"[",
		)

		for index := range held.Len() {
			out = _Shape(
				held.Index(index),
				ids,
				out,
			)
		}

		return append(
			out,
			"]",
		)

	case *starlark.Dict:
		tag, found := _Tag(
			held,
			ids,
		)
		if found {
			return append(
				out,
				tag,
			)
		}

		out = append(
			out,
			"dict"+tag+"{",
		)

		for key, item := range held.Entries() {
			out = _Shape(
				key,
				ids,
				out,
			)
			out = _Shape(
				item,
				ids,
				out,
			)
		}

		return append(
			out,
			"}",
		)

	case *starlark.Set:
		tag, found := _Tag(
			held,
			ids,
		)
		if found {
			return append(
				out,
				tag,
			)
		}

		out = append(
			out,
			"set"+tag+"{",
		)

		for member := range held.Elements() {
			out = _Shape(
				member,
				ids,
				out,
			)
		}

		return append(
			out,
			"}",
		)

	case starlark.Tuple:
		out = append(
			out,
			"(",
		)

		for _, item := range held {
			out = _Shape(
				item,
				ids,
				out,
			)
		}

		return append(
			out,
			")",
		)
	}

	return append(
		out,
		value.Type()+":"+value.String(),
	)
}

// _Tag is the number of a container, and whether _Shape already gave it one.
//
// Revisions:
//   - 2026-10-29 01:22: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Tag(
	value starlark.Value,
	ids map[starlark.Value]int,
) (string, bool) {
	id, found := ids[value]
	if found {
		return fmt.Sprintf(
			"#%d",
			id,
		), true
	}

	id = len(ids)
	ids[value] = id

	return fmt.Sprintf(
		"#%d",
		id,
	), false
}

// _Owned fails the test when the copy holds a list, dict, or set of the
// original. A copy that shares one is not a copy a script can change.
//
// Revisions:
//   - 2026-10-29 01:22: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Owned(
	t *testing.T,
	original starlark.Value,
	copied starlark.Value,
) {
	t.Helper()

	var (
		theirs = map[starlark.Value]bool{}
		ours   = map[starlark.Value]bool{}
	)

	_Collect(
		original,
		theirs,
	)
	_Collect(
		copied,
		ours,
	)

	for held := range ours {
		if theirs[held] {
			t.Fatalf(
				"the copy shares %v with the original",
				held,
			)
		}
	}
}

// _Collect puts every list, dict, and set under value into found.
//
// Revisions:
//   - 2026-10-29 01:22: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Collect(
	value starlark.Value,
	found map[starlark.Value]bool,
) {
	switch held := value.(type) {
	case *starlark.List, *starlark.Dict, *starlark.Set:
		if found[held] {
			return
		}

		found[held] = true
	}

	switch held := value.(type) {
	case *starlark.List:
		for index := range held.Len() {
			_Collect(
				held.Index(index),
				found,
			)
		}

	case *starlark.Dict:
		for key, item := range held.Entries() {
			_Collect(
				key,
				found,
			)
			_Collect(
				item,
				found,
			)
		}

	case *starlark.Set:
		for member := range held.Elements() {
			_Collect(
				member,
				found,
			)
		}

	case starlark.Tuple:
		for _, item := range held {
			_Collect(
				item,
				found,
			)
		}
	}
}
