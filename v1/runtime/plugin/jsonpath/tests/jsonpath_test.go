package tests

// These tests are table driven because the thing under test is a
// specification: RFC 6901 for pointers, RFC 6902 for patch. A case per rule is
// what makes it possible to see which rule is missing.
//
// Revisions:
//   - 2026-09-20 00:29: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)

import (
	"errors"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/jsonpath"
)

const (
	SCRIPT = "jsonpath_test.star"
)

// _Eval runs one Starlark expression against the plugin environment and
// returns what it produced, as its String form.
//
// Revisions:
//   - 2026-09-20 01:23: initial creation
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Eval(
	t *testing.T,
	expression string,
) (string, error) {
	t.Helper()

	env, err := plugin.Environment(plugin.Registered())
	if err != nil {
		t.Fatalf(
			"environment: %v",
			err,
		)
	}

	value, err := starlark.EvalOptions(
		common.OPTIONS,
		&starlark.Thread{
			Name: SCRIPT,
		},
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
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Check(
	t *testing.T,
	cases []struct{ name, expression, want string },
) {
	t.Helper()

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				got, err := _Eval(
					t,
					item.expression,
				)
				if err != nil {
					t.Fatalf(
						"%s: %v",
						item.expression,
						err,
					)
				}

				if got != item.want {
					t.Fatalf(
						"%s gave %s, want %s",
						item.expression,
						got,
						item.want,
					)
				}
			},
		)
	}
}

// TestPointer_ResolvesRFC6901 covers the pointer rules, including the two that
// are easy to get wrong: escaping order, and what the empty pointer means.
//
// Revisions:
//   - 2026-09-20 01:25: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestPointer_ResolvesRFC6901(t *testing.T) {
	const DOC = `{"a": {"b": [10, 20]}, "e/f": 1, "g~h": 2, "": 3}`

	_Check(
		t,
		[]struct{ name, expression, want string }{
			{
				"whole document",
				`extract_json(` + DOC + `, "")`,
				`{"a": {"b": [10, 20]}, "e/f": 1, "g~h": 2, "": 3}`,
			},
			{
				"member",
				`extract_json(` + DOC + `, "/a")`,
				`{"b": [10, 20]}`,
			},
			{
				"nested member",
				`extract_json(` + DOC + `, "/a/b")`,
				`[10, 20]`,
			},
			{
				"list index",
				`extract_json(` + DOC + `, "/a/b/1")`,
				`20`,
			},
			{
				"escaped slash",
				`extract_json(` + DOC + `, "/e~1f")`,
				`1`,
			},
			{
				"escaped tilde",
				`extract_json(` + DOC + `, "/g~0h")`,
				`2`,
			},
			{
				"empty key",
				`extract_json(` + DOC + `, "/")`,
				`3`,
			},
			{
				"missing member",
				`extract_json(` + DOC + `, "/nope")`,
				`None`,
			},
			{
				"index past the end",
				`extract_json(` + DOC + `, "/a/b/9")`,
				`None`,
			},
		},
	)
}

// TestPointer_RefusesWhatIsNotAPointer proves a malformed pointer is the
// script being wrong, and is an error rather than a None.
//
// Revisions:
//   - 2026-09-20 01:26: initial creation
//   - 2026-09-21 08:09: a signed index is not one either
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestPointer_RefusesWhatIsNotAPointer(t *testing.T) {
	for _, pointer := range []string{`"a/b"`, `"/a/01"`, `"/a/"`, `"/a/+0"`, `"/a/-1"`} {
		_, err := _Eval(
			t,
			`extract_json({"a": [1]}, `+pointer+`)`,
		)
		if err == nil {
			t.Fatalf(
				"%s was accepted as a pointer",
				pointer,
			)
		}
	}
}

// TestPatch_AppliesRFC6902 covers each operation and the rules that separate
// them - add inserting where replace requires, and "-" appending.
//
// Revisions:
//   - 2026-09-20 01:27: initial creation
//   - 2026-09-21 08:09: a move onto itself, which the specification allows
//   - 2026-10-03 19:28: a move of the whole document onto itself, and a member moved onto
//     itself keeping its place
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestPatch_AppliesRFC6902(t *testing.T) {
	_Check(
		t,
		[]struct{ name, expression, want string }{
			{
				"add a member",
				`patch_json({"a": 1}, [{"op": "add", "path": "/b", "value": 2}])`,
				`{"a": 1, "b": 2}`,
			},
			{
				"add inserts into a list",
				`patch_json({"a":` +
					` [1, 3]}, [{"op": "add", "path": "/a/1", ` +
					`"value": 2}])`,
				`{"a": [1, 2, 3]}`,
			},
			{
				"add" +
					" appends with a dash",
				`patch_json({"a": [1]}, [{"op": "add", "path": "/a/-", ` +
					`"value": 2}])`,
				`{"a": [1, 2]}`,
			},
			{
				"replace a member",
				`patch_json({"a": 1}, [{"op": "replace", "path": "/a", ` +
					`"value": 2}])`,
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
				`patch_json({"a": 1}, [{"op": "move", "from": "/a", ` +
					`"path": "/b"}])`,
				`{"b":` +
					` 1}`,
			},
			{
				"copy",
				`patch_json({"a": 1}, [{"op": "copy", "from": "/a", ` +
					`"path": "/b"}])`,
				`{"a": 1, "b": 1}`,
			},
			{
				"test that passes leaves the document",
				`patch_json({"a":` +
					` 1}, [{"op": "test", "path": "/a", "value": 1}])`,
				`{"a": 1}`,
			},
			{
				"move onto itself changes nothing",
				`patch_json({"a": {"b": 1}}, [{"op": "move", "from": "/a", ` +
					`"path": ` +
					`"/a"}])`,
				`{"a": {"b": 1}}`,
			},
			{
				"move" +
					" the whole document onto itself",
				`patch_json({"a": 1}, [{"op": "move", "from": "", "path": ""}])`,
				`{"a": 1}`,
			},
			{
				"move onto itself keeps the member's place",
				`patch_json({"a":` +
					` 1, "b": 2}, [{"op": "move", "from": "/a", ` +
					`"path": ` +
					`"/a"}])`,
				`{"a": 1, "b": 2}`,
			},
			{
				"operations apply in order",
				`patch_json({"a": 1}, [{"op": "add", "path": "/b", "value": 2},` +
					`{"op": ` +
					`"remove", "path": "/a"}])`,
				`{"b": 2}`,
			},
		},
	)
}

// TestPatch_RemovesFromAList covers each index edge of a remove below a list:
// the first element, the last, the only one, and the refusals. RFC 6902
// section 4.2: "The target location MUST exist for the operation to be
// successful."
//
// Revisions:
//   - 2026-11-02 00:58: initial creation
//   - 2026-11-02 08:12: a list in a dict, and a list in a list
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestPatch_RemovesFromAList(t *testing.T) {
	_Check(
		t,
		[]struct{ name, expression, want string }{
			{
				"the first element",
				`patch_json({"a": [1, 2, 3]}, [{"op": "remove", "path": "/a/0"}])`,
				`{"a": [2, 3]}`,
			},
			{
				"the last element",
				`patch_json({"a": [1, 2, 3]}, [{"op": "remove", "path": "/a/2"}])`,
				`{"a": [1, 2]}`,
			},
			{
				"the only element",
				`patch_json({"a": [1]}, [{"op": "remove", "path": "/a/0"}])`,
				`{"a": []}`,
			},
			{
				"two in a row",
				`patch_json({"a": [1, 2, 3]}, ` +
					`[{"op": "remove", "path": "/a/0"}, ` +
					`{"op": "remove", "path": "/a/0"}])`,
				`{"a": [3]}`,
			},
			{
				"a list in a dict in the root",
				`patch_json({"a": {"b": [1, 2, 3]}}, ` +
					`[{"op": "remove", "path": "/a/b/1"}])`,
				`{"a": {"b": [1, 3]}}`,
			},
			{
				"a list in a list",
				`patch_json([[1, 2, 3], [4]], [{"op": "remove", "path": "/0/1"}])`,
				`[[1, 3], [4]]`,
			},
			{
				"a list in a list, the last element",
				`patch_json([[1], [4, 5]], [{"op": "remove", "path": "/1/1"}])`,
				`[[1], [4]]`,
			},
		},
	)

	refused := []struct{ name, expression, carries string }{
		{
			"one past the last element",
			`patch_json({"a": [1, 2]}, [{"op": "remove", "path": "/a/2"}])`,
			"outside a list of 2",
		},
		{
			"an empty list",
			`patch_json({"a": []}, [{"op": "remove", "path": "/a/0"}])`,
			"outside a list of 0",
		},
		{
			"the append step",
			`patch_json({"a": [1]}, [{"op": "remove", "path": "/a/-"}])`,
			"not an index",
		},
	}

	for _, item := range refused {
		t.Run(
			item.name,
			func(t *testing.T) {
				_, err := _Eval(
					t,
					item.expression,
				)
				if err == nil {
					t.Fatalf(
						"%s was accepted",
						item.expression,
					)
				}

				if !strings.Contains(
					err.Error(),
					item.carries,
				) {
					t.Fatalf(
						"%s failed with %v, want it to mention %q",
						item.expression,
						err,
						item.carries,
					)
				}
			},
		)
	}
}

// TestPatch_RefusesWhatTheSpecificationRefuses covers the error cases §10.7
// names one by one, because each is a rule somebody could leave out and never
// notice.
//
// Revisions:
//   - 2026-09-20 01:29: initial creation
//   - 2026-10-03 19:28: a move onto itself of what is not there
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestPatch_RefusesWhatTheSpecificationRefuses(t *testing.T) {
	cases := []struct{ name, expression, carries string }{
		{
			"unknown op",
			`patch_json({}, [{"op": "invent", "path": "/a"}])`,
			"unknown patch operation",
		},
		{
			"missing path",
			`patch_json({}, [{"op": "remove"}])`,
			"missing a field",
		},
		{
			"missing value",
			`patch_json({}, [{"op": "add", "path": "/a"}])`,
			"missing a field",
		},
		{
			"remove what is not there",
			`patch_json({}, [{"op": "remove", "path": "/a"}])`,
			"no such path",
		},
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
			`patch_json({"a": {"b": 1}}, [{"op": "move", "from": "/a", "path": ` +
				`"/a/b"}])`,
			"into itself",
		},
		{
			"move onto itself what is not there",
			`patch_json({"a": 1}, [{"op": "move", "from": "/b", "path": "/b"}])`,
			"no such path",
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				_, err := _Eval(
					t,
					item.expression,
				)
				if err == nil {
					t.Fatalf(
						"%s was accepted",
						item.expression,
					)
				}

				if !strings.Contains(
					err.Error(),
					item.carries,
				) {
					t.Fatalf(
						"%s failed with %v, want it to mention %q",
						item.expression,
						err,
						item.carries,
					)
				}
			},
		)
	}
}

// TestPatch_MalformedOperationIsErrOperation proves an op with an unknown name
// and an op missing a field both match jsonpath.ErrOperation under errors.Is:
// the sentinel for a missing field was merged into it (C98, Q251).
//
// Revisions:
//   - 2026-11-08 13:25: initial creation (C98, Q251)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestPatch_MalformedOperationIsErrOperation(t *testing.T) {
	cases := []struct{ name, expression string }{
		{
			"unknown op",
			`patch_json({}, [{"op": "invent", "path": "/a"}])`,
		},
		{
			"missing path",
			`patch_json({}, [{"op": "remove"}])`,
		},
		{
			"missing value",
			`patch_json({}, [{"op": "add", "path": "/a"}])`,
		},
		{
			"op is not text",
			`patch_json({}, [{"op": 1, "path": "/a"}])`,
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				_, err := _Eval(
					t,
					item.expression,
				)
				if !errors.Is(
					err,
					jsonpath.ErrOperation,
				) {
					t.Fatalf(
						"%s gave %v, want jsonpath.ErrOperation",
						item.expression,
						err,
					)
				}
			},
		)
	}
}

// TestPatch_PlacesAtTheLastStep covers each branch of the two place
// operations of RFC 6902: add (section 4.1) inserts into a list and sets a
// member, replace (section 4.3) overwrites an element or a member. Each case
// records the error text, so a change to the shared walk cannot alter it.
//
// Revisions:
//   - 2026-11-02 00:30: initial creation, before _Place took the list step as a func
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestPatch_PlacesAtTheLastStep(t *testing.T) {
	_Check(
		t,
		[]struct{ name, expression, want string }{
			{
				"add inserts before the first element",
				`patch_json([1], [{"op": "add", "path": "/0", "value": 0}])`,
				`[0, 1]`,
			},
			{
				"add inserts at the length",
				`patch_json([1], [{"op": "add", "path": "/1", "value": 2}])`,
				`[1, 2]`,
			},
			{
				"add onto an empty list with a dash",
				`patch_json([], [{"op": "add", "path": "/-", "value": 1}])`,
				`[1]`,
			},
			{
				"add under a nested list",
				`patch_json({"a": [[1]]}, [{"op": "add", "path": "/a/0/0", ` +
					`"value": 2}])`,
				`{"a": [[2, 1]]}`,
			},
			{
				"add sets an existing member",
				`patch_json({"a": 1}, [{"op": "add", "path": "/a", "value": 2}])`,
				`{"a": 2}`,
			},
			{
				"replace overwrites an element",
				`patch_json({"a": [1, 2]}, [{"op": "replace", "path": "/a/1", ` +
					`"value": 3}])`,
				`{"a": [1, 3]}`,
			},
			{
				"replace overwrites a nested member",
				`patch_json({"a": [{"b": 1}]}, [{"op": "replace", "path": ` +
					`"/a/0/b", "value": 2}])`,
				`{"a": [{"b": 2}]}`,
			},
		},
	)

	cases := []struct{ name, expression, want string }{
		{
			"add beyond the length",
			`patch_json({"a": [1]}, [{"op": "add", "path": "/a/5", "value": 2}])`,
			`patch_json: operation 0: 5 is outside a list of 2: no such path`,
		},
		{
			"add at a name that is not an index",
			`patch_json({"a": [1]}, [{"op": "add", "path": "/a/x", "value": 2}])`,
			`patch_json: operation 0: "x" is not an index: not a json pointer`,
		},
		{
			"add under a missing parent",
			`patch_json({"a": 1}, [{"op": "add", "path": "/b/c", "value": 2}])`,
			`patch_json: operation 0: "b": no such path`,
		},
		{
			"add under a scalar",
			`patch_json({"a": 1}, [{"op": "add", "path": "/a/b", "value": 2}])`,
			`patch_json: operation 0: int cannot be indexed`,
		},
		{
			"replace beyond the length",
			`patch_json({"a": [1]}, [{"op": "replace", "path": "/a/5", ` +
				`"value": 2}])`,
			`patch_json: operation 0: "/a/5": 5 is outside a list of 1: ` +
				`no such path`,
		},
		{
			"replace with a dash",
			`patch_json({"a": [1]}, [{"op": "replace", "path": "/a/-", ` +
				`"value": 2}])`,
			`patch_json: operation 0: "/a/-": "-" is not an index: ` +
				`not a json pointer`,
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				_, err := _Eval(
					t,
					item.expression,
				)
				if err == nil {
					t.Fatalf(
						"%s was accepted",
						item.expression,
					)
				}

				if err.Error() != item.want {
					t.Fatalf(
						"%s failed with %q, want %q",
						item.expression,
						err.Error(),
						item.want,
					)
				}
			},
		)
	}
}

// TestPatch_WritesBelowAList proves every operation that descends keeps the
// list on the path as it was: only the element on the path is written again,
// and no element is inserted beside it. The cases catch a list at an upper
// level, not the last, treated as an insert.
//
// Revisions:
//   - 2026-11-02 00:30: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestPatch_WritesBelowAList(t *testing.T) {
	_Check(
		t,
		[]struct{ name, expression, want string }{
			{
				"add below a list element",
				`patch_json({"a": [{"b": 1, "c": 2}, 9]},` +
					` [{"op": "add", "path": "/a/0/x", "value": 5}])`,
				`{"a": [{"b": 1, "c": 2, "x": 5}, 9]}`,
			},
			{
				"add below the second list element",
				`patch_json({"a": [9, {"b": 1}]},` +
					` [{"op": "add", "path": "/a/1/x", "value": 5}])`,
				`{"a": [9, {"b": 1, "x": 5}]}`,
			},
			{
				"remove below a list element",
				`patch_json({"a": [{"b": 1, "c": 2}, 9]},` +
					` [{"op": "remove", "path": "/a/0/b"}])`,
				`{"a": [{"c": 2}, 9]}`,
			},
			{
				"replace below a list element",
				`patch_json({"a": [{"b": 1, "c": 2}, 9]},` +
					` [{"op": "replace", "path": "/a/0/b", "value": 7}])`,
				`{"a": [{"b": 7, "c": 2}, 9]}`,
			},
			{
				"move from below a list element",
				`patch_json({"a": [{"b": 1, "c": 2}, 9]},` +
					` [{"op": "move", "from": "/a/0/b", "path": "/d"}])`,
				`{"a": [{"c": 2}, 9], "d": 1}`,
			},
			{
				"move to below a list element",
				`patch_json({"a": [{"b": 1}, 9], "d": 5},` +
					` [{"op": "move", "from": "/d", "path": "/a/0/e"}])`,
				`{"a": [{"b": 1, "e": 5}, 9]}`,
			},
			{
				"move from and to below list elements",
				`patch_json({"a": [{"b": 1, "c": 2}, 9]},` +
					` [{"op": "move", "from": "/a/0/b", "path": "/a/0/e"}])`,
				`{"a": [{"c": 2, "e": 1}, 9]}`,
			},
			{
				"copy from below a list element",
				`patch_json({"a": [{"b": 1, "c": 2}, 9]},` +
					` [{"op": "copy", "from": "/a/0/b", "path": "/d"}])`,
				`{"a": [{"b": 1, "c": 2}, 9], "d": 1}`,
			},
			{
				"copy to below a list element",
				`patch_json({"a": [{"b": 1}, 9], "d": 5},` +
					` [{"op": "copy", "from": "/d", "path": "/a/0/e"}])`,
				`{"a": [{"b": 1, "e": 5}, 9], "d": 5}`,
			},
			{
				"test below a list element",
				`patch_json({"a": [{"b": 1, "c": 2}, 9]},` +
					` [{"op": "test", "path": "/a/0/b", "value": 1}])`,
				`{"a": [{"b": 1, "c": 2}, 9]}`,
			},
		},
	)
}

// TestPatch_LeavesTheInputAlone proves the document is never mutated, which is
// what in_place=False means and what makes it safe to patch something another
// thread can already see.
//
// Revisions:
//   - 2026-09-20 01:31: initial creation
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestPatch_LeavesTheInputAlone(t *testing.T) {
	const SOURCE = `
before = {"a": [1, 2]}
after = patch_json(before, [{"op": "add", "path": "/a/-", "value": 3}])
`

	env, err := plugin.Environment(plugin.Registered())
	if err != nil {
		t.Fatalf(
			"environment: %v",
			err,
		)
	}

	globals, err := starlark.ExecFileOptions(
		common.OPTIONS,
		&starlark.Thread{
			Name: SCRIPT,
		},
		SCRIPT,
		[]byte(SOURCE),
		env,
	)
	if err != nil {
		t.Fatalf(
			"execute: %v",
			err,
		)
	}

	if globals["before"].String() != `{"a": [1, 2]}` {
		t.Fatalf(
			"the input was changed: %s",
			globals["before"].String(),
		)
	}

	if globals["after"].String() != `{"a": [1, 2, 3]}` {
		t.Fatalf(
			"the result is %s",
			globals["after"].String(),
		)
	}
}

// TestQueries_AnswerQuestionsRatherThanFail covers the four rules that
// separate "this document does not have that" from "you asked wrongly".
//
// Revisions:
//   - 2026-09-20 01:32: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestQueries_AnswerQuestionsRatherThanFail(t *testing.T) {
	const DOC = `{"a": {"b": [1, 2, 3]}, "deep": {"nested": {"id": "found"}}, "text": "abcd"}`

	_Check(
		t,
		[]struct{ name, expression, want string }{
			{
				"match",
				`match_json(` + DOC + `, "/a/b/0", 1)`,
				`True`,
			},
			{
				"match is false, not an error, when missing",
				`match_json(` + DOC + `, "/nope", 1)`,
				`False`,
			},
			{
				"match compares 1 and 1.0 as equal",
				`match_json(` + DOC + `, "/a/b/0", 1.0)`,
				`True`,
			},
			{
				"length of a list",
				`len_json(` + DOC + `, "/a/b")`,
				`3`,
			},
			{
				"length of a string",
				`len_json(` + DOC + `, "/text")`,
				`4`,
			},
			{
				"length of a dict",
				`len_json(` + DOC + `, "/a")`,
				`1`,
			},
			{
				"length of what is missing is zero",
				`len_json(` + DOC + `, "/nope")`,
				`0`,
			},
			{
				"find a key at depth",
				`find_key(` + DOC + `, "id")`,
				`"found"`,
			},
			{
				"find what is not there",
				`find_key(` + DOC + `, "absent")`,
				`None`,
			},
		},
	)
}

// _Global runs a script and returns one of the names it bound.
//
// Eval takes an expression, and showing that a document is untouched needs
// three statements: patch it, change what came back, then look at the
// original.
//
// Revisions:
//   - 2026-09-24 16:15: initial creation
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Global(
	t *testing.T,
	script string,
	name string,
) (string, error) {
	t.Helper()

	env, err := plugin.Environment(plugin.Registered())
	if err != nil {
		t.Fatalf(
			"environment: %v",
			err,
		)
	}

	globals, err := starlark.ExecFileOptions(
		common.OPTIONS,
		&starlark.Thread{
			Name: SCRIPT,
		},
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
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
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
		{
			"the original is untouched",
			FLAT,
			"doc",
			`{"a": [1]}`,
		},
		{
			"and the copy is its own list",
			FLAT,
			"out",
			`{"a": [1], "b": [1, 2]}`,
		},
		{
			"a nested container is copied too",
			NESTED,
			"doc",
			`{"a": {"x": [1]}}`,
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				got, err := _Global(
					t,
					item.script,
					item.read,
				)
				if err != nil {
					t.Fatalf(
						"%s: %v",
						item.read,
						err,
					)
				}

				if got != item.want {
					t.Fatalf(
						"%s was %s, want %s",
						item.read,
						got,
						item.want,
					)
				}
			},
		)
	}
}

// TestPatch_ManyOperationsLeaveTheInputAlone holds a patch that writes again
// and again under the containers it has already made, which it may change in
// place, and still reaches the caller's, which it may not.
//
// A copy written into after it is made is the case in-place editing could
// break: the copy is its own, so writing to it changes nothing at its source.
//
// Revisions:
//   - 2026-10-03 16:47: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestPatch_ManyOperationsLeaveTheInputAlone(t *testing.T) {
	const (
		// MANY writes again and again under what it has already copied.
		MANY = `
doc = {"a": {"b": 1, "items": [1, 2]}, "keep": [0]}
out = patch_json(doc, [
    {"op": "replace", "path": "/a/b", "value": 2},
    {"op": "add", "path": "/a/c", "value": 3},
    {"op": "add", "path": "/a/items/-", "value": 3},
    {"op": "add", "path": "/a/items/0", "value": 0},
    {"op": "replace", "path": "/a/items/1", "value": 10},
    {"op": "copy", "from": "/a", "path": "/d"},
    {"op": "add", "path": "/d/e", "value": 4},
    {"op": "add", "path": "/d/items/-", "value": 99},
    {"op": "remove", "path": "/a/b"},
    {"op": "move", "from": "/a/c", "path": "/moved"},
    {"op": "remove", "path": "/a/items/0"},
])
`

		// UNTOUCHED is the document MANY hands its patch, and LANDED what the
		// patch hands back.
		UNTOUCHED = `{"a": {"b": 1, "items": [1, 2]}, "keep": [0]}`
		LANDED    = `{"a": {"items": [10, 2, 3]}, "keep": [0], ` +
			`"d": {"b": 2, "items": [0, 10, 2, 3, 99], "c": 3, "e": 4}, "moved": 3}`

		// SIBLING writes once under each of two containers of the caller's.
		SIBLING = `
doc = {"x": {"y": 1}, "z": {"w": 1}}
out = patch_json(doc, [
    {"op": "replace", "path": "/x/y", "value": 2},
    {"op": "replace", "path": "/z/w", "value": 2},
])
`
	)

	cases := []struct{ name, script, read, want string }{
		{
			"the input is untouched",
			MANY,
			"doc",
			UNTOUCHED,
		},
		{
			"every operation lands, the copy apart from its source",
			MANY,
			"out",
			LANDED,
		},
		{
			"a container still the caller's is copied",
			SIBLING,
			"doc",
			`{"x": {"y": 1}, "z": {"w": 1}}`,
		},
		{
			"and written",
			SIBLING,
			"out",
			`{"x": {"y": 2}, "z": {"w": 2}}`,
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				got, err := _Global(
					t,
					item.script,
					item.read,
				)
				if err != nil {
					t.Fatalf(
						"%s: %v",
						item.read,
						err,
					)
				}

				if got != item.want {
					t.Fatalf(
						"%s was %s, want %s",
						item.read,
						got,
						item.want,
					)
				}
			},
		)
	}
}

// TestPatch_AMoveKeepsTheValueItTook records the other half, so that fixing
// copy is not read as a rule about both.
//
// Revisions:
//   - 2026-09-24 16:15: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestPatch_AMoveKeepsTheValueItTook(t *testing.T) {
	got, err := _Eval(
		t,
		`patch_json({"a": [1]}, [{"op": "move", "from": "/a", "path": "/b"}])`,
	)
	if err != nil {
		t.Fatal(err)
	}

	if got != `{"b": [1]}` {
		t.Fatalf(
			"got %s, want the value moved",
			got,
		)
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
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
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
		{
			"add leaves the document alone",
			ADDED,
			"doc",
			`{"a": [1]}`,
		},
		{
			"and the addition is its own list",
			ADDED,
			"out",
			`{"a": [1], "b": [1, 2]}`,
		},
		{
			"replace leaves it alone too",
			REPLACED,
			"doc",
			`{"a": [1], "b": [9]}`,
		},
		{
			"a value the caller still holds is not theirs to be changed",
			OWNED,
			"mine",
			`[1]`,
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				got, err := _Global(
					t,
					item.script,
					item.read,
				)
				if err != nil {
					t.Fatalf(
						"%s: %v",
						item.read,
						err,
					)
				}

				if got != item.want {
					t.Fatalf(
						"%s was %s, want %s",
						item.read,
						got,
						item.want,
					)
				}
			},
		)
	}
}

// TestPointer_KeepsTheValidEscapes pins the two escapes RFC 6901 section 3
// defines, "~0" and "~1", and the order of section 4 ("~01" is the key "~1").
// Each row passed before C22.
//
// Revisions:
//   - 2026-11-02 16:45: initial creation (C22, RFC 6901 sections 3 and 4)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestPointer_KeepsTheValidEscapes(t *testing.T) {
	const DOC = `{"~": 1, "/": 2, "~1": 3, "m~n": 4, "a/b": 5}`

	_Check(
		t,
		[]struct{ name, expression, want string }{
			{
				"tilde",
				`extract_json(` + DOC + `, "/~0")`,
				`1`,
			},
			{
				"slash",
				`extract_json(` + DOC + `, "/~1")`,
				`2`,
			},
			{
				"tilde then one",
				`extract_json(` + DOC + `, "/~01")`,
				`3`,
			},
			{
				"tilde inside",
				`extract_json(` + DOC + `, "/m~0n")`,
				`4`,
			},
			{
				"slash inside",
				`extract_json(` + DOC + `, "/a~1b")`,
				`5`,
			},
		},
	)
}

// TestPointer_RefusesAnInvalidEscape is RFC 6901 section 3: "escaped = "~" ( "0"
// / "1" )", with "~" excluded from "unescaped", and "It is an error condition
// if a JSON Pointer value does not conform to this syntax". Section 7 lists
// invalid pointer syntax as an error condition. All five callers of Steps
// are checked, so a caller that skipped the check shows here.
//
// Revisions:
//   - 2026-11-02 16:45: initial creation (C22, RFC 6901 sections 3 and 7)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestPointer_RefusesAnInvalidEscape(t *testing.T) {
	const (
		DOC = `{"~": 1, "a~": 2, "a": {"~": 3}}`
	)

	cases := []struct{ name, expression string }{
		{
			"extract lone tilde",
			`extract_json(` + DOC + `, "/~")`,
		},
		{
			"extract trailing tilde",
			`extract_json(` + DOC + `, "/a~")`,
		},
		{
			"extract unknown two",
			`extract_json(` + DOC + `, "/~2")`,
		},
		{
			"extract unknown letter",
			`extract_json(` + DOC + `, "/~x")`,
		},
		{
			"extract tilde tilde one",
			`extract_json(` + DOC + `, "/~~1")`,
		},
		{
			"extract in a later token",
			`extract_json(` + DOC + `, "/a/~")`,
		},
		{
			"extract tilde before slash",
			`extract_json(` + DOC + `, "/a~/b")`,
		},
		{
			"match",
			`match_json(` + DOC + `, "/~", 1)`,
		},
		{
			"length",
			`len_json(` + DOC + `, "/~")`,
		},
		{
			"add path",
			`patch_json({"a": 1}, [{"op": "add", "path": "/b~", "value": 2}])`,
		},
		{
			"remove path",
			`patch_json({"b~": 1}, [{"op": "remove", "path": "/b~"}])`,
		},
		{
			"replace path",
			`patch_json({"b~": 1}, [{"op": "replace", "path": "/b~2", "value": 2}])`,
		},
		{
			"test path",
			`patch_json({"b~": 1}, [{"op": "test", "path": "/b~", "value": 1}])`,
		},
		{
			"move from",
			`patch_json({"a": 1}, [{"op": "move", "from": "/a~", "path": "/b"}])`,
		},
		{
			"move path",
			`patch_json({"a": 1}, [{"op": "move", "from": "/a", "path": "/b~x"}])`,
		},
		{
			"copy from",
			`patch_json({"a": 1}, [{"op": "copy", "from": "/~", "path": "/b"}])`,
		},
		{
			"copy path",
			`patch_json({"a": 1}, [{"op": "copy", "from": "/a", "path": "/b~"}])`,
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				got, err := _Eval(
					t,
					item.expression,
				)
				if err == nil {
					t.Fatalf(
						"%s gave %s, want ErrPointer",
						item.expression,
						got,
					)
				}

				if !strings.Contains(
					err.Error(),
					jsonpath.ErrPointer.Error(),
				) {
					t.Fatalf(
						"%s gave %v, want %v",
						item.expression,
						err,
						jsonpath.ErrPointer,
					)
				}
			},
		)
	}
}

// TestPatch_IgnoresUnknownMembers is RFC 6902 section 4: "Members that are not
// explicitly defined for the operation in question MUST be ignored". lark
// keeps this, and the test pins it so a strictness change does not remove it.
//
// Revisions:
//   - 2026-11-02 16:45: initial creation (C22, RFC 6902 section 4)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestPatch_IgnoresUnknownMembers(t *testing.T) {
	_Check(
		t,
		[]struct{ name, expression, want string }{
			{
				"extra member",
				`patch_json({"a": 1}, [{"op": "add", "path": "/b", "value": 2, ` +
					`"extra": 9}])`,
				`{"a": 1, "b": 2}`,
			},
		},
	)
}

// FuzzPointer feeds extract_json any pointer and asserts RFC 6901. A pointer
// is accepted only if it is empty or starts with "/" (s3) and each "~" is
// followed by 0 or 1 (s3, s7). Any other pointer raises ErrPointer and never
// panics. The seeds are the T1-T3 cases of the Q8 research.
//
// Revisions:
//   - 2026-11-02 16:45: initial creation (C22)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func FuzzPointer(f *testing.F) {
	for _, seed := range []string{
		"", "/", "/a", "/a/b", "/~0", "/~1", "/~01", "/m~0n", "/a~1b", "/~", "/a~",
		"/~2", "/~x", "/~~1", "/a~/b", "a/b", "/a/01", "/a/-1", "/a/+0", "/a/0",
	} {
		f.Add(seed)
	}

	env, err := plugin.Environment(plugin.Registered())
	if err != nil {
		f.Fatalf(
			"environment: %v",
			err,
		)
	}

	var (
		builtin = env["extract_json"]
		doc     = starlark.NewDict(0)
	)

	f.Fuzz(func(
		t *testing.T,
		pointer string,
	) {
		_, err := starlark.Call(
			&starlark.Thread{
				Name: SCRIPT,
			},
			builtin,
			starlark.Tuple{doc, starlark.String(pointer)},
			nil,
		)

		var valid = pointer == "" || strings.HasPrefix(
			pointer,
			"/",
		)
		for at := 0; valid && at < len(pointer); at++ {
			if pointer[at] != '~' {
				continue
			}

			at++
			valid = at < len(pointer) && (pointer[at] == '0' || pointer[at] == '1')
		}

		if !valid && (err == nil || !strings.Contains(
			err.Error(),
			jsonpath.ErrPointer.Error(),
		)) {
			t.Fatalf(
				"pointer %q gave %v, want ErrPointer",
				pointer,
				err,
			)
		}

		if valid && err != nil && strings.Contains(
			err.Error(),
			"has a \"~\"",
		) {
			t.Fatalf(
				"pointer %q refused for its escapes: %v",
				pointer,
				err,
			)
		}
	})
}
