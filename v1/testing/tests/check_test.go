// Probes for the compiler skip and for store Check edges the product
// tests do not name.
//
// Revisions:
//   - 2026-09-24 21:14: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
package tests

import (
	"context"
	"errors"
	"path"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/state"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

var (
	ErrSpelling = errors.New("refused on spelling")
	ErrFirst    = errors.New("first checker refused")
	ErrSecond   = errors.New("second checker refused")
	ErrPair     = errors.New("pair refused")
)

const (
	SPELLING = "spelling"
	ALPHA    = "alpha"
	BETA     = "beta"
	SILENT   = "silent"
)

// _Files is a loader over a map of path to source.
//
// Revisions:
//   - 2026-09-24 21:14: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Files struct {
	files map[string]string
}

// Resolve treats target as already resolved.
//
// Revisions:
//   - 2026-09-24 21:06: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (f *_Files) Resolve(
	from string,
	target string,
) (string, error) {
	return path.Clean(target), nil
}

// Load reads the mapped source.
//
// Revisions:
//   - 2026-09-24 21:06: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (f *_Files) Load(name string) ([]byte, error) {
	src, ok := f.files[name]
	if !ok {
		return nil, errors.New("missing " + name)
	}

	return []byte(src), nil
}

// _Spelling refuses every tree, so a skip is the only way a compile succeeds.
//
// Revisions:
//   - 2026-09-24 21:14: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Spelling struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-24 21:06: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (s *_Spelling) Name() string {
	return SPELLING
}

// Values supplies the one name.
//
// Revisions:
//   - 2026-09-24 21:06: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (s *_Spelling) Values() starlark.StringDict {
	return starlark.StringDict{
		SPELLING: starlark.None,
	}
}

// Check refuses any file at all.
//
// Revisions:
//   - 2026-09-24 21:06: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (s *_Spelling) Check(tree *syntax.File) error {
	return ErrSpelling
}

// _Pair supplies two names and refuses every tree.
//
// Revisions:
//   - 2026-09-24 21:14: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Pair struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-24 21:06: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (p *_Pair) Name() string {
	return "pair"
}

// Values supplies two names.
//
// Revisions:
//   - 2026-09-24 21:06: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (p *_Pair) Values() starlark.StringDict {
	return starlark.StringDict{
		ALPHA: starlark.None,
		BETA:  starlark.None,
	}
}

// Check refuses any file at all.
//
// Revisions:
//   - 2026-09-24 21:06: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (p *_Pair) Check(tree *syntax.File) error {
	return ErrPair
}

// _First refuses every tree with ErrFirst.
//
// Revisions:
//   - 2026-09-24 21:14: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _First struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-24 21:06: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (f *_First) Name() string {
	return "first"
}

// Values supplies one name.
//
// Revisions:
//   - 2026-09-24 21:06: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (f *_First) Values() starlark.StringDict {
	return starlark.StringDict{"one": starlark.None}
}

// Check refuses any file at all.
//
// Revisions:
//   - 2026-09-24 21:06: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (f *_First) Check(tree *syntax.File) error {
	return ErrFirst
}

// _Second refuses every tree with ErrSecond.
//
// Revisions:
//   - 2026-09-24 21:14: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Second struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-24 21:06: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (s *_Second) Name() string {
	return "second"
}

// Values supplies one name.
//
// Revisions:
//   - 2026-09-24 21:06: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (s *_Second) Values() starlark.StringDict {
	return starlark.StringDict{"two": starlark.None}
}

// Check refuses any file at all.
//
// Revisions:
//   - 2026-09-24 21:06: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (s *_Second) Check(tree *syntax.File) error {
	return ErrSecond
}

// _Quiet supplies a name and does not implement Checking.
//
// Revisions:
//   - 2026-09-24 21:14: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Quiet struct{}

// Name is what this plugin is called when a conflict has to name it.
//
// Revisions:
//   - 2026-09-24 21:06: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (q *_Quiet) Name() string {
	return SILENT
}

// Values supplies one name.
//
// Revisions:
//   - 2026-09-24 21:06: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (q *_Quiet) Values() starlark.StringDict {
	return starlark.StringDict{SILENT: starlark.None}
}

// _Compiled compiles src and returns any error.
//
// Revisions:
//   - 2026-10-24 16:30: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Compiled(
	t *testing.T,
	src string,
) error {
	t.Helper()

	_, err := runtime.Compile(&common.Source{
		Entry: "probe.star",
		Text:  []byte(src),
	})

	return err
}

// _Ran compiles and runs src, returning any error from either step.
//
// Revisions:
//   - 2026-10-24 16:30: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Ran(
	t *testing.T,
	src string,
) error {
	t.Helper()

	built, err := runtime.Compile(&common.Source{
		Entry: "probe.star",
		Text:  []byte(src),
	})
	if err != nil {
		return err
	}

	run, err := runtime.Start(
		context.Background(),
		built,
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = run.Wait()

	return err
}

// TestSet_NestedDefIsRefusedWhenItRuns records that a nested def is not
// visible to the source check.
//
// Revisions:
//   - 2026-09-24 21:06: initial creation
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrConvert, formerly ErrNotProtoValue (C121, Q349)
func TestSet_NestedDefIsRefusedWhenItRuns(t *testing.T) {
	src := `
def main():
    def helper():
        return 1
    state.set("k", helper)
`

	if err := _Compiled(
		t,
		src,
	); err != nil {
		t.Fatalf(
			"compile: %v",
			err,
		)
	}

	err := _Ran(
		t,
		src,
	)
	if !errors.Is(
		err,
		unpack.ErrConvert,
	) {
		t.Fatalf(
			"run: %v, want ErrConvert",
			err,
		)
	}
}

// TestSet_MainIsRefusedAtCompile is a top-level def like any other.
//
// Revisions:
//   - 2026-09-24 21:06: initial creation
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrConvert, formerly ErrNotProtoValue (C121, Q349)
func TestSet_MainIsRefusedAtCompile(t *testing.T) {
	err := _Compiled(
		t,
		`
def main():
    state.set("k", main)
`,
	)
	if !errors.Is(
		err,
		unpack.ErrConvert,
	) {
		t.Fatalf(
			"compile: %v, want ErrConvert",
			err,
		)
	}
}

// TestSet_KeywordFormIsRefusedWhenItRuns records that the source check
// only sees two positional arguments.
//
// Revisions:
//   - 2026-09-24 21:06: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestSet_KeywordFormIsRefusedWhenItRuns(t *testing.T) {
	var (
		src = `
def helper():
    return 1

def main():
    state.set("k", value=helper)
`
		err = _Compiled(
			t,
			src,
		)
	)
	t.Logf(
		"compile: %v",

		err,
	)

	if err != nil {
		return
	}

	err = _Ran(
		t,
		src,
	)
	t.Logf(

		"run: %v",
		err,
	)

	if err == nil {
		t.Fatal("keyword form was accepted")
	}
}

// TestSet_ALoadedFileIsChecked compiles the loaded unit too.
//
// Revisions:
//   - 2026-09-24 21:06: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrConvert, formerly ErrNotProtoValue (C121, Q349)
func TestSet_ALoadedFileIsChecked(t *testing.T) {
	var (
		loader = &_Files{
			files: map[string]string{
				"lib.star": `
def helper():
    return 1

def store():
    state.set("k", helper)
`,
			},
		}
		_, err = runtime.Compile(&common.Source{
			Entry: "app.star",
			Text: []byte(`
load("lib.star", "store")

def main():
    store()
`),
			Loader: loader,
		})
	)
	if !errors.Is(
		err,
		unpack.ErrConvert,
	) {
		t.Fatalf(
			"compile: %v, want ErrConvert from the loaded file",
			err,
		)
	}
}

// TestSet_ADictHoldingAFunctionIsRefusedWhenItRuns is a container the
// table does not have.
//
// Revisions:
//   - 2026-09-24 21:06: initial creation
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrConvert, formerly ErrNotProtoValue (C121, Q349)
func TestSet_ADictHoldingAFunctionIsRefusedWhenItRuns(t *testing.T) {
	err := _Ran(
		t,
		`
def helper():
    return 1

def main():
    state.set("k", {"a": helper})
`,
	)
	if !errors.Is(
		err,
		unpack.ErrConvert,
	) {
		t.Fatalf(
			"run: %v, want ErrConvert",
			err,
		)
	}
}

// TestSet_AnUpdateReturningAHandleStopsTheRun is a handle the callback
// produced, not one passed to set.
//
// Revisions:
//   - 2026-09-24 21:06: initial creation
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrConvert, formerly ErrNotProtoValue (C121, Q349)
func TestSet_AnUpdateReturningAHandleStopsTheRun(t *testing.T) {
	err := _Ran(
		t,
		`
def worker():
    return 1

def main():
    state.update("k", lambda v: spawn(worker))
`,
	)
	if !errors.Is(
		err,
		unpack.ErrConvert,
	) {
		t.Fatalf(
			"run: %v, want ErrConvert",
			err,
		)
	}
}

// TestCheck_ADefOfTheNameIsASkip is the product test's assignment, as a def.
//
// Revisions:
//   - 2026-09-24 21:06: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestCheck_ADefOfTheNameIsASkip(t *testing.T) {
	var (
		plugins = runtime.WithPlugins(new(_Spelling))
		_, err  = runtime.Compile(
			&common.Source{
				Entry: "taken.star",
				Text: []byte(`
def spelling():
    return 1

def main():
    return spelling()
`),
			},
			plugins,
		)
	)
	if err != nil {
		t.Fatalf(
			"a file that took the name by a def was refused: %v",
			err,
		)
	}
}

// TestCheck_TakingOneNameSkipsTheWholePlugin is the conservative skip.
//
// Revisions:
//   - 2026-09-24 21:06: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestCheck_TakingOneNameSkipsTheWholePlugin(t *testing.T) {
	var (
		plugins = runtime.WithPlugins(new(_Pair))
		_, err  = runtime.Compile(
			&common.Source{
				Entry: "uses.star",
				Text:  []byte("def main():\n    return 1\n"),
			},
			plugins,
		)
	)
	if !errors.Is(
		err,
		ErrPair,
	) {
		t.Fatalf(
			"a file that took neither name: %v, want the plugin asked",
			err,
		)
	}

	_, err = runtime.Compile(
		&common.Source{
			Entry: "taken.star",
			Text: []byte(`
alpha = 1

def main():
    return 1
`),
		},
		plugins,
	)
	if err != nil {
		t.Fatalf(
			"a file that took one of two names was refused: %v",
			err,
		)
	}
}

// TestCheck_APluginThatDoesNotCheckIsLeftAlone is the optional seam.
//
// Revisions:
//   - 2026-09-24 21:06: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestCheck_APluginThatDoesNotCheckIsLeftAlone(t *testing.T) {
	var (
		plugins = runtime.WithPlugins(
			new(_Quiet),
			new(_Spelling),
		)
		_, err = runtime.Compile(
			&common.Source{
				Entry: "uses.star",
				Text:  []byte("def main():\n    return 1\n"),
			},
			plugins,
		)
	)
	if !errors.Is(
		err,
		ErrSpelling,
	) {
		t.Fatalf(
			"got %v, want the checking plugin asked",
			err,
		)
	}
}

// TestCheck_TheFirstRefusalStopsTheWalk is two checkers, first refuses.
//
// Revisions:
//   - 2026-09-24 21:06: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestCheck_TheFirstRefusalStopsTheWalk(t *testing.T) {
	var (
		plugins = runtime.WithPlugins(
			new(_First),
			new(_Second),
		)
		_, err = runtime.Compile(
			&common.Source{
				Entry: "uses.star",
				Text:  []byte("def main():\n    return 1\n"),
			},
			plugins,
		)
	)
	if !errors.Is(
		err,
		ErrFirst,
	) {
		t.Fatalf(
			"got %v, want the first checker",
			err,
		)
	}

	if errors.Is(
		err,
		ErrSecond,
	) {
		t.Fatal("the second checker was asked after the first refused")
	}
}

// TestCheck_ALoadAliasTakesTheLocalName is a fifth way to bind a global.
//
// Revisions:
//   - 2026-09-24 21:06: initial creation
//   - 2026-10-02 13:12: hands the compiler the plugin, there being no registry
//     to build
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestCheck_ALoadAliasTakesTheLocalName(t *testing.T) {
	var (
		loader = &_Files{
			files: map[string]string{
				// The loaded file must bind the name too, or a Check that
				// refuses every tree refuses the unit before the load is
				// even considered.
				"lib.star": "exported = 1\nspelling = 1\n",
			},
		}
		plugins = runtime.WithPlugins(new(_Spelling))
		_, err  = runtime.Compile(
			&common.Source{
				Entry: "app.star",
				Text: []byte(`
load("lib.star", spelling="exported")

def main():
    return spelling
`),
				Loader: loader,
			},
			plugins,
		)
	)
	if err != nil {
		t.Fatalf(
			"a load alias of the name was refused: %v",
			err,
		)
	}
}

// TestCheck_ANestedDestructureTakesTheName is a tuple inside a tuple.
//
// Revisions:
//   - 2026-09-24 21:06: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestCheck_ANestedDestructureTakesTheName(t *testing.T) {
	var (
		plugins = runtime.WithPlugins(new(_Spelling))
		_, err  = runtime.Compile(
			&common.Source{
				Entry: "taken.star",
				Text: []byte(`
(spelling, (a, b)) = (1, (2, 3))

def main():
    return spelling
`),
			},
			plugins,
		)
	)
	if err != nil {
		t.Fatalf(
			"a nested destructure of the name was refused: %v",
			err,
		)
	}
}

// TestCheck_ALoopDestructureTakesTheName is a for unpacking two names.
//
// Revisions:
//   - 2026-09-24 21:06: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestCheck_ALoopDestructureTakesTheName(t *testing.T) {
	var (
		plugins = runtime.WithPlugins(new(_Spelling))
		_, err  = runtime.Compile(
			&common.Source{
				Entry: "taken.star",
				Text: []byte(`
for spelling, x in [(1, 2)]:
    pass

def main():
    return 1
`),
			},
			plugins,
		)
	)
	if err != nil {
		t.Fatalf(
			"a loop destructure of the name was refused: %v",
			err,
		)
	}
}

// TestSet_AParenthesizedFunctionIsVisible is the source still plainly a
// function, with extra parens.
//
// Revisions:
//   - 2026-09-24 21:08: initial creation
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrConvert, formerly ErrNotProtoValue (C121, Q349)
func TestSet_AParenthesizedFunctionIsVisible(t *testing.T) {
	var (
		src = `
def helper():
    return 1

def main():
    state.set("k", (helper))
`
		err = _Compiled(
			t,
			src,
		)
	)

	t.Logf(
		"compile: %v",
		err,
	)

	if err == nil {
		t.Logf(
			"run: %v",
			_Ran(
				t,
				src,
			),
		)
		t.Fatal("parenthesized helper compiled; want ErrConvert at compile")
	}

	if !errors.Is(
		err,
		unpack.ErrConvert,
	) {
		t.Fatalf(
			"compile: %v, want ErrConvert",
			err,
		)
	}
}

// TestSet_AStarredCallIsRefusedWhenItRuns records the splat form.
//
// Revisions:
//   - 2026-09-24 21:08: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestSet_AStarredCallIsRefusedWhenItRuns(t *testing.T) {
	var (
		src = `
def helper():
    return 1

def main():
    state.set(*["k", helper])
`
		err = _Compiled(
			t,
			src,
		)
	)
	t.Logf(
		"star compile: %v",
		err,
	)

	if err != nil {
		return
	}

	err = _Ran(
		t,

		src,
	)
	t.Logf(
		"star run: %v",
		err,
	)

	if err == nil {
		t.Fatal("starred set of a function was accepted")
	}
}

// TestSet_ABuiltinIsRefusedWhenItRuns is a function the file did not declare.
//
// Revisions:
//   - 2026-09-24 21:08: initial creation
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrConvert, formerly ErrNotProtoValue (C121, Q349)
func TestSet_ABuiltinIsRefusedWhenItRuns(t *testing.T) {
	src := `
def main():
    state.set("k", len)
`

	if err := _Compiled(
		t,
		src,
	); err != nil {
		t.Fatalf(
			"compile: %v",
			err,
		)
	}

	err := _Ran(
		t,
		src,
	)
	if !errors.Is(
		err,
		unpack.ErrConvert,
	) {
		t.Fatalf(
			"run: %v, want ErrConvert",
			err,
		)
	}
}

// TestCheck_AComprehensionDoesNotTakeTheName records whether a
// comprehension variable is a global.
//
// Revisions:
//   - 2026-09-24 21:06: initial creation
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestCheck_AComprehensionDoesNotTakeTheName(t *testing.T) {
	var (
		plugins = runtime.WithPlugins(new(_Spelling))
		_, err  = runtime.Compile(
			&common.Source{
				Entry: "comp.star",
				Text: []byte(`
xs = [spelling for spelling in [1]]

def main():
    return xs
`),
			},

			plugins,
		)
	)
	t.Logf(
		"comprehension compile: %v",
		err,
	)

	if err == nil {
		t.Fatal("a comprehension variable skipped the check; want it asked")
	}

	if !errors.Is(
		err,
		ErrSpelling,
	) {
		t.Fatalf(
			"got %v, want the plugin asked",
			err,
		)
	}
}
