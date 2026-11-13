package tests

import (
	"strings"
	"testing"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
)

const (
	CYCLE_WANT  = "cycle in the load graph: a.star -> b.star -> a.star"
	SHAKEN_WANT = "def main():\n    print(\"a\")\n\n\n"
)

// TestMerge_CycleText proves a refused cycle says the cycle and its ring,
// the modules from the first visit to the load that closes it, joined by an
// arrow. The words come from script's walk, which graph shares.
//
// Revisions:
//   - 2026-11-05 23:24: initial creation (C60, Q187)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses runtime.Merge, formerly Amalgamate; renamed from
//     TestAmalgamate_CycleText (C121, Q339, Q353)
func TestMerge_CycleText(t *testing.T) {
	var files = _Files{
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
	}

	_, err := runtime.Merge(&common.Source{
		Entry:  "main.star",
		Text:   []byte(files["main.star"]),
		Loader: files,
	})
	if err == nil {
		t.Fatal("got nil, want a refusal")
	}

	if err.Error() != CYCLE_WANT {
		t.Fatalf(
			"got %q, want %q",
			err.Error(),
			CYCLE_WANT,
		)
	}
}

// TestMerge_ShakenTextExact proves a statement the shake takes out leaves
// its lines and nothing else: the line after it starts where it did, so the end
// of a cut is the end of the last line of the statement and not one byte more.
// The dead def ends the text with no line break, where one byte more is past the
// end.
//
// Revisions:
//   - 2026-11-05 23:24: initial creation (C60, Q188)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 23:01: renamed from TestAmalgamate_ShakenTextExact (C121, Q353)
func TestMerge_ShakenTextExact(t *testing.T) {
	merged := _Merged(
		t,
		_Files{
			"main.star": _Join(
				"def main():\n",
				"    print(\"a\")\n",
				"def dead():\n",
				"    return 1",
			),
		},
	)

	if string(merged.Text) != SHAKEN_WANT {
		t.Fatalf(
			"got %q, want %q",
			merged.Text,
			SHAKEN_WANT,
		)
	}
}

// TestMerge_NamesAreBounded proves each print site of the merge that can
// show a name from a script writes it bounded, and quoted when it is a module
// name (Q4, B128). A module has a line break and 5000 bytes in its name, an
// identifier 5000 bytes. One row per site: dropping the bound or the quote at
// that site makes its row fail.
//
// Revisions:
//   - 2026-11-06 00:19: initial creation (C63, B128); one row per print site
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses runtime.Merge, formerly Amalgamate; renamed from
//     TestAmalgamate_NamesAreBounded (C121, Q339, Q353)
func TestMerge_NamesAreBounded(t *testing.T) {
	var (
		long = strings.Repeat(
			"a",
			5000,
		)
		broken = "a\nb.star\n" + long
		cases  = []struct {
			name  string
			entry string
			files _Files
		}{
			{
				"names _Asked: a name the module does not define",
				"main.star",
				_Files{
					broken: "def f():\n    return 1\n",
					"main.star": "load(\"a\\nb.star\\n" + long + "\", \"" +
						long + "\")\n" +
						"def main():\n    print(" + long + "())\n",
				},
			},
			{
				"splice _Loaded: an alias whose substitute is a local",
				long + ".star",
				_Files{
					"m.star": "def " + long + "(w):\n    shout = w\n" +
						"    return shout.upper()\n",
					long + ".star": "load(\"m.star\", shout = \"" +
						long + "\")\n" +
						"def main():\n    " + long + " = 1\n" +
						"    print(shout(\"a\"), " + long + ")\n",
				},
			},
		}
	)

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				_, err := runtime.Merge(&common.Source{
					Entry:  tc.entry,
					Text:   []byte(tc.files[tc.entry]),
					Loader: tc.files,
				})
				if err == nil {
					t.Fatal("got nil, want a refusal")
				}

				got := err.Error()
				if strings.Contains(
					got,
					"\n",
				) || len(got) > 2000 {
					t.Fatalf(
						"got %d bytes: %q",
						len(got),
						got[:min(
							len(got),
							200,
						)],
					)
				}
			},
		)
	}
}
