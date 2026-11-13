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
//
// Revisions:
//   - 2026-09-20 00:24: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
package math

import (
	"math"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"

	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/math/internal/number"
	"github.com/thebagchi/lark/v1/runtime/plugin/math/internal/spec"
)

var (
	// ErrNumber is number.ErrNumber.
	//
	// Revisions:
	//   - 2026-11-10 01:37: initial creation (C120b, Q319)
	ErrNumber = number.ErrNumber

	// ErrBase is number.ErrBase.
	//
	// Revisions:
	//   - 2026-11-10 01:37: initial creation (C120b, Q319)
	ErrBase = number.ErrBase
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
//
// Revisions:
//   - 2026-09-20 00:24: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
type _Math struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-20 00:28: initial creation
//   - 2026-11-08 00:27: names read from internal/spec (C91w12a)
func (m *_Math) Name() string {
	return spec.NAME
}

// Values returns the math module.
//
// Revisions:
//   - 2026-09-20 00:28: initial creation
//   - 2026-09-24 00:53: built here rather than taken from the library, so that
//     the names the library lacks can be among them
//   - 2026-11-08 00:27: names read from internal/spec (C91w12a)
func (m *_Math) Values() starlark.StringDict {
	members := _MathConstants()

	for name, fn := range _UnaryFns() {
		members[name] = _Unary(
			name,
			fn,
		)
	}

	for name, fn := range _BinaryFns() {
		members[name] = _Binary(
			name,
			fn,
		)
	}

	return starlark.StringDict{
		spec.NAME: &starlarkstruct.Module{
			Name:    spec.NAME,
			Members: members,
		},
	}
}

// _MathConstants is the numbers and the names that are not a plain wrap.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-08 00:27: names read from internal/spec (C91w12a)
//   - 2026-11-10 11:32: uses number.Ceil, number.Floor, number.Trunc, number.Log and
//     number.GCD, which moved to internal/number (C120b, Q315)
func _MathConstants() starlark.StringDict {
	return starlark.StringDict{
		"e":   starlark.Float(math.E),
		"pi":  starlark.Float(math.Pi),
		"tau": starlark.Float(spec.TURN),
		"inf": starlark.Float(math.Inf(1)),
		"nan": starlark.Float(math.NaN()),
		"ceil": starlark.NewBuiltin(
			"ceil",
			number.Ceil,
		),
		"floor": starlark.NewBuiltin(
			"floor",
			number.Floor,
		),
		"trunc": starlark.NewBuiltin(
			"trunc",
			number.Trunc,
		),
		"log": starlark.NewBuiltin(
			"log",
			number.Log,
		),
		"gcd": starlark.NewBuiltin(
			"gcd",
			number.GCD,
		),
		"isnan": _Asks(
			"isnan",
			math.IsNaN,
		),
		"isinf": _Asks(
			"isinf",
			_Inf,
		),
	}
}

// _Inf reports whether x is an infinity.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
func _Inf(x float64) bool {
	return math.IsInf(
		x,
		0,
	)
}

// _UnaryFns is each one-argument function the module publishes.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
func _UnaryFns() map[string]func(float64) float64 {
	return map[string]func(float64) float64{
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
	}
}

// _BinaryFns is each two-argument function the module publishes.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
func _BinaryFns() map[string]func(
	float64,
	float64,
) float64 {
	return map[string]func(
		float64,
		float64,
	) float64{
		"copysign":  math.Copysign,
		"mod":       math.Mod,
		"pow":       math.Pow,
		"remainder": math.Remainder,
		"atan2":     math.Atan2,
		"hypot":     math.Hypot,
	}
}

// _Unary wraps a one-argument function, which answers in floats whatever it
// was given.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-02 00:52: leaves the reading of the argument to _Reading
func _Unary(
	name string,
	fn func(float64) float64,
) *starlark.Builtin {
	return _Reading(
		name,
		func(x float64) starlark.Value {
			return starlark.Float(fn(x))
		},
	)
}

// _Binary wraps a two-argument function, which answers in floats.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Binary(
	name string,
	fn func(
		float64,
		float64,
	) float64,
) *starlark.Builtin {
	return starlark.NewBuiltin(
		name,
		func(
			thread *starlark.Thread,
			held *starlark.Builtin,
			args starlark.Tuple,
			kwargs []starlark.Tuple,
		) (starlark.Value, error) {
			return _Pair(
				name,
				fn,
				args,
				kwargs,
			)
		},
	)
}

// _Pair applies fn to the two numbers a call was given.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-10 11:32: uses number.Float, which moved to internal/number (C120b, Q315)
func _Pair(
	name string,
	fn func(
		float64,
		float64,
	) float64,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (starlark.Value, error) {
	var first, second starlark.Value

	const NEEDED = 2

	err := starlark.UnpackPositionalArgs(
		name,
		args,
		kwargs,
		NEEDED,
		&first,
		&second,
	)
	if err != nil {
		return nil, err
	}

	x, err := number.Float(
		name,
		first,
	)
	if err != nil {
		return nil, err
	}

	y, err := number.Float(
		name,
		second,
	)
	if err != nil {
		return nil, err
	}

	return starlark.Float(fn(
		x,
		y,
	)), nil
}

// _Asks wraps a question about a float, which answers in bools.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-02 00:52: leaves the reading of the argument to _Reading
func _Asks(
	name string,
	fn func(float64) bool,
) *starlark.Builtin {
	return _Reading(
		name,
		func(x float64) starlark.Value {
			return starlark.Bool(fn(x))
		},
	)
}

// _Reading wraps a function of one number as a builtin. It reads the argument
// with _One, so every such builtin refuses a bad call in the same words, and
// lets answer say what the number gives. A plain function with a closure is
// enough: a generic over the result type would still need a wrapping step.
//
// Revisions:
//   - 2026-11-02 00:52: initial creation, the builtin that _Unary and _Asks shared
func _Reading(
	name string,
	answer func(float64) starlark.Value,
) *starlark.Builtin {
	return starlark.NewBuiltin(
		name,
		func(
			thread *starlark.Thread,
			held *starlark.Builtin,
			args starlark.Tuple,
			kwargs []starlark.Tuple,
		) (starlark.Value, error) {
			x, err := _One(
				name,
				args,
				kwargs,
			)
			if err != nil {
				return nil, err
			}

			return answer(x), nil
		},
	)
}

// _One reads a builtin's single numeric argument.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-10 11:32: uses number.Float, which moved to internal/number (C120b, Q315)
func _One(
	who string,
	args starlark.Tuple,
	kwargs []starlark.Tuple,
) (float64, error) {
	var given starlark.Value

	err := starlark.UnpackPositionalArgs(
		who,
		args,
		kwargs,
		1,
		&given,
	)
	if err != nil {
		return 0, err
	}

	return number.Float(
		who,
		given,
	)
}

// _Degrees is a turn in radians as one in degrees.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-08 00:27: names read from internal/spec (C91w12a)
func _Degrees(x float64) float64 {
	return spec.DEGREES * x / spec.TURN
}

// _Radians is a turn in degrees as one in radians.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-08 00:27: names read from internal/spec (C91w12a)
func _Radians(x float64) float64 {
	return spec.TURN * x / spec.DEGREES
}
