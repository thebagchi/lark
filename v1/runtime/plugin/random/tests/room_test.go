package tests

import (
	"errors"
	"math"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/memwatch"
	"github.com/thebagchi/lark/v1/runtime/plugin/random/internal/draw"
)

// _Huge is an iterable that states a length it does not hold. It stands for a
// set or a dict too big to build in a test: the draw must refuse the length
// before it asks for a slice of it, so Iterate must never run. The embedded
// Iterable is nil, and calling it would panic.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C116, B235)
type _Huge struct {
	starlark.Iterable
	Size int
}

// Len is the length the iterable states.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C116, B235)
func (h *_Huge) Len() int {
	return h.Size
}

// TestCollected_RefusesALengthThatDoesNotFit is Q23 for the slice that
// random.shuffle and random.choice of a set or a dict build from Len(): a
// length that does not fit the free memory ends with memwatch.ErrRoom, as
// random.bytes does, and no slice is made.
//
// The check: go test -race -run TestCollected_RefusesALengthThatDoesNotFit
// ./v1/runtime/plugin/random/tests. Before the fix, the call asks Go for one
// huge block and ends with a fatal out-of-memory or a panic on make.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C116, B235)
//   - 2026-11-10 17:15: uses ErrRoom, formerly ErrNoRoom (C121, Q349)
func TestCollected_RefusesALengthThatDoesNotFit(t *testing.T) {
	cases := []struct {
		name string
		size int
	}{
		{
			"sixteen terabytes",
			1 << 40,
		},
		{
			"the largest length",
			math.MaxInt,
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				_, err := draw.Collected(
					nil,
					&_Huge{
						Size: item.size,
					},
				)
				if !errors.Is(
					err,
					memwatch.ErrRoom,
				) {
					t.Fatalf(
						"err = %v, want memwatch.ErrRoom",
						err,
					)
				}
			},
		)
	}
}

// TestChoice_RefusesAnUnindexedLengthThatDoesNotFit is the same refusal as
// seen through random.choice, whose set and dict path collects the values.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C116, B235)
//   - 2026-11-10 17:15: uses ErrRoom, formerly ErrNoRoom (C121, Q349)
func TestChoice_RefusesAnUnindexedLengthThatDoesNotFit(t *testing.T) {
	_, err := draw.Choice(
		new(starlark.Thread),
		starlark.NewBuiltin(
			"choice",
			nil,
		),
		starlark.Tuple{&_Huge{
			Size: 1 << 40,
		}},
		nil,
	)
	if !errors.Is(
		err,
		memwatch.ErrRoom,
	) {
		t.Fatalf(
			"err = %v, want memwatch.ErrRoom",
			err,
		)
	}
}

// TestShuffle_RefusesALengthThatDoesNotFit is the breaker's case: a script
// shuffles a range of 2^40 values, which would need 16 TB.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C116, B235)
//   - 2026-11-10 17:15: uses ErrRoom, formerly ErrNoRoom (C121, Q349)
func TestShuffle_RefusesALengthThatDoesNotFit(t *testing.T) {
	_, err := _Expression(
		t,
		"return random.shuffle(range(1 << 40))",
	)
	if !errors.Is(
		err,
		memwatch.ErrRoom,
	) {
		t.Fatalf(
			"err = %v, want memwatch.ErrRoom",
			err,
		)
	}
}
