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
package random

import (
	cryptorand "crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"sync"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

var (
	// ErrEmpty is returned for a choice or a sample from nothing, which has
	// no answer to give.
	ErrEmpty = errors.New("nothing to choose from")

	// ErrRange is returned for a range whose end is below its start, and for
	// a count below zero.
	ErrRange = errors.New("out of range")
)

const (
	// NAME is the module, and the names it holds.
	NAME    = "random"
	SEED    = "seed"
	INT     = "int"
	FLOAT   = "float"
	CHOICE  = "choice"
	SHUFFLE = "shuffle"
	BYTES   = "bytes"
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
type _Random struct{}

// _Source is one run's generator, and the lock that makes it safe for the
// threads of that run to draw at once.
//
// A generator is not safe for concurrent use, and every thread of a run shares
// this one, so the lock is what makes the sharing legal rather than merely
// usual.
type _Source struct {
	guard sync.Mutex
	held  *rand.Rand
}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func (r *_Random) Name() string {
	return NAME
}

// Values returns the random module.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func (r *_Random) Values() starlark.StringDict {
	return starlark.StringDict{
		NAME: &starlarkstruct.Module{
			Name: NAME,
			Members: starlark.StringDict{
				SEED:    starlark.NewBuiltin(NAME+"."+SEED, _Seed),
				INT:     starlark.NewBuiltin(NAME+"."+INT, _Int),
				FLOAT:   starlark.NewBuiltin(NAME+"."+FLOAT, _Float),
				CHOICE:  starlark.NewBuiltin(NAME+"."+CHOICE, _Choice),
				SHUFFLE: starlark.NewBuiltin(NAME+"."+SHUFFLE, _Shuffle),
				BYTES:   starlark.NewBuiltin(NAME+"."+BYTES, _Bytes),
			},
		},
	}
}

// _Of returns the source belonging to the run on thread, making one the first
// time this execution asks.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Of(thread *starlark.Thread) (*_Source, error) {
	return scheduler.Shared(thread, NAME, func() *_Source {
		return &_Source{held: rand.New(rand.NewPCG(_Unpredictable(), _Unpredictable()))}
	})
}

// _Unpredictable is a seed nobody can guess, for a run that asked for no
// particular sequence.
//
// From crypto/rand, which cannot fail in any way this package could act on: a
// process whose entropy source is broken has worse problems than a repeated
// sequence, and there is nowhere here to report it to. A failure falls back to
// zero, which is a seed like any other.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Unpredictable() uint64 {
	var held [8]byte

	_, err := cryptorand.Read(held[:])
	if err != nil {
		return 0
	}

	return binary.LittleEndian.Uint64(held[:])
}

// _Seed makes this run's source hand out the sequence that seed names.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Seed(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var seed int64

	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 1, &seed)
	if err != nil {
		return nil, err
	}

	source, err := _Of(thread)
	if err != nil {
		return nil, err
	}

	source.guard.Lock()
	defer source.guard.Unlock()

	source.held = rand.New(rand.NewPCG(uint64(seed), uint64(seed)))

	return starlark.None, nil
}

// _Int is a whole number from low to high, both ends included.
//
// Both ends, as Python's randint has them, because a caller asking for a die
// writes 1 and 6 and means both.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-09-24 16:23: takes the width in unsigned, which the whole int64
//     range does not fit in signed
func _Int(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var low, high int64

	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 2, &low, &high)
	if err != nil {
		return nil, err
	}

	if high < low {
		return nil, fmt.Errorf("%s: %d is below %d: %w", fn.Name(), high, low, ErrRange)
	}

	source, err := _Of(thread)
	if err != nil {
		return nil, err
	}

	source.guard.Lock()
	defer source.guard.Unlock()

	return starlark.MakeInt64(int64(uint64(low) + _Draw(source, low, high))), nil
}

// _Draw is one value from zero up to the width of low..high, both ends
// included.
//
// Unsigned throughout, because the width of the whole int64 range is one more
// than an int64 can hold: high-low+1 wraps to the most negative int64 for a
// range that wide, and a generator handed that panics. The unsigned difference
// is exact for every pair, and the caller's addition is unsigned too, so
// nothing on this path can overflow.
//
// Callers hold the lock.
//
// Revisions:
//   - 2026-09-24 16:23: initial creation
func _Draw(source *_Source, low int64, high int64) uint64 {
	span := uint64(high) - uint64(low)

	// Every uint64 is inside a range this wide, so there is nothing to bound.
	if span == math.MaxUint64 {
		return source.held.Uint64()
	}

	return source.held.Uint64N(span + 1)
}

// _Float is a number from zero up to but not including one.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Float(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 0)
	if err != nil {
		return nil, err
	}

	source, err := _Of(thread)
	if err != nil {
		return nil, err
	}

	source.guard.Lock()
	defer source.guard.Unlock()

	return starlark.Float(source.held.Float64()), nil
}

// _Choice is one element of a sequence.
//
// Returns ErrEmpty for a sequence with nothing in it, rather than None: a
// caller who meant to choose from something got nothing, and None would be
// indistinguishable from a sequence that held one.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Choice(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	held, err := _Elements(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	if len(held) == 0 {
		return nil, fmt.Errorf("%s: %w", fn.Name(), ErrEmpty)
	}

	source, err := _Of(thread)
	if err != nil {
		return nil, err
	}

	source.guard.Lock()
	defer source.guard.Unlock()

	return held[source.held.IntN(len(held))], nil
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
func _Shuffle(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	held, err := _Elements(fn, args, kwargs)
	if err != nil {
		return nil, err
	}

	mixed := make([]starlark.Value, len(held))
	copy(mixed, held)

	source, err := _Of(thread)
	if err != nil {
		return nil, err
	}

	source.guard.Lock()
	defer source.guard.Unlock()

	source.held.Shuffle(len(mixed), func(i int, j int) {
		mixed[i], mixed[j] = mixed[j], mixed[i]
	})

	return starlark.NewList(mixed), nil
}

// _Bytes is count random bytes.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Bytes(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var count int

	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 1, &count)
	if err != nil {
		return nil, err
	}

	if count < 0 {
		return nil, fmt.Errorf("%s: %d bytes: %w", fn.Name(), count, ErrRange)
	}

	source, err := _Of(thread)
	if err != nil {
		return nil, err
	}

	source.guard.Lock()
	defer source.guard.Unlock()

	held := make([]byte, count)

	for index := range held {
		held[index] = byte(source.held.UintN(256))
	}

	return starlark.Bytes(held), nil
}

// _Elements reads a builtin's one argument as the values it holds.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Elements(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) ([]starlark.Value, error) {
	var given starlark.Iterable

	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 1, &given)
	if err != nil {
		return nil, err
	}

	var held []starlark.Value

	iter := given.Iterate()
	defer iter.Done()

	var value starlark.Value

	for iter.Next(&value) {
		held = append(held, value)
	}

	return held, nil
}
