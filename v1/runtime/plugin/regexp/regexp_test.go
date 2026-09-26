package regexp_test

import (
	"errors"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/dialect"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	larkregexp "github.com/thebagchi/lark/v1/runtime/plugin/regexp"
)

// SCRIPT is what a failure calls the expression it was given.
const SCRIPT = "regexp_test.star"

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

// TestMatch_IsAnchoredAndSearchIsNot is the difference between the two, which
// is the whole reason there are two.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func TestMatch_IsAnchoredAndSearchIsNot(t *testing.T) {
	_Check(t, []struct{ name, expression, want string }{
		{"match at the start", `regexp.match(r"\w+", "bob@corp").text`, `"bob"`},
		{"match not at the start", `regexp.match(r"corp", "bob@corp")`, "None"},
		{"search finds it anywhere", `regexp.search(r"corp", "bob@corp").text`, `"corp"`},
		{"search finds nothing", `regexp.search(r"zzz", "bob@corp")`, "None"},
	})
}

// TestMatch_CarriesItsPartsAndPositions is the shape a match hands back.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func TestMatch_CarriesItsPartsAndPositions(t *testing.T) {
	const FOUND = `regexp.search(r"(?P<user>\w+)@(\w+)", "to bob@corp now")`

	_Check(t, []struct{ name, expression, want string }{
		{"text", FOUND + ".text", `"bob@corp"`},
		{"start", FOUND + ".start", "3"},
		{"end", FOUND + ".end", "11"},
		{"groups", FOUND + ".groups", `["bob", "corp"]`},
		{"named", FOUND + `.named["user"]`, `"bob"`},
		{"unnamed groups are not in named", FOUND + ".named", `{"user": "bob"}`},
		{
			"a group that took part in nothing is None",
			`regexp.search(r"(a)|(b)", "a").groups`,
			`["a", None]`,
		},
		{
			"positions slice the subject back out",
			`regexp.search(r"corp", "bob@corp")` +
				`.start == "bob@corp".index("corp")`,
			"True",
		},
	})
}

// TestFindAll_ReadsLikeSearch records that every match is the same shape.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func TestFindAll_ReadsLikeSearch(t *testing.T) {
	_Check(t, []struct{ name, expression, want string }{
		{
			"every match",
			`[m.text for m in regexp.findall(r"\d+", "a1 b22 c333")]`,
			`["1", "22", "333"]`,
		},
		{
			"with their groups",
			`[m.groups for m in regexp.findall(r"(\w)(\d)", "a1 b2")]`,
			`[["a", "1"], ["b", "2"]]`,
		},
		{"none at all", `regexp.findall(r"z", "abc")`, "[]"},
	})
}

// TestSub_NamesGroupsTheEnginesWay is the replacement syntax, which is Go's
// and not Python's.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func TestSub_NamesGroupsTheEnginesWay(t *testing.T) {
	_Check(t, []struct{ name, expression, want string }{
		{"by number", `regexp.sub(r"(\w+)@(\w+)", "$2/$1", "bob@corp")`, `"corp/bob"`},
		{
			"by name",
			`regexp.sub(r"(?P<u>\w+)@\w+", "${u} at work", "bob@corp")`,
			`"bob at work"`,
		},
		{"a literal dollar", `regexp.sub(r"x", "$$", "x")`, `"$"`},
		{"every one by default", `regexp.sub(r"a", "-", "banana")`, `"b-n-n-"`},
		{"counted", `regexp.sub(r"a", "-", "banana", count = 2)`, `"b-n-na"`},
		{"counted past the end", `regexp.sub(r"a", "-", "banana", count = 99)`, `"b-n-n-"`},
	})
}

// TestSplit_CutsAtEveryMatch records what split does, and what a count means.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func TestSplit_CutsAtEveryMatch(t *testing.T) {
	_Check(t, []struct{ name, expression, want string }{
		{"every cut", `regexp.split(r",\s*", "a, b,c")`, `["a", "b", "c"]`},
		{"counted", `regexp.split(r",", "a,b,c,d", count = 1)`, `["a", "b,c,d"]`},
		{"nothing to cut at", `regexp.split(r";", "abc")`, `["abc"]`},
	})
}

// TestQuote_MakesTextMatchItself is what a script reaches for when the pattern
// came from data.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func TestQuote_MakesTextMatchItself(t *testing.T) {
	_Check(t, []struct{ name, expression, want string }{
		{"escapes", `regexp.quote("a.b*c")`, `"a\\.b\\*c"`},
		{
			"and then matches literally",
			`regexp.search(regexp.quote("a.b"), "xa.by").text`,
			`"a.b"`,
		},
		{
			"where unquoted it would not",
			`regexp.search(regexp.quote("a.b"), "axby")`,
			"None",
		},
	})
}

// TestRegexp_RefusesWhatRE2CannotRead is the cost of the engine, written down:
// the three features that buy backtracking are absent, and a pattern asking
// for one is refused rather than quietly meaning something else.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
func TestRegexp_RefusesWhatRE2CannotRead(t *testing.T) {
	cases := []struct {
		name       string
		expression string
	}{
		{"lookahead", `regexp.search(r"a(?=b)", "ab")`},
		{"lookbehind", `regexp.search(r"(?<=a)b", "ab")`},
		{"a backreference", `regexp.search(r"(a)\1", "aa")`},
		{"an unclosed group", `regexp.search(r"(a", "a")`},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			_, err := _Eval(t, item.expression)
			if !errors.Is(err, larkregexp.ErrPattern) {
				t.Fatalf("%s gave %v, want ErrPattern", item.expression, err)
			}
		})
	}
}
