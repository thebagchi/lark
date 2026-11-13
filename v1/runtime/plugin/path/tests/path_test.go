package tests

import (
	"errors"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	larkpath "github.com/thebagchi/lark/v1/runtime/plugin/path"
)

// SCRIPT is what a failure calls the expression it was given.
//
// Revisions:
//   - 2026-09-24 17:26: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
const (
	SCRIPT = "path_test.star"
)

// _Eval evaluates one expression against the default environment.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
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
//   - 2026-09-24 00:53: initial creation
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

// TestPath_ReadsAPathApart is every way this takes one to pieces.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestPath_ReadsAPathApart(t *testing.T) {
	_Check(
		t,
		[]struct{ name, expression, want string }{
			{
				"dir",
				`path.dir("/srv/work/run.log")`,
				`"/srv/work"`,
			},
			{
				"base",
				`path.base("/srv/work/run.log")`,
				`"run.log"`,
			},
			{
				"ext",
				`path.ext("/srv/work/run.log")`,
				`".log"`,
			},
			{
				"stem",
				`path.stem("/srv/work/run.log")`,
				`"run"`,
			},
			{
				"stem of a dotfile",
				`path.stem("/etc/.bashrc")`,
				`""`,
			},
			{
				"ext of a dotfile",
				`path.ext("/etc/.bashrc")`,
				`".bashrc"`,
			},
			{
				"ext where there is none",
				`path.ext("/srv/work/run")`,
				`""`,
			},
			{
				"split",
				`path.split("/srv/work/run.log")`,
				`["/srv/work", "run.log"]`,
			},
			{
				"parts",
				`path.parts("/srv/work/run.log")`,
				`["/", "srv", "work", "run.log"]`,
			},
			{
				"parts of a relative path",
				`path.parts("srv/work")`,
				`["srv", "work"]`,
			},
			{
				"isabs",
				`path.isabs("/srv")`,
				"True",
			},
			{
				"isabs of a relative path",
				`path.isabs("srv")`,
				"False",
			},
		},
	)
}

// TestPath_JoinsAndCleans is what a caller building a path relies on: the
// pieces go together however they were spelled.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestPath_JoinsAndCleans(t *testing.T) {
	_Check(
		t,
		[]struct{ name, expression, want string }{
			{
				"two",
				`path.join("srv", "work")`,
				`"srv/work"`,
			},
			{
				"many",
				`path.join("/srv", "work", "logs", "run.log")`,
				`"/srv/work/logs/run.log"`,
			},
			{
				"spare separators",
				`path.join("a/", "/b")`,
				`"a/b"`,
			},
			{
				"an empty piece",
				`path.join("a", "", "b")`,
				`"a/b"`,
			},
			{
				"one",
				`path.join("a")`,
				`"a"`,
			},
			{
				"none",
				`path.join()`,
				`""`,
			},
			{
				"clean",
				`path.clean("/srv/./work/../work/run.log")`,
				`"/srv/work/run.log"`,
			},
			{
				"clean keeps it relative",
				`path.clean("./a/b")`,
				`"a/b"`,
			},
		},
	)
}

// TestPath_IsTextOnly records that nothing here asks a disk anything, which is
// what separates it from file.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestPath_IsTextOnly(t *testing.T) {
	_Check(
		t,
		[]struct{ name, expression, want string }{
			{
				"a path that does not exist reads the same",
				`path.base("/no/such/place/at/all.txt")`,
				`"all.txt"`,
			},
			{
				"and joins the same",
				`path.join("/no/such", "place")`,
				`"/no/such/place"`,
			},
		},
	)
}

// TestPath_RefusesWhatIsNotText records the mistake a caller can make.
//
// Revisions:
//   - 2026-09-24 00:53: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrPath, formerly ErrNotAPath (C121, Q349)
func TestPath_RefusesWhatIsNotText(t *testing.T) {
	_, err := _Eval(
		t,
		`path.join("a", 1)`,
	)
	if !errors.Is(
		err,
		larkpath.ErrPath,
	) {
		t.Fatalf(
			"got %v, want ErrPath",
			err,
		)
	}
}
