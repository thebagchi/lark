package tests

import (
	"testing"

	"google.golang.org/protobuf/proto"

	"github.com/thebagchi/lark/v1/runtime/internal/artifact"
)

// TestEntry_RefusalText pins the exact text each caller gives for each refusal
// of an entry point: Run and Save, for a module with no main, for a main that
// takes arguments, and (Save only) for a loaded artifact whose entry unit has
// no syntax tree.
//
// Revisions:
//   - 2026-11-01 14:00: initial creation, so A6 changes no error text
//   - 2026-11-09 11:26: reads its scripts from memory, not from testdata (C114, Q281)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestEntry_RefusalText(t *testing.T) {
	loaded := func(t *testing.T) *artifact.Artifact {
		t.Helper()

		bundle := _Handmade(
			t,
			"app.star",
		)
		bundle.Entry = "lib.star"

		raw, err := proto.Marshal(bundle)
		if err != nil {
			t.Fatalf(
				"marshal: %v",
				err,
			)
		}

		art, err := artifact.Load(raw)
		if err != nil {
			t.Fatalf(
				"load: %v",
				err,
			)
		}

		return art
	}

	const (
		HINT = " (define def main() with no required parameters; " +
			"a file without it is a module, for load)"
		NO_MAIN  = "lib.star: no entry point" + HINT
		WITH_ARG = "mainargs.star: main at mainargs.star:1:5 " +
			"must take no required parameters: no entry point" + HINT
	)

	tests := []struct {
		name    string
		fixture string
		verb    string
		loaded  bool
		want    string
	}{
		{
			"RunNoMain",
			"lib.star",
			"run",
			false,
			NO_MAIN,
		},
		{
			"RunMainWithArguments",
			"mainargs.star",
			"run",
			false,
			WITH_ARG,
		},
		{
			"SaveNoMain",
			"lib.star",
			"save",
			false,
			NO_MAIN,
		},
		{
			"SaveMainWithArguments",
			"mainargs.star",
			"save",
			false,
			WITH_ARG,
		},
		{
			"SaveLoadedWithoutTree",
			"",
			"save",
			true,
			NO_MAIN,
		},
	}

	for _, tc := range tests {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var err error

				switch {
				case tc.loaded:
					_, err = loaded(t).Save()
				case tc.verb == "run":
					_, err = artifact.Run(
						t.Context(),
						_Built(
							t,
							tc.fixture,
						),
					)
				default:
					_, err = _Built(
						t,
						tc.fixture,
					).Save()
				}

				if err == nil {
					t.Fatal("got nil, want a refusal")
				}

				if got := err.Error(); got != tc.want {
					t.Fatalf(
						"got %q, want %q",
						got,
						tc.want,
					)
				}
			},
		)
	}
}
