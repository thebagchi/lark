package tests

import (
	"context"
	"errors"
	"testing"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
)

// _Foreign is an Artifact lark did not compile: it has a Save method and
// nothing else, so it satisfies the interface and holds no compiled code.
//
// Revisions:
//   - 2026-11-07 23:04: initial creation (C91w6, Q118)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
type _Foreign struct{}

// Save answers a few bytes, so a test can see that the value was not run.
//
// Revisions:
//   - 2026-11-07 23:04: initial creation (C91w6, Q118)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func (f *_Foreign) Save() ([]byte, error) {
	return []byte("foreign"), nil
}

// TestForeignArtifact proves Start and Eval refuse a nil artifact and an
// artifact of a type lark did not compile, with ErrArtifact (Q118). Start
// returns a nil run at once (Q125). Eval returns a result that is not nil,
// with an empty graph and no prints. Success: every case passes under -race.
//
// Run it: go test -race -count=1 -run TestForeignArtifact ./v1/runtime
//
// Revisions:
//   - 2026-11-07 23:04: initial creation (C91w6, Q118, Q125)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrArtifact and runtime.Eval, formerly ErrNotCompiled and Evaluate
//     (C121, Q338, Q348)
func TestForeignArtifact(t *testing.T) {
	cases := []struct {
		name string
		art  common.Artifact
	}{
		{
			name: "a type with only Save",
			art:  &_Foreign{},
		},
		{
			name: "a nil interface",
			art:  nil,
		},
	}

	for _, tc := range cases {
		t.Run(
			"Start/"+tc.name,
			func(t *testing.T) {
				run, err := runtime.Start(
					context.Background(),
					tc.art,
				)
				if !errors.Is(
					err,
					runtime.ErrArtifact,
				) {
					t.Fatalf(
						"got error %v, want ErrArtifact",
						err,
					)
				}

				if run != nil {
					t.Fatalf(
						"got run %v, want nil",
						run,
					)
				}
			},
		)

		t.Run(
			"Eval/"+tc.name,
			func(t *testing.T) {
				result, err := runtime.Eval(
					context.Background(),
					tc.art,
				)
				if !errors.Is(
					err,
					runtime.ErrArtifact,
				) {
					t.Fatalf(
						"got error %v, want ErrArtifact",
						err,
					)
				}

				if result == nil {
					t.Fatal("got a nil result, want one with an empty graph")
				}

				if result.Value != nil ||
					result.Graph == nil ||
					len(result.Graph.GetFunctions()) != 0 ||
					len(result.Prints) != 0 {
					t.Fatalf(
						"got %+v, want an empty result",
						result,
					)
				}
			},
		)
	}
}

// TestCompile_RefusedGivesNilArtifact proves a refused Compile and a refused
// Load return a nil Artifact, not an interface that holds a nil pointer: the
// host's `built == nil` is true.
//
// Revisions:
//   - 2026-11-07 23:04: initial creation (C91w6)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrSource, formerly ErrNoSource (C121, Q348)
func TestCompile_RefusedGivesNilArtifact(t *testing.T) {
	built, err := runtime.Compile(nil)
	if !errors.Is(
		err,
		runtime.ErrSource,
	) {
		t.Fatalf(
			"Compile: got error %v, want ErrSource",
			err,
		)
	}

	if built != nil {
		t.Fatalf(
			"Compile: got %v, want a nil Artifact",
			built,
		)
	}

	loaded, err := runtime.Load([]byte("not a bundle"))
	if err == nil {
		t.Fatal("Load: want an error for bytes that are not a bundle")
	}

	if loaded != nil {
		t.Fatalf(
			"Load: got %v, want a nil Artifact",
			loaded,
		)
	}
}
