// Package draw holds the random source of the random plugin and the builtins
// that draw from it: bytes, choice, float, int and seed. The plugin type, its
// registration and the shuffle stay in package random.
//
// Revisions:
//   - 2026-11-10 01:37: initial creation (C120b, Q315)
package draw

import (
	cryptorand "crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"math/rand/v2"
	"sync"
	"unsafe"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/memwatch"
	shared "github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/random/internal/spec"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

var (
	// ErrEmpty is returned for a choice or a sample from nothing, which has
	// no answer to give.
	//
	// Revisions:
	//   - 2026-09-24 17:26: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
	//   - 2026-11-10 01:37: moved from v1/runtime/plugin/random (C120b, Q315)
	ErrEmpty = errors.New("nothing to choose from")
)

// _Source is one run's generator, and the lock that makes it safe for the
// threads of that run to draw at once.
//
// A generator is not safe for concurrent use, and every thread of a run shares
// this one, so the lock is what makes the sharing legal rather than merely
// usual.
//
// Revisions:
//   - 2026-09-24 17:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/random and renamed the fields guard to
//     Guard, held to Held, for the tests in tests/ (C120b, Q315, Q314)
type _Source struct {
	Guard sync.Mutex
	Held  *rand.Rand
}

// Of returns the source belonging to the run on thread, making one the first
// time this execution asks.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-08 00:27: names read from internal/spec (C91w12a)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/random and renamed _Of to Of, for the
//     tests in tests/ (C120b, Q315, Q314)
func Of(thread *starlark.Thread) (*_Source, error) {
	return scheduler.Shared(
		thread,
		spec.NAME,
		func() *_Source {
			return &_Source{
				Held: rand.New(rand.NewPCG(
					_Unpredictable(),
					_Unpredictable(),
				)),
			}
		},
	)
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
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/random (C120b, Q315)
func _Unpredictable() uint64 {
	var held [8]byte

	_, err := cryptorand.Read(held[:])
	if err != nil {
		return 0
	}

	return binary.LittleEndian.Uint64(held[:])
}

// Seed makes this run's source hand out the sequence that seed names.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/random and renamed _Seed to Seed, for the
//     tests in tests/ (C120b, Q315, Q314)
func Seed(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var seed int64

	err := starlark.UnpackPositionalArgs(
		shared.Called(fn),
		args,
		kwargs,
		1,
		&seed,
	)
	if err != nil {
		return nil, err
	}

	source, err := Of(thread)
	if err != nil {
		return nil, err
	}

	source.Guard.Lock()
	defer source.Guard.Unlock()

	source.Held = rand.New(rand.NewPCG(
		uint64(seed),
		uint64(seed),
	))

	return starlark.None, nil
}

// Int is a whole number from low to high, both ends included.
//
// Both ends, as Python's randint has them, because a caller asking for a die
// writes 1 and 6 and means both.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-09-24 16:23: takes the width in unsigned, which the whole int64
//     range does not fit in signed
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/random and renamed _Int to Int, for the
//     tests in tests/ (C120b, Q315, Q314)
func Int(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var low, high int64

	const NEEDED = 2

	err := starlark.UnpackPositionalArgs(
		shared.Called(fn),
		args,
		kwargs,
		NEEDED,
		&low,
		&high,
	)
	if err != nil {
		return nil, err
	}

	if high < low {
		return nil, fmt.Errorf(
			"%s: %d is below %d: %w",
			shared.Called(fn),
			high,
			low,
			unpack.ErrRange,
		)
	}

	source, err := Of(thread)
	if err != nil {
		return nil, err
	}

	source.Guard.Lock()
	defer source.Guard.Unlock()

	return starlark.MakeInt64(int64(uint64(low) + _Draw(
		source,
		low,
		high,
	))), nil
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
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/random (C120b, Q315)
func _Draw(
	source *_Source,
	low int64,
	high int64,
) uint64 {
	span := uint64(high) - uint64(low)

	// Every uint64 is inside a range this wide, so there is nothing to bound.
	if span == math.MaxUint64 {
		return source.Held.Uint64()
	}

	return source.Held.Uint64N(span + 1)
}

// Float is a number from zero up to but not including one.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/random and renamed _Float to Float, for
//     the tests in tests/ (C120b, Q315, Q314)
func Float(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	const NEEDED = 0

	err := starlark.UnpackPositionalArgs(
		shared.Called(fn),
		args,
		kwargs,
		NEEDED,
	)
	if err != nil {
		return nil, err
	}

	source, err := Of(thread)
	if err != nil {
		return nil, err
	}

	source.Guard.Lock()
	defer source.Guard.Unlock()

	return starlark.Float(source.Held.Float64()), nil
}

// Choice is one element of a sequence.
//
// Returns ErrEmpty for a sequence with nothing in it, rather than None: a
// caller who meant to choose from something got nothing, and None would be
// indistinguishable from a sequence that held one.
//
// The draw is one IntN of the length, whatever the kind of iterable, so a seed
// names the same choices as it did when the iterable was copied first.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-02 11:29: reads a list, a tuple or a range where it lies, rather than copying
//     it to a slice first
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/random and renamed _Choice to Choice, for
//     the tests in tests/ (C120b, Q315, Q314)
//   - 2026-11-10 11:45: refuses a set or a dict whose length does not fit the free memory
//     (C116, B235)
func Choice(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	given, err := Given(
		fn,
		args,
		kwargs,
	)
	if err != nil {
		return nil, err
	}

	// An Indexable that is also Iterable (list, tuple, range, string.elems())
	// yields its elements in index order, so Index(i) is the i-th value the old
	// copy held. A dict or a set has no index and is collected in yield order.
	indexed, ok := given.(starlark.Indexable)
	if !ok {
		held, err := Collected(
			fn,
			given,
		)
		if err != nil {
			return nil, err
		}

		return ChooseCollected(
			thread,
			fn,
			held,
		)
	}

	if indexed.Len() == 0 {
		return nil, fmt.Errorf(
			"%s: %w",
			shared.Called(fn),
			ErrEmpty,
		)
	}

	source, err := Of(thread)
	if err != nil {
		return nil, err
	}

	source.Guard.Lock()
	defer source.Guard.Unlock()

	return indexed.Index(source.Held.IntN(indexed.Len())), nil
}

// ChooseCollected is one value of held, which is the yield order of an
// iterable that has no index.
//
// Revisions:
//   - 2026-11-02 11:30: initial creation, from _Choice
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/random and renamed _ChooseCollected to
//     ChooseCollected, for the tests in tests/ (C120b, Q315, Q314)
func ChooseCollected(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	held []starlark.Value,
) (starlark.Value, error) {
	if len(held) == 0 {
		return nil, fmt.Errorf(
			"%s: %w",
			shared.Called(fn),
			ErrEmpty,
		)
	}

	source, err := Of(thread)
	if err != nil {
		return nil, err
	}

	source.Guard.Lock()
	defer source.Guard.Unlock()

	return held[source.Held.IntN(len(held))], nil
}

// Bytes is count random bytes.
//
// A count that does not fit the free memory is refused (memwatch.ErrRoom)
// before the buffer is made.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-03 00:30: hands the bytes over, with no copy of them (C37)
//   - 2026-11-03 22:21: refuses a count that does not fit the free memory (C46)
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-08 00:27: names read from internal/spec (C91w12a)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/random and renamed _Bytes to Bytes, for
//     the tests in tests/ (C120b, Q315, Q314)
//   - 2026-11-10 17:15: uses ErrRoom, formerly ErrNoRoom (C121, Q349)
func Bytes(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var count int

	err := starlark.UnpackPositionalArgs(
		shared.Called(fn),
		args,
		kwargs,
		1,
		&count,
	)
	if err != nil {
		return nil, err
	}

	if count < 0 {
		return nil, fmt.Errorf(
			"%s: %d bytes: %w",
			shared.Called(fn),
			count,
			unpack.ErrRange,
		)
	}

	err = memwatch.Room(
		shared.Called(fn),
		uint64(count),
	)
	if err != nil {
		return nil, err
	}

	source, err := Of(thread)
	if err != nil {
		return nil, err
	}

	source.Guard.Lock()
	defer source.Guard.Unlock()

	held := make(
		[]byte,
		count,
	)

	for index := range held {
		held[index] = byte(source.Held.UintN(spec.BYTE))
	}

	return starlark.Bytes(unpack.Handover(held)), nil
}

// Given reads a builtin's one argument, which must be iterable.
//
// Revisions:
//   - 2026-11-02 11:29: initial creation, from _Elements
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/random and renamed _Given to Given, for
//     the tests in tests/ (C120b, Q315, Q314)
func Given(
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Iterable, error) {
	var given starlark.Iterable

	err := starlark.UnpackPositionalArgs(
		shared.Called(fn),
		args,
		kwargs,
		1,
		&given,
	)
	if err != nil {
		return nil, err
	}

	return given, nil
}

// Collected is the values of given, in the order it yields them. The slice
// is made at the length given states once, as a dict and a set do, rather than
// grown as it fills.
//
// A length that does not fit the free memory is refused (memwatch.ErrRoom)
// before the slice is made, as Bytes does. fn names the builtin in that error.
//
// Revisions:
//   - 2026-11-02 11:29: initial creation, from _Elements
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/random and renamed _Collected to
//     Collected, for the tests in tests/ (C120b, Q315, Q314)
//   - 2026-11-10 11:45: takes fn, returns an error, and refuses a length that does not fit
//     the free memory (C116, B235, Q23)
//   - 2026-11-10 17:15: uses ErrRoom, formerly ErrNoRoom (C121, Q349)
func Collected(
	fn *starlark.Builtin,
	given starlark.Iterable,
) ([]starlark.Value, error) {
	var (
		held  []starlark.Value
		value starlark.Value
	)

	sized, ok := given.(starlark.Sequence)
	if ok {
		err := memwatch.Room(
			shared.Called(fn),
			_Bytes(sized.Len()),
		)
		if err != nil {
			return nil, err
		}

		held = make(
			[]starlark.Value,
			0,
			sized.Len(),
		)
	}

	iter := given.Iterate()

	defer iter.Done()

	for iter.Next(&value) {
		held = append(
			held,
			value,
		)
	}

	return held, nil
}

// _Bytes is the memory that a slice of count values takes. A negative count
// takes none, and a count too big for the result gives the largest size, which
// no memory fits.
//
// Revisions:
//   - 2026-11-10 11:45: initial creation (C116, B235)
func _Bytes(count int) uint64 {
	const VALUE = uint64(unsafe.Sizeof(starlark.Value(nil)))

	if count < 0 {
		return 0
	}

	if uint64(count) > math.MaxUint64/VALUE {
		return math.MaxUint64
	}

	return uint64(count) * VALUE
}
