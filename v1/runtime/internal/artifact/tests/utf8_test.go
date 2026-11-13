package tests

import (
	"errors"
	"strings"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/artifact"
)

// TestSave_RefusesSourceThatIsNotUTF8 proves a script whose merged source is
// not valid UTF-8 runs as it always did, and only a bundle is refused, with an
// error that names the file and holds none of the bad bytes. A valid script
// still bundles.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a, plan Q51)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrUTF8, formerly ErrNotUTF8 (C121, Q348)
func TestSave_RefusesSourceThatIsNotUTF8(t *testing.T) {
	var (
		bad  = "# \xff\ndef main():\n    return 1\n"
		good = "# fine\ndef main():\n    return 1\n"
	)

	tests := []struct {
		name   string
		entry  string
		files  _Sources
		refuse string
	}{
		{
			name:   "in the script",
			entry:  "c.star",
			files:  _Sources{},
			refuse: "\"c.star\"",
		},
		{
			name:  "in a module",
			entry: "main.star",
			files: _Sources{
				"m.star": "# \xff\ndef f():\n    return 1\n",
			},
			refuse: "\"m.star\"",
		},
		{
			name:  "valid",
			entry: "main.star",
			files: _Sources{
				"m.star": "def f():\n    return 1\n",
			},
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				text := good

				switch {
				case tc.name == "in the script":
					text = bad
				case len(tc.files) > 0:
					text = "load(\"m.star\", \"f\")\n" + good
				}

				built, err := artifact.Compile(&common.Source{
					Entry:  tc.entry,
					Text:   []byte(text),
					Loader: tc.files,
				})
				if err != nil {
					t.Fatalf(
						"compile: %v",
						err,
					)
				}

				_, err = artifact.Run(
					t.Context(),
					built,
				)
				if err != nil {
					t.Fatalf(
						"run: %v",
						err,
					)
				}

				_, err = built.Save()

				if tc.refuse == "" {
					if err != nil {
						t.Fatalf(
							"a valid script should bundle: %v",
							err,
						)
					}

					return
				}

				if !errors.Is(
					err,
					artifact.ErrUTF8,
				) {
					t.Fatalf(
						"got %v, want ErrUTF8",
						err,
					)
				}

				if !strings.Contains(
					err.Error(),
					tc.refuse,
				) || strings.Contains(
					err.Error(),
					"\xff",
				) {
					t.Fatalf(
						"error %q should name %s and hold no bad byte",
						err,
						tc.refuse,
					)
				}
			},
		)
	}
}
