// These tests check two things: that what the library's module did, this does
// identically, and that what it could not say, this says. The first matters
// more - a script written against the old module must not change behaviour.
package math_test

import (
	"errors"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/runtime/dialect"
	"github.com/thebagchi/lark/runtime/plugin"
	larkmath "github.com/thebagchi/lark/runtime/plugin/math"
)

// SCRIPT is what a failure calls the expression it was given.
const SCRIPT = "math_test.star"

// _Eval evaluates one expression against the default environment.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Eval(t *testing.T, expression string) (string, error) {
	t.Helper()

	env, err := plugin.DEFAULT.Environment()
	if err != nil {
		t.Fatalf("environment: %v", err)
	}

	value, err := starlark.EvalOptions(
		dialect.OPTIONS,
		&starlark.Thread{Name: SCRIPT},
		SCRIPT,
		expression,
		env,
	)
	if err != nil {
		return "", err
	}

	return value.String(), nil
}

// _Check runs a table of expression / expected pairs.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func _Check(t *testing.T, cases []struct{ name, expression, want string }) {
	t.Helper()

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			got, err := _Eval(t, item.expression)
			if err != nil {
				t.Fatalf("%s: %v", item.expression, err)
			}

			if got != item.want {
				t.Fatalf("%s gave %s, want %s", item.expression, got, item.want)
			}
		})
	}
}

// TestMath_KeepsWhatTheLibrarySaid is the half that matters most: every
// behaviour a script could already rely on, including the two that answer in
// integers while everything around them answers in floats.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func TestMath_KeepsWhatTheLibrarySaid(t *testing.T) {
	_Check(t, []struct{ name, expression, want string }{
		{"ceil is an int", `math.ceil(2.1)`, "3"},
		{"floor is an int", `math.floor(2.9)`, "2"},
		{"ceil of an int is that int", `math.ceil(7)`, "7"},
		{"round is a float", `math.round(2.5)`, "3.0"},
		{"sqrt", `math.sqrt(9)`, "3.0"},
		{"pow", `math.pow(2, 10)`, "1024.0"},
		{"fabs", `math.fabs(-2.5)`, "2.5"},
		{"mod", `math.mod(7, 3)`, "1.0"},
		{"hypot", `math.hypot(3, 4)`, "5.0"},
		{"log of e", `math.log(math.e)`, "1.0"},
		{"log in a base", `math.log(8, 2)`, "3.0"},
		{"degrees", `math.degrees(math.pi)`, "180.0"},
		{"radians", `math.round(math.radians(180) * 1000) / 1000`, "3.142"},
		{"e", `math.round(math.e * 1000) / 1000`, "2.718"},
		{"pi", `math.round(math.pi * 1000) / 1000`, "3.142"},
	})
}

// TestMath_SaysWhatTheLibraryCouldNot is the reason this is written here.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func TestMath_SaysWhatTheLibraryCouldNot(t *testing.T) {
	_Check(t, []struct{ name, expression, want string }{
		{"tau", `math.round(math.tau * 1000) / 1000`, "6.283"},
		{"inf", `math.inf > 0`, "True"},
		{"inf is infinite", `math.isinf(math.inf)`, "True"},
		{"negative inf is too", `math.isinf(-math.inf)`, "True"},
		{"a number is not", `math.isinf(1.0)`, "False"},
		{"nan equals itself here", `math.nan == math.nan`, "True"},
		{"and equals another nan", `math.nan == math.inf - math.inf`, "True"},
		{"nan is nan", `math.isnan(math.nan)`, "True"},
		{"a number is not nan", `math.isnan(1.0)`, "False"},
		{"log2", `math.log2(1024)`, "10.0"},
		{"log10", `math.log10(1000)`, "3.0"},
		{"trunc towards zero", `math.trunc(2.9)`, "2"},
		{"trunc of a negative", `math.trunc(-2.9)`, "-2"},
		{"floor of a negative goes down", `math.floor(-2.9)`, "-3"},
		{"gcd", `math.gcd(12, 18)`, "6"},
		{"gcd of negatives", `math.gcd(-12, 18)`, "6"},
		{"gcd with zero", `math.gcd(0, 5)`, "5"},
	})
}

// TestMath_RefusesWhatIsNotANumber records the two mistakes a caller can make.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func TestMath_RefusesWhatIsNotANumber(t *testing.T) {
	cases := []struct {
		name       string
		expression string
		want       error
	}{
		{"a word", `math.sqrt("nine")`, larkmath.ErrNumber},
		{"a word to ceil", `math.ceil("nine")`, larkmath.ErrNumber},
		{"base one", `math.log(8, 1)`, larkmath.ErrBase},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			_, err := _Eval(t, item.expression)
			if !errors.Is(err, item.want) {
				t.Fatalf("%s gave %v, want %v", item.expression, err, item.want)
			}
		})
	}
}
