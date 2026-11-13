package tests

import (
	"errors"
	"strconv"
	"testing"

	"go.starlark.net/starlark"
	"google.golang.org/protobuf/types/known/structpb"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/trail"
)

// _LarkNull is a null value, for the tests of FromValue.
//
// Revisions:
//   - 2026-11-13 13:06: initial creation (C132, Q437)
func _LarkNull(held structpb.NullValue) *workflowpb.Value {
	return &workflowpb.Value{
		Kind: &workflowpb.Value_Null{
			Null: held,
		},
	}
}

// _LarkBool is a bool value, for the tests of FromValue.
//
// Revisions:
//   - 2026-11-13 13:06: initial creation (C132, Q437)
func _LarkBool(held bool) *workflowpb.Value {
	return &workflowpb.Value{
		Kind: &workflowpb.Value_Bool{
			Bool: held,
		},
	}
}

// _LarkInt is an int value, for the tests of FromValue.
//
// Revisions:
//   - 2026-11-13 13:06: initial creation (C132, Q437)
func _LarkInt(held int64) *workflowpb.Value {
	return &workflowpb.Value{
		Kind: &workflowpb.Value_Int{
			Int: held,
		},
	}
}

// _LarkFloat is a float value, for the tests of FromValue.
//
// Revisions:
//   - 2026-11-13 13:06: initial creation (C132, Q437)
func _LarkFloat(held float64) *workflowpb.Value {
	return &workflowpb.Value{
		Kind: &workflowpb.Value_Float{
			Float: held,
		},
	}
}

// _LarkText is a string value, for the tests of FromValue.
//
// Revisions:
//   - 2026-11-13 13:06: initial creation (C132, Q437)
func _LarkText(held string) *workflowpb.Value {
	return &workflowpb.Value{
		Kind: &workflowpb.Value_String_{
			String_: held,
		},
	}
}

// _LarkRaw is a bytes value, for the tests of FromValue.
//
// Revisions:
//   - 2026-11-13 13:06: initial creation (C132, Q437)
func _LarkRaw(held []byte) *workflowpb.Value {
	return &workflowpb.Value{
		Kind: &workflowpb.Value_Bytes{
			Bytes: held,
		},
	}
}

// _LarkItems is a list value of the items, for the tests of FromValue.
//
// Revisions:
//   - 2026-11-13 13:06: initial creation (C132, Q437)
func _LarkItems(held ...*workflowpb.Value) *workflowpb.Value {
	return &workflowpb.Value{
		Kind: &workflowpb.Value_List{
			List: &workflowpb.List{
				Items: held,
			},
		},
	}
}

// _LarkDict is a map value of the entries, for the tests of FromValue.
//
// Revisions:
//   - 2026-11-13 13:06: initial creation (C132, Q437)
func _LarkDict(held map[string]*workflowpb.Value) *workflowpb.Value {
	return &workflowpb.Value{
		Kind: &workflowpb.Value_Map{
			Map: &workflowpb.Map{
				Entries: held,
			},
		},
	}
}

// TestFromValue_Kinds proves each kind of a lark Value is its own value for a script:
// an int is an int and a float is a float, so 1 and 1.0 stay two values, bytes are
// bytes, a map is a dict whose keys are inserted in sorted order, and a Value with no
// kind and a null that holds anything but null wrap ErrConvert.
//
// The check: go test -race -count=1 -run TestFromValue_Kinds ./v1/runtime/plugin/unpack/tests
//
// Revisions:
//   - 2026-11-13 13:06: initial creation (C132, Q437)
func TestFromValue_Kinds(t *testing.T) {
	for _, tc := range []struct {
		name  string
		given *workflowpb.Value
		want  string
		kind  string
		err   error
	}{
		{
			"null",
			_LarkNull(0),
			"None",
			"NoneType",
			nil,
		},
		{
			"null that holds 5",
			_LarkNull(5),
			"",
			"",
			unpack.ErrConvert,
		},
		{
			"true",
			_LarkBool(true),
			"True",
			"bool",
			nil,
		},
		{
			"an int",
			_LarkInt(1),
			"1",
			"int",
			nil,
		},
		{
			"the smallest int64",
			_LarkInt(-1 << 63),
			"-9223372036854775808",
			"int",
			nil,
		},
		{
			"a float",
			_LarkFloat(1),
			"1.0",
			"float",
			nil,
		},
		{
			"a string",
			_LarkText("aGk="),
			`"aGk="`,
			"string",
			nil,
		},
		{
			"bytes",
			_LarkRaw([]byte("hi\xff")),
			`b"hi\xff"`,
			"bytes",
			nil,
		},
		{
			"no bytes",
			_LarkRaw(nil),
			`b""`,
			"bytes",
			nil,
		},
		{
			"no kind",
			&workflowpb.Value{},
			"",
			"",
			unpack.ErrConvert,
		},
		{
			"a nil value",
			nil,
			"",
			"",
			unpack.ErrConvert,
		},
		{
			"a list",
			_LarkItems(
				_LarkInt(1),
				_LarkFloat(2),
			),
			"[1, 2.0]",
			"list",
			nil,
		},
		{
			"a list with an item of no kind",
			_LarkItems(&workflowpb.Value{}),
			"",
			"",
			unpack.ErrConvert,
		},
		{
			"a map, in the order of its keys",
			_LarkDict(map[string]*workflowpb.Value{
				"b": _LarkInt(2),
				"a": _LarkInt(1),
				"c": _LarkInt(3),
			}),
			`{"a": 1, "b": 2, "c": 3}`,
			"dict",
			nil,
		},
		{
			"a map with an entry of no kind",
			_LarkDict(map[string]*workflowpb.Value{
				"k": {},
			}),
			"",
			"",
			unpack.ErrConvert,
		},
		{
			"an empty map",
			&workflowpb.Value{
				Kind: &workflowpb.Value_Map{},
			},
			"{}",
			"dict",
			nil,
		},
		{
			"an empty list",
			&workflowpb.Value{
				Kind: &workflowpb.Value_List{},
			},
			"[]",
			"list",
			nil,
		},
	} {
		t.Run(
			tc.name,
			func(t *testing.T) {
				got, err := unpack.FromValue(tc.given)
				if tc.err != nil {
					if !errors.Is(
						err,
						tc.err,
					) {
						t.Fatalf(
							"err = %v, want %v",
							err,
							tc.err,
						)
					}

					return
				}

				if err != nil {
					t.Fatal(err)
				}

				if got.String() != tc.want || got.Type() != tc.kind {
					t.Fatalf(
						"got %s (%s), want %s (%s)",
						got,
						got.Type(),
						tc.want,
						tc.kind,
					)
				}
			},
		)
	}
}

// TestFromValue_MakesNewValuesEachTime proves two conversions of one Value give two
// lists, so a list one run changes is not the list another run is handed.
//
// The check: go test -race -count=1 -run TestFromValue_MakesNewValuesEachTime
// ./v1/runtime/plugin/unpack/tests
//
// Revisions:
//   - 2026-11-13 13:06: initial creation (C132, Q437)
func TestFromValue_MakesNewValuesEachTime(t *testing.T) {
	var given = _LarkItems(_LarkInt(1))

	first, err := unpack.FromValue(given)
	if err != nil {
		t.Fatal(err)
	}

	second, err := unpack.FromValue(given)
	if err != nil {
		t.Fatal(err)
	}

	err = first.(*starlark.List).Append(starlark.MakeInt(2))
	if err != nil {
		t.Fatal(err)
	}

	if second.String() != "[1]" || len(given.GetList().GetItems()) != 1 {
		t.Fatalf(
			"a change of one list reached %s and %v",
			second,
			given,
		)
	}
}

// TestFromValue_CycleAndDepth proves a list that holds itself is trail.ErrCyclic and
// a value nested past common.DEPTH is trail.ErrDeep, as every walk over a value has it
// (Q2).
//
// The check: go test -race -count=1 -run TestFromValue_CycleAndDepth
// ./v1/runtime/plugin/unpack/tests
//
// Revisions:
//   - 2026-11-13 13:00: initial creation (C132, Q2, Q437)
func TestFromValue_CycleAndDepth(t *testing.T) {
	var cyclic = &workflowpb.List{}

	cyclic.Items = []*workflowpb.Value{{
		Kind: &workflowpb.Value_List{
			List: cyclic,
		},
	}}

	_, err := unpack.FromValue(&workflowpb.Value{
		Kind: &workflowpb.Value_List{
			List: cyclic,
		},
	})
	if !errors.Is(
		err,
		trail.ErrCyclic,
	) {
		t.Fatalf(
			"a cycle: err = %v, want %v",
			err,
			trail.ErrCyclic,
		)
	}

	var entries = map[string]*workflowpb.Value{}

	entries["k"] = &workflowpb.Value{
		Kind: &workflowpb.Value_Map{
			Map: &workflowpb.Map{
				Entries: entries,
			},
		},
	}

	_, err = unpack.FromValue(&workflowpb.Value{
		Kind: &workflowpb.Value_Map{
			Map: &workflowpb.Map{
				Entries: entries,
			},
		},
	})
	if !errors.Is(
		err,
		trail.ErrCyclic,
	) {
		t.Fatalf(
			"a map that holds itself: err = %v, want %v",
			err,
			trail.ErrCyclic,
		)
	}

	var deep = _LarkBool(false)

	for range common.DEPTH + 2 {
		deep = _LarkItems(deep)
	}

	_, err = unpack.FromValue(deep)
	if !errors.Is(
		err,
		trail.ErrDeep,
	) {
		t.Fatalf(
			"a deep value: err = %v, want %v",
			err,
			trail.ErrDeep,
		)
	}
}

// TestFromValue_WideIsNotDeep proves a value that is wide and shallow is no deep one:
// a list of 20,000 empty lists and a map of 20,000 empty maps convert, so the walk
// leaves a list or a map as it comes out of it. A walk that did not leave would count
// the siblings as nesting and refuse them with trail.ErrDeep past common.DEPTH (Q2).
//
// The check: go test -race -count=1 -run TestFromValue_WideIsNotDeep
// ./v1/runtime/plugin/unpack/tests
//
// Revisions:
//   - 2026-11-13 17:16: initial creation (C132, Q2, Q437)
func TestFromValue_WideIsNotDeep(t *testing.T) {
	const WIDTH = 2 * common.DEPTH

	var (
		items = make(
			[]*workflowpb.Value,
			0,
			WIDTH,
		)
		entries = make(
			map[string]*workflowpb.Value,
			WIDTH,
		)
	)

	for index := range WIDTH {
		items = append(
			items,
			_LarkItems(),
		)
		entries[strconv.Itoa(index)] = _LarkDict(nil)
	}

	for name, given := range map[string]*workflowpb.Value{
		"a list of lists": _LarkItems(items...),
		"a map of maps":   _LarkDict(entries),
		"two lists of lists": _LarkItems(
			_LarkItems(items...),
			_LarkItems(items...),
		),
	} {
		got, err := unpack.FromValue(given)
		if err != nil {
			t.Fatalf(
				"%s: %v",
				name,
				err,
			)
		}

		if got.Truth() != starlark.True {
			t.Fatalf(
				"%s: got %s, want a value that is not empty",
				name,
				got.Type(),
			)
		}
	}
}
