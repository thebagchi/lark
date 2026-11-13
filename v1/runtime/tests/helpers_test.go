package tests

// Tests of runtime.Export, from outside the package.
//
// Revisions:
//   - 2026-11-10 11:00: renamed from export_test.go (C120b, Q326)

import (
	"context"
	"errors"
	"testing"

	"go.starlark.net/starlark"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/structpb"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/trail"
)

// _Exported builds a one-pair dict from a key and a value, ending the test on an error.
//
// Revisions:
//   - 2026-11-08 10:46: initial creation (C73)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Exported(
	t *testing.T,
	pairs ...starlark.Value,
) *starlark.Dict {
	t.Helper()

	dict := starlark.NewDict(len(pairs) / 2)

	for index := 0; index < len(pairs); index += 2 {
		err := dict.SetKey(
			pairs[index],
			pairs[index+1],
		)
		if err != nil {
			t.Fatalf(
				"SetKey: %v",
				err,
			)
		}
	}

	return dict
}

// _Nested is a list holding an int and a dict that holds a tuple of None.
//
// Revisions:
//   - 2026-11-08 10:46: initial creation (C73)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Nested(t *testing.T) starlark.Value {
	t.Helper()

	return starlark.NewList([]starlark.Value{
		starlark.MakeInt(1),
		_Exported(
			t,
			starlark.String("k"),
			starlark.Tuple{starlark.None},
		),
	})
}

// _Set is a set holding the one member 1, so its order is not in question.
//
// Revisions:
//   - 2026-11-08 12:40: initial creation (C73)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Set(t *testing.T) *starlark.Set {
	t.Helper()

	set := starlark.NewSet(1)
	if err := set.Insert(starlark.MakeInt(1)); err != nil {
		t.Fatalf(
			"Insert: %v",
			err,
		)
	}

	return set
}

// TestExport_Converts proves each value kind arrives as the protobuf value the
// conversion table in Export's doc names.
//
// Revisions:
//   - 2026-11-08 10:46: initial creation (C73, Q234)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestExport_Converts(t *testing.T) {
	cases := []struct {
		name string
		give starlark.Value
		want any
	}{
		{
			"none",
			starlark.None,
			nil,
		},
		{
			"true",
			starlark.True,
			true,
		},
		{
			"false",
			starlark.False,
			false,
		},
		{
			"small int",
			starlark.MakeInt(7),
			float64(7),
		},
		{
			"negative int",
			starlark.MakeInt(-7),
			float64(-7),
		},
		{
			"smallest exact int",
			starlark.MakeInt64(-(1 << 53)),
			float64(-(1 << 53)),
		},
		{
			"a set arrives as a list",
			_Set(t),
			[]any{float64(1)},
		},
		{
			"largest exact int",
			starlark.MakeInt64(1 << 53),
			float64(1 << 53),
		},
		{
			"float",
			starlark.Float(1.5),
			1.5,
		},
		{
			"string",
			starlark.String("hi"),
			"hi",
		},
		{
			"bytes",
			starlark.Bytes("hi"),
			"hi",
		},
		{
			"list",
			starlark.NewList([]starlark.Value{
				starlark.MakeInt(1),
				starlark.String("a"),
			}),
			[]any{float64(1), "a"},
		},
		{
			"tuple as list",
			starlark.Tuple{
				starlark.True,
				starlark.None,
			},
			[]any{true, nil},
		},
		{
			"dict with string keys",
			_Exported(
				t,
				starlark.String("a"),
				starlark.MakeInt(1),
				starlark.String("b"),
				starlark.String("x"),
			),
			map[string]any{
				"a": float64(1),
				"b": "x",
			},
		},
		{
			"nested",
			_Nested(t),
			[]any{
				float64(1),
				map[string]any{
					"k": []any{nil},
				},
			},
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				got, err := runtime.Export(
					context.Background(),
					tc.give,
				)
				if err != nil {
					t.Fatalf(
						"Export: %v",
						err,
					)
				}

				want, err := structpb.NewValue(tc.want)
				if err != nil {
					t.Fatalf(
						"NewValue: %v",
						err,
					)
				}

				if !proto.Equal(
					got,
					want,
				) {
					t.Errorf(
						"Export = %v, want %v",
						got,
						want,
					)
				}
			},
		)
	}
}

// _Function is a builtin function, a value the wire cannot name.
//
// Revisions:
//   - 2026-11-08 10:46: initial creation (C73)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Function() starlark.Value {
	return starlark.NewBuiltin(
		"f",
		func(
			*starlark.Thread,
			*starlark.Builtin,
			starlark.Tuple,
			[]starlark.Tuple,
		) (starlark.Value, error) {
			return starlark.None, nil
		},
	)
}

// TestExport_Refuses proves a value the wire cannot name gives unpack.ErrConvert,
// the one sentinel Export documents (Q259).
//
// Revisions:
//   - 2026-11-08 10:46: initial creation (C73, Q234, Q259, Q260)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 11:47: a typed-nil list, dict and set are refused (C117, B240)
//   - 2026-11-10 17:15: uses ErrConvert, formerly ErrNotProtoValue (C121, Q349)
func TestExport_Refuses(t *testing.T) {
	var (
		function = _Function()
		cases    = map[string]starlark.Value{
			"a nil value": nil,
			"a nil list":  (*starlark.List)(nil),
			"a nil dict":  (*starlark.Dict)(nil),
			"a nil set":   (*starlark.Set)(nil),
			"a function":  function,
			"a dict keyed by an int": _Exported(
				t,
				starlark.MakeInt(1),
				starlark.None,
			),
			"2^53 + 1":                 starlark.MakeInt64(1<<53 + 1),
			"-2^53 - 1":                starlark.MakeInt64(-(1 << 53) - 1),
			"an int beyond int64":      starlark.MakeInt64(1).Lsh(70),
			"a function inside a list": starlark.NewList([]starlark.Value{function}),
			"a function inside a dict": _Exported(
				t,
				starlark.String("f"),
				function,
			),
			"a function inside a tuple": starlark.Tuple{function},
		}
	)

	for name, give := range cases {
		t.Run(
			name,
			func(t *testing.T) {
				got, err := runtime.Export(
					context.Background(),
					give,
				)
				if !errors.Is(
					err,
					unpack.ErrConvert,
				) {
					t.Errorf(
						"Export = %v, %v; want unpack.ErrConvert",
						got,
						err,
					)
				}
			},
		)
	}
}

// TestExport_Cycle proves a list that holds itself is refused, not walked
// forever.
//
// Revisions:
//   - 2026-11-08 10:46: initial creation (C73, Q234)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrCyclic, formerly ErrSelfContaining (C121, Q349)
func TestExport_Cycle(t *testing.T) {
	list := starlark.NewList(nil)

	if err := list.Append(list); err != nil {
		t.Fatalf(
			"Append: %v",
			err,
		)
	}

	_, err := runtime.Export(
		context.Background(),
		list,
	)
	if !errors.Is(
		err,
		trail.ErrCyclic,
	) {
		t.Errorf(
			"Export = %v, want trail.ErrCyclic",
			err,
		)
	}
}

// TestExport_PastTheDepthLimitIsTooDeep proves a value nested past common.DEPTH
// is trail.ErrDeep, as the doc says.
//
// Revisions:
//   - 2026-11-08 12:40: initial creation (C73, Q234)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrDeep, formerly ErrTooDeep (C121, Q349)
func TestExport_PastTheDepthLimitIsTooDeep(t *testing.T) {
	var past starlark.Value = starlark.NewList(nil)

	for range common.DEPTH {
		past = starlark.NewList([]starlark.Value{past})
	}

	_, err := runtime.Export(
		context.Background(),
		past,
	)
	if !errors.Is(
		err,
		trail.ErrDeep,
	) {
		t.Errorf(
			"Export = %v, want trail.ErrDeep",
			err,
		)
	}
}

// SIZE is the number of elements in the large value: far more than common.POLL.
//
// Revisions:
//   - 2026-11-08 10:46: initial creation (C73)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
const (
	SIZE = 100000
)

// TestExport_CancelledContextStopsALargeValue proves a cancelled context ends
// the walk of a value with far more elements than common.POLL (Q258).
//
// Revisions:
//   - 2026-11-08 10:46: initial creation (C73, Q258)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestExport_CancelledContextStopsALargeValue(t *testing.T) {
	elements := make(
		[]starlark.Value,
		SIZE,
	)
	for index := range elements {
		elements[index] = starlark.MakeInt(index)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := runtime.Export(
		ctx,
		starlark.NewList(elements),
	)
	if !errors.Is(
		err,
		context.Canceled,
	) {
		t.Errorf(
			"Export = %v, want context.Canceled",
			err,
		)
	}
}
