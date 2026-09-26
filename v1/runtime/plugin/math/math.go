// Package math gives a script the math module. Importing it is what enables
// it.
//
// Written here rather than taken from go.starlark.net, which is where it came
// from until 2026-09-24. The library's module is missing what a script reaches
// for often enough to notice - the infinities, the two named logarithms, a
// truncation, and the two questions a caller asks about a float it did not
// compute itself.
//
// What the library did, this does identically. A unary function answers in
// floats, ceil and floor answer in ints, and log takes its base second. That
// is deliberate: a script that worked against the library's module works
// against this one, so the only thing this changes is what else it can say.
//
// isnan is the only way to ask. This interpreter does not compare floats the
// way IEEE 754 says: measured on 2026-09-24, nan == nan is True here and nan
// sorts above every number, where Python has the first false and the second
// undefined. A script testing for nan by comparing it to itself gets the
// wrong answer silently, which is what isnan exists to prevent.
package math

import (
	"errors"
	"fmt"
	"math"
	"math/big"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/plugin"
)

var (
	// ErrNumber is returned for an argument that is neither a float nor an
	// int.
	ErrNumber = errors.New("wants a float or an int")

	// ErrBase is returned for a logarithm in base one, which names no power.
	ErrBase = errors.New("no logarithm has base one")
)

const (
	// NAME is the module.
	NAME = "math"

	// DEGREES is how many a turn has, which the two conversions share.
	DEGREES = 360
)

// init registers this plugin, so that a host importing this package for its
// side effect is the whole of enabling it.
//
// Revisions:
//   - 2026-09-20 00:28: initial creation
func init() {
	plugin.Register(new(_Math))
}

// _Math is the plugin. Empty: every function works on what it is given.
type _Math struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-20 00:28: initial creation
func (m *_Math) Name() string {
	return NAME
}

// Values returns the math module.
//
// Revisions:
//   - 2026-09-20 00:28: initial creation
//   - 2026-09-24 00:53: built here rather than taken from the library, so that
//     the names the library lacks can be among them
func (m *_Math) Values() starlark.StringDict {
	members := starlark.StringDict{
		"e":   starlark.Float(math.E),
		"pi":  starlark.Float(math.Pi),
		"tau": starlark.Float(2 * math.Pi),
		"inf": starlark.Float(math.Inf(1)),
		"nan": starlark.Float(math.NaN()),

		"ceil":  starlark.NewBuiltin("ceil", _Ceil),
		"floor": starlark.NewBuiltin("floor", _Floor),
		"trunc": starlark.NewBuiltin("trunc", _Trunc),
		"log":   starlark.NewBuiltin("log", _Log),
		"gcd":   starlark.NewBuiltin("gcd", _GCD),
		"isnan": _Asks("isnan", math.IsNaN),
		"isinf": _Asks("isinf", func(x float64) bool { return math.IsInf(x, 0) }),
	}

	for name, fn := range map[string]func(float64) float64{
		"fabs":    math.Abs,
		"round":   math.Round,
		"exp":     math.Exp,
		"sqrt":    math.Sqrt,
		"acos":    math.Acos,
		"asin":    math.Asin,
		"atan":    math.Atan,
		"cos":     math.Cos,
		"sin":     math.Sin,
		"tan":     math.Tan,
		"acosh":   math.Acosh,
		"asinh":   math.Asinh,
		"atanh":   math.Atanh,
		"cosh":    math.Cosh,
		"sinh":    math.Sinh,
		"tanh":    math.Tanh,
		"gamma":   math.Gamma,
		"log2":    math.Log2,
		"log10":   math.Log10,
		"degrees": _Degrees,
		"radians": _Radians,
	} {
		members[name] = _Unary(name, fn)
	}

	for name, fn := range map[string]func(float64, float64) float64{
		"copysign":  math.Copysign,
		"mod":       math.Mod,
		"pow":       math.Pow,
		"remainder": math.Remainder,
		"atan2":     math.Atan2,
		"hypot":     math.Hypot,
	} {
		members[name] = _Binary(name, fn)
	}

	return starlark.StringDict{
		NAME: &starlarkstruct.Module{Name: NAME, Members: members},
	}
}

// _Unary wraps a one-argument function, which answers in floats whatever it
// was given.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Unary(name string, fn func(float64) float64) *starlark.Builtin {
	return starlark.NewBuiltin(name, func(
		thread *starlark.Thread,
		held *starlark.Builtin,
		args starlark.Tuple,
		kwargs []starlark.Tuple,
	) (starlark.Value, error) {
		x, err := _One(name, args, kwargs)
		if err != nil {
			return nil, err
		}

		return starlark.Float(fn(x)), nil
	})
}

// _Binary wraps a two-argument function, which answers in floats.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Binary(name string, fn func(float64, float64) float64) *starlark.Builtin {
	return starlark.NewBuiltin(name, func(
		thread *starlark.Thread,
		held *starlark.Builtin,
		args starlark.Tuple,
		kwargs []starlark.Tuple,
	) (starlark.Value, error) {
		var first, second starlark.Value

		err := starlark.UnpackPositionalArgs(name, args, kwargs, 2, &first, &second)
		if err != nil {
			return nil, err
		}

		x, err := _Float(name, first)
		if err != nil {
			return nil, err
		}

		y, err := _Float(name, second)
		if err != nil {
			return nil, err
		}

		return starlark.Float(fn(x, y)), nil
	})
}

// _Asks wraps a question about a float, which answers in bools.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Asks(name string, fn func(float64) bool) *starlark.Builtin {
	return starlark.NewBuiltin(name, func(
		thread *starlark.Thread,
		held *starlark.Builtin,
		args starlark.Tuple,
		kwargs []starlark.Tuple,
	) (starlark.Value, error) {
		x, err := _One(name, args, kwargs)
		if err != nil {
			return nil, err
		}

		return starlark.Bool(fn(x)), nil
	})
}

// _Ceil is the smallest integer at or above x.
//
// An integer, as the library's own is, and as Python's is. An int given back
// unchanged rather than sent through a float, because a large one would not
// survive the trip.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Ceil(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	return _Whole(fn.Name(), args, kwargs, math.Ceil)
}

// _Floor is the largest integer at or below x.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Floor(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	return _Whole(fn.Name(), args, kwargs, math.Floor)
}

// _Trunc is x with its fraction dropped, which rounds towards zero.
//
// An integer, like ceil and floor, because it belongs to that family: the
// three differ in which way they go and in nothing else.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Trunc(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	return _Whole(fn.Name(), args, kwargs, math.Trunc)
}

// _Whole answers in integers, which is what the three rounding directions do.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Whole(
	who string,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
	fn func(float64) float64,
) (starlark.Value, error) {
	var given starlark.Value

	err := starlark.UnpackPositionalArgs(who, args, kwargs, 1, &given)
	if err != nil {
		return nil, err
	}

	switch held := given.(type) {
	case starlark.Int:
		return held, nil

	case starlark.Float:
		return starlark.NumberToInt(starlark.Float(fn(float64(held))))
	}

	return nil, fmt.Errorf("%s got %s: %w", who, given.Type(), ErrNumber)
}

// _Log is the logarithm of x, in base e unless a second argument says
// otherwise.
//
// Returns ErrBase for base one, which names no power of anything.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Log(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var (
		given starlark.Value
		named starlark.Value
	)

	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 1, &given, &named)
	if err != nil {
		return nil, err
	}

	x, err := _Float(fn.Name(), given)
	if err != nil {
		return nil, err
	}

	base := math.E

	if named != nil {
		base, err = _Float(fn.Name(), named)
		if err != nil {
			return nil, err
		}
	}

	if base == 1 {
		return nil, fmt.Errorf("%s: %w", fn.Name(), ErrBase)
	}

	return starlark.Float(math.Log(x) / math.Log(base)), nil
}

// _GCD is the greatest common divisor of two integers.
//
// Integers only, and an integer back. A divisor of two floats is a question
// with no answer, and taking floats here would make one up.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _GCD(
	thread *starlark.Thread,
	fn *starlark.Builtin,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var first, second starlark.Int

	err := starlark.UnpackPositionalArgs(fn.Name(), args, kwargs, 2, &first, &second)
	if err != nil {
		return nil, err
	}

	held := new(big.Int).GCD(nil, nil, new(big.Int).Abs(first.BigInt()), new(big.Int).Abs(second.BigInt()))

	return starlark.MakeBigInt(held), nil
}

// _One reads a builtin's single numeric argument.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _One(who string, args starlark.Tuple, kwargs []starlark.Tuple) (float64, error) {
	var given starlark.Value

	err := starlark.UnpackPositionalArgs(who, args, kwargs, 1, &given)
	if err != nil {
		return 0, err
	}

	return _Float(who, given)
}

// _Float is one value as a float, accepting an int as every one of these does.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Float(who string, given starlark.Value) (float64, error) {
	switch held := given.(type) {
	case starlark.Float:
		return float64(held), nil

	case starlark.Int:
		return float64(held.Float()), nil
	}

	return 0, fmt.Errorf("%s got %s: %w", who, given.Type(), ErrNumber)
}

// _Degrees is a turn in radians as one in degrees.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Degrees(x float64) float64 {
	return DEGREES * x / (2 * math.Pi)
}

// _Radians is a turn in degrees as one in radians.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Radians(x float64) float64 {
	return 2 * math.Pi * x / DEGREES
}
