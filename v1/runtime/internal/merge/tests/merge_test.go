package tests

import (
	"bytes"
	"errors"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
)

// _Files is a loader over a map: a module is a key, resolved beside the file
// that loads it, as script's default loader does for the file system.
//
// Revisions:
//   - 2026-11-03 06:29: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Files map[string]string

// Resolve joins target to the directory of from.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (f _Files) Resolve(
	from string,
	target string,
) (string, error) {
	return path.Join(
		path.Dir(from),
		target,
	), nil
}

// Load returns the text kept under name.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (f _Files) Load(name string) ([]byte, error) {
	text, found := f[name]
	if !found {
		return nil, os.ErrNotExist
	}

	return []byte(text), nil
}

// _Join is the lines of a script, each ending on a line break.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Join(lines ...string) string {
	return strings.Join(
		lines,
		"",
	)
}

// _Named is a plugin of the host's, with the names it gives.
//
// Revisions:
//   - 2026-11-03 06:29: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Named struct {
	names starlark.StringDict
}

// Name is what a conflict between plugins calls this one.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (n *_Named) Name() string {
	return "named"
}

// Values is the names this plugin gives.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (n *_Named) Values() starlark.StringDict {
	return n.names
}

var (
	MSG = regexp.MustCompile(`msg=("(?:[^"\\]|\\.)*"|\S+)`)
)

// _Log runs source and returns the lines it printed, with the times made equal
// and the lines sorted, since threads print in a free order.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Log(
	t *testing.T,
	source *common.Source,
	args map[string]any,
	opts []common.CompileOption,
) (string, error) {
	t.Helper()

	built, err := runtime.Compile(
		source,
		opts...,
	)
	if err != nil {
		return "", err
	}

	var out bytes.Buffer

	run, err := runtime.Start(
		t.Context(),
		built,
		runtime.WithLog(&out),
		runtime.WithArgs(args),
	)
	if err != nil {
		t.Fatal(err)
	}

	_, err = run.Wait()

	var said []string

	for _, found := range MSG.FindAllStringSubmatch(
		out.String(),
		-1,
	) {
		text, quoted := strconv.Unquote(found[1])
		if quoted != nil {
			text = found[1]
		}

		said = append(
			said,
			text,
		)
	}

	return strings.Join(
		said,
		"|",
	), err
}

// _Sorted is the lines _Log joined, in order, since threads print in a free
// order.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Sorted(said string) string {
	lines := strings.Split(
		said,
		"|",
	)
	sort.Strings(lines)

	return strings.Join(
		lines,
		"|",
	)
}

// _Merged is the merged source of main.star in files.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses runtime.Merge, formerly Amalgamate (C121, Q339)
func _Merged(
	t *testing.T,
	files _Files,
	opts ...common.CompileOption,
) *common.Source {
	t.Helper()

	merged, err := runtime.Merge(
		&common.Source{
			Entry:  "main.star",
			Text:   []byte(files["main.star"]),
			Loader: files,
		},
		opts...,
	)
	if err != nil {
		t.Fatalf(
			"merge: %v",
			err,
		)
	}

	return merged
}

// TestMerge_SameOutput states the rule of the feature: a script and its
// merge, run with a loader that refuses every load, print the same lines.
//
// Each row is a case of poc/research/amalgamate.md, section 2.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 23:01: renamed from TestAmalgamate_SameOutput (C121, Q353)
func TestMerge_SameOutput(t *testing.T) {
	cases := []struct {
		name  string
		out   string
		files _Files
		args  map[string]any
	}{
		{
			name: "plain load",
			out:  "A",
			files: _Files{
				"m.star": _Join(
					"def shout(w):\n",
					"    return w.upper()\n",
				),
				"main.star": _Join(
					"load(\"m.star\", \"shout\")\n",
					"def main():\n",
					"    print(shout(\"a\"))\n",
				),
			},
		},
		{
			name: "diamond runs the shared module once",
			out:  "init base|bl br",
			files: _Files{
				"base.star": _Join(
					"print(\"init base\")\n",
					"def b():\n",
					"    return \"b\"\n",
				),
				"l.star": _Join(
					"load(\"base.star\", \"b\")\n",
					"def l():\n",
					"    return b() + \"l\"\n",
				),
				"r.star": _Join(
					"load(\"base.star\", \"b\")\n",
					"def r():\n",
					"    return b() + \"r\"\n",
				),
				"main.star": _Join(
					"load(\"l.star\", \"l\")\n",
					"load(\"r.star\", \"r\")\n",
					"def main():\n",
					"    print(l(), r())\n",
				),
			},
		},
		{
			name: "a module loaded under two spellings runs once",
			out:  "init m|1 2",
			files: _Files{
				"m.star": _Join(
					"print(\"init m\")\n",
					"def f():\n",
					"    return 1\n",
				),
				"n.star": _Join(
					"load(\"./m.star\", \"f\")\n",
					"def g():\n",
					"    return f() + 1\n",
				),
				"main.star": _Join(
					"load(\"m.star\", \"f\")\n",
					"load(\"n.star\", \"g\")\n",
					"def main():\n",
					"    print(f(), g())\n",
				),
			},
		},
		{
			name: "module level statements run dependencies first",
			out:  "init a|init b|before load|init main|2",
			files: _Files{
				"a.star": _Join(
					"print(\"init a\")\n",
					"X = 1\n",
				),
				"b.star": _Join(
					"load(\"a.star\", \"X\")\n",
					"print(\"init b\")\n",
					"Y = X + 1\n",
				),
				"main.star": _Join(
					"print(\"before load\")\n",
					"load(\"b.star\", \"Y\")\n",
					"print(\"init main\")\n",
					"def main():\n",
					"    print(Y)\n",
				),
			},
		},
		{
			name: "constants",
			out:  `["x1", "x2"]`,
			files: _Files{
				"c.star": _Join(
					"PREFIX = \"x\"\n",
					"NAMES = [PREFIX + \"1\", PREFIX + \"2\"]\n",
				),
				"main.star": _Join(
					"load(\"c.star\", \"NAMES\")\n",
					"def main():\n",
					"    print(NAMES)\n",
				),
			},
		},
		{
			name: "arg declared in a module",
			out:  "db.internal",
			files: _Files{
				"cfg.star": "server = arg(\"host\", \"localhost\")\n",
				"main.star": _Join(
					"load(\"cfg.star\", \"server\")\n",
					"def main():\n",
					"    print(server)\n",
				),
			},
			args: map[string]any{"host": "db.internal"},
		},
		{
			name: "the same arg string in two modules",
			out:  "db.internal db.internal",
			files: _Files{
				"a.star": "ha = arg(\"host\", \"x\")\n",
				"b.star": "hb = arg(\"host\", \"y\")\n",
				"main.star": _Join(
					"load(\"a.star\", \"ha\")\n",
					"load(\"b.star\", \"hb\")\n",
					"def main():\n",
					"    print(ha, hb)\n",
				),
			},
			args: map[string]any{"host": "db.internal"},
		},
		{
			name: "collision: one name in two modules",
			out:  "a b",
			files: _Files{
				"a.star": _Join(
					"def helper():\n",
					"    return \"a\"\n",
					"def fa():\n",
					"    return helper()\n",
				),
				"b.star": _Join(
					"def helper():\n",
					"    return \"b\"\n",
					"def fb():\n",
					"    return helper()\n",
				),
				"main.star": _Join(
					"load(\"a.star\", \"fa\")\n",
					"load(\"b.star\", \"fb\")\n",
					"def main():\n",
					"    print(fa(), fb())\n",
				),
			},
		},
		{
			name: "collision with the entry",
			out:  "a main",
			files: _Files{
				"a.star": _Join(
					"def helper():\n",
					"    return \"a\"\n",
					"def fa():\n",
					"    return helper()\n",
				),
				"main.star": _Join(
					"load(\"a.star\", \"fa\")\n",
					"def helper():\n",
					"    return \"main\"\n",
					"def main():\n",
					"    print(fa(), helper())\n",
				),
			},
		},
		{
			name: "alias: a load that renames",
			out:  "A",
			files: _Files{
				"m.star": _Join(
					"def yell(w):\n",
					"    return w.upper()\n",
				),
				"main.star": _Join(
					"load(\"m.star\", shout = \"yell\")\n",
					"def main():\n",
					"    print(shout(\"a\"))\n",
				),
			},
		},
		{
			name: "alias used by the module itself",
			out:  "A BB",
			files: _Files{
				"m.star": _Join(
					"def yell(w):\n",
					"    return w.upper()\n",
					"def twice(w):\n",
					"    return yell(w) + yell(w)\n",
				),
				"main.star": _Join(
					"load(\"m.star\", shout = \"yell\", \"twice\")\n",
					"def main():\n",
					"    print(shout(\"a\"), twice(\"b\"))\n",
				),
			},
		},
		{
			name: "two loads alias one name differently",
			out:  "A N",
			files: _Files{
				"m.star": _Join(
					"def yell(w):\n",
					"    return w.upper()\n",
				),
				"n.star": _Join(
					"load(\"m.star\", cry = \"yell\")\n",
					"def fn():\n",
					"    return cry(\"n\")\n",
				),
				"main.star": _Join(
					"load(\"m.star\", shout = \"yell\")\n",
					"load(\"n.star\", \"fn\")\n",
					"def main():\n",
					"    print(shout(\"a\"), fn())\n",
				),
			},
		},
		{
			name: "alias whose local name a module also defines",
			out:  "A n",
			files: _Files{
				"m.star": _Join(
					"def yell(w):\n",
					"    return w.upper()\n",
				),
				"n.star": _Join(
					"def shout():\n",
					"    return \"n\"\n",
				),
				"main.star": _Join(
					"load(\"m.star\", shout = \"yell\")\n",
					"load(\"n.star\", other = \"shout\")\n",
					"def main():\n",
					"    print(shout(\"a\"), other())\n",
				),
			},
		},
		{
			name: "alias whose local name is a local of the defining module",
			out:  "A",
			files: _Files{
				"m.star": _Join(
					"def yell(w):\n",
					"    shout = w.upper()\n",
					"    return shout\n",
				),
				"main.star": _Join(
					"load(\"m.star\", shout = \"yell\")\n",
					"def main():\n",
					"    print(shout(\"a\"))\n",
				),
			},
		},
		{
			name: "private name in two modules",
			out:  "a b",
			files: _Files{
				"ma.star": _Join(
					"_fmt = \"a\"\n",
					"def fa():\n",
					"    return _fmt\n",
				),
				"mb.star": _Join(
					"_fmt = \"b\"\n",
					"def fb():\n",
					"    return _fmt\n",
				),
				"main.star": _Join(
					"load(\"ma.star\", \"fa\")\n",
					"load(\"mb.star\", \"fb\")\n",
					"def main():\n",
					"    print(fa(), fb())\n",
				),
			},
		},
		{
			name: "module shadows a builtin that main uses",
			out:  "module 2",
			files: _Files{
				"m.star": _Join(
					"def max(a, b):\n",
					"    return \"module\"\n",
					"def fm():\n",
					"    return max(1, 2)\n",
				),
				"main.star": _Join(
					"load(\"m.star\", \"fm\")\n",
					"def main():\n",
					"    print(fm(), max(1, 2))\n",
				),
			},
		},
		{
			name: "keyword names, parameters, lambdas and locals are not renamed",
			out:  `a ["kw", "loc", "b"]`,
			files: _Files{
				"a.star": _Join(
					"def helper():\n",
					"    return \"a\"\n",
					"def fa():\n",
					"    return helper()\n",
				),
				"b.star": _Join(
					"def helper():\n",
					"    return \"b\"\n",
				) +
					_Join(
						"def tag(helper):\n",
						"    return helper\n",
					) +
					_Join(
						"def local():\n",
						"    helper = \"loc\"\n",
						"    return helper\n",
					) +
					_Join(
						"def fb():\n",
						"    f = lambda: helper()\n",
						"    return [tag(helper = \"kw\"),\n",
						"        local(), f()]\n",
					),
				"main.star": _Join(
					"load(\"a.star\", \"fa\")\n",
					"load(\"b.star\", \"fb\")\n",
					"def main():\n",
					"    print(fa(), fb())\n",
				),
			},
		},
		{
			name: "a module defines main too",
			out:  "module main",
			files: _Files{
				"m.star": _Join(
					"def main():\n",
					"    return \"module main\"\n",
					"def fm():\n",
					"    return main()\n",
				),
				"main.star": _Join(
					"load(\"m.star\", \"fm\")\n",
					"def main():\n",
					"    print(fm())\n",
				),
			},
		},
		{
			name: "loop variable at top level in two modules",
			out:  "1 2",
			files: _Files{
				"a.star": _Join(
					"for i in range(2):\n",
					"    pass\n",
					"A = i\n",
				),
				"b.star": _Join(
					"for i in range(3):\n",
					"    pass\n",
					"B = i\n",
				),
				"main.star": _Join(
					"load(\"a.star\", \"A\")\n",
					"load(\"b.star\", \"B\")\n",
					"def main():\n",
					"    print(A, B)\n",
				),
			},
		},
		{
			name: "a name loaded from a module whose own name was renamed",
			out:  "a b",
			files: _Files{
				"a.star": _Join(
					"def helper():\n",
					"    return \"a\"\n",
				),
				"b.star": _Join(
					"def helper():\n",
					"    return \"b\"\n",
				),
				"c.star": _Join(
					"load(\"b.star\", \"helper\")\n",
					"def fc():\n",
					"    return helper()\n",
				),
				"main.star": _Join(
					"load(\"a.star\", \"helper\")\n",
					"load(\"c.star\", \"fc\")\n",
					"def main():\n",
					"    print(helper(), fc())\n",
				),
			},
		},
		{
			name: "a multi-line load keeps the lines after it",
			out:  "1 2",
			files: _Files{
				"m.star": _Join(
					"def f():\n",
					"    return 1\n",
					"def g():\n",
					"    return 2\n",
				),
				"main.star": _Join(
					"load(\n",
					"    \"m.star\",\n",
					"    \"f\",\n",
					"    \"g\",\n",
					")\n",
					"def main():\n",
					"    print(f(), g())\n",
				),
			},
		},
		{
			name: "one export under two spellings",
			out:  "f f",
			files: _Files{
				"m.star": _Join(
					"def f():\n",
					"    return \"f\"\n",
				),
				"main.star": _Join(
					"load(\"m.star\", a = \"f\", b = \"f\")\n",
					"def main():\n",
					"    print(a(), b())\n",
				),
			},
		},
		{
			name: "one export plain and under an alias nobody uses",
			out:  "f",
			files: _Files{
				"m.star": _Join(
					"def f():\n",
					"    return \"f\"\n",
				),
				"main.star": _Join(
					"load(\"m.star\", \"f\")\n",
					"load(\"m.star\", g = \"f\")\n",
					"def main():\n",
					"    print(f())\n",
				),
			},
		},
		{
			name: "no load at all",
			out:  "alone",
			files: _Files{
				"main.star": _Join(
					"def main():\n",
					"    print(\"alone\")\n",
				),
			},
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var (
					merged = _Merged(
						t,
						tc.files,
					)
					library = &common.Source{
						Entry:  "main.star",
						Text:   []byte(tc.files["main.star"]),
						Loader: tc.files,
					}
				)

				if strings.Contains(
					string(merged.Text),
					"load(",
				) {
					t.Fatalf(
						"a load is left in the merge:\n%s",
						merged.Text,
					)
				}

				// The text of the merge, and the tree Compile makes of the
				// script itself, must both print what the script means.
				for what, source := range map[string]*common.Source{
					"text": merged,
					"tree": library,
				} {
					got, err := _Log(
						t,
						source,
						tc.args,
						nil,
					)
					if err != nil {
						t.Fatalf(
							"the %s does not run: %v\n%s",
							what,
							err,
							merged.Text,
						)
					}

					if got != tc.out {
						t.Fatalf(
							"the %s prints\n%s\nwant\n%s\nmerged:\n%s",
							what,
							got,
							tc.out,
							merged.Text,
						)
					}
				}
			},
		)
	}
}

// TestMerge_Names states which spelling a global takes. A row lists the
// names the merged text must define and the names it must not.
//
// The Q15 rules: a collision no load names puts the module prefix on every
// side; an alias gives the global the local name; the entry keeps main.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 23:01: renamed from TestAmalgamate_Names (C121, Q353)
func TestMerge_Names(t *testing.T) {
	cases := []struct {
		name  string
		files _Files
		has   []string
		not   []string
	}{
		{
			name: "a collision puts the prefix on every side",
			files: _Files{
				"ma.star": _Join(
					"def _fmt():\n",
					"    return \"a\"\n",
					"def fa():\n",
					"    return _fmt()\n",
				),
				"mb.star": _Join(
					"def _fmt():\n",
					"    return \"b\"\n",
					"def fb():\n",
					"    return _fmt()\n",
				),
				"main.star": _Join(
					"load(\"ma.star\", \"fa\")\n",
					"load(\"mb.star\", \"fb\")\n",
					"def main():\n",
					"    print(fa(), fb())\n",
				),
			},
			has: []string{
				"def ma__fmt(",
				"def mb__fmt(",
				"return ma__fmt()",
				"return mb__fmt()",
			},
			not: []string{"def _fmt("},
		},
		{
			name: "an alias gives the global the local name",
			files: _Files{
				"m.star": _Join(
					"def yell(w):\n",
					"    return w.upper()\n",
				),
				"main.star": _Join(
					"load(\"m.star\", shout = \"yell\")\n",
					"def main():\n",
					"    print(shout(\"a\"))\n",
				),
			},
			has: []string{"def shout(", "print(shout("},
			not: []string{"yell"},
		},
		{
			name: "the entry keeps main and the module's main takes the prefix",
			files: _Files{
				"m.star": _Join(
					"def main():\n",
					"    return \"module main\"\n",
					"def fm():\n",
					"    return main()\n",
				),
				"main.star": _Join(
					"load(\"m.star\", \"fm\")\n",
					"def main():\n",
					"    print(fm())\n",
				),
			},
			has: []string{
				"def m_main(",
				"return m_main()",
				"def main():\n    print(fm())",
			},
		},
		{
			name: "a name no other file declares keeps its spelling",
			files: _Files{
				"m.star": _Join(
					"def shout(w):\n",
					"    return w.upper()\n",
				),
				"main.star": _Join(
					"load(\"m.star\", \"shout\")\n",
					"def main():\n",
					"    print(shout(\"a\"))\n",
				),
			},
			has: []string{"def shout(", "print(shout("},
		},
		{
			name: "a module global that a file reads as a builtin takes the prefix",
			files: _Files{
				"m.star": _Join(
					"def max(a, b):\n",
					"    return \"module\"\n",
					"def fm():\n",
					"    return max(1, 2)\n",
				),
				"main.star": _Join(
					"load(\"m.star\", \"fm\")\n",
					"def main():\n",
					"    print(fm(), max(1, 2))\n",
				),
			},
			has: []string{
				"def m_max(",
				"return m_max(1, 2)",
				"print(fm(), max(1, 2))",
			},
		},
		{
			name: "a prefixed name that is taken gets a number",
			files: _Files{
				"a.star": _Join(
					"def helper():\n",
					"    return \"a\"\n",
					"def a_helper():\n",
					"    return \"taken\" + helper()\n",
				),
				"b.star": _Join(
					"def helper():\n",
					"    return \"b\"\n",
				),
				"main.star": _Join(
					"load(\"a.star\", \"a_helper\")\n",
					"load(\"b.star\", \"helper\")\n",
					"def main():\n",
					"    print(a_helper(), helper())\n",
				),
			},
			has: []string{"def a_helper_2(", "def b_helper("},
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				text := string(_Merged(
					t,
					tc.files,
				).Text)

				for _, want := range tc.has {
					if !strings.Contains(
						text,
						want,
					) {
						t.Errorf(
							"merge lacks %q:\n%s",
							want,
							text,
						)
					}
				}

				for _, bad := range tc.not {
					if strings.Contains(
						text,
						bad,
					) {
						t.Errorf(
							"merge holds %q:\n%s",
							bad,
							text,
						)
					}
				}
			},
		)
	}
}

// TestMerge_Deterministic proves two merges of one script give one text.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 23:01: renamed from TestAmalgamate_Deterministic (C121, Q353)
func TestMerge_Deterministic(t *testing.T) {
	var (
		files = _Files{
			"ma.star": _Join(
				"def _fmt():\n",
				"    return \"a\"\n",
				"def fa():\n",
				"    return _fmt()\n",
			),
			"mb.star": _Join(
				"def _fmt():\n",
				"    return \"b\"\n",
				"def fb():\n",
				"    return _fmt()\n",
			),
			"main.star": _Join(
				"load(\"ma.star\", \"fa\")\n",
				"load(\"mb.star\", \"fb\")\n",
				"def main():\n",
				"    print(fa(), fb())\n",
			),
		}
		first = _Merged(
			t,
			files,
		).Text
	)

	for range 20 {
		if !bytes.Equal(
			_Merged(
				t,
				files,
			).Text,
			first,
		) {
			t.Fatal("two merges differ")
		}
	}
}

// TestMerge_Refusals states what a merge refuses, and with which value.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses runtime.Merge and ErrExport, formerly Amalgamate and ErrNoExport;
//     renamed from TestAmalgamate_Refusals (C121, Q339, Q348, Q353)
func TestMerge_Refusals(t *testing.T) {
	cases := []struct {
		name  string
		files _Files
		want  error
	}{
		{
			name: "a cycle",
			files: _Files{
				"a.star": _Join(
					"load(\"b.star\", \"fb\")\n",
					"def fa():\n",
					"    return fb()\n",
				),
				"b.star": _Join(
					"load(\"a.star\", \"fa\")\n",
					"def fb():\n",
					"    return fa()\n",
				),
				"main.star": _Join(
					"load(\"a.star\", \"fa\")\n",
					"def main():\n",
					"    print(fa())\n",
				),
			},
			want: runtime.ErrCycle,
		},
		{
			name: "a name the module does not define",
			files: _Files{
				"m.star": _Join(
					"def f():\n",
					"    return 1\n",
				),
				"main.star": _Join(
					"load(\"m.star\", \"g\")\n",
					"def main():\n",
					"    print(g())\n",
				),
			},
			want: runtime.ErrExport,
		},
		{
			name: "an alias whose substitute is a local of the loader",
			files: _Files{
				"m.star": _Join(
					"def yell(w):\n",
					"    shout = w\n",
					"    return shout.upper()\n",
				),
				"main.star": _Join(
					"load(\"m.star\", shout = \"yell\")\n",
					"def main():\n",
					"    yell = 1\n",
					"    print(shout(\"a\"), yell)\n",
				),
			},
			want: runtime.ErrCapture,
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				_, err := runtime.Merge(&common.Source{
					Entry:  "main.star",
					Text:   []byte(tc.files["main.star"]),
					Loader: tc.files,
				})
				if !errors.Is(
					err,
					tc.want,
				) {
					t.Fatalf(
						"got %v, want %v",
						err,
						tc.want,
					)
				}
			},
		)
	}
}

// TestMerge_UntrustedNames proves the merged text never holds a module's
// name: a name with a newline cannot break out of anything.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 23:01: renamed from TestAmalgamate_UntrustedNames (C121, Q353)
func TestMerge_UntrustedNames(t *testing.T) {
	var (
		files = _Files{
			"m\nprint(\"injected\").star": "def f():\n    return 1\n",
			"main.star": _Join(
				"load(\"m\\n",
				"print(\\\"injected\\\").star\", \"f\")\n",
				"def main():\n",
				"    print(f())\n",
			),
		}
		text = string(_Merged(
			t,
			files,
		).Text)
	)

	if strings.Contains(
		text,
		"injected",
	) {
		t.Fatalf(
			"the merge holds the module's name:\n%s",
			text,
		)
	}
}

// TestMerge_Frozen records the known divergence of Q15: globals freeze
// after the merged top level has run and before main is called, so a module may
// now write a list another module owns while the top level runs. Before the
// merge this script stopped with a frozen error; now it runs.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 23:01: renamed from TestAmalgamate_Frozen (C121, Q353)
func TestMerge_Frozen(t *testing.T) {
	var (
		files = _Files{
			"a.star": "items = []\n",
			"b.star": _Join(
				"load(\"a.star\", \"items\")\n",
				"items.append(1)\n",
				"B = items\n",
			),
			"main.star": _Join(
				"load(\"b.star\", \"B\")\n",
				"def main():\n",
				"    print(B)\n",
			),
		}
	)

	got, err := _Log(
		t,
		_Merged(
			t,
			files,
		),
		nil,
		nil,
	)
	if err != nil {
		t.Fatalf(
			"the merge should run: %v",
			err,
		)
	}

	if !strings.Contains(
		got,
		"[1]",
	) {
		t.Fatalf(
			"the merge printed %q",
			got,
		)
	}
}

// TestMerge_Plugins proves a name a host's plugin gives is known to the
// merge: a module's use of it stays, and without the plugin the merge refuses
// it as the compile does.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses runtime.Merge, formerly Amalgamate; renamed from
//     TestAmalgamate_Plugins (C121, Q339, Q353)
func TestMerge_Plugins(t *testing.T) {
	var (
		files = _Files{
			"m.star": _Join(
				"def f():\n",
				"    return greeting + \"!\"\n",
			),
			"main.star": _Join(
				"load(\"m.star\", \"f\")\n",
				"def main():\n",
				"    print(f(), greeting)\n",
			),
		}
		given = runtime.WithPlugins(&_Named{
			names: starlark.StringDict{"greeting": starlark.String("hello")},
		})
	)

	t.Run(
		"with the plugin",
		func(t *testing.T) {
			var (
				merged = _Merged(
					t,
					files,
					given,
				)
				got, err = _Log(
					t,
					merged,
					nil,
					[]common.CompileOption{given},
				)
			)
			if err != nil {
				t.Fatalf(
					"the merge does not run: %v",
					err,
				)
			}

			if !strings.Contains(
				got,
				"hello!",
			) {
				t.Fatalf(
					"got %q",
					got,
				)
			}
		},
	)

	t.Run(
		"a module declares the plugin's name",
		func(t *testing.T) {
			var clash = _Files{
				"m.star": _Join(
					"def greeting():\n",
					"    return \"module\"\n",
					"def f():\n",
					"    return greeting()\n",
				),
				"main.star": _Join(
					"load(\"m.star\", \"f\")\n",
					"def main():\n",
					"    print(f(), greeting)\n",
				),
			}

			var merged = _Merged(
				t,
				clash,
				given,
			)

			if !strings.Contains(
				string(merged.Text),
				"def m_greeting(",
			) {
				t.Fatalf(
					"the module's name keeps its spelling:\n%s",
					merged.Text,
				)
			}

			got, err := _Log(
				t,
				merged,
				nil,
				[]common.CompileOption{given},
			)
			if err != nil {
				t.Fatalf(
					"the merge does not run: %v",
					err,
				)
			}

			if !strings.Contains(
				got,
				"module",
			) || !strings.Contains(
				got,
				"hello",
			) {
				t.Fatalf(
					"got %q",
					got,
				)
			}
		},
	)

	t.Run(
		"without the plugin",
		func(t *testing.T) {
			_, err := runtime.Merge(&common.Source{
				Entry:  "main.star",
				Text:   []byte(files["main.star"]),
				Loader: files,
			})
			if err == nil {
				t.Fatal("the merge should refuse a name nothing gives")
			}
		},
	)
}

// TestMerge_LoaderRefuses proves the merged source carries a loader that
// refuses every load, so a load left in the merge would fail, not read a file.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 23:01: renamed from TestAmalgamate_LoaderRefuses (C121, Q353)
func TestMerge_LoaderRefuses(t *testing.T) {
	var (
		files = _Files{
			"m.star": _Join(
				"def f():\n",
				"    return 1\n",
			),
			"main.star": _Join(
				"load(\"m.star\", \"f\")\n",
				"def main():\n",
				"    print(f())\n",
			),
		}
		merged = _Merged(
			t,
			files,
		)
		_, err = merged.Loader.Load("m.star")
	)
	if err == nil {
		t.Fatal("the merged source's loader should refuse")
	}
}

// TestMerge_Samples proves a merged bundle runs as the unmerged script
// does, for every sample that has an entry point.
//
// clock.star needs a plugin of the host's and strings.star is a module; both
// are left to the golden check of poc/baseline.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
//   - 2026-11-07 18:50: the samples path gains one level after the move (C91w3)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 00:25: moved to tests/, with one more ".." in the samples path (C120a, Q311)
//   - 2026-11-10 17:15: uses runtime.Merge, formerly Amalgamate; renamed from
//     TestAmalgamate_Samples (C121, Q339, Q353)
func TestMerge_Samples(t *testing.T) {
	var (
		skipped    = []string{"clock.star", "strings.star"}
		files, err = filepath.Glob(filepath.Join(
			"..",
			"..",
			"..",
			"..",
			"..",
			"samples",
			"*.star",
		))
	)
	if err != nil || len(files) == 0 {
		t.Fatalf(
			"no samples found: %v",
			err,
		)
	}

	for _, file := range files {
		name := filepath.Base(file)
		if slices.Contains(
			skipped,
			name,
		) {
			continue
		}

		t.Run(
			name,
			func(t *testing.T) {
				text, err := os.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}

				var (
					source = func() *common.Source {
						return &common.Source{
							Entry: file,
							Text:  text,
						}
					}
					want, wantErr = _Log(
						t,
						source(),
						nil,
						nil,
					)
					merged, mergeErr = runtime.Merge(source())
				)
				if mergeErr != nil {
					t.Fatalf(
						"merge: %v",
						mergeErr,
					)
				}

				got, gotErr := _Log(
					t,
					merged,
					nil,
					nil,
				)

				if (wantErr == nil) != (gotErr == nil) {
					t.Fatalf(
						"the original ends with %v, the merge with %v",
						wantErr,
						gotErr,
					)
				}

				if _Sorted(got) != _Sorted(want) {
					t.Fatalf(
						"the merge prints\n%s\nthe original prints\n%s",
						got,
						want,
					)
				}
			},
		)
	}
}

// TestMerge_FrozenAtRun proves globals freeze before main is called, as
// plan Q15 says: a function that changes a list a top level built is refused.
// Without the freeze the append would succeed.
//
// Revisions:
//   - 2026-11-02 23:48: initial creation (C44a)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 23:01: renamed from TestAmalgamate_FrozenAtRun (C121, Q353)
func TestMerge_FrozenAtRun(t *testing.T) {
	_, err := _Log(
		t,
		&common.Source{
			Entry: "main.star",
			Text: []byte(_Join(
				"L = [1]\n",
				"def main():\n",
				"    L.append(2)\n",
			)),
		},
		nil,
		nil,
	)
	if err == nil || !strings.Contains(
		err.Error(),
		"frozen",
	) {
		t.Fatalf(
			"got %v, want a frozen error",
			err,
		)
	}
}
