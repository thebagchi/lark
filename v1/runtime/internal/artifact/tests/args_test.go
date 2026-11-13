package tests

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/artifact"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

const (
	// MAIN is what each argument-count script is compiled as, and LOADABLE the module
	// it may load.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	//   - 2026-11-13 23:55: says argument-count script, formerly arity script (C136, Q475)
	MAIN     = "main.star"
	LOADABLE = "lib.star"

	// LOADED is what LOADABLE holds: a function, and a lambda bound to a
	// name, which is no function a flow lists.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	LOADED = `
def g(x):
    pass

h = lambda x: x
`
)

// _Sources is a loader over scripts held in memory, by name.
//
// Revisions:
//   - 2026-10-04 01:25: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Sources map[string]string

// Resolve reads target as a file beside from.
//
// Revisions:
//   - 2026-10-02 01:27: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (s _Sources) Resolve(
	from string,
	target string,
) (string, error) {
	return path.Join(
		path.Dir(from),
		target,
	), nil
}

// Load returns the script held under name.
//
// Revisions:
//   - 2026-10-02 01:27: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrUnit, formerly ErrNoUnit (C121, Q348)
func (s _Sources) Load(name string) ([]byte, error) {
	src, ok := s[name]
	if !ok {
		return nil, fmt.Errorf(
			"%s: %w",
			name,
			artifact.ErrUnit,
		)
	}

	return []byte(src), nil
}

// _Args compiles a script whose main makes the one call given, beside the
// functions it may call and a local n it may pass, and returns what the compile
// said.
//
// Revisions:
//   - 2026-10-02 01:27: initial creation
//   - 2026-10-08 17:28: main binds n, so a call can pass an operation such as n + 1
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 23:04: renamed from _Arity (C121, Q354)
func _Args(call string) error {
	src := `
load("lib.star", "g", "h")

def f(a, b = 1):
    pass

def rest(a, *more):
    pass

def keywords(a, **more):
    pass

def only(*, a):
    pass

def main():
    n = 1
    ` + call + `
`

	var (
		loader = _Sources{LOADABLE: LOADED}
		err    error
	)

	_, err = artifact.Compile(&common.Source{
		Entry:  MAIN,
		Text:   []byte(src),
		Loader: loader,
	})

	return err
}

// TestArgs_RefusesACallItsFunctionDoesNotTake checks the calls refused before
// the run: too few, too many, a keyword with no parameter, one parameter given
// twice, a keyword-only parameter passed by position, an operation one past the
// positional parameters of a function that takes **kwargs, and a loaded
// function passed nothing.
//
// Revisions:
//   - 2026-10-02 01:27: initial creation
//   - 2026-10-08 17:28: an operation one past the positional parameters
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 23:04: renamed from TestArity_RefusesACallItsFunctionDoesNotTake; uses _Args,
//     formerly _Arity (C121, Q354)
//   - 2026-11-13 23:55: uses spelling.ErrArgs, formerly ErrArity (C136, Q475)
func TestArgs_RefusesACallItsFunctionDoesNotTake(t *testing.T) {
	refused := []string{
		"f()",
		"f(1, 2, 3)",
		"f(1, c = 2)",
		"f(1, a = 2)",
		"total = f() or 1",
		"only()",
		"only(1)",
		"keywords(1, n + 1)",
		"g()",
	}

	for _, call := range refused {
		t.Run(
			call,
			func(t *testing.T) {
				err := _Args(call)
				if !errors.Is(
					err,
					spelling.ErrArgs,
				) {
					t.Fatalf(
						"got %v, want ErrArgs",
						err,
					)
				}
			},
		)
	}
}

// TestArgs_AcceptsWhatStarlarkBinds checks the calls that compile: a default
// left out or named, an operation passed by position, more than the parameters
// when a function takes *args or **kwargs, a call that unpacks, a name a
// parameter shadows, a loaded lambda, and a function the script does not
// define.
//
// Revisions:
//   - 2026-10-02 01:27: initial creation
//   - 2026-10-08 17:28: an operation passed by position
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 23:04: renamed from TestArity_AcceptsWhatStarlarkBinds; uses _Args, formerly
//     _Arity (C121, Q354)
func TestArgs_AcceptsWhatStarlarkBinds(t *testing.T) {
	accepted := []string{
		"f(1)",
		"f(1, 2)",
		"f(a = 1)",
		"f(1, b = 2)",
		"f(n + 1)",
		"rest(1, 2, 3)",
		"keywords(1, z = 2)",
		"only(a = 1)",
		"f(*[1, 2, 3])",
		"g(**{})",
		"g(1)",
		"h()",
		"len('x')",
	}

	for _, call := range accepted {
		t.Run(
			call,
			func(t *testing.T) {
				err := _Args(call)
				if err != nil {
					t.Fatalf(
						"refused: %v",
						err,
					)
				}
			},
		)
	}
}

// TestArgs_CountsOnlyTheFunctionTheNameMeans checks a parameter that shadows
// a function is not counted as the function.
//
// Revisions:
//   - 2026-10-02 01:27: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 23:04: renamed from TestArity_CountsOnlyTheFunctionTheNameMeans (C121, Q354)
func TestArgs_CountsOnlyTheFunctionTheNameMeans(t *testing.T) {
	var src = `
def f(a):
    pass

def call(f):
    f()

def main():
    call(len)
`

	_, err := artifact.Compile(&common.Source{
		Entry: MAIN,
		Text:  []byte(src),
	})
	if err != nil {
		t.Fatalf(
			"refused: %v",
			err,
		)
	}
}

// TestArgs_SaysWhereTheCallIs checks the refusal opens with the call's file,
// line and column, as a parse or a resolve error does.
//
// Revisions:
//   - 2026-10-02 01:27: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 23:04: renamed from TestArity_SaysWhereTheCallIs (C121, Q354)
func TestArgs_SaysWhereTheCallIs(t *testing.T) {
	var src = `
def f(a):
    pass

def main():
    f()
`

	_, err := artifact.Compile(&common.Source{
		Entry: MAIN,
		Text:  []byte(src),
	})
	if err == nil || !strings.HasPrefix(
		err.Error(),
		MAIN+":6:5: call of f",
	) {
		t.Fatalf(
			"got %v, want a refusal at %s:6:5",
			err,
			MAIN,
		)
	}
}

// TestArgs_RefusalText pins the whole text of each refusal: the call's
// position, the function it names and the reason. A change to how the walk
// finds a call must leave every text as it was.
//
// Revisions:
//   - 2026-11-02 00:31: initial creation, for the C12b extraction
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 23:04: renamed from TestArity_RefusalText; uses _Args, formerly _Arity (C121,
//     Q354)
func TestArgs_RefusalText(t *testing.T) {
	const REASON = "a call passes arguments the function does not take"

	tests := []struct {
		call string
		name string
	}{
		{
			call: "f()",
			name: "f",
		},
		{
			call: "f(1, 2, 3)",
			name: "f",
		},
		{
			call: "f(1, c = 2)",
			name: "f",
		},
		{
			call: "f(1, a = 2)",
			name: "f",
		},
		{
			call: "only()",
			name: "only",
		},
		{
			call: "only(1)",
			name: "only",
		},
		{
			call: "keywords(1, n + 1)",
			name: "keywords",
		},
		{
			call: "g()",
			name: "g",
		},
	}

	for _, test := range tests {
		t.Run(
			test.call,
			func(t *testing.T) {
				var (
					err  = _Args(test.call)
					want = fmt.Sprintf(
						"%s:18:5: call of %s: %s",
						MAIN,
						test.name,
						REASON,
					)
				)

				if err == nil || err.Error() != want {
					t.Fatalf(
						"got %v, want %q",
						err,
						want,
					)
				}
			},
		)
	}
}
