package artifact_test

import (
	"errors"
	"fmt"
	"path"
	"strings"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/artifact"
	"github.com/thebagchi/lark/v1/runtime/script"
)

const (
	// MAIN is what each arity script is compiled as, and LOADABLE the module
	// it may load.
	MAIN     = "main.star"
	LOADABLE = "lib.star"

	// LOADED is what LOADABLE holds: a function, and a lambda bound to a
	// name, which is no function a flow lists.
	LOADED = `
def g(x):
    pass

h = lambda x: x
`
)

// _Sources is a loader over scripts held in memory, by name.
type _Sources map[string]string

// Resolve reads target as a file beside from.
//
// Revisions:
//   - 2026-10-02 01:27: initial creation
func (s _Sources) Resolve(from string, target string) (string, error) {
	return path.Join(path.Dir(from), target), nil
}

// Load returns the script held under name.
//
// Revisions:
//   - 2026-10-02 01:27: initial creation
func (s _Sources) Load(name string) ([]byte, error) {
	src, ok := s[name]
	if !ok {
		return nil, fmt.Errorf("%s: %w", name, artifact.ERR_NO_UNIT)
	}

	return []byte(src), nil
}

// _Arity compiles a script whose main makes the one call given, beside the
// functions it may call, and returns what the compile said.
//
// Revisions:
//   - 2026-10-02 01:27: initial creation
func _Arity(call string) error {
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
    ` + call + `
`

	loader := _Sources{LOADABLE: LOADED}

	_, err := artifact.Compile(&script.Source{
		Entry:  MAIN,
		Text:   []byte(src),
		Loader: loader,
	})

	return err
}

// TestArity_RefusesACallItsFunctionDoesNotTake checks the calls refused before
// the run: too few, too many, a keyword with no parameter, one parameter given
// twice, a keyword-only parameter passed by position, and a loaded function
// passed nothing.
//
// Revisions:
//   - 2026-10-02 01:27: initial creation
func TestArity_RefusesACallItsFunctionDoesNotTake(t *testing.T) {
	refused := []string{
		"f()",
		"f(1, 2, 3)",
		"f(1, c = 2)",
		"f(1, a = 2)",
		"total = f() or 1",
		"only()",
		"only(1)",
		"g()",
	}

	for _, call := range refused {
		t.Run(call, func(t *testing.T) {
			err := _Arity(call)
			if !errors.Is(err, artifact.ERR_ARITY) {
				t.Fatalf("got %v, want ERR_ARITY", err)
			}
		})
	}
}

// TestArity_AcceptsWhatStarlarkBinds checks the calls that compile: a default
// left out or named, more than the parameters when a function takes *args or
// **kwargs, a call that unpacks, a name a parameter shadows, a loaded lambda,
// and a function the script does not define.
//
// Revisions:
//   - 2026-10-02 01:27: initial creation
func TestArity_AcceptsWhatStarlarkBinds(t *testing.T) {
	accepted := []string{
		"f(1)",
		"f(1, 2)",
		"f(a = 1)",
		"f(1, b = 2)",
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
		t.Run(call, func(t *testing.T) {
			err := _Arity(call)
			if err != nil {
				t.Fatalf("refused: %v", err)
			}
		})
	}
}

// TestArity_CountsOnlyTheFunctionTheNameMeans checks a parameter that shadows
// a function is not counted as the function.
//
// Revisions:
//   - 2026-10-02 01:27: initial creation
func TestArity_CountsOnlyTheFunctionTheNameMeans(t *testing.T) {
	src := `
def f(a):
    pass

def call(f):
    f()

def main():
    call(len)
`

	_, err := artifact.Compile(&script.Source{Entry: MAIN, Text: []byte(src)})
	if err != nil {
		t.Fatalf("refused: %v", err)
	}
}

// TestArity_SaysWhereTheCallIs checks the refusal opens with the call's file,
// line and column, as a parse or a resolve error does.
//
// Revisions:
//   - 2026-10-02 01:27: initial creation
func TestArity_SaysWhereTheCallIs(t *testing.T) {
	src := `
def f(a):
    pass

def main():
    f()
`

	_, err := artifact.Compile(&script.Source{Entry: MAIN, Text: []byte(src)})
	if err == nil || !strings.HasPrefix(err.Error(), MAIN+":6:5: call of f") {
		t.Fatalf("got %v, want a refusal at %s:6:5", err, MAIN)
	}
}
