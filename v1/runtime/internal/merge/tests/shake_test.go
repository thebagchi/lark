package tests

import (
	"strings"
	"testing"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/script"
)

// TestShake_Text states what the merged text keeps and what it drops. Each row
// is a rule of plan Q15: only the definitions a run can reach stay, the
// statements with an effect stay, and the comments go.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation (C44b)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-12 22:09: a statement that shares a line is shaken alone (C130, B285, Q400)
func TestShake_Text(t *testing.T) {
	cases := []struct {
		name  string
		files _Files
		has   []string
		lacks []string
	}{
		{
			name: "an unused def is removed",
			files: _Files{
				"main.star": _Join(
					"def unused():\n",
					"    return 1\n",
					"def main():\n",
					"    print(\"a\")\n",
				),
			},
			has:   []string{"def main():"},
			lacks: []string{"unused"},
		},
		{
			name: "a def main reaches through other defs stays",
			files: _Files{
				"main.star": _Join(
					"def c():\n",
					"    return 3\n",
					"def b():\n",
					"    return c()\n",
					"def a():\n",
					"    return b()\n",
					"def dead():\n",
					"    return c()\n",
					"def main():\n",
					"    print(a())\n",
				),
			},
			has:   []string{"def a():", "def b():", "def c():"},
			lacks: []string{"def dead"},
		},
		{
			name: "a def only a dead def calls is removed",
			files: _Files{
				"main.star": _Join(
					"def helper():\n",
					"    return 1\n",
					"def dead():\n",
					"    return helper()\n",
					"def main():\n",
					"    print(2)\n",
				),
			},
			lacks: []string{"helper", "dead"},
		},
		{
			name: "a def handed to spawn, repeat, retry and timeout as a value stays",
			files: _Files{
				"main.star": _Join(
					"def s():\n",
					"    return 1\n",
					"def r():\n",
					"    return 2\n",
					"def t():\n",
					"    return 3\n",
					"def w():\n",
					"    return 4\n",
					"def main():\n",
					"    join(spawn(s))\n",
					"    repeat(1, r)\n",
					"    retry(1, t)\n",
					"    timeout(1000, w)\n",
				),
			},
			has: []string{"def s():", "def r():", "def t():", "def w():"},
		},
		{
			name: "a def named only in a string is removed",
			files: _Files{
				"main.star": _Join(
					"def f():\n",
					"    return 1\n",
					"def main():\n",
					"    print(\"f\")\n",
				),
			},
			has:   []string{"print(\"f\")"},
			lacks: []string{"def f"},
		},
		{
			name: "a def a lambda calls stays",
			files: _Files{
				"main.star": _Join(
					"def work(x):\n",
					"    return x\n",
					"def main():\n",
					"    join(spawn(lambda: work(1)))\n",
				),
			},
			has: []string{"def work(x):"},
		},
		{
			name: "a statement with an effect stays, with what it calls",
			files: _Files{
				"m.star": _Join(
					"def loud():\n",
					"    return \"init\"\n",
					"print(loud())\n",
					"def quiet():\n",
					"    return 1\n",
				),
				"main.star": _Join(
					"load(\"m.star\", \"quiet\")\n",
					"def main():\n",
					"    print(1)\n",
				),
			},
			has:   []string{"print(loud())", "def loud():"},
			lacks: []string{"def quiet"},
		},
		{
			name: "an unused arg declaration goes, and a used one stays",
			files: _Files{
				"main.star": _Join(
					"host = arg(\"host\", \"h\")\n",
					"token = arg(\"token\")\n",
					"def main():\n",
					"    print(host)\n",
				),
			},
			has:   []string{"host = arg(\"host\", \"h\")"},
			lacks: []string{"token"},
		},
		{
			name: "an unused load symbol is dropped and a used one stays",
			files: _Files{
				"m.star": _Join(
					"def used():\n",
					"    return 1\n",
					"def spare():\n",
					"    return 2\n",
				),
				"main.star": _Join(
					"load(\"m.star\", \"used\", \"spare\")\n",
					"def main():\n",
					"    print(used())\n",
				),
			},
			has:   []string{"def used():"},
			lacks: []string{"def spare", "load("},
		},
		{
			name: "a name loaded through a module that loads it stays",
			files: _Files{
				"base.star": _Join(
					"def deep():\n",
					"    return 1\n",
					"def other():\n",
					"    return 2\n",
				),
				"mid.star": _Join(
					"load(\"base.star\", \"deep\", \"other\")\n",
					"def through():\n",
					"    return deep()\n",
				),
				"main.star": _Join(
					"load(\"mid.star\", \"through\")\n",
					"def main():\n",
					"    print(through())\n",
				),
			},
			has:   []string{"def deep():", "def through():"},
			lacks: []string{"def other"},
		},
		{
			name: "a constant nobody reads is removed and one a def reads stays",
			files: _Files{
				"main.star": _Join(
					"UNUSED = [1, 2, {\"a\": -1}]\n",
					"USED = {\"k\": \"v\"}\n",
					"def main():\n",
					"    print(USED)\n",
				),
			},
			has:   []string{"USED = "},
			lacks: []string{"UNUSED"},
		},
		{
			name: "an unused x = f() and the f only it reaches are gone",
			files: _Files{
				"main.star": _Join(
					"def f():\n",
					"    return 1\n",
					"x = f()\n",
					"def main():\n",
					"    print(2)\n",
				),
			},
			has:   []string{"def main():"},
			lacks: []string{"def f", "x = f()"},
		},
		{
			name: "a used x = f() is kept with its f",
			files: _Files{
				"main.star": _Join(
					"def f():\n",
					"    return 1\n",
					"x = f()\n",
					"def main():\n",
					"    print(x)\n",
				),
			},
			has: []string{"def f():", "x = f()"},
		},
		{
			name: "a bare call f() is kept with its f",
			files: _Files{
				"main.star": _Join(
					"def f():\n",
					"    print(1)\n",
					"f()\n",
					"def main():\n",
					"    print(2)\n",
				),
			},
			has: []string{"def f():", "\nf()\n"},
		},
		{
			name: "an unused dict assignment goes",
			files: _Files{
				"main.star": _Join(
					"X = {\"a\": 1, \"a\": 2}\n",
					"def main():\n",
					"    print(1)\n",
				),
			},
			lacks: []string{"X"},
		},
		{
			name: "a tuple, a list and a paren target go together",
			files: _Files{
				"main.star": _Join(
					"A, B = 1, 2\n",
					"[C, D] = [3, 4]\n",
					"(E) = 5\n",
					"def main():\n",
					"    print(B)\n",
				),
			},
			has:   []string{"A, B = 1, 2"},
			lacks: []string{"C, D", "E"},
		},
		{
			name: "an unused lambda assignment goes, a call default included",
			files: _Files{
				"main.star": _Join(
					"F = lambda x = 1: x\n",
					"G = lambda x = len(\"a\"): x\n",
					"def main():\n",
					"    print(1)\n",
				),
			},
			lacks: []string{"F =", "G ="},
		},
		{
			name: "an assignment to an element stays",
			files: _Files{
				"main.star": _Join(
					"D = {}\n",
					"D[\"k\"] = 1\n",
					"def main():\n",
					"    print(1)\n",
				),
			},
			has: []string{"D = {}", "D[\"k\"] = 1"},
		},
		{
			name: "an unused read of a later name goes",
			files: _Files{
				"main.star": _Join(
					"A = B\n",
					"B = 1\n",
					"def main():\n",
					"    print(1)\n",
				),
			},
			lacks: []string{"A =", "B ="},
		},
		{
			name: "an entry with no main is not shaken",
			files: _Files{
				"main.star": _Join(
					"def lib():\n",
					"    return 1\n",
				),
			},
			has: []string{"def lib():"},
		},
		{
			name: "two unused statements sharing a line are both shaken",
			files: _Files{
				"main.star": _Join(
					"X = 1; Y = 2\n",
					"def main():\n",
					"    print(1)\n",
				),
			},
			lacks: []string{"X =", "Y ="},
		},
		{
			name: "a statement sharing a line is shaken alone, and the other stays",
			files: _Files{
				"main.star": _Join(
					"X = 1; Y = 2\n",
					"def main():\n",
					"    print(Y)\n",
				),
			},
			has:   []string{"Y = 2"},
			lacks: []string{"X ="},
		},
		{
			name: "a statement sharing a line with a call goes by its own reach",
			files: _Files{
				"main.star": _Join(
					"X = 1; print(2)\n",
					"def main():\n",
					"    print(3)\n",
				),
			},
			has:   []string{"print(2)"},
			lacks: []string{"X ="},
		},
		{
			name: "a hash inside a string is no comment",
			files: _Files{
				"main.star": _Join(
					"A = \"a # b\"\n",
					"B = 'c # d'\n",
					"C = \"\"\"e # f\n",
					"g # h\"\"\"\n",
					"D = r\"i # j\"\n",
					"def main():\n",
					"    print(A, B, C, D, \"#\")  # gone\n",
				),
			},
			has: []string{
				"\"a # b\"",
				"'c # d'",
				"e # f\ng # h",
				"r\"i # j\"",
				"print(A, B, C, D, \"#\")",
			},
			lacks: []string{"gone"},
		},
		{
			name: "every kind of comment is removed",
			files: _Files{
				"m.star": _Join(
					"# module header\n",
					"def f():  # suffix\n",
					"    # inside\n",
					"    return [  # in a list\n",
					"        1,\n",
					"    ]\n",
					"# trailing\n",
				),
				"main.star": _Join(
					"# before the load\n",
					"load(\n",
					"    \"m.star\",  # in the load\n",
					"    \"f\",\n",
					")\n",
					"    # indented\n",
					"def main():\n",
					"    print(f())\n",
					"# end of file",
				),
			},
			has:   []string{"def f():", "return ["},
			lacks: []string{"#", "header", "suffix", "inside", "trailing", "indented"},
		},
		{
			name: "a comment inside a def that is dropped goes with it",
			files: _Files{
				"main.star": _Join(
					"def dead():\n",
					"    # note\n",
					"    return 1  # more\n",
					"def main():\n",
					"    print(1)\n",
				),
			},
			lacks: []string{"#", "note", "more", "dead"},
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

				for _, bad := range tc.lacks {
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

// TestShake_Runs proves a shaken script does what the script does: the lines
// printed, from the compiled tree and from the rendered text, are the ones the
// author expects. Each row keeps a definition a run reaches by a path that is
// not a plain call.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation (C44b)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestShake_Runs(t *testing.T) {
	cases := []struct {
		name  string
		out   string
		files _Files
		args  map[string]any
	}{
		{
			name: "spawn, repeat, retry and timeout reach a def by its value",
			out:  "r|s|t|w",
			files: _Files{
				"main.star": _Join(
					"def s():\n",
					"    print(\"s\")\n",
					"def r():\n",
					"    print(\"r\")\n",
					"def t():\n",
					"    print(\"t\")\n",
					"def w():\n",
					"    print(\"w\")\n",
					"def dead():\n",
					"    print(\"dead\")\n",
					"def main():\n",
					"    repeat(1, r)\n",
					"    join(spawn(s))\n",
					"    retry(1, t)\n",
					"    timeout(5000, w)\n",
				),
			},
		},
		{
			name: "init statements use defs and constants",
			out:  "init 3|main",
			files: _Files{
				"m.star": _Join(
					"BASE = 1\n",
					"UNUSED = 9\n",
					"def add(x):\n",
					"    return x + BASE\n",
					"print(\"init\", add(2))\n",
				),
				"main.star": _Join(
					"load(\"m.star\", \"add\", \"UNUSED\")\n",
					"def main():\n",
					"    print(\"main\")\n",
				),
			},
		},
		{
			name: "an arg declaration binds a name a def reads",
			out:  "db",
			files: _Files{
				"main.star": _Join(
					"server = arg(\"host\", \"x\")\n",
					"def show():\n",
					"    print(server)\n",
					"def main():\n",
					"    show()\n",
				),
			},
			args: map[string]any{"host": "db"},
		},
		{
			name: "a def under a renamed spelling",
			out:  "a|b",
			files: _Files{
				"ma.star": _Join(
					"def fmt():\n",
					"    return \"a\"\n",
					"def drop():\n",
					"    return 0\n",
				),
				"mb.star": _Join(
					"def fmt():\n",
					"    return \"b\"\n",
				),
				"main.star": _Join(
					"load(\"ma.star\", a = \"fmt\")\n",
					"load(\"mb.star\", b = \"fmt\")\n",
					"def main():\n",
					"    print(a() + \"|\" + b())\n",
				),
			},
		},
		{
			name: "a string that names a def does not run it",
			out:  "f",
			files: _Files{
				"main.star": _Join(
					"def f():\n",
					"    print(\"ran\")\n",
					"def main():\n",
					"    print(\"f\")\n",
				),
			},
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				source := &common.Source{
					Entry:  "main.star",
					Text:   []byte(tc.files["main.star"]),
					Loader: tc.files,
				}

				var (
					got, err = _Log(
						t,
						source,
						tc.args,
						nil,
					)
				)
				if err != nil {
					t.Fatalf(
						"compiled tree: %v",
						err,
					)
				}

				if _Sorted(got) != _Sorted(tc.out) {
					t.Errorf(
						"compiled tree prints %q, want %q",
						got,
						tc.out,
					)
				}

				got, err = _Log(
					t,
					_Merged(
						t,
						tc.files,
					),
					tc.args,
					nil,
				)
				if err != nil {
					t.Fatalf(
						"text: %v",
						err,
					)
				}

				if _Sorted(got) != _Sorted(tc.out) {
					t.Errorf(
						"text prints %q, want %q",
						got,
						tc.out,
					)
				}
			},
		)
	}
}

// TestShake_Lines proves a kept line keeps its number: the merged text has the
// lines of the files, one after the other, and a line of a file is either
// itself or blank.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation (C44b)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestShake_Lines(t *testing.T) {
	files := _Files{
		"m.star": _Join(
			"# header\n",
			"def dead():  # note\n",
			"    return 1\n",
			"\n",
			"def live():\n",
			"    return 2  # why\n",
			"unused = live(\n",
			"    # inside\n",
			")\n",
			"print(\"m\")\n",
		),
		"main.star": _Join(
			"load(\"m.star\", \"live\", \"dead\")\n",
			"# comment\n",
			"X = [\n",
			"    1,\n",
			"]\n",
			"def main():\n",
			"    print(live())\n",
		),
	}

	var (
		text = string(_Merged(
			t,
			files,
		).Text)
		merged = strings.Split(
			text,
			"\n",
		)
		want = strings.Split(
			files["m.star"]+files["main.star"],
			"\n",
		)
	)

	if len(merged) != len(want) {
		t.Fatalf(
			"merge has %d lines, the files %d:\n%s",
			len(merged),
			len(want),
			text,
		)
	}

	for idx, line := range merged {
		line = strings.TrimRight(
			line,
			" ",
		)
		if line == "" {
			continue
		}

		code, _, _ := strings.Cut(
			want[idx],
			"  #",
		)
		if line != want[idx] && line != code {
			t.Errorf(
				"line %d is %q, the files have %q",
				idx+1,
				line,
				want[idx],
			)
		}
	}
}

// TestShake_ErrorLine proves a failure names the line its author wrote it at,
// in the entry and in a module, with a def shaken out and comments above it.
//
// The numbers are those the same scripts report on the base, before C44b.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation (C44b)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestShake_ErrorLine(t *testing.T) {
	cases := []struct {
		name  string
		want  string
		files _Files
	}{
		{
			name: "the entry",
			want: "main.star:6:",
			files: _Files{
				"main.star": _Join(
					"# one\n",
					"def dead():\n",
					"    return 1\n",
					"# four\n",
					"def main():\n",
					"    fail(\"boom\")\n",
				),
			},
		},
		{
			name: "a module",
			want: "m.star:5:",
			files: _Files{
				"m.star": _Join(
					"# one\n",
					"def dead():\n",
					"    return 1\n",
					"def bad():\n",
					"    fail(\"boom\")\n",
				),
				"main.star": _Join(
					"load(\"m.star\", \"bad\")\n",
					"def main():\n",
					"    bad()\n",
				),
			},
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				_, err := _Log(
					t,
					&common.Source{
						Entry:  "main.star",
						Text:   []byte(tc.files["main.star"]),
						Loader: tc.files,
					},
					nil,
					nil,
				)
				if err == nil || !strings.Contains(
					err.Error(),
					tc.want,
				) {
					t.Errorf(
						"got %v, want a failure at %s",
						err,
						tc.want,
					)
				}
			},
		)
	}
}

// TestShake_OddText proves a text the scanner reads in an odd way is merged
// whole: a NUL ends the file for the parser. Cutting it by line feeds would leave
// a script that differs from the one that ran. A lone carriage return is a line
// break to the parser too, and is written as a line feed before the merge reads
// the text (Q405), so the rows with one are merged like any other text. Both
// found by FuzzMerge.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation (C44b)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses runtime.Merge, formerly Amalgamate (C121, Q339)
//   - 2026-11-13 00:04: a NUL row, and the doc says a lone return is normalised (C130, Q405)
func TestShake_OddText(t *testing.T) {
	cases := []struct {
		name string
		text string
	}{
		{
			"a NUL inside a comment",
			"#\x00\nA",
		},
		{
			"a NUL after the statements",
			"def dead():\n    pass\ndef main():\n    pass\n\x00def tail():\n    pass",
		},
		{
			"a lone carriage return inside brackets",
			"(#\r)",
		},
		{
			"a lone carriage return between statements",
			"def dead():\r    pass\rdef main():\r    pass\r",
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var (
					source = &common.Source{
						Entry: "main.star",
						Text:  []byte(tc.text),
					}
					_, err = runtime.Compile(source)
				)
				if err != nil {
					t.Fatalf(
						"the file does not compile: %v",
						err,
					)
				}

				merged, err := runtime.Merge(source)
				if err != nil {
					t.Fatal(err)
				}

				_, err = runtime.Compile(&common.Source{
					Entry:  "main.star",
					Text:   merged.Text,
					Loader: merged.Loader,
				})
				if err != nil {
					t.Fatalf(
						"the merge does not compile: %v",
						err,
					)
				}
			},
		)
	}
}

// FuzzMerge proves, for any one-file text that compiles, that the merge
// succeeds, writes a text that compiles, and has as many lines as the file had.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation (C44b, CLAUDE.md 1.10)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses runtime.Merge, formerly Amalgamate (C121, Q339)
//   - 2026-11-13 00:02: counts the lines of the file as Normal writes them (C130, Q405)
func FuzzMerge(f *testing.F) {
	for _, seed := range []string{
		"def main():\n    pass\n",
		"def dead():\n    return 1  # c\ndef main():\n    print(\"#\")\n",
		"X = {\"a\": 1}\nY = [1]\ndef main():\n    print(X)\n",
		"a = 1; b = 2\ndef main(): pass\n",
		"# only a comment",
		"def f():\n    return (1,\n        2)\nprint(f())\n",
		"s = '''x\n# y'''\ndef main():\n    print(s)\n",
	} {
		f.Add(seed)
	}

	f.Fuzz(func(
		t *testing.T,
		text string,
	) {
		var (
			source = &common.Source{
				Entry: "main.star",
				Text:  []byte(text),
			}
			_, err = runtime.Compile(source)
		)
		if err != nil {
			return
		}

		merged, err := runtime.Merge(source)
		if err != nil {
			t.Fatalf(
				"the file compiles and the merge does not: %v",
				err,
			)
		}

		_, err = runtime.Compile(&common.Source{
			Entry:  "main.star",
			Text:   merged.Text,
			Loader: merged.Loader,
		})
		if err != nil {
			t.Fatalf(
				"the merge does not compile: %v\n%s",
				err,
				merged.Text,
			)
		}

		var (
			normal = string(script.Normal([]byte(text)))
			want   = strings.Count(
				normal,
				"\n",
			)
			got = strings.Count(
				string(merged.Text),
				"\n",
			)
		)

		if !strings.HasSuffix(
			normal,
			"\n",
		) {
			want++
		}

		if got != want {
			t.Fatalf(
				"the merge has %d lines, the file %d:\n%s",
				got,
				want,
				merged.Text,
			)
		}
	})
}
