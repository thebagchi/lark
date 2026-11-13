package tests

// These tests pin what a read-only pointer builtin answers and says. They were
// written before the cursor walk of C36 and passed on the walk that built a
// step slice, so a passing run after the change shows the answers did not move.
//
// Revisions:
//   - 2026-11-02 17:27: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)

import (
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin"
)

const (
	// LOCATE_DOC holds a key with a slash, a key with a tilde, an empty key and
	// a list, so each kind of token and each kind of miss has a target.
	//
	// Revisions:
	//   - 2026-11-02 17:27: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	LOCATE_DOC = `{"a": {"b": [1, 2, 3]}, "e/f": {"g": 1}, "h~i": 2, "": 3, "t": "x"}`
)

// TestQueries_ANoneForEveryKindOfMiss proves that a pointer into a document
// that lacks the target is an answer, not an error, in all three queries.
//
// A key, a dash, a leading zero or an empty token met by a LIST is not a miss:
// it is ErrPointer, and TestErrors_KeepTheirText pins that. Met by an object,
// the same token is only a key that is not there, and it is a miss.
//
// Revisions:
//   - 2026-11-02 17:12: initial creation (C36)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestQueries_ANoneForEveryKindOfMiss(t *testing.T) {
	cases := []struct{ name, pointer string }{
		{
			"missing key",
			"/nope",
		},
		{
			"missing key below a key",
			"/a/nope",
		},
		{
			"index past the end",
			"/a/b/3",
		},
		{
			"index far past the end",
			"/a/b/999",
		},
		{
			"index on an object",
			"/a/0",
		},
		{
			"dash as a key",
			"/a/-",
		},
		{
			"leading zero as a key",
			"/a/01",
		},
		{
			"empty token as a key",
			"/a/",
		},
		{
			"step into a number",
			"/a/b/0/x",
		},
		{
			"step into a string",
			"/t/x",
		},
		{
			"escaped key that is absent",
			"/e~1x",
		},
		{
			"escaped key below an escaped key",
			"/e~1f/g~0h",
		},
		{
			"bad index behind a missing key",
			"/nope/01",
		},
		{
			"dash behind a missing key",
			"/nope/-",
		},
		{
			"leading zero behind a number",
			"/a/b/0/01",
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				var (
					path = `, "` + item.pointer + `"`
					call = path + ")"
				)

				_Check(
					t,
					[]struct{ name, expression, want string }{
						{
							"extract",
							"extract_json(" + LOCATE_DOC + call,
							`None`,
						},
						{
							"match",
							"match_json(" + LOCATE_DOC + path + ", 1)",
							`False`,
						},
						{
							"length",
							"len_json(" + LOCATE_DOC + call,
							`0`,
						},
					},
				)
			},
		)
	}
}

// TestQueries_AHitIsTheValue proves that the walk still finds what is there,
// through each kind of token.
//
// Revisions:
//   - 2026-11-02 17:12: initial creation (C36)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestQueries_AHitIsTheValue(t *testing.T) {
	_Check(
		t,
		[]struct{ name, expression, want string }{
			{
				"whole document",
				`extract_json({"a": 1}, "")`,
				`{"a": 1}`,
			},
			{
				"nested",
				`extract_json(` + LOCATE_DOC + `, "/a/b/2")`,
				`3`,
			},
			{
				"first index",
				`extract_json(` + LOCATE_DOC + `, "/a/b/0")`,
				`1`,
			},
			{
				"escaped slash",
				`extract_json(` + LOCATE_DOC + `, "/e~1f/g")`,
				`1`,
			},
			{
				"escaped tilde",
				`extract_json(` + LOCATE_DOC + `, "/h~0i")`,
				`2`,
			},
			{
				"empty key",
				`extract_json(` + LOCATE_DOC + `, "/")`,
				`3`,
			},
			{
				"match",
				`match_json(` + LOCATE_DOC + `, "/a/b/1", 2)`,
				`True`,
			},
			{
				"length",
				`len_json(` + LOCATE_DOC + `, "/a/b")`,
				`3`,
			},
		},
	)
}

// TestErrors_KeepTheirText pins every refusal a pointer gets, with its text.
//
// The texts were recorded from the code before C36. A script author reads
// them, and the golden samples print some.
//
// Revisions:
//   - 2026-11-02 17:12: initial creation (C36)
//   - 2026-11-04 21:49: pins the escaped form of the prefix a query shows (C59, Q141)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestErrors_KeepTheirText(t *testing.T) {
	const (
		DOC         = `{"a": {"b": [1, 2, 3]}, "e/f": 1, "t": "x"}`
		HUGE_DIGITS = "99999999999999999999"
		OP          = "patch_json: operation 0: "
		NOT_POINTER = "not a json pointer"
	)

	// _Patch is a patch of one operation on DOC. A move reads its path as "from".
	_Patch := func(
		kind,
		path string,
	) string {
		if kind == "move" {
			return `patch_json(` + DOC + `, [{"op": "move", "from": "` + path +
				`", "path": "/z"}])`
		}

		return `patch_json(` + DOC + `, [{"op": "` + kind + `", "path": "` + path +
			`", "value": 1}])`
	}

	var cases = []struct{ name, expression, want string }{
		{
			"no leading slash",
			`extract_json(` + DOC + `, "a")`,
			`extract_json: "a" does not start with "/": not a json pointer`,
		},
		{
			"trailing tilde",
			`extract_json(` + DOC + `, "/a~")`,
			`extract_json: "/a~" has a "~" not followed by 0 or 1: not a json pointer`,
		},
		{
			"tilde two in a later token",
			`extract_json(` + DOC + `, "/a/~2")`,
			`extract_json: "/a/~2" has a "~" not followed by 0 or 1: ` + NOT_POINTER,
		},
		{
			"leading zero",
			`extract_json(` + DOC + `, "/a/b/01")`,
			`extract_json: "/a/b/01": "01" is not an index: not a json pointer`,
		},
		{
			"a failing step that is not the last",
			`extract_json({"a": [1]}, "/a/x/y")`,
			`extract_json: "/a/x": "x" is not an index: not a json pointer`,
		},
		{
			"escaped slash in a key above a list",
			`extract_json({"a/b": [1]}, "/a~1b/x")`,
			`extract_json: "/a~1b/x": "x" is not an index: not a json pointer`,
		},
		{
			"escaped tilde in a key above a list",
			`extract_json({"m~n": [1]}, "/m~0n/-")`,
			`extract_json: "/m~0n/-": "-" is not an index: not a json pointer`,
		},
		{
			"escaped tilde as the failing step",
			`extract_json({"a": [1]}, "/a/~0")`,
			`extract_json: "/a/~0": "~" is not an index: not a json pointer`,
		},
		{
			"dash index",
			`extract_json(` + DOC + `, "/a/b/-")`,
			`extract_json: "/a/b/-": "-" is not an index: not a json pointer`,
		},
		{
			"empty token on a list",
			`extract_json(` + DOC + `, "/a/b/")`,
			`extract_json: "/a/b/": "" is not an index: not a json pointer`,
		},
		{
			"key on a list",
			`extract_json(` + DOC + `, "/a/b/x")`,
			`extract_json: "/a/b/x": "x" is not an index: not a json pointer`,
		},
		{
			"signed index",
			`extract_json(` + DOC + `, "/a/b/+1")`,
			`extract_json: "/a/b/+1": "+1" is not an index: not a json pointer`,
		},
		{
			"index too large for an int",
			`extract_json(` + DOC + `, "/a/b/` + HUGE_DIGITS + `")`,
			`extract_json: "/a/b/` + HUGE_DIGITS + `": "` + HUGE_DIGITS +
				`" is not an index: ` + NOT_POINTER,
		},
		{
			"match leading zero",
			`match_json(` + DOC + `, "/a/b/01", 1)`,
			`match_json: "/a/b/01": "01" is not an index: not a json pointer`,
		},
		{
			"length dash",
			`len_json(` + DOC + `, "/a/b/-")`,
			`len_json: "/a/b/-": "-" is not an index: not a json pointer`,
		},
		{
			"length of a number",
			`len_json(` + DOC + `, "/a/b/0")`,
			`len_json: int has no length: cannot be indexed`,
		},
		{
			"patch test leading zero",
			_Patch(
				"test",
				"/a/b/01",
			),
			OP + `"/a/b/01": "01" is not an index: not a json pointer`,
		},
		{
			"patch test index past the end",
			_Patch(
				"test",
				"/a/b/9",
			),
			OP + `"/a/b/9": 9 is outside a list of 3: no such path`,
		},
		{
			"patch test missing key",
			_Patch(
				"test",
				"/nope",
			),
			OP + `"/nope": no such path`,
		},
		{
			"patch test into a string",
			_Patch(
				"test",
				"/t/x",
			),
			OP + `"/t/x": string cannot be indexed`,
		},
		{
			"patch remove dash",
			_Patch(
				"remove",
				"/a/b/-",
			),
			OP + `"/a/b/-": "-" is not an index: not a json pointer`,
		},
		{
			"patch replace past the end",
			_Patch(
				"replace",
				"/a/b/7",
			),
			OP + `"/a/b/7": 7 is outside a list of 3: no such path`,
		},
		{
			"patch replace below a missing key",
			_Patch(
				"replace",
				"/a/q/7",
			),
			OP + `"/a/q": no such path`,
		},
		{
			"patch test below an escaped slash",
			_Patch(
				"test",
				"/e~1f/x",
			),
			OP + `"/e~1f/x": int cannot be indexed`,
		},
		{
			"patch replace below an escaped slash",
			_Patch(
				"replace",
				"/e~1f/x",
			),
			OP + `"/e~1f/x": int cannot be indexed`,
		},
		{
			"patch remove below an escaped slash",
			_Patch(
				"remove",
				"/e~1f/x",
			),
			OP + `"/e~1f/x": int cannot be indexed`,
		},
		{
			"patch move from below an escaped slash",
			_Patch(
				"move",
				"/e~1f/x",
			),
			OP + `"/e~1f/x": int cannot be indexed`,
		},
		{
			"patch test an escaped tilde as the failing step",
			`patch_json({"m~n": [1]}, [{"op": "test", "path": "/m~0n/-", ` +
				`"value": 1}])`,
			OP + `"/m~0n/-": "-" is not an index: not a json pointer`,
		},
		{
			"patch move from a leading zero",
			_Patch(
				"move",
				"/a/b/07",
			),
			OP + `"/a/b/07": "07" is not an index: not a json pointer`,
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
						"%s gave %s, want the error %q",
						item.expression,
						got,
						item.want,
					)
				}

				if err.Error() != item.want {
					t.Fatalf(
						"%s gave %q, want %q",
						item.expression,
						err.Error(),
						item.want,
					)
				}
			},
		)
	}
}

// TestExtract_AMissCostsAsMuchAsAHit proves that a miss builds no error: for
// each kind of miss, the allocations of extract_json are at most those of a hit
// of the same depth.
//
// Revisions:
//   - 2026-11-02 17:12: initial creation (C36)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestExtract_AMissCostsAsMuchAsAHit(t *testing.T) {
	env, err := plugin.Environment(plugin.Registered())
	if err != nil {
		t.Fatalf(
			"environment: %v",
			err,
		)
	}

	extract, ok := env["extract_json"].(*starlark.Builtin)
	if !ok {
		t.Fatal("extract_json is not a builtin")
	}

	value, err := starlark.EvalOptions(
		common.OPTIONS,
		&starlark.Thread{
			Name: SCRIPT,
		},
		SCRIPT,
		LOCATE_DOC,
		env,
	)
	if err != nil {
		t.Fatal(err)
	}

	var (
		thread = &starlark.Thread{
			Name: SCRIPT,
		}
		allocs = func(pointer string) float64 {
			args := starlark.Tuple{
				value,
				starlark.String(pointer),
			}

			return testing.AllocsPerRun(
				100,
				func() {
					_, err := extract.CallInternal(
						thread,
						args,
						nil,
					)
					if err != nil {
						t.Fatal(err)
					}
				},
			)
		}
		hit = allocs("/a/b/2")
	)

	cases := []struct{ name, pointer string }{
		{
			"missing key",
			"/a/nope/x",
		},
		{
			"index past the end",
			"/a/b/9",
		},
		{
			"index on an object",
			"/a/0/x",
		},
		{
			"dash as a key",
			"/a/-",
		},
		{
			"step into a number",
			"/a/b/0/x",
		},
		{
			"escaped miss",
			"/e~1x/y",
		},
	}

	for _, item := range cases {
		t.Run(
			item.name,
			func(t *testing.T) {
				miss := allocs(item.pointer)
				if miss > hit {
					t.Fatalf(
						"%s: %v allocs, a hit costs %v",
						item.pointer,
						miss,
						hit,
					)
				}
			},
		)
	}
}
