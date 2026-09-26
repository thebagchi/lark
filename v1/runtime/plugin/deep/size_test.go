package deep_test

import (
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/deep"
)

const (
	// _LONG is a string long enough that its bytes dominate the fixed word
	// every value pays, so a test can tell one from the other.
	_LONG = 4096

	// _WIDE is how many elements the list fixtures hold.
	_WIDE = 100
)

// TestSize_CountsWhatItCanCount is the shape of the estimate rather than exact
// figures: a value costs something, a longer string costs more, and a container
// costs more than what is in it.
//
// Exact numbers are not asserted, because they are an estimate by design - Go
// will not say what a value occupies. What has to hold is the ordering, since
// that is what a budget decides on.
//
// Revisions:
//   - 2026-09-27 01:52: initial creation
func TestSize_CountsWhatItCanCount(t *testing.T) {
	short := deep.Size(starlark.String("x"))
	long := deep.Size(starlark.String(string(make([]byte, _LONG))))

	if short <= 0 {
		t.Fatalf("a one-character string costs %d, want more than nothing", short)
	}

	if long <= short {
		t.Fatalf("%d bytes cost %d and one byte cost %d", _LONG, long, short)
	}

	if long < _LONG {
		t.Fatalf("%d bytes cost %d, which is less than the bytes", _LONG, long)
	}

	// None costs something, so a list of them is not free.
	if deep.Size(starlark.None) <= 0 {
		t.Fatal("None costs nothing, so a million of them would too")
	}

	held := starlark.NewList(nil)

	for range _WIDE {
		err := held.Append(starlark.String("x"))
		if err != nil {
			t.Fatal(err)
		}
	}

	if deep.Size(held) <= int64(_WIDE)*short {
		t.Fatalf("a list of %d costs %d, want more than its contents",
			_WIDE, deep.Size(held))
	}
}

// TestSize_WeighsATuple is the case that panicked.
//
// The first version marked every container it walked in a map keyed by
// starlark.Value, to end a cycle. A tuple is a Go slice, a slice is not
// hashable, and marking one crashed - which two tests elsewhere caught only
// because they happened to store a tuple. A tuple needs no mark: it is immutable
// and copied by value, so a cycle back to one runs through a list or a dict on
// the way, and that is marked.
//
// Revisions:
//   - 2026-09-27 01:52: initial creation
func TestSize_WeighsATuple(t *testing.T) {
	held := starlark.Tuple{starlark.String("a"), starlark.MakeInt(1)}

	got := deep.Size(held)
	if got <= 0 {
		t.Fatalf("a tuple costs %d, want more than nothing", got)
	}

	// Nested, which is where the mark would have been taken.
	nested := starlark.Tuple{held, held}

	if deep.Size(nested) <= got {
		t.Fatalf("a tuple of two tuples costs %d, one costs %d", deep.Size(nested), got)
	}

	// And inside the containers that are marked.
	inside := starlark.NewList([]starlark.Value{held, nested})
	if deep.Size(inside) <= got {
		t.Fatalf("a list holding tuples costs %d", deep.Size(inside))
	}
}

// TestSize_ATuplesCycleTerminates is the other half of the same worry.
//
// A list holding itself is the cycle a mark exists to stop. Reaching it through
// a tuple is the arrangement that made the first version crash instead.
//
// Revisions:
//   - 2026-09-27 01:52: initial creation
func TestSize_ATuplesCycleTerminates(t *testing.T) {
	held := starlark.NewList(nil)

	err := held.Append(starlark.Tuple{starlark.String("a"), held})
	if err != nil {
		t.Fatal(err)
	}

	// Returns rather than recurring forever, which is the whole assertion.
	got := deep.Size(held)
	if got <= 0 {
		t.Fatalf("a list reaching itself through a tuple costs %d", got)
	}
}

// TestSize_ADictCycleTerminates covers the same for a dictionary, whose keys are
// walked as well as its values.
//
// Revisions:
//   - 2026-09-27 01:52: initial creation
func TestSize_ADictCycleTerminates(t *testing.T) {
	held := starlark.NewDict(1)

	err := held.SetKey(starlark.String("self"), held)
	if err != nil {
		t.Fatal(err)
	}

	got := deep.Size(held)
	if got <= 0 {
		t.Fatalf("a dictionary holding itself costs %d", got)
	}
}

// TestSize_SaysSomethingAboutWhatItDoesNotKnow keeps an unrecognised value from
// being free.
//
// A store would have refused one, so this is the belt rather than the braces -
// but a value that costs nothing is a value a budget cannot bound, and that is
// the wrong way for this to fail.
//
// Revisions:
//   - 2026-09-27 01:52: initial creation
func TestSize_SaysSomethingAboutWhatItDoesNotKnow(t *testing.T) {
	if deep.Size(starlark.NewBuiltin("f", nil)) <= 0 {
		t.Fatal("a value this does not know costs nothing")
	}
}
