package tests

import (
	"context"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/deep"
)

const (
	// RUNS is how many calls an allocation count averages over.
	//
	// Revisions:
	//   - 2026-10-29 06:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	RUNS = 100
)

// _Held is a value built the way a script would build one.
//
// Revisions:
//   - 2026-09-24 20:22: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Held(values ...starlark.Value) *starlark.List {
	return starlark.NewList(values)
}

// _Scalars is one of each value that holds nothing, so a walk into it has
// nothing to remember.
//
// Revisions:
//   - 2026-10-29 01:22: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Scalars() map[string]starlark.Value {
	return map[string]starlark.Value{
		"none":    starlark.None,
		"a bool":  starlark.True,
		"an int":  starlark.MakeInt(7),
		"a float": starlark.Float(1.5),
		"a word":  starlark.String("x"),
		"bytes":   starlark.Bytes("x"),
	}
}

// TestCopy_AScalarAllocatesNothing keeps Copy from building its seen map for
// a value that it returns as it is. A store copies every value a script
// reads, and most are one number or one word.
//
// Revisions:
//   - 2026-10-29 01:22: initial creation
//   - 2026-11-01 13:00: RUNS moved here from size_test.go, which went with Size
//   - 2026-11-02 16:14: Copy takes a context (C26)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestCopy_AScalarAllocatesNothing(t *testing.T) {
	for name, value := range _Scalars() {
		t.Run(
			name,
			func(t *testing.T) {
				var err error

				made := testing.AllocsPerRun(
					RUNS,
					func() {
						_, err = deep.Copy(
							context.Background(),
							value,
						)
					},
				)

				if err != nil {
					t.Fatal(err)
				}

				if made != 0 {
					t.Fatalf(
						"%v allocations to copy %s",
						made,
						name,
					)
				}
			},
		)
	}
}

// TestIsData_AcceptsWhatAStoreCanHold is every shape a script builds out of
// data, including the containers nested in each other.
//
// Revisions:
//   - 2026-09-24 20:22: initial creation
//   - 2026-11-02 16:14: IsData takes a context and returns an error (C26)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestIsData_AcceptsWhatAStoreCanHold(t *testing.T) {
	var (
		deeper = starlark.NewDict(1)
		err    = deeper.SetKey(
			starlark.String("k"),
			_Held(starlark.MakeInt(1)),
		)
	)
	if err != nil {
		t.Fatal(err)
	}

	cases := map[string]starlark.Value{
		"none":    starlark.None,
		"a bool":  starlark.True,
		"an int":  starlark.MakeInt(7),
		"a float": starlark.Float(1.5),
		"a word":  starlark.String("x"),
		"bytes":   starlark.Bytes("x"),
		"a list": _Held(
			starlark.MakeInt(1),
			starlark.String("a"),
		),
		"a tuple": starlark.Tuple{starlark.MakeInt(1)},
		"a dict":  deeper,
		"nested": _Held(
			deeper,
			starlark.Tuple{starlark.None},
		),
	}

	for name, value := range cases {
		t.Run(
			name,
			func(t *testing.T) {
				bad, ok, err := deep.IsData(
					context.Background(),
					value,
				)
				if err != nil {
					t.Fatal(err)
				}

				if !ok {
					t.Fatalf(
						"%s was refused, at %v",
						name,
						bad,
					)
				}
			},
		)
	}
}

// TestIsData_RefusesWhatIsNotData names the thing that failed rather than the
// container it was found in, which is the whole reason the first return
// exists.
//
// Revisions:
//   - 2026-09-24 20:22: initial creation
//   - 2026-11-02 16:14: IsData takes a context and returns an error (C26)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestIsData_RefusesWhatIsNotData(t *testing.T) {
	var (
		code = starlark.NewBuiltin(
			"helper",
			func(
				thread *starlark.Thread,
				fn *starlark.Builtin,
				args starlark.Tuple,
				kwargs []starlark.Tuple,
			) (starlark.Value, error) {
				return starlark.None, nil
			},
		)
		cases = map[string]starlark.Value{
			"a builtin": code,
			"one inside a list": _Held(
				starlark.MakeInt(1),
				code,
			),
			"one inside a tuple": starlark.Tuple{starlark.None, code},
			"one deeper still":   _Held(_Held(_Held(code))),
		}
	)

	for name, value := range cases {
		t.Run(
			name,
			func(t *testing.T) {
				bad, ok, err := deep.IsData(
					context.Background(),
					value,
				)
				if err != nil {
					t.Fatal(err)
				}

				if ok {
					t.Fatalf(
						"%s was accepted",
						name,
					)
				}

				if bad != code {
					t.Fatalf(
						"%s blamed %v, want the builtin itself",
						name,
						bad,
					)
				}
			},
		)
	}
}

// TestIsData_TerminatesOnAValueThatHoldsItself is the case the walk carries a
// seen set for. Starlark allows x = [1]; x.append(x), and a check that
// followed that reference would not come back.
//
// Revisions:
//   - 2026-09-24 20:22: initial creation
//   - 2026-11-02 16:14: IsData takes a context and returns an error (C26)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestIsData_TerminatesOnAValueThatHoldsItself(t *testing.T) {
	var (
		held = _Held(starlark.MakeInt(1))
		err  = held.Append(held)
	)
	if err != nil {
		t.Fatal(err)
	}

	bad, ok, err := deep.IsData(
		context.Background(),
		held,
	)
	if err != nil {
		t.Fatal(err)
	}

	if !ok {
		t.Fatalf(
			"a list holding itself was refused, at %v",
			bad,
		)
	}
}
