package tests

import (
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/graph"
)

// DEMO is the script of plan Q163: one argument used, one not.
//
// Revisions:
//   - 2026-11-05 19:04: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
const (
	DEMO = `
host = arg("host", "localhost")
token = arg("token")
def main():
    print("connect", host)
`
)

// TestOf_ListsOnlyWhatARunReaches proves the flow applies the shake a compile
// applies (Q164): an unused argument, constant and function is not listed, a
// used one is, and so is whatever a used one reaches.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation (C44b, Q164)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestOf_ListsOnlyWhatARunReaches(t *testing.T) {
	cases := []struct {
		name   string
		files  map[string]string
		args   []string
		consts []string
		funcs  []string
	}{
		{
			name:  "the demo",
			files: map[string]string{"main.star": DEMO},
			args:  []string{"host"},
			funcs: nil,
		},
		{
			name: "an unused constant, argument and function",
			files: map[string]string{
				"main.star": strings.Join(
					[]string{
						"limit = 3\n",
						"spare = 4\n",
						"extra = arg(\"extra\", 1)\n",
						"def helper():\n",
						"    pass\n",
						"def dead():\n",
						"    pass\n",
						"def main():\n",
						"    helper()\n",
						"    print(limit)\n",
					}, "",
				),
			},
			consts: []string{"limit"},
			funcs:  []string{"helper"},
		},
		{
			name: "an unused x = f() goes with the f only it reaches",
			files: map[string]string{
				"main.star": strings.Join(
					[]string{
						"def f():\n",
						"    return 1\n",
						"x = f()\n",
						"def main():\n",
						"    print(2)\n",
					}, "",
				),
			},
			funcs: nil,
		},
		{
			name: "a module's unused definition",
			files: map[string]string{
				"m.star": strings.Join(
					[]string{
						"def used():\n",
						"    pass\n",
						"def spare():\n",
						"    pass\n",
						"unused = arg(\"u\", 1)\n",
					}, "",
				),
				"main.star": strings.Join(
					[]string{
						"load(\"m.star\", \"used\", \"spare\")\n",
						"def main():\n",
						"    used()\n",
					}, "",
				),
			},
			funcs: []string{"used"},
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				flow, err := graph.Of(&common.Source{
					Entry:  "main.star",
					Text:   []byte(tc.files["main.star"]),
					Loader: _Mapped(tc.files),
				})
				if err != nil {
					t.Fatal(err)
				}

				var funcs []string

				for _, fn := range flow.GetFunctions() {
					funcs = append(
						funcs,
						fn.GetName(),
					)
				}

				sort.Strings(funcs)

				if !slices.Equal(
					funcs,
					tc.funcs,
				) {
					t.Errorf(
						"functions %v, want %v",
						funcs,
						tc.funcs,
					)
				}

				if !slices.Equal(
					_Keys(flow.GetArgs()),
					tc.args,
				) {
					t.Errorf(
						"arguments %v, want %v",
						_Keys(flow.GetArgs()),
						tc.args,
					)
				}

				if !slices.Equal(
					_Keys(flow.GetConstants()),
					tc.consts,
				) {
					t.Errorf(
						"constants %v, want %v",
						_Keys(flow.GetConstants()),
						tc.consts,
					)
				}
			},
		)
	}
}

// TestOf_AgreesWithTheCompile proves the flow of a script lists the arguments
// a compile's merged text declares, no more and no fewer.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation (C44b, Q164)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses runtime.Merge, formerly Amalgamate (C121, Q339)
func TestOf_AgreesWithTheCompile(t *testing.T) {
	source := &common.Source{
		Entry: "main.star",
		Text:  []byte(DEMO),
	}

	var (
		flow, err = graph.Of(source)
	)
	if err != nil {
		t.Fatal(err)
	}

	merged, err := runtime.Merge(source)
	if err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"host", "token"} {
		var (
			listed = flow.GetArgs()[name] != nil
			merges = strings.Contains(
				string(merged.Text),
				name+" = arg(",
			)
		)

		if listed != merges {
			t.Errorf(
				"%s: the flow lists it %v, the merged text declares it %v",
				name,
				listed,
				merges,
			)
		}
	}
}

// _Keys is the keys of a map, sorted.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation (C44b, Q164)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Keys[V any](held map[string]V) []string {
	var keys []string

	for key := range held {
		keys = append(
			keys,
			key,
		)
	}

	sort.Strings(keys)

	return keys
}

// _Mapped is a loader over a map of file texts.
//
// Revisions:
//   - 2026-11-05 19:04: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Mapped map[string]string

// Resolve is target, since every module stands in one directory.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation (C44b, Q164)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (m _Mapped) Resolve(
	from string,
	target string,
) (string, error) {
	return target, nil
}

// Load is the text kept under name.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation (C44b, Q164)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (m _Mapped) Load(name string) ([]byte, error) {
	return []byte(m[name]), nil
}
