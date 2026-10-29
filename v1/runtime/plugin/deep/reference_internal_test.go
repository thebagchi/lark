package deep

// This test is internal because what Copy and Size did before they skipped the
// map for a scalar is unexported: Copy was _Into with a fresh map, and Size
// was the walk that _WeighBefore keeps below, word for word.

import (
	"fmt"
	"math/big"
	"slices"
	"testing"

	"go.starlark.net/starlark"
)

const (
	// BIG_SHIFT makes an integer too large for a machine word, the one case
	// NUMBER undercounts.
	BIG_SHIFT = 100
)

// TestSize_SameAsBefore is Size against the walk it replaced, over every shape
// a store holds: each scalar, each container empty and nested, a value reached
// twice, and a value that holds itself.
//
// Revisions:
//   - 2026-10-29 01:22: initial creation
func TestSize_SameAsBefore(t *testing.T) {
	for name, value := range _Cases(t) {
		t.Run(
			name,
			func(t *testing.T) {
				var (
					want = _WeighBefore(
						value,
						map[starlark.Value]bool{},
					)
					got = Size(value)
				)

				if got != want {
					t.Fatalf(
						"size %d, before it was %d",
						got,
						want,
					)
				}
			},
		)
	}
}

// TestCopy_SameAsBefore is Copy against _Into with a fresh map, which is what
// Copy was. The two copies must have one shape, sharing and cycles included,
// and the new copy must own every container it holds.
//
// Revisions:
//   - 2026-10-29 01:22: initial creation
func TestCopy_SameAsBefore(t *testing.T) {
	for name, value := range _Cases(t) {
		t.Run(
			name,
			func(t *testing.T) {
				var (
					want, wanted = _Into(
						value,
						map[starlark.Value]starlark.Value{},
					)
					got, err = Copy(value)
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

// _WeighBefore is _Weigh as it was before Size skipped the map for a scalar
// and read a list by index. It is the reference that TestSize_SameAsBefore
// holds Size to.
//
// Revisions:
//   - 2026-10-29 01:22: initial creation, a copy of _Weigh
func _WeighBefore(
	value starlark.Value,
	seen map[starlark.Value]bool,
) int64 {
	switch held := value.(type) {
	case starlark.NoneType, starlark.Bool:
		return WORD

	case starlark.Int, starlark.Float:
		return NUMBER

	case starlark.String:
		return WORD + int64(len(held))

	case starlark.Bytes:
		return WORD + int64(len(held))

	case *starlark.List:
		return _InsideBefore(
			held,
			held.Len(),
			seen,
		)

	case *starlark.Set:
		return _InsideBefore(
			held,
			held.Len(),
			seen,
		)

	case starlark.Tuple:
		return _SlotsBefore(
			held,
			seen,
		)

	case *starlark.Dict:
		return _KeyedBefore(
			held,
			seen,
		)
	}

	return WORD
}

// _SlotsBefore is _Slots as it was.
//
// Revisions:
//   - 2026-10-29 01:22: initial creation, a copy of _Slots
func _SlotsBefore(
	held starlark.Tuple,
	seen map[starlark.Value]bool,
) int64 {
	total := WORD + int64(len(held))*ENTRY

	for _, one := range held {
		total += _WeighBefore(
			one,
			seen,
		)
	}

	return total
}

// _InsideBefore is _Inside as it was, with an iterator for a list and a set.
//
// Revisions:
//   - 2026-10-29 01:22: initial creation, a copy of _Inside
func _InsideBefore(
	held starlark.Iterable,
	length int,
	seen map[starlark.Value]bool,
) int64 {
	if seen[held] {
		return 0
	}

	seen[held] = true

	var (
		total = WORD + int64(length)*ENTRY
		walk  = held.Iterate()
	)

	defer walk.Done()

	var one starlark.Value

	for walk.Next(&one) {
		total += _WeighBefore(
			one,
			seen,
		)
	}

	return total
}

// _KeyedBefore is _Keyed as it was.
//
// Revisions:
//   - 2026-10-29 01:22: initial creation, a copy of _Keyed
func _KeyedBefore(
	held *starlark.Dict,
	seen map[starlark.Value]bool,
) int64 {
	if seen[held] {
		return 0
	}

	seen[held] = true

	total := WORD + int64(held.Len())*ENTRY

	for key, value := range held.Entries() {
		total += _WeighBefore(
			key,
			seen,
		)
		total += _WeighBefore(
			value,
			seen,
		)
	}

	return total
}
