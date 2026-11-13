package tests

import (
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/graph"
)

// TestOf_ReadsALoneCarriageReturnAsALineFeed proves the flow Of derives from a
// script whose line breaks are lone carriage returns is the flow it derives from
// the same script with line feeds, text and all: an init statement over lines
// keeps the line feed the parser sees, a function body and main keep theirs, and
// the unread statements are shaken alike. Of reads the source through
// script.Normalised for this, and without that call the text of a statement
// would hold the carriage return (Q405).
//
// The check: go test -count=1 -run TestOf_ReadsALoneCarriageReturnAsALineFeed
// ./v1/runtime/internal/graph/tests
//
// Revisions:
//   - 2026-11-13 00:45: initial creation (C130, Q405)
func TestOf_ReadsALoneCarriageReturnAsALineFeed(t *testing.T) {
	var tests = []struct {
		name string
		text string
	}{
		{
			"an init statement over lines",
			"[1,\n 2, \"m\"]\ndef main():\n    pass\n",
		},
		{
			"an init call over lines",
			"print(1,\n  2)\ndef main():\n    pass\n",
		},
		{
			"a function and main",
			"def f(x):\n    y = (x,\n         1)\n    return y\n" +
				"def main():\n    f(1)\n    z = [\n        2]\n",
		},
		{
			"a string over lines",
			"s = 'a'\nprint(s,\n  'b')\ndef main():\n    pass\n",
		},
		{
			"an unread argument and a comment",
			"x = arg(\"a\")  # c\ndef main():\n    pass\n",
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var (
					want = _Derived(
						t,
						tc.text,
					)
					got = _Derived(
						t,
						strings.ReplaceAll(
							tc.text,
							"\n",
							"\r",
						),
					)
				)

				if !proto.Equal(
					got,
					want,
				) {
					t.Fatalf(
						"carriage returns:\n%v\nline feeds:\n%v",
						got,
						want,
					)
				}

				if _, err := graph.Of(&common.Source{
					Entry: "script.star",
					Text:  []byte("\r"),
				}); err == nil {
					t.Fatal("a file of one carriage return has a main")
				}
			},
		)
	}
}
