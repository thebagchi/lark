package tests

import (
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/artifact"
	"github.com/thebagchi/lark/v1/runtime/trail"
)

const (
	// ARG_SCRIPT returns the argument "v", None when none was supplied.
	//
	// Revisions:
	//   - 2026-11-05 18:15: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	ARG_SCRIPT = `
v = arg("v", None)

def main():
    return v
`
)

// _Twice returns a pointer to a pointer to text.
//
// Revisions:
//   - 2026-11-05 17:20: initial creation (C85)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Twice(text string) **string {
	held := &text

	return &held
}

// _Nothing is a function, which JSON cannot hold.
//
// Revisions:
//   - 2026-11-05 17:10: initial creation (C85)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Nothing() {
	// Empty
}

// _Port is a named type whose underlying kind is a basic kind.
//
// Revisions:
//   - 2026-11-05 18:15: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Port int

// _Size, _Ratio and _Flag are named types of the other basic kinds.
//
// Revisions:
//   - 2026-11-05 18:15: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type (
	_Size  uint
	_Ratio float64
	_Flag  bool
)

// _Anys is a named slice of any, which the reflection path walks.
//
// Revisions:
//   - 2026-11-05 18:15: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Anys []any

// _Bytes is a named slice of bytes, and _Tree a named map that can hold itself.
//
// Revisions:
//   - 2026-11-05 18:15: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type (
	_Bytes []byte
	_Tree  map[string]any
)

// _Window returns a []any whose second element is its own first-element
// window: a[:1] holds only a[0], so it is no cycle.
//
// Revisions:
//   - 2026-11-05 17:30: initial creation (C85 fix round 1)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Window() any {
	a := make(
		[]any,
		2,
	)
	a[1] = a[:1]

	return a
}

// _TypedWindow is _Window for a slice the reflection path walks.
//
// Revisions:
//   - 2026-11-05 17:30: initial creation (C85 fix round 1)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _TypedWindow() any {
	a := make(
		[]any,
		2,
	)
	a[1] = a[:1]

	return _Anys(a)
}

// _Shared returns a list holding one slice twice, by sibling paths.
//
// Revisions:
//   - 2026-11-05 17:30: initial creation (C85 fix round 1)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Shared() any {
	one := []any{1.0}

	return []any{one, one}
}

// _Label is a named string type, used as a map key.
//
// Revisions:
//   - 2026-11-05 18:15: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Label string

// _Passed runs ARG_SCRIPT with v supplied and returns what main returned.
//
// Revisions:
//   - 2026-11-05 16:50: initial creation (C85)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Passed(
	t *testing.T,
	given any,
) (string, error) {
	t.Helper()

	got, err := runtime.Run(
		t.Context(),
		&common.Source{
			Entry: "arg.star",
			Text:  []byte(ARG_SCRIPT),
		},
		artifact.WithArgs(map[string]any{
			"v": given,
		}),
	)
	if err != nil {
		return "", err
	}

	return got.String(), nil
}

// _Nested returns a list nested depth deep: [[[...]]].
//
// Revisions:
//   - 2026-11-05 16:50: initial creation (C85)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Nested(depth int) any {
	var held any = []any{}

	for range depth {
		held = []any{held}
	}

	return held
}

// TestWithArgs_ConvertsTypedValues proves a typed slice, a typed map, a named
// basic type and a pointer reach the script as the list, dict, number and value
// they hold, and that the types the old check took are unchanged (C85, Q167).
//
// Revisions:
//   - 2026-11-05 16:50: initial creation (C85)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-13 12:57: a float64 is a float and a []byte is bytes, by the strict rules (C132,
//     Q437)
func TestWithArgs_ConvertsTypedValues(t *testing.T) {
	var (
		port  = _Port(8080)
		text  = "x"
		nilTo *int
	)

	tests := []struct {
		name  string
		given any
		want  string
	}{
		{
			"strings",
			[]string{"a", "b"},
			`["a", "b"]`,
		},
		{
			"ints",
			[]int{1, 2},
			`[1, 2]`,
		},
		{
			"array",
			[2]bool{true, false},
			`[True, False]`,
		},
		{
			"empty typed slice",
			[]string{},
			`[]`,
		},
		{
			"nil typed slice",
			[]string(nil),
			`[]`,
		},
		{
			"string map",
			map[string]string{"k": "v"},
			`{"k": "v"}`,
		},
		{
			"named key",
			map[_Label]int{"k": 1},
			`{"k": 1}`,
		},
		{
			"nested typed map",
			map[string][]map[string]int{"a": {{"b": 1}}},
			`{"a": [{"b": 1}]}`,
		},
		{
			"typed slice in []any",
			[]any{[]string{"a"}},
			`[["a"]]`,
		},
		{
			"named int",
			port,
			`8080`,
		},
		{
			"named int in slice",
			[]_Port{1, 2},
			`[1, 2]`,
		},
		{
			"pointer",
			&text,
			`"x"`,
		},
		{
			"pointer to slice",
			&[]int{3},
			`[3]`,
		},
		{
			"nil pointer",
			nilTo,
			`None`,
		},
		{
			"nil",
			nil,
			`None`,
		},
		{
			"plain any list",
			[]any{"a", 1.0},
			`["a", 1.0]`,
		},
		{
			"bytes keep their meaning",
			[]byte("hi"),
			`b"hi"`,
		},
		{
			"typed slice inside a plain map",
			map[string]any{"k": []string{"a"}},
			`{"k": ["a"]}`,
		},
		{
			"plain map in a typed slice",
			[]map[string]any{{"k": []int{1}}},
			`[{"k": [1]}]`,
		},
		{
			"array of interfaces",
			[2]any{nil, "x"},
			`[None, "x"]`,
		},
		{
			"named uint, float and string",
			[]any{_Size(3), _Ratio(0.5), _Label("l"), _Flag(true)},
			`[3, 0.5, "l", True]`,
		},
		{
			"plain map with nothing to change in a list",
			[]any{map[string]any{"n": 1.0}},
			`[{"n": 1.0}]`,
		},
		{
			"named byte slice is bytes",
			_Bytes("hi"),
			`b"hi"`,
		},
		{
			"prefix window of a slice (not a cycle)",
			_Window(),
			`[None, [None]]`,
		},
		{
			"one slice twice, by sibling paths (a DAG)",
			_Shared(),
			`[[1.0], [1.0]]`,
		},
		{
			"typed prefix window",
			_TypedWindow(),
			`[None, [None]]`,
		},
		{
			"empty maps",
			[]any{map[string]any{}, map[string]int{}},
			`[{}, {}]`,
		},
		{
			"pointer to a pointer",
			_Twice("y"),
			`"y"`,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				got, err := _Passed(
					t,
					tt.given,
				)
				if err != nil {
					t.Fatalf(
						"got error %v, want %s",
						err,
						tt.want,
					)
				}

				if got != tt.want {
					t.Fatalf(
						"got %s, want %s",
						got,
						tt.want,
					)
				}
			},
		)
	}
}

// _Degrees is a named float type, to reach the reflect path of the walk.
//
// Revisions:
//   - 2026-11-10 11:51: initial creation (C117, B244)
type _Degrees float64

// TestWithArgs_RefusesWhatJSONCannotHold proves a function, a channel, a struct
// and a map with a key that is not a string are refused with ErrJSON, and
// the text names the argument and the Go type, quoted. NaN and the infinities
// are refused the same way (Q297, B244).
//
// Revisions:
//   - 2026-11-05 16:50: initial creation (C85)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 11:51: NaN and the infinities (C117, B244, Q297)
//   - 2026-11-10 17:15: uses ErrJSON, formerly ErrNotJSON (C121, Q348)
func TestWithArgs_RefusesWhatJSONCannotHold(t *testing.T) {
	tests := []struct {
		name  string
		given any
		want  []string
	}{
		{
			"func",
			_Nothing,
			[]string{`argument "v"`, `"func()"`},
		},
		{
			"func in slice",
			[]any{_Nothing},
			[]string{`argument "v"`, `"func()"`},
		},
		{
			"chan",
			make(chan int),
			[]string{`"chan int"`},
		},
		{
			"struct",
			struct{ A int }{
				1,
			},
			[]string{`"struct { A int }"`},
		},
		{
			"int key",
			map[int]string{1: "a"},
			[]string{`"map[int]string"`},
		},
		{
			"chan in a typed map",
			map[string]chan int{"k": nil},
			[]string{`"chan int"`},
		},
		{
			"chan in a typed slice",
			[]chan int{nil},
			[]string{`"chan int"`},
		},
		{
			"func in a plain map",
			map[string]any{"k": _Nothing},
			[]string{`"func()"`},
		},
		{
			"complex",
			complex(
				1,
				2,
			),
			[]string{`"complex128"`},
		},
		{
			"NaN",
			math.NaN(),
			[]string{`argument "v"`, `NaN is not a JSON number`},
		},
		{
			"positive infinity",
			math.Inf(1),
			[]string{`+Inf is not a JSON number`},
		},
		{
			"negative infinity",
			math.Inf(-1),
			[]string{`-Inf is not a JSON number`},
		},
		{
			"float32 NaN",
			float32(math.NaN()),
			[]string{`NaN is not a JSON number`},
		},
		{
			"named float infinity",
			_Degrees(math.Inf(1)),
			[]string{`+Inf is not a JSON number`},
		},
		{
			"json.Number NaN",
			json.Number("NaN"),
			[]string{`NaN is not a JSON number`},
		},
		{
			"json.Number infinity",
			json.Number("-Infinity"),
			[]string{`-Inf is not a JSON number`},
		},
		{
			"NaN in a typed slice",
			[]float64{1, math.NaN()},
			[]string{`NaN is not a JSON number`},
		},
		{
			"infinity in a plain map",
			map[string]any{"k": math.Inf(1)},
			[]string{`+Inf is not a JSON number`},
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				_, err := _Passed(
					t,
					tt.given,
				)
				if !errors.Is(
					err,
					artifact.ErrJSON,
				) {
					t.Fatalf(
						"got %v, want ErrJSON",
						err,
					)
				}

				for _, part := range tt.want {
					if !strings.Contains(
						err.Error(),
						part,
					) {
						t.Fatalf(
							"error %q lacks %s",
							err,
							part,
						)
					}
				}
			},
		)
	}
}

// TestWithArgs_BoundsTheKeyItQuotes proves a very long argument name is cut
// in the error text (Q4).
//
// Revisions:
//   - 2026-11-05 16:50: initial creation (C85)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrJSON, formerly ErrNotJSON (C121, Q348)
func TestWithArgs_BoundsTheKeyItQuotes(t *testing.T) {
	var long = strings.Repeat(
		"a",
		5000,
	)

	_, err := runtime.Run(
		t.Context(),
		&common.Source{
			Entry: "arg.star",
			Text:  []byte(ARG_SCRIPT),
		},
		artifact.WithArgs(map[string]any{
			long: _Nothing,
		}),
	)
	if !errors.Is(
		err,
		artifact.ErrJSON,
	) {
		t.Fatalf(
			"got %v, want ErrJSON",
			err,
		)
	}

	if len(err.Error()) > 1000 {
		t.Fatalf(
			"error is %d bytes, want it bounded",
			len(err.Error()),
		)
	}
}

// TestWithArgs_RefusesACycle proves a map, a slice or a pointer that holds
// itself is refused with trail.ErrCyclic, for the plain types and the
// typed ones, rather than overflowing the stack.
//
// Revisions:
//   - 2026-11-05 16:50: initial creation (C85)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrJSON and ErrCyclic, formerly ErrNotJSON and ErrSelfContaining
//     (C121, Q348, Q349)
func TestWithArgs_RefusesACycle(t *testing.T) {
	plainMap := map[string]any{}
	plainMap["self"] = plainMap

	plainList := []any{nil}
	plainList[0] = plainList

	typedMap := map[string]any{}
	typedMap["list"] = []string{"a"}
	typedMap["self"] = typedMap

	branching := map[string]any{}
	branching["a"] = branching
	branching["b"] = branching

	viaPointer := new(any)
	*viaPointer = viaPointer

	window := make(
		[]any,
		1,
	)
	window[0] = window[:1]

	typedRing := make(
		_Anys,
		2,
	)
	typedRing[1] = typedRing

	tree := _Tree{}
	tree["self"] = tree

	tests := []struct {
		name  string
		given any
	}{
		{
			"named map",
			tree,
		},
		{
			"slice holding its own window",
			window,
		},
		{
			"named slice holding itself",
			typedRing,
		},
		{
			"plain map",
			plainMap,
		},
		{
			"plain list",
			plainList,
		},
		{
			"typed value beside a cycle",
			typedMap,
		},
		{
			"branching cycle",
			branching,
		},
		{
			"pointer through an interface",
			viaPointer,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				_, err := _Passed(
					t,
					tt.given,
				)
				if !errors.Is(
					err,
					artifact.ErrJSON,
				) {
					t.Fatalf(
						"got %v, want ErrJSON",
						err,
					)
				}

				if !errors.Is(
					err,
					trail.ErrCyclic,
				) {
					t.Fatalf(
						"got %v, want ErrCyclic",
						err,
					)
				}
			},
		)
	}
}

// _Typed returns a value nested depth deep that holds typed containers.
//
// Revisions:
//   - 2026-11-05 17:10: initial creation (C85)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Typed(depth int) any {
	var held any = []string{}

	for range depth {
		held = [][]any{{held}}
	}

	return held
}

// TestWithArgs_BoundsTheDepth proves a value nested to common.DEPTH is taken
// and one nested past it is refused with trail.ErrDeep, for plain and typed
// containers alike.
//
// Revisions:
//   - 2026-11-05 16:50: initial creation (C85)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrJSON and ErrDeep, formerly ErrNotJSON and ErrTooDeep (C121, Q348,
//     Q349)
func TestWithArgs_BoundsTheDepth(t *testing.T) {
	tests := []struct {
		name  string
		given any
		deep  bool
	}{
		{
			"plain at the limit",
			_Nested(common.DEPTH - 2),
			false,
		},
		{
			"plain past the limit",
			_Nested(common.DEPTH + 1),
			true,
		},
		{
			"typed past the limit",
			_Typed(common.DEPTH),
			true,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				_, err := _Passed(
					t,
					tt.given,
				)
				if !tt.deep {
					if err != nil && errors.Is(
						err,
						trail.ErrDeep,
					) {
						t.Fatalf(
							"got %v, want it taken",
							err,
						)
					}

					return
				}

				if !errors.Is(
					err,
					artifact.ErrJSON,
				) ||
					!errors.Is(
						err,
						trail.ErrDeep,
					) {
					t.Fatalf(
						"got %v, want ErrDeep",
						err,
					)
				}
			},
		)
	}
}

// BenchmarkWithArgs_Plain measures WithArgs for arguments the old check took:
// plain []any and map[string]any. Its allocs/op must not rise with C85.
//
// Revisions:
//   - 2026-11-05 16:50: initial creation (C85)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func BenchmarkWithArgs_Plain(b *testing.B) {
	args := map[string]any{
		"host": "db",
		"port": 5432,
		"tags": []any{"a", "b", []any{1, 2}},
		"opts": map[string]any{"tier": "gold", "on": true},
	}

	b.ReportAllocs()

	for b.Loop() {
		_ = artifact.WithArgs(args)
	}
}
