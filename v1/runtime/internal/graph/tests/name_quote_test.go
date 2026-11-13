package tests

import (
	"strconv"
	"strings"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/graph"
)

const (
	QUOTE_ENTRY = "main.star"
	QUOTE_MAX   = 2000
)

// TestRefusal_ModuleNameIsQuoted proves a module name a script chose, with a
// line break or with 5000 bytes, stays one quoted and bounded piece of text in
// the error of a module that does not parse (Q4, B128). The refusal for a load
// whose module is no string (ErrUnresolved) cannot be reached: the parser refuses
// that load first.
//
// Revisions:
//   - 2026-11-05 23:40: initial creation (C63, B128)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestRefusal_ModuleNameIsQuoted(t *testing.T) {
	var (
		long = strings.Repeat(
			"a",
			5000,
		) + ".star"
		tests = []struct {
			name   string
			module string
			text   string
		}{
			{
				"parse, line break",
				"a\nb.star",
				"def (:\n",
			},
			{
				"parse, long name",
				long,
				"def (:\n",
			},
			{
				"parse, load operand, line break",
				"a\nb.star",
				"load(1, \"x\")\n",
			},
			{
				"parse, load operand, long name",
				long,
				"load(1, \"x\")\n",
			},
		}
	)

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				_, err := graph.Of(&common.Source{
					Entry: QUOTE_ENTRY,
					Text: []byte(
						"load(" + `"` + strings.ReplaceAll(
							test.module,
							"\n",
							`\n`,
						) + `"` +
							", \"x\")\ndef main():\n    pass\n",
					),
					Loader: _Mapped{test.module: test.text},
				})
				if err == nil {
					t.Fatal("got nil, want a refusal")
				}

				got := err.Error()
				if strings.Contains(
					got,
					"\n",
				) {
					t.Fatalf(
						"raw line break in %q",
						got,
					)
				}

				if !strings.Contains(
					got,
					`"`,
				) {
					t.Fatalf(
						"no quoted name in %q",
						got,
					)
				}

				if len(got) > QUOTE_MAX {
					t.Fatalf(
						"error has %d bytes, want at most %d",
						len(got),
						QUOTE_MAX,
					)
				}
			},
		)
	}
}

// _ScriptSite is one print site that a name from a script can reach: a script
// that derivation refuses, with the loader that holds its modules. entry is the
// name of the entry file, QUOTE_ENTRY when empty.
//
// Revisions:
//   - 2026-11-06 06:22: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _ScriptSite struct {
	site  string
	entry string
	text  string
	files _Mapped
}

// TestRefusal_ScriptNamesAreBounded proves each print site of derivation that
// can show a name from a script writes it bounded, and quoted when it is a
// module name (Q4, Q209, B128). A module name has a line break or 5000 bytes;
// an identifier or the entry name has 5000 bytes. One row per site: dropping
// the bound or the quote at that site makes its row fail.
//
// Revisions:
//   - 2026-11-06 00:18: initial creation (C63, B128); one row per print site
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestRefusal_ScriptNamesAreBounded(t *testing.T) {
	var (
		long = strings.Repeat(
			"a",
			5000,
		)
		other = strings.Repeat(
			"b",
			5000,
		)
		broken = "a\nb.star"
		wide   = long + ".star"
		entry  = func(loads ...[2]string) string {
			var text strings.Builder

			for _, load := range loads {
				text.WriteString(
					"load(" + strconv.Quote(load[0]) +
						", " + strconv.Quote(load[1]) + ")\n",
				)
			}

			text.WriteString("def main():\n")

			for _, load := range loads {
				text.WriteString("    " + load[1] + "()\n")
			}

			return text.String()
		}
		two = func(fn string) string {
			return "def " + fn + "():\n    pass\n"
		}
		sites = []_ScriptSite{
			{
				site: "source _Declare: collision",
				text: entry(
					[2]string{broken, long},
					[2]string{wide, "g"},
				),
				files: _Mapped{
					broken: two(long),
					wide:   two(long) + "def g():\n    " + long + "()\n",
				},
			},
			{
				site:  "source _Declare: signature",
				text:  entry([2]string{broken, long}),
				files: _Mapped{broken: "def " + long + "(a=1):\n    pass\n"},
			},
			{
				site: "source _Shadows: builtin read in another module",
				text: entry(
					[2]string{broken, "fm"},
					[2]string{wide, "fu"},
				),
				files: _Mapped{
					broken: "def max(a, b):\n    return 1\n\n" +
						"def fm():\n    return max(1, 2)\n",
					wide: "def fu():\n    return max(1, 2)\n",
				},
			},
			{
				site: "source _Loads: alias",
				text: "load(\"m.star\", " + other + " = " +
					strconv.Quote(long) + ")\n" +
					"def main():\n    pass\n",
				files: _Mapped{"m.star": two(long)},
			},
			{
				site: "derive _Constants: constant assigned twice",
				text: entry(
					[2]string{broken, "f"},
					[2]string{wide, "g"},
				),
				files: _Mapped{
					broken: long + " = 1\ndef f():\n    return " + long + "\n",
					wide:   long + " = 2\ndef g():\n    return " + long + "\n",
				},
			},
			{
				site:  "derive _Constants: augmented assignment",
				text:  entry([2]string{broken, "f"}),
				files: _Mapped{broken: "X = 1\nX += 1\n" + two("f")},
			},
			{
				site:  "derive _Placed: statement of a module",
				text:  entry([2]string{broken, "f"}),
				files: _Mapped{broken: "print(1)\n" + two("f")},
			},
			{
				site: "derive _Bound: argument that is refused",
				text: long + " = arg()\ndef main():\n    print(" + long + ")\n",
			},
			{
				site: "derive _Bound: constant that is refused",
				text: "def f(x):\n    return x\n\nport = arg(\"port\")\n" +
					long + " = f(port)\n" +
					"def main():\n    print(" + long + ")\n",
			},
			{
				site:  "derive Of: entry that does not parse",
				entry: wide,
				text:  "def (:\n",
			},
			{
				site:  "derive Of: entry with a refused def",
				entry: wide,
				text:  "def f(a=1):\n    pass\n\ndef main():\n    f()\n",
			},
			{
				site:  "derive Of: entry that reads a builtin a module defines",
				entry: wide,
				text:  entry([2]string{"m.star", "fm"}) + "    max(1, 2)\n",
				files: _Mapped{
					"m.star": "def max(a, b):\n    return 1\n\n" +
						"def fm():\n    return max(1, 2)\n",
				},
			},
			{
				site:  "derive Of: entry with no main",
				entry: wide,
				text:  two("f"),
			},
		}
	)

	for _, site := range sites {
		t.Run(
			site.site,
			func(t *testing.T) {
				name := site.entry
				if name == "" {
					name = QUOTE_ENTRY
				}

				_, err := graph.Of(&common.Source{
					Entry:  name,
					Text:   []byte(site.text),
					Loader: site.files,
				})

				_Bounded(
					t,
					err,
				)
			},
		)
	}
}

// _Bounded fails the test unless err is a refusal of one line, at most
// QUOTE_MAX bytes long.
//
// Revisions:
//   - 2026-11-06 00:18: initial creation (C63, B128)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Bounded(
	t *testing.T,
	err error,
) {
	t.Helper()

	if err == nil {
		t.Fatal("got nil, want a refusal")
	}

	got := err.Error()
	if strings.Contains(
		got,
		"\n",
	) {
		t.Fatalf(
			"raw line break in %q",
			got[:min(
				len(got),
				300,
			)],
		)
	}

	if len(got) > QUOTE_MAX {
		t.Fatalf(
			"error has %d bytes, want at most %d",
			len(got),
			QUOTE_MAX,
		)
	}
}
