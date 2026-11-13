// Package number holds the arithmetic builtins of the math plugin that the tests
// reach: ceil, floor, trunc, gcd and log, with the number conversion they share.
// The plugin type, its registration and the other builtins stay in package math.
//
// Revisions:
//   - 2026-11-10 01:37: initial creation (C120b, Q315)
package number

import (
	"errors"
	"fmt"
	"math"
	"math/big"

	"go.starlark.net/starlark"

	shared "github.com/thebagchi/lark/v1/runtime/plugin/internal/common"
)

var (
	// ErrNumber is returned for an argument that is neither a float nor an
	// int.
	//
	// Revisions:
	//   - 2026-09-24 17:26: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved from v1/runtime/plugin/math (C120b, Q315)
	ErrNumber = errors.New("wants a float or an int")

	// ErrBase is returned for a logarithm in base one, which names no power.
	//
	// Revisions:
	//   - 2026-09-24 17:26: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved from v1/runtime/plugin/math (C120b, Q315)
	ErrBase = errors.New("no logarithm has base one")
)

// Ceil is the smallest integer at or above x.
//
// An integer, as the library's own is, and as Python's is. An int given back
// unchanged rather than sent through a float, because a large one would not
// survive the trip.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/math and renamed _Ceil to Ceil, for the
//     tests in tests/ (C120b, Q315, Q314)
func Ceil(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	return _Whole(
		shared.Called(fn),
		args,
		kwargs,
		math.Ceil,
	)
}

// Floor is the largest integer at or below x.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/math and renamed _Floor to Floor, for the
//     tests in tests/ (C120b, Q315, Q314)
func Floor(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	return _Whole(
		shared.Called(fn),
		args,
		kwargs,
		math.Floor,
	)
}

// Trunc is x with its fraction dropped, which rounds towards zero.
//
// An integer, like ceil and floor, because it belongs to that family: the
// three differ in which way they go and in nothing else.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/math and renamed _Trunc to Trunc, for the
//     tests in tests/ (C120b, Q315, Q314)
func Trunc(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	return _Whole(
		shared.Called(fn),
		args,
		kwargs,
		math.Trunc,
	)
}

// _Whole answers in integers, which is what the three rounding directions do.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-05 17:10: the refusal is _NotNumber's (B038, C96)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/math (C120b, Q315)
func _Whole(
	who string,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
	fn func(float64) float64,
) (starlark.Value, error) {
	var given starlark.Value

	err := starlark.UnpackPositionalArgs(
		who,
		args,
		kwargs,
		1,
		&given,
	)
	if err != nil {
		return nil, err
	}

	switch held := given.(type) {
	case starlark.Int:
		return held, nil

	case starlark.Float:
		return starlark.NumberToInt(starlark.Float(fn(float64(held))))
	}

	return nil, _NotNumber(
		who,
		given,
	)
}

// Log is the logarithm of x, in base e unless a second argument says
// otherwise.
//
// Returns ErrBase for base one, which names no power of anything.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/math and renamed _Log to Log, for the
//     tests in tests/ (C120b, Q315, Q314)
func Log(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		given starlark.Value
		named starlark.Value
	)

	err := starlark.UnpackPositionalArgs(
		shared.Called(fn),
		args,
		kwargs,
		1,
		&given,
		&named,
	)
	if err != nil {
		return nil, err
	}

	x, err := Float(
		shared.Called(fn),
		given,
	)
	if err != nil {
		return nil, err
	}

	base, err := _Base(
		shared.Called(fn),
		named,
	)
	if err != nil {
		return nil, err
	}

	if base == 1 {
		return nil, fmt.Errorf(
			"%s: %w",
			shared.Called(fn),
			ErrBase,
		)
	}

	return starlark.Float(math.Log(x) / math.Log(base)), nil
}

// _Base is the logarithm base a call named, or e when it named none.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/math (C120b, Q315)
func _Base(
	who string,
	named starlark.Value,
) (float64, error) {
	if named == nil {
		return math.E, nil
	}

	return Float(
		who,
		named,
	)
}

// GCD is the greatest common divisor of two integers.
//
// Integers only, and an integer back. A divisor of two floats is a question
// with no answer, and taking floats here would make one up.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-06 21:23: names the builtin with Called, nil-safe (C102, Q213)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/math and renamed _GCD to GCD, for the
//     tests in tests/ (C120b, Q315, Q314)
func GCD(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var first, second starlark.Int

	const NEEDED = 2

	err := starlark.UnpackPositionalArgs(
		shared.Called(fn),
		args,
		kwargs,
		NEEDED,
		&first,
		&second,
	)
	if err != nil {
		return nil, err
	}

	held := new(
		big.Int).GCD(
		nil,
		nil,
		new(big.Int).Abs(first.BigInt()),
		new(big.Int).Abs(second.BigInt()),
	)

	return starlark.MakeBigInt(held), nil
}

// Float is one value as a float, accepting an int as every one of these does.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-05 17:10: the refusal is _NotNumber's (B038, C96)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/math and renamed _Float to Float, for the
//     tests in tests/ (C120b, Q315, Q314)
func Float(
	who string,
	given starlark.Value,
) (float64, error) {
	switch held := given.(type) {
	case starlark.Float:
		return float64(held), nil

	case starlark.Int:
		return float64(held.Float()), nil
	}

	return 0, _NotNumber(
		who,
		given,
	)
}

// _NotNumber is the refusal of a value that is neither an int nor a float,
// shared by _Whole and Float: "who got TYPE: ErrNumber".
//
// Revisions:
//   - 2026-11-05 17:10: initial creation, from the error text _Whole and _Float
//     each wrote (B038, C96)
//   - 2026-11-10 01:37: moved from v1/runtime/plugin/math (C120b, Q315)
func _NotNumber(
	who string,
	given starlark.Value,
) error {
	return fmt.Errorf(
		"%s got %s: %w",
		who,
		given.Type(),
		ErrNumber,
	)
}
