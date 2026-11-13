// Tests of package wire, from outside it: wire.Value is exported, so nothing
// here needs an internal path. The round trip back to Starlark uses
// unpack.FromProto, the converter host._Starlark wraps.

package tests

import (
	"context"
	"errors"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/wire"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/trail"
)

// TestValue_CrossesAndComesBack is the round trip for everything that can make
// it, and the refusal for everything that cannot.
//
// The two directions are not inverses and the table says where: a tuple
// arrives as a list, and bytes arrive as a string. A whole number comes back
// as an int and a fraction as a float, the same rule the args plugin uses.
//
// Revisions:
//   - 2026-09-25 06:48: initial creation
//   - 2026-11-02 10:55: a whole number comes back as an int, not a float
//   - 2026-11-02 16:14: _Value takes a trail (C26)
//   - 2026-11-08 12:40: moved to package wire from host, as an external test (C73, Q257)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestValue_CrossesAndComesBack(t *testing.T) {
	cases := []struct {
		name string
		sent starlark.Value
		back string
	}{
		{
			"none",
			starlark.None,
			"None",
		},
		{
			"true",
			starlark.Bool(true),
			"True",
		},
		{
			"false",
			starlark.Bool(false),
			"False",
		},
		{
			"an integer arrives as an int",
			starlark.MakeInt(7),
			"7",
		},
		{
			"a float",
			starlark.Float(1.5),
			"1.5",
		},
		{
			"a string",
			starlark.String("hello"),
			`"hello"`,
		},
		{
			"bytes arrive as a string",
			starlark.Bytes("hi"),
			`"hi"`,
		},
		{
			"a list",
			starlark.NewList([]starlark.Value{
				starlark.MakeInt(1),
				starlark.String("a"),
			}),
			`[1, "a"]`,
		},
		{
			"a tuple arrives as a list",
			starlark.Tuple{starlark.MakeInt(1), starlark.MakeInt(2)},
			"[1, 2]",
		},
		{
			"an empty list",
			starlark.NewList(nil),
			"[]",
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				crossed, err := wire.Value(
					trail.New(context.Background()),
					item.sent,
				)
				if err != nil {
					t.Fatalf(
						"%s did not cross: %v",
						item.sent,
						err,
					)
				}

				got, err := unpack.FromProto(crossed)
				if err != nil {
					t.Fatalf(
						"%s did not come back: %v",
						item.sent,
						err,
					)
				}

				if got.String() != item.back {
					t.Fatalf(
						"%s came back as %s, want %s",
						item.sent,
						got,
						item.back,
					)
				}
			},
		)
	}
}

// TestValue_ADictCrossesByItsStringKeys is the one place the wire is stricter
// than the store.
//
// A protobuf struct keys by string. deep.IsData allows a number as a key,
// because a store can hold one, so a dictionary that a store would take is a
// dictionary this refuses - and it says which key rather than that the
// dictionary was wrong.
//
// Revisions:
//   - 2026-09-25 06:48: initial creation
//   - 2026-11-02 16:14: _Value takes a trail (C26)
//   - 2026-11-08 12:40: moved to package wire from host, as an external test (C73, Q257)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrConvert, formerly ErrNotProtoValue (C121, Q349)
func TestValue_ADictCrossesByItsStringKeys(t *testing.T) {
	var (
		held = starlark.NewDict(1)
		err  = held.SetKey(
			starlark.String("k"),
			starlark.MakeInt(1),
		)
	)
	if err != nil {
		t.Fatal(err)
	}

	crossed, err := wire.Value(
		trail.New(context.Background()),
		held,
	)
	if err != nil {
		t.Fatalf(
			"a dictionary keyed by a string: %v",
			err,
		)
	}

	got, err := unpack.FromProto(crossed)
	if err != nil {
		t.Fatal(err)
	}

	if got.String() != `{"k": 1}` {
		t.Fatalf(
			"got %s",
			got,
		)
	}

	numbered := starlark.NewDict(1)

	err = numbered.SetKey(
		starlark.MakeInt(1),
		starlark.String("v"),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = wire.Value(
		trail.New(context.Background()),
		numbered,
	)
	if !errors.Is(
		err,
		unpack.ErrConvert,
	) {
		t.Fatalf(
			"a dictionary keyed by a number gave %v, want ErrConvert",
			err,
		)
	}
}

// TestValue_RefusesWhatItCannotName covers the kinds that have no protobuf
// shape, and the integer that has one but would not survive it.
//
// An integer past what a float64 holds exactly would arrive as a different
// number than it left. Saying so beats handing a plugin a quiet
// approximation - the kind of defect that surfaces as arithmetic being wrong
// somewhere else entirely.
//
// This test found the first guard doing nothing: it compared the float round
// trip, and the two numbers it had to tell apart are the same float64.
//
// Revisions:
//   - 2026-09-25 06:48: initial creation
//   - 2026-11-02 16:14: _Value takes a trail (C26)
//   - 2026-11-08 12:40: moved to package wire from host, as an external test (C73, Q257)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 11:46: a typed-nil list, dict and set are refused, not
//     dereferenced (C117, B240)
//   - 2026-11-10 17:15: uses ErrConvert, formerly ErrNotProtoValue (C121, Q349)
func TestValue_RefusesWhatItCannotName(t *testing.T) {
	cases := map[string]starlark.Value{
		"a nil value": nil,
		"a nil list":  (*starlark.List)(nil),
		"a nil dict":  (*starlark.Dict)(nil),
		"a nil set":   (*starlark.Set)(nil),
		"a builtin": starlark.NewBuiltin(
			"f",
			nil,
		),
		"an integer too precise": starlark.MakeInt64(wire.EXACT + 1),
		"and a negative one":     starlark.MakeInt64(-wire.EXACT - 1),
	}

	for name, given := range cases {
		t.Run(
			name,
			func(t *testing.T) {
				_, err := wire.Value(
					trail.New(context.Background()),
					given,
				)
				if !errors.Is(
					err,
					unpack.ErrConvert,
				) {
					t.Fatalf(
						"got %v, want ErrConvert",
						err,
					)
				}

				t.Logf(
					"refused with: %v",
					err,
				)
			},
		)
	}
}

// TestValue_AContainerHoldingItselfIsACycleError proves a list, a dict and a
// cycle through a tuple are each trail.ErrCyclic, an error of the call
// that is not fatal, where they once overflowed the stack of Go.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-08 12:40: moved to package wire from host, as an external test (C73, Q257)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrCyclic, formerly ErrSelfContaining (C121, Q349)
func TestValue_AContainerHoldingItselfIsACycleError(t *testing.T) {
	var (
		list = starlark.NewList(nil)
		err  = list.Append(list)
	)
	if err != nil {
		t.Fatal(err)
	}

	dict := starlark.NewDict(1)

	err = dict.SetKey(
		starlark.String("self"),
		dict,
	)
	if err != nil {
		t.Fatal(err)
	}

	inner := starlark.NewList(nil)

	err = inner.Append(starlark.Tuple{inner})
	if err != nil {
		t.Fatal(err)
	}

	cases := map[string]starlark.Value{
		"a list":               list,
		"a dict":               dict,
		"a cycle through them": inner,
	}

	for name, given := range cases {
		t.Run(
			name,
			func(t *testing.T) {
				_, err := wire.Value(
					trail.New(context.Background()),
					given,
				)
				if !errors.Is(
					err,
					trail.ErrCyclic,
				) {
					t.Fatalf(
						"got %v, want ErrCyclic",
						err,
					)
				}

				if errors.Is(
					err,
					trail.ErrFatal,
				) {
					t.Fatalf(
						"a cycle must not be fatal: %v",
						err,
					)
				}
			},
		)
	}
}

// TestValue_ASharedListIsNotACycle proves a list reached by two routes crosses.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-08 12:40: moved to package wire from host, as an external test (C73, Q257)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestValue_ASharedListIsNotACycle(t *testing.T) {
	var (
		shared = starlark.NewList(nil)
		_, err = wire.Value(
			trail.New(context.Background()),
			starlark.Tuple{shared, shared},
		)
	)
	if err != nil {
		t.Fatal(err)
	}
}

// TestValue_PastTheDepthLimitIsTooDeep proves a value nested past
// common.DEPTH is trail.ErrDeep, which wraps trail.ErrFatal, and one at the limit
// crosses.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-08 12:40: moved to package wire from host, as an external test (C73, Q257)
//   - 2026-11-08 13:25: names the package of each sentinel (C98 round 2)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrDeep, formerly ErrTooDeep (C121, Q349)
func TestValue_PastTheDepthLimitIsTooDeep(t *testing.T) {
	var (
		atIt starlark.Value = starlark.NewList(nil)
		past starlark.Value = starlark.NewList(nil)
	)

	for range common.DEPTH - 1 {
		atIt = starlark.NewList([]starlark.Value{atIt})
	}

	for range common.DEPTH {
		past = starlark.NewList([]starlark.Value{past})
	}

	_, err := wire.Value(
		trail.New(context.Background()),
		atIt,
	)
	if err != nil {
		t.Fatalf(
			"at the limit: %v",
			err,
		)
	}

	_, err = wire.Value(
		trail.New(context.Background()),
		past,
	)
	if !errors.Is(
		err,
		trail.ErrDeep,
	) || !errors.Is(
		err,
		trail.ErrFatal,
	) {
		t.Fatalf(
			"past the limit = %v, want ErrDeep",
			err,
		)
	}
}

// TestValue_StopsWhenTheContextIsDone proves the conversion looks at the
// context: a long flat list is refused with the context's error.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-08 12:40: moved to package wire from host, as an external test (C73, Q257)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestValue_StopsWhenTheContextIsDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	long := make(
		[]starlark.Value,
		4*common.POLL,
	)
	for index := range long {
		long[index] = starlark.None
	}

	_, err := wire.Value(
		trail.New(ctx),
		starlark.NewList(long),
	)
	if !errors.Is(
		err,
		context.Canceled,
	) {
		t.Fatalf(
			"got %v, want context.Canceled",
			err,
		)
	}
}

// TestValue_ASetCrossesAsAList proves a set, which deep.IsData accepts, crosses
// as a list of its members (B004). Before, it had no case and was refused.
//
// Revisions:
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-08 12:40: moved to package wire from host, as an external test (C73, Q257)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestValue_ASetCrossesAsAList(t *testing.T) {
	var (
		set = starlark.NewSet(2)
		err = set.Insert(starlark.MakeInt(1))
	)
	if err != nil {
		t.Fatal(err)
	}

	err = set.Insert(starlark.String("a"))
	if err != nil {
		t.Fatal(err)
	}

	crossed, err := wire.Value(
		trail.New(context.Background()),
		set,
	)
	if err != nil {
		t.Fatalf(
			"a set did not cross: %v",
			err,
		)
	}

	const WANT = "[1, \"a\"]"

	back, err := unpack.FromProto(crossed)
	if err != nil {
		t.Fatal(err)
	}

	if back.String() != WANT {
		t.Fatalf(
			"got %s, want %s",
			back,
			WANT,
		)
	}
}
