package tests

import (
	"testing"
)

const (
	// ERR marks an expected error in the table.
	//
	// Revisions:
	//   - 2026-11-02 08:11: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	ERR = "ERR "
)

// TestMath_OneArgumentBuiltinsKeepTheirWords pins every one-argument builtin,
// the float ones and the isnan and isinf questions: the value of a valid call
// and the full text of each refusal (no argument, two, a keyword, a string,
// a bool). The table was captured from the code before _Unary and _Asks
// shared a reader.
//
// Revisions:
//   - 2026-11-02 00:52: initial creation
//   - 2026-11-09 11:41: reads the table ONE_ARGUMENT, not a file (C114, Q282)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestMath_OneArgumentBuiltinsKeepTheirWords(t *testing.T) {
	for _, row := range ONE_ARGUMENT {
		var (
			expression = row.expr
			want       = row.want
		)

		t.Run(
			expression,
			func(t *testing.T) {
				got, err := _Eval(
					t,
					expression,
				)
				if err != nil {
					got = ERR + err.Error()
				}

				if got != want {
					t.Fatalf(
						"%s gave %q, want %q",
						expression,
						got,
						want,
					)
				}
			},
		)
	}
}

// BEGIN GOLDEN: no test writes this table; edit by hand only to review.

// ONE_ARGUMENT lists, for each one-argument builtin, an expression and what it
// gave when the table was recorded: the value, or ERR and the full error text.
// No test writes the table: edit a row by hand only to review it.
//
// Revisions:
//   - 2026-11-02 08:11: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-09 11:41: a Go table, not testdata/one_argument.tsv (C114, Q282)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
var (
	ONE_ARGUMENT = []struct{ expr, want string }{
		{
			`math.acos(4.0)`,
			`nan`,
		},
		{
			`math.acos(4)`,
			`nan`,
		},
		{
			`math.acos()`,
			`ERR acos: got 0 arguments, want 1`,
		},
		{
			`math.acos(1, 2)`,
			`ERR acos: got 2 arguments, want 1`,
		},
		{
			`math.acos("x")`,
			`ERR acos got string: wants a float or an int`,
		},
		{
			`math.acos(x=1)`,
			`ERR acos: unexpected keyword arguments`,
		},
		{
			`math.acos(True)`,
			`ERR acos got bool: wants a float or an int`,
		},
		{
			`math.acosh(4.0)`,
			`2.0634370688955608`,
		},
		{
			`math.acosh(4)`,
			`2.0634370688955608`,
		},
		{
			`math.acosh()`,
			`ERR acosh: got 0 arguments, want 1`,
		},
		{
			`math.acosh(1, 2)`,
			`ERR acosh: got 2 arguments, want 1`,
		},
		{
			`math.acosh("x")`,
			`ERR acosh got string: wants a float or an int`,
		},
		{
			`math.acosh(x=1)`,
			`ERR acosh: unexpected keyword arguments`,
		},
		{
			`math.acosh(True)`,
			`ERR acosh got bool: wants a float or an int`,
		},
		{
			`math.asin(4.0)`,
			`nan`,
		},
		{
			`math.asin(4)`,
			`nan`,
		},
		{
			`math.asin()`,
			`ERR asin: got 0 arguments, want 1`,
		},
		{
			`math.asin(1, 2)`,
			`ERR asin: got 2 arguments, want 1`,
		},
		{
			`math.asin("x")`,
			`ERR asin got string: wants a float or an int`,
		},
		{
			`math.asin(x=1)`,
			`ERR asin: unexpected keyword arguments`,
		},
		{
			`math.asin(True)`,
			`ERR asin got bool: wants a float or an int`,
		},
		{
			`math.asinh(4.0)`,
			`2.0947125472611012`,
		},
		{
			`math.asinh(4)`,
			`2.0947125472611012`,
		},
		{
			`math.asinh()`,
			`ERR asinh: got 0 arguments, want 1`,
		},
		{
			`math.asinh(1, 2)`,
			`ERR asinh: got 2 arguments, want 1`,
		},
		{
			`math.asinh("x")`,
			`ERR asinh got string: wants a float or an int`,
		},
		{
			`math.asinh(x=1)`,
			`ERR asinh: unexpected keyword arguments`,
		},
		{
			`math.asinh(True)`,
			`ERR asinh got bool: wants a float or an int`,
		},
		{
			`math.atan(4.0)`,
			`1.3258176636680323`,
		},
		{
			`math.atan(4)`,
			`1.3258176636680323`,
		},
		{
			`math.atan()`,
			`ERR atan: got 0 arguments, want 1`,
		},
		{
			`math.atan(1, 2)`,
			`ERR atan: got 2 arguments, want 1`,
		},
		{
			`math.atan("x")`,
			`ERR atan got string: wants a float or an int`,
		},
		{
			`math.atan(x=1)`,
			`ERR atan: unexpected keyword arguments`,
		},
		{
			`math.atan(True)`,
			`ERR atan got bool: wants a float or an int`,
		},
		{
			`math.atanh(4.0)`,
			`nan`,
		},
		{
			`math.atanh(4)`,
			`nan`,
		},
		{
			`math.atanh()`,
			`ERR atanh: got 0 arguments, want 1`,
		},
		{
			`math.atanh(1, 2)`,
			`ERR atanh: got 2 arguments, want 1`,
		},
		{
			`math.atanh("x")`,
			`ERR atanh got string: wants a float or an int`,
		},
		{
			`math.atanh(x=1)`,
			`ERR atanh: unexpected keyword arguments`,
		},
		{
			`math.atanh(True)`,
			`ERR atanh got bool: wants a float or an int`,
		},
		{
			`math.cos(4.0)`,
			`-0.6536436208636119`,
		},
		{
			`math.cos(4)`,
			`-0.6536436208636119`,
		},
		{
			`math.cos()`,
			`ERR cos: got 0 arguments, want 1`,
		},
		{
			`math.cos(1, 2)`,
			`ERR cos: got 2 arguments, want 1`,
		},
		{
			`math.cos("x")`,
			`ERR cos got string: wants a float or an int`,
		},
		{
			`math.cos(x=1)`,
			`ERR cos: unexpected keyword arguments`,
		},
		{
			`math.cos(True)`,
			`ERR cos got bool: wants a float or an int`,
		},
		{
			`math.cosh(4.0)`,
			`27.308232836016487`,
		},
		{
			`math.cosh(4)`,
			`27.308232836016487`,
		},
		{
			`math.cosh()`,
			`ERR cosh: got 0 arguments, want 1`,
		},
		{
			`math.cosh(1, 2)`,
			`ERR cosh: got 2 arguments, want 1`,
		},
		{
			`math.cosh("x")`,
			`ERR cosh got string: wants a float or an int`,
		},
		{
			`math.cosh(x=1)`,
			`ERR cosh: unexpected keyword arguments`,
		},
		{
			`math.cosh(True)`,
			`ERR cosh got bool: wants a float or an int`,
		},
		{
			`math.degrees(4.0)`,
			`229.1831180523293`,
		},
		{
			`math.degrees(4)`,
			`229.1831180523293`,
		},
		{
			`math.degrees()`,
			`ERR degrees: got 0 arguments, want 1`,
		},
		{
			`math.degrees(1, 2)`,
			`ERR degrees: got 2 arguments, want 1`,
		},
		{
			`math.degrees("x")`,
			`ERR degrees got string: wants a float or an int`,
		},
		{
			`math.degrees(x=1)`,
			`ERR degrees: unexpected keyword arguments`,
		},
		{
			`math.degrees(True)`,
			`ERR degrees got bool: wants a float or an int`,
		},
		{
			`math.exp(4.0)`,
			`54.598150033144236`,
		},
		{
			`math.exp(4)`,
			`54.598150033144236`,
		},
		{
			`math.exp()`,
			`ERR exp: got 0 arguments, want 1`,
		},
		{
			`math.exp(1, 2)`,
			`ERR exp: got 2 arguments, want 1`,
		},
		{
			`math.exp("x")`,
			`ERR exp got string: wants a float or an int`,
		},
		{
			`math.exp(x=1)`,
			`ERR exp: unexpected keyword arguments`,
		},
		{
			`math.exp(True)`,
			`ERR exp got bool: wants a float or an int`,
		},
		{
			`math.fabs(4.0)`,
			`4.0`,
		},
		{
			`math.fabs(4)`,
			`4.0`,
		},
		{
			`math.fabs()`,
			`ERR fabs: got 0 arguments, want 1`,
		},
		{
			`math.fabs(1, 2)`,
			`ERR fabs: got 2 arguments, want 1`,
		},
		{
			`math.fabs("x")`,
			`ERR fabs got string: wants a float or an int`,
		},
		{
			`math.fabs(x=1)`,
			`ERR fabs: unexpected keyword arguments`,
		},
		{
			`math.fabs(True)`,
			`ERR fabs got bool: wants a float or an int`,
		},
		{
			`math.gamma(4.0)`,
			`6.0`,
		},
		{
			`math.gamma(4)`,
			`6.0`,
		},
		{
			`math.gamma()`,
			`ERR gamma: got 0 arguments, want 1`,
		},
		{
			`math.gamma(1, 2)`,
			`ERR gamma: got 2 arguments, want 1`,
		},
		{
			`math.gamma("x")`,
			`ERR gamma got string: wants a float or an int`,
		},
		{
			`math.gamma(x=1)`,
			`ERR gamma: unexpected keyword arguments`,
		},
		{
			`math.gamma(True)`,
			`ERR gamma got bool: wants a float or an int`,
		},
		{
			`math.isinf(4.0)`,
			`False`,
		},
		{
			`math.isinf(4)`,
			`False`,
		},
		{
			`math.isinf()`,
			`ERR isinf: got 0 arguments, want 1`,
		},
		{
			`math.isinf(1, 2)`,
			`ERR isinf: got 2 arguments, want 1`,
		},
		{
			`math.isinf("x")`,
			`ERR isinf got string: wants a float or an int`,
		},
		{
			`math.isinf(x=1)`,
			`ERR isinf: unexpected keyword arguments`,
		},
		{
			`math.isinf(True)`,
			`ERR isinf got bool: wants a float or an int`,
		},
		{
			`math.isnan(4.0)`,
			`False`,
		},
		{
			`math.isnan(4)`,
			`False`,
		},
		{
			`math.isnan()`,
			`ERR isnan: got 0 arguments, want 1`,
		},
		{
			`math.isnan(1, 2)`,
			`ERR isnan: got 2 arguments, want 1`,
		},
		{
			`math.isnan("x")`,
			`ERR isnan got string: wants a float or an int`,
		},
		{
			`math.isnan(x=1)`,
			`ERR isnan: unexpected keyword arguments`,
		},
		{
			`math.isnan(True)`,
			`ERR isnan got bool: wants a float or an int`,
		},
		{
			`math.log10(4.0)`,
			`0.6020599913279624`,
		},
		{
			`math.log10(4)`,
			`0.6020599913279624`,
		},
		{
			`math.log10()`,
			`ERR log10: got 0 arguments, want 1`,
		},
		{
			`math.log10(1, 2)`,
			`ERR log10: got 2 arguments, want 1`,
		},
		{
			`math.log10("x")`,
			`ERR log10 got string: wants a float or an int`,
		},
		{
			`math.log10(x=1)`,
			`ERR log10: unexpected keyword arguments`,
		},
		{
			`math.log10(True)`,
			`ERR log10 got bool: wants a float or an int`,
		},
		{
			`math.log2(4.0)`,
			`2.0`,
		},
		{
			`math.log2(4)`,
			`2.0`,
		},
		{
			`math.log2()`,
			`ERR log2: got 0 arguments, want 1`,
		},
		{
			`math.log2(1, 2)`,
			`ERR log2: got 2 arguments, want 1`,
		},
		{
			`math.log2("x")`,
			`ERR log2 got string: wants a float or an int`,
		},
		{
			`math.log2(x=1)`,
			`ERR log2: unexpected keyword arguments`,
		},
		{
			`math.log2(True)`,
			`ERR log2 got bool: wants a float or an int`,
		},
		{
			`math.radians(4.0)`,
			`0.06981317007977318`,
		},
		{
			`math.radians(4)`,
			`0.06981317007977318`,
		},
		{
			`math.radians()`,
			`ERR radians: got 0 arguments, want 1`,
		},
		{
			`math.radians(1, 2)`,
			`ERR radians: got 2 arguments, want 1`,
		},
		{
			`math.radians("x")`,
			`ERR radians got string: wants a float or an int`,
		},
		{
			`math.radians(x=1)`,
			`ERR radians: unexpected keyword arguments`,
		},
		{
			`math.radians(True)`,
			`ERR radians got bool: wants a float or an int`,
		},
		{
			`math.round(4.0)`,
			`4.0`,
		},
		{
			`math.round(4)`,
			`4.0`,
		},
		{
			`math.round()`,
			`ERR round: got 0 arguments, want 1`,
		},
		{
			`math.round(1, 2)`,
			`ERR round: got 2 arguments, want 1`,
		},
		{
			`math.round("x")`,
			`ERR round got string: wants a float or an int`,
		},
		{
			`math.round(x=1)`,
			`ERR round: unexpected keyword arguments`,
		},
		{
			`math.round(True)`,
			`ERR round got bool: wants a float or an int`,
		},
		{
			`math.sin(4.0)`,
			`-0.7568024953079282`,
		},
		{
			`math.sin(4)`,
			`-0.7568024953079282`,
		},
		{
			`math.sin()`,
			`ERR sin: got 0 arguments, want 1`,
		},
		{
			`math.sin(1, 2)`,
			`ERR sin: got 2 arguments, want 1`,
		},
		{
			`math.sin("x")`,
			`ERR sin got string: wants a float or an int`,
		},
		{
			`math.sin(x=1)`,
			`ERR sin: unexpected keyword arguments`,
		},
		{
			`math.sin(True)`,
			`ERR sin got bool: wants a float or an int`,
		},
		{
			`math.sinh(4.0)`,
			`27.28991719712775`,
		},
		{
			`math.sinh(4)`,
			`27.28991719712775`,
		},
		{
			`math.sinh()`,
			`ERR sinh: got 0 arguments, want 1`,
		},
		{
			`math.sinh(1, 2)`,
			`ERR sinh: got 2 arguments, want 1`,
		},
		{
			`math.sinh("x")`,
			`ERR sinh got string: wants a float or an int`,
		},
		{
			`math.sinh(x=1)`,
			`ERR sinh: unexpected keyword arguments`,
		},
		{
			`math.sinh(True)`,
			`ERR sinh got bool: wants a float or an int`,
		},
		{
			`math.sqrt(4.0)`,
			`2.0`,
		},
		{
			`math.sqrt(4)`,
			`2.0`,
		},
		{
			`math.sqrt()`,
			`ERR sqrt: got 0 arguments, want 1`,
		},
		{
			`math.sqrt(1, 2)`,
			`ERR sqrt: got 2 arguments, want 1`,
		},
		{
			`math.sqrt("x")`,
			`ERR sqrt got string: wants a float or an int`,
		},
		{
			`math.sqrt(x=1)`,
			`ERR sqrt: unexpected keyword arguments`,
		},
		{
			`math.sqrt(True)`,
			`ERR sqrt got bool: wants a float or an int`,
		},
		{
			`math.tan(4.0)`,
			`1.1578212823495775`,
		},
		{
			`math.tan(4)`,
			`1.1578212823495775`,
		},
		{
			`math.tan()`,
			`ERR tan: got 0 arguments, want 1`,
		},
		{
			`math.tan(1, 2)`,
			`ERR tan: got 2 arguments, want 1`,
		},
		{
			`math.tan("x")`,
			`ERR tan got string: wants a float or an int`,
		},
		{
			`math.tan(x=1)`,
			`ERR tan: unexpected keyword arguments`,
		},
		{
			`math.tan(True)`,
			`ERR tan got bool: wants a float or an int`,
		},
		{
			`math.tanh(4.0)`,
			`0.999329299739067`,
		},
		{
			`math.tanh(4)`,
			`0.999329299739067`,
		},
		{
			`math.tanh()`,
			`ERR tanh: got 0 arguments, want 1`,
		},
		{
			`math.tanh(1, 2)`,
			`ERR tanh: got 2 arguments, want 1`,
		},
		{
			`math.tanh("x")`,
			`ERR tanh got string: wants a float or an int`,
		},
		{
			`math.tanh(x=1)`,
			`ERR tanh: unexpected keyword arguments`,
		},
		{
			`math.tanh(True)`,
			`ERR tanh got bool: wants a float or an int`,
		},
	}
)

// END GOLDEN
