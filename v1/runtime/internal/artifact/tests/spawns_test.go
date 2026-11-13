package tests

import (
	"slices"
	"testing"

	"google.golang.org/protobuf/proto"

	artifactpb "github.com/thebagchi/lark/proto/gen/artifact"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/artifact"
)

// SPAWNING is a script with a spawn of each shape: by name, bound, in a
// lambda, inside a join, in a nested call, and three that name no function.
//
// Revisions:
//   - 2026-11-05 08:03: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
const (
	SPAWNING = `
def patient():
    sleep(1)

def bad():
    assert(False, "no")

def leaf():
    return 1

def helper():
    spawn(leaf)

def main():
    helper()
    h = spawn(bad)
    join(h, spawn(patient))
    g = spawn(lambda: leaf())
    k = spawn(lambda: 1)
    f = patient
    spawn(f)
    join(g)
`
)

// _Sites compiles text and returns the spawn sites the artifact reports.
//
// Revisions:
//   - 2026-11-05 00:27: initial creation (C45)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Sites(
	t *testing.T,
	text string,
) []common.Spawn {
	t.Helper()

	built, err := artifact.Compile(&common.Source{
		Entry: "sites.star",
		Text:  []byte(text),
	})
	if err != nil {
		t.Fatalf(
			"compile: %v",
			err,
		)
	}

	return built.Spawns()
}

// TestSpawns_Shapes checks which spawns the artifact reports: those inside a
// def that name a function, directly or as the one call of a lambda, in the
// order they are written. A spawn of a variable, of a lambda that is not one
// call, or outside any def is not reported, since no name can be read for it.
//
// Revisions:
//   - 2026-11-05 00:27: initial creation (C45)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestSpawns_Shapes(t *testing.T) {
	cases := map[string]struct {
		text string
		want []common.Spawn
	}{
		"every shape": {
			text: SPAWNING,
			want: []common.Spawn{
				{
					Caller: "helper",
					Target: "leaf",
				},
				{
					Caller: "main",
					Target: "bad",
				},
				{
					Caller: "main",
					Target: "patient",
				},
				{
					Caller: "main",
					Target: "leaf",
				},
			},
		},
		"no spawn": {
			text: "def main():\n    return 1\n",
			want: nil,
		},
		"top level is no def": {
			text: "def a():\n    return 1\n\nspawn(a)\n\ndef main():\n    return 1\n",
			want: nil,
		},
		"a module's spawn": {
			text: "def a():\n    return 1\n\ndef main():\n    spawn(a)\n",
			want: []common.Spawn{
				{
					Caller: "main",
					Target: "a",
				},
			},
		},
	}

	for name, tc := range cases {
		t.Run(
			name,
			func(t *testing.T) {
				got := _Sites(
					t,
					tc.text,
				)

				if !slices.Equal(
					got,
					tc.want,
				) {
					t.Fatalf(
						"sites %v, want %v",
						got,
						tc.want,
					)
				}
			},
		)
	}
}

// TestSpawns_SurviveABundle checks that a bundle read back reports the sites
// the compiled artifact did, from the merged source the bundle carries, and
// that a bundle with no source reports none.
//
// Success: the loaded artifact's sites equal the compiled one's; a bundle with
// its unit's source emptied (as one written before the field existed) reports
// nil and does not fail.
//
// Revisions:
//   - 2026-11-05 00:27: initial creation (C45)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestSpawns_SurviveABundle(t *testing.T) {
	built, err := artifact.Compile(&common.Source{
		Entry: "sites.star",
		Text:  []byte(SPAWNING),
	})
	if err != nil {
		t.Fatalf(
			"compile: %v",
			err,
		)
	}

	saved, err := built.Save()
	if err != nil {
		t.Fatalf(
			"save: %v",
			err,
		)
	}

	loaded, err := artifact.Load(saved)
	if err != nil {
		t.Fatalf(
			"load: %v",
			err,
		)
	}

	if !slices.Equal(
		loaded.Spawns(),
		built.Spawns(),
	) || len(loaded.Spawns()) == 0 {
		t.Fatalf(
			"loaded sites %v, compiled %v",
			loaded.Spawns(),
			built.Spawns(),
		)
	}

	bundle := new(artifactpb.Artifact)

	err = proto.Unmarshal(
		saved,
		bundle,
	)
	if err != nil {
		t.Fatalf(
			"unmarshal: %v",
			err,
		)
	}

	for _, unit := range bundle.GetUnits() {
		unit.Source = ""
	}

	bare, err := proto.Marshal(bundle)
	if err != nil {
		t.Fatalf(
			"marshal: %v",
			err,
		)
	}

	old, err := artifact.Load(bare)
	if err != nil {
		t.Fatalf(
			"load without source: %v",
			err,
		)
	}

	if got := old.Spawns(); got != nil {
		t.Fatalf(
			"sites %v from a bundle with no source, want none",
			got,
		)
	}
}

// TestSpawns_Nil checks that a nil or zero artifact reports no sites.
//
// Revisions:
//   - 2026-11-05 00:27: initial creation (C45)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestSpawns_Nil(t *testing.T) {
	var nothing *artifact.Artifact

	if got := nothing.Spawns(); got != nil {
		t.Fatalf(
			"nil artifact: %v",
			got,
		)
	}

	if got := new(artifact.Artifact).Spawns(); got != nil {
		t.Fatalf(
			"zero artifact: %v",
			got,
		)
	}
}

// TestSpawns_ShakenDefHasNoSite proves a spawn written in a def nothing
// reaches is no site, for the compiled artifact and for the bundle read back:
// C45's NOT_STARTED list agrees with the shaken source (Q164).
//
// Revisions:
//   - 2026-11-05 17:15: initial creation (C44b, Q164)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestSpawns_ShakenDefHasNoSite(t *testing.T) {
	var (
		text = "def leaf():\n    return 1\n\n" +
			"def dead():\n    spawn(leaf)\n\n" +
			"def main():\n    join(spawn(leaf))\n"
		built, err = artifact.Compile(&common.Source{
			Entry: "sites.star",
			Text:  []byte(text),
		})
	)
	if err != nil {
		t.Fatal(err)
	}

	saved, err := built.Save()
	if err != nil {
		t.Fatal(err)
	}

	loaded, err := artifact.Load(saved)
	if err != nil {
		t.Fatal(err)
	}

	want := []common.Spawn{{
		Caller: "main",
		Target: "leaf",
	}}

	for name, got := range map[string][]common.Spawn{
		"compiled": built.Spawns(),
		"loaded":   loaded.Spawns(),
	} {
		if !slices.Equal(
			got,
			want,
		) {
			t.Errorf(
				"%s sites %v, want %v",
				name,
				got,
				want,
			)
		}
	}
}
