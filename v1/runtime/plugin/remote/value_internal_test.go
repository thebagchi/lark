package remote

import (
	"errors"
	"testing"

	"go.starlark.net/starlark"
)

// TestValue_CrossesAndComesBack is the round trip for everything that can make
// it, and the refusal for everything that cannot.
//
// The two directions are not inverses and the table says where: a tuple
// arrives as a list, bytes arrive as a string, and an integer arrives as a
// float, because protobuf has one number and it is a float64. A caller who
// expected otherwise would find out in a script rather than here.
//
// Revisions:
//   - 2026-09-25 06:48: initial creation
func TestValue_CrossesAndComesBack(t *testing.T) {
	cases := []struct {
		name string
		sent starlark.Value
		back string
	}{
		{"none", starlark.None, "None"},
		{"true", starlark.Bool(true), "True"},
		{"false", starlark.Bool(false), "False"},
		{"an integer arrives as a float", starlark.MakeInt(7), "7.0"},
		{"a float", starlark.Float(1.5), "1.5"},
		{"a string", starlark.String("hello"), `"hello"`},
		{"bytes arrive as a string", starlark.Bytes("hi"), `"hi"`},
		{
			"a list",
			starlark.NewList([]starlark.Value{
				starlark.MakeInt(1),
				starlark.String("a"),
			}),
			`[1.0, "a"]`,
		},
		{
			"a tuple arrives as a list",
			starlark.Tuple{starlark.MakeInt(1), starlark.MakeInt(2)},
			"[1.0, 2.0]",
		},
		{"an empty list", starlark.NewList(nil), "[]"},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			crossed, err := _Value(item.sent)
			if err != nil {
				t.Fatalf("%s did not cross: %v", item.sent, err)
			}

			got, err := _Starlark(crossed)
			if err != nil {
				t.Fatalf("%s did not come back: %v", item.sent, err)
			}

			if got.String() != item.back {
				t.Fatalf("%s came back as %s, want %s", item.sent, got, item.back)
			}
		})
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
func TestValue_ADictCrossesByItsStringKeys(t *testing.T) {
	held := starlark.NewDict(1)

	err := held.SetKey(starlark.String("k"), starlark.MakeInt(1))
	if err != nil {
		t.Fatal(err)
	}

	crossed, err := _Value(held)
	if err != nil {
		t.Fatalf("a dictionary keyed by a string: %v", err)
	}

	got, err := _Starlark(crossed)
	if err != nil {
		t.Fatal(err)
	}

	if got.String() != `{"k": 1.0}` {
		t.Fatalf("got %s", got)
	}

	numbered := starlark.NewDict(1)

	err = numbered.SetKey(starlark.MakeInt(1), starlark.String("v"))
	if err != nil {
		t.Fatal(err)
	}

	_, err = _Value(numbered)
	if !errors.Is(err, ErrValue) {
		t.Fatalf("a dictionary keyed by a number gave %v, want ErrValue", err)
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
func TestValue_RefusesWhatItCannotName(t *testing.T) {
	cases := map[string]starlark.Value{
		"a builtin":              starlark.NewBuiltin("f", nil),
		"an integer too precise": starlark.MakeInt64(EXACT + 1),
		"and a negative one":     starlark.MakeInt64(-EXACT - 1),
	}

	for name, given := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := _Value(given)
			if !errors.Is(err, ErrValue) {
				t.Fatalf("got %v, want ErrValue", err)
			}

			t.Logf("refused with: %v", err)
		})
	}
}
