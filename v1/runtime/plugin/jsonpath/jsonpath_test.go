// These tests are table driven because the thing under test is a
// specification: RFC 6901 for pointers, RFC 6902 for patch. A case per rule is
// what makes it possible to see which rule is missing.
package jsonpath_test

import (
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/dialect"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/jsonpath"
)

const SCRIPT = "jsonpath_test.star"

// _Eval runs one Starlark expression against the plugin environment and
// returns what it produced, as its String form.
//
// Revisions:
//   - 2026-09-20 01:23: initial creation
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
//   - 2026-09-20 01:24: initial creation
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

// TestPointer_ResolvesRFC6901 covers the pointer rules, including the two that
// are easy to get wrong: escaping order, and what the empty pointer means.
//
// Revisions:
//   - 2026-09-20 01:25: initial creation
func TestPointer_ResolvesRFC6901(t *testing.T) {
	const doc = `{"a": {"b": [10, 20]}, "e/f": 1, "g~h": 2, "": 3}`

	_Check(t, []struct{ name, expression, want string }{
		{
			"whole document",
			`extract_json(` + doc + `, "")`,
			`{"a": {"b": [10, 20]}, "e/f": 1, "g~h": 2, "": 3}`,
		},
		{"member", `extract_json(` + doc + `, "/a")`, `{"b": [10, 20]}`},
		{"nested member", `extract_json(` + doc + `, "/a/b")`, `[10, 20]`},
		{"list index", `extract_json(` + doc + `, "/a/b/1")`, `20`},
		{"escaped slash", `extract_json(` + doc + `, "/e~1f")`, `1`},
		{"escaped tilde", `extract_json(` + doc + `, "/g~0h")`, `2`},
		{"empty key", `extract_json(` + doc + `, "/")`, `3`},
		{"missing member", `extract_json(` + doc + `, "/nope")`, `None`},
		{"index past the end", `extract_json(` + doc + `, "/a/b/9")`, `None`},
	})
}

// TestPointer_RefusesWhatIsNotAPointer proves a malformed pointer is the
// script being wrong, and is an error rather than a None.
//
// Revisions:
//   - 2026-09-20 01:26: initial creation
//   - 2026-09-21 08:09: a signed index is not one either
func TestPointer_RefusesWhatIsNotAPointer(t *testing.T) {
	for _, pointer := range []string{`"a/b"`, `"/a/01"`, `"/a/"`, `"/a/+0"`, `"/a/-1"`} {
		_, err := _Eval(t, `extract_json({"a": [1]}, `+pointer+`)`)
		if err == nil {
			t.Fatalf("%s was accepted as a pointer", pointer)
		}
	}
}

// TestPatch_AppliesRFC6902 covers each operation and the rules that separate
// them - add inserting where replace requires, and "-" appending.
//
// Revisions:
//   - 2026-09-20 01:27: initial creation
//   - 2026-09-21 08:09: a move onto itself, which the specification allows
func TestPatch_AppliesRFC6902(t *testing.T) {
	_Check(t, []struct{ name, expression, want string }{
		{
			"add a member",
			`patch_json({"a": 1}, [{"op": "add", "path": "/b", "value": 2}])`,
			`{"a": 1, "b": 2}`,
		},
		{
			"add inserts into a list",
			`patch_json({"a": [1, 3]}, [{"op": "add", "path": "/a/1", "value": 2}])`,
			`{"a": [1, 2, 3]}`,
		},
		{
			"add appends with a dash",
			`patch_json({"a": [1]}, [{"op": "add", "path": "/a/-", "value": 2}])`,
			`{"a": [1, 2]}`,
		},
		{
			"replace a member",
			`patch_json({"a": 1}, [{"op": "replace", "path": "/a", "value": 2}])`,
			`{"a": 2}`,
		},
		{
			"remove a member",
			`patch_json({"a": 1, "b": 2}, [{"op": "remove", "path": "/b"}])`,
			`{"a": 1}`,
		},
		{
			"remove an element",
			`patch_json({"a": [1, 2, 3]}, [{"op": "remove", "path": "/a/1"}])`,
			`{"a": [1, 3]}`,
		},
		{
			"move",
			`patch_json({"a": 1}, [{"op": "move", "from": "/a", "path": "/b"}])`,
			`{"b": 1}`,
		},
		{
			"copy",
			`patch_json({"a": 1}, [{"op": "copy", "from": "/a", "path": "/b"}])`,
			`{"a": 1, "b": 1}`,
		},
		{
			"test that passes leaves the document",
			`patch_json({"a": 1}, [{"op": "test", "path": "/a", "value": 1}])`,
			`{"a": 1}`,
		},
		{
			"move onto itself changes nothing",
			`patch_json({"a": {"b": 1}}, [{"op": "move", "from": "/a", "path": "/a"}])`,
			`{"a": {"b": 1}}`,
		},
		{
			"operations apply in order",
			`patch_json({"a": 1}, [{"op": "add", "path": "/b", "value": 2}, {"op": "remove", "path": "/a"}])`,
			`{"b": 2}`,
		},
	})
}

// TestPatch_RefusesWhatTheSpecificationRefuses covers the error cases §10.7
// names one by one, because each is a rule somebody could leave out and never
// notice.
//
// Revisions:
//   - 2026-09-20 01:29: initial creation
func TestPatch_RefusesWhatTheSpecificationRefuses(t *testing.T) {
	cases := []struct{ name, expression, carries string }{
		{
			"unknown op",
			`patch_json({}, [{"op": "invent", "path": "/a"}])`,
			"unknown patch operation",
		},
		{"missing path", `patch_json({}, [{"op": "remove"}])`, "missing a field"},
		{"missing value", `patch_json({}, [{"op": "add", "path": "/a"}])`, "missing a field"},
		{"remove what is not there", `patch_json({}, [{"op": "remove", "path": "/a"}])`, "no such path"},
		{
			"replace what is not there",
			`patch_json({}, [{"op": "replace", "path": "/a", "value": 1}])`,
			"no such path",
		},
		{
			"test that fails",
			`patch_json({"a": 1}, [{"op": "test", "path": "/a", "value": 2}])`,
			"test failed",
		},
		{
			"move into its own child",
			`patch_json({"a": {"b": 1}}, [{"op": "move", "from": "/a", "path": "/a/b"}])`,
			"into itself",
		},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			_, err := _Eval(t, item.expression)
			if err == nil {
				t.Fatalf("%s was accepted", item.expression)
			}

			if !strings.Contains(err.Error(), item.carries) {
				t.Fatalf("%s failed with %v, want it to mention %q",
					item.expression, err, item.carries)
			}
		})
	}
}

// TestPatch_LeavesTheInputAlone proves the document is never mutated, which is
// what in_place=False means and what makes it safe to patch something another
// thread can already see.
//
// Revisions:
//   - 2026-09-20 01:31: initial creation
func TestPatch_LeavesTheInputAlone(t *testing.T) {
	const source = `
before = {"a": [1, 2]}
after = patch_json(before, [{"op": "add", "path": "/a/-", "value": 3}])
`

	env, err := plugin.DEFAULT.Environment()
	if err != nil {
		t.Fatalf("environment: %v", err)
	}

	globals, err := starlark.ExecFileOptions(
		dialect.OPTIONS,
		&starlark.Thread{Name: SCRIPT},
		SCRIPT,
		[]byte(source),
		env,
	)
	if err != nil {
		t.Fatalf("execute: %v", err)
	}

	if globals["before"].String() != `{"a": [1, 2]}` {
		t.Fatalf("the input was changed: %s", globals["before"].String())
	}

	if globals["after"].String() != `{"a": [1, 2, 3]}` {
		t.Fatalf("the result is %s", globals["after"].String())
	}
}

// TestQueries_AnswerQuestionsRatherThanFail covers the four rules that
// separate "this document does not have that" from "you asked wrongly".
//
// Revisions:
//   - 2026-09-20 01:32: initial creation
func TestQueries_AnswerQuestionsRatherThanFail(t *testing.T) {
	const doc = `{"a": {"b": [1, 2, 3]}, "deep": {"nested": {"id": "found"}}, "text": "abcd"}`

	_Check(t, []struct{ name, expression, want string }{
		{"match", `match_json(` + doc + `, "/a/b/0", 1)`, `True`},
		{"match is false, not an error, when missing", `match_json(` + doc + `, "/nope", 1)`, `False`},
		{"match compares 1 and 1.0 as equal", `match_json(` + doc + `, "/a/b/0", 1.0)`, `True`},
		{"length of a list", `len_json(` + doc + `, "/a/b")`, `3`},
		{"length of a string", `len_json(` + doc + `, "/text")`, `4`},
		{"length of a dict", `len_json(` + doc + `, "/a")`, `1`},
		{"length of what is missing is zero", `len_json(` + doc + `, "/nope")`, `0`},
		{"find a key at depth", `find_key(` + doc + `, "id")`, `"found"`},
		{"find what is not there", `find_key(` + doc + `, "absent")`, `None`},
	})
}

// _Global runs a script and returns one of the names it bound.
//
// Eval takes an expression, and showing that a document is untouched needs
// three statements: patch it, change what came back, then look at the
// original.
//
// Revisions:
//   - 2026-09-24 16:15: initial creation
func _Global(t *testing.T, script string, name string) (string, error) {
	t.Helper()

	env, err := plugin.DEFAULT.Environment()
	if err != nil {
		t.Fatalf("environment: %v", err)
	}

	globals, err := starlark.ExecFileOptions(
		dialect.OPTIONS,
		&starlark.Thread{Name: SCRIPT},
		SCRIPT,
		script,
		env,
	)
	if err != nil {
		return "", err
	}

	return globals[name].String(), nil
}

// TestPatch_ACopyGetsItsOwnContainers is the promise on _Patch that the
// document handed in is never modified.
//
// Copy walked to the value and inserted that same value. The containers along
// the path were copied; the value was not - so one list was in two places, and
// appending to what was now at the target changed what was at the source.
//
// Move may keep the value it took, because the value left the old path. Only
// copy needs its own.
//
// Revisions:
//   - 2026-09-24 16:15: initial creation
func TestPatch_ACopyGetsItsOwnContainers(t *testing.T) {
	const FLAT = `
doc = {"a": [1]}
out = patch_json(doc, [{"op": "copy", "from": "/a", "path": "/b"}])
out["b"].append(2)
`

	const NESTED = `
doc = {"a": {"x": [1]}}
out = patch_json(doc, [{"op": "copy", "from": "/a", "path": "/b"}])
out["b"]["x"].append(2)
`

	cases := []struct{ name, script, read, want string }{
		{"the original is untouched", FLAT, "doc", `{"a": [1]}`},
		{"and the copy is its own list", FLAT, "out", `{"a": [1], "b": [1, 2]}`},
		{"a nested container is copied too", NESTED, "doc", `{"a": {"x": [1]}}`},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			got, err := _Global(t, item.script, item.read)
			if err != nil {
				t.Fatalf("%s: %v", item.read, err)
			}

			if got != item.want {
				t.Fatalf("%s was %s, want %s", item.read, got, item.want)
			}
		})
	}
}

// TestPatch_AMoveKeepsTheValueItTook records the other half, so that fixing
// copy is not read as a rule about both.
//
// Revisions:
//   - 2026-09-24 16:15: initial creation
func TestPatch_AMoveKeepsTheValueItTook(t *testing.T) {
	got, err := _Eval(t, `patch_json({"a": [1]}, [{"op": "move", "from": "/a", "path": "/b"}])`)
	if err != nil {
		t.Fatal(err)
	}

	if got != `{"b": [1]}` {
		t.Fatalf("got %s, want the value moved", got)
	}
}

// TestPatch_AWrittenValueGetsItsOwnContainers is the other half of the rule
// copy now follows.
//
// _Insert already copies every container along the path, so the document's
// structure was never shared - only the leaf a caller supplied in the patch.
// A caller who passed a value they still hold, part of the document included,
// could change what came back and change the input with it, against the
// promise on _Patch.
//
// Revisions:
//   - 2026-09-24 16:23: initial creation
func TestPatch_AWrittenValueGetsItsOwnContainers(t *testing.T) {
	const ADDED = `
doc = {"a": [1]}
out = patch_json(doc, [{"op": "add", "path": "/b", "value": doc["a"]}])
out["b"].append(2)
`

	const REPLACED = `
doc = {"a": [1], "b": [9]}
out = patch_json(doc, [{"op": "replace", "path": "/b", "value": doc["a"]}])
out["b"].append(2)
`

	const OWNED = `
mine = [1]
doc = {}
out = patch_json(doc, [{"op": "add", "path": "/b", "value": mine}])
out["b"].append(2)
`

	cases := []struct{ name, script, read, want string }{
		{"add leaves the document alone", ADDED, "doc", `{"a": [1]}`},
		{"and the addition is its own list", ADDED, "out", `{"a": [1], "b": [1, 2]}`},
		{"replace leaves it alone too", REPLACED, "doc", `{"a": [1], "b": [9]}`},
		{"a value the caller still holds is not theirs to be changed", OWNED, "mine", `[1]`},
	}

	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			got, err := _Global(t, item.script, item.read)
			if err != nil {
				t.Fatalf("%s: %v", item.read, err)
			}

			if got != item.want {
				t.Fatalf("%s was %s, want %s", item.read, got, item.want)
			}
		})
	}
}
