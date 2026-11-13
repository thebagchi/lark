// Package random gives a script a source of random values. Importing it is
// what enables it.
//
// The source belongs to one run, as state's store does, and is found on the
// thread rather than held here: two runs of one artifact draw independently,
// and every thread inside a run draws from the same stream.
//
// Seeding is what makes a run repeatable, and it repeats less than it looks
// like it does. A seeded source hands out one sequence; which thread receives
// which number depends on the order the threads ask, and that order is the
// scheduler's rather than the script's. So a single-threaded run with a seed
// repeats exactly, and a concurrent one repeats only in the values drawn and
// not in who drew them. A script that needs a thread to repeat gives that
// thread its own seed and draws only there.
//
// Unseeded, the source is seeded from crypto/rand, so a run nobody seeded is
// unpredictable rather than the same every time the process starts.
//
// Revisions:
//   - 2026-09-24 17:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
package random

import (
	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/random/internal/draw"
	"github.com/thebagchi/lark/v1/runtime/plugin/random/internal/spec"
)

var (
	// ErrEmpty is draw.ErrEmpty.
	//
	// Revisions:
	//   - 2026-11-10 01:37: initial creation (C120b, Q319)
	ErrEmpty = draw.ErrEmpty
)

// init registers this plugin, so that a host importing this package for its
// side effect is the whole of enabling it.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func init() {
	plugin.Register(new(_Random))
}

// _Random is the plugin. Empty: a source belongs to a run, not to the plugin.
//
// Revisions:
//   - 2026-09-24 17:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Random struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-08 00:27: names read from internal/spec (C91w12a)
func (r *_Random) Name() string {
	return spec.NAME
}

// Values returns the random module.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-08 00:27: names read from internal/spec (C91w12a)
//   - 2026-11-10 11:32: uses draw.Seed, draw.Int, draw.Float, draw.Choice and draw.Bytes, which
//     moved to internal/draw (C120b, Q315)
func (r *_Random) Values() starlark.StringDict {
	return starlark.StringDict{
		spec.NAME: &starlarkstruct.Module{
			Name: spec.NAME,
			Members: starlark.StringDict{
				spec.SEED: starlark.NewBuiltin(
					spec.NAME+"."+spec.SEED,
					draw.Seed,
				),
				spec.INT: starlark.NewBuiltin(
					spec.NAME+"."+spec.INT,
					draw.Int,
				),
				spec.FLOAT: starlark.NewBuiltin(
					spec.NAME+"."+spec.FLOAT,
					draw.Float,
				),
				spec.CHOICE: starlark.NewBuiltin(
					spec.NAME+"."+spec.CHOICE,
					draw.Choice,
				),
				spec.SHUFFLE: starlark.NewBuiltin(
					spec.NAME+"."+spec.SHUFFLE,
					_Shuffle,
				),
				spec.BYTES: starlark.NewBuiltin(
					spec.NAME+"."+spec.BYTES,
					draw.Bytes,
				),
			},
		},
	}
}

// _Shuffle is a sequence in a new order.
//
// A new list rather than the one it was given, reordered. Module scope freezes
// before anything concurrent runs, so a script cannot be handed back the list
// it declared - and a function that worked at the top of a script and failed
// inside a thread would be worse than one that never reorders in place.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-10 11:32: uses draw.Of, which moved to internal/draw (C120b, Q315)
//   - 2026-11-10 11:45: shuffles the slice that Collected made, with no second copy, so the
//     one memory check covers the one block (C116, B235)
func _Shuffle(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	held, err := _Elements(
		fn,
		args,
		kwargs,
	)
	if err != nil {
		return nil, err
	}

	// held is made by Collected for this call alone, so it is the new list
	// already: a second slice would be a second block the memory check never saw.
	source, err := draw.Of(thread)
	if err != nil {
		return nil, err
	}

	source.Guard.Lock()
	defer source.Guard.Unlock()

	source.Held.Shuffle(
		len(held),
		func(
			i int,
			j int,
		) {
			held[i], held[j] = held[j], held[i]
		},
	)

	return starlark.NewList(held), nil
}

// _Elements reads a builtin's one argument as the values it holds.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-02 11:29: in two steps, _Given and _Collected, so _Choice can skip the second
//   - 2026-11-10 11:32: uses draw.Given and draw.Collected, which moved to internal/draw
//     (C120b, Q315)
//   - 2026-11-10 11:45: refuses a length that does not fit the free memory (C116, B235)
func _Elements(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) ([]starlark.Value, error) {
	given, err := draw.Given(
		fn,
		args,
		kwargs,
	)
	if err != nil {
		return nil, err
	}

	return draw.Collected(
		fn,
		given,
	)
}
