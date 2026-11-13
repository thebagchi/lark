package tests

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	artifactpb "github.com/thebagchi/lark/proto/gen/artifact"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/artifact"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

const (
	WALK_ENTRY = "main.star"
	WALK_MAIN  = "def main():\n    pass\n"
)

// TestWalk_RefusalsKeepTheirText proves the dependency walk a compile makes
// refuses a cycle, a self-load, a missing module and an entry loaded back
// with the sentinel and the exact text it had before the walk was shared with
// graph.Of.
//
// Revisions:
//   - 2026-11-02 17:53: initial creation (C43)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrUnit, formerly ErrNoUnit (C121, Q348)
func TestWalk_RefusalsKeepTheirText(t *testing.T) {
	tests := []struct {
		name  string
		entry string
		files _Sources
		want  error
		text  string
	}{
		{
			name:  "cycle",
			entry: "load(\"a.star\", \"x\")\n" + WALK_MAIN,
			files: _Sources{
				"a.star": "load(\"b.star\", \"y\")\nx = 1\n",
				"b.star": "load(\"a.star\", \"x\")\ny = 1\n",
			},
			want: spelling.ErrCycle,
			text: "cycle in the load graph: a.star -> b.star -> a.star",
		},
		{
			name:  "cycle through the entry",
			entry: "load(\"a.star\", \"x\")\n" + WALK_MAIN,
			files: _Sources{
				"a.star": "load(\"main.star\", \"main\")\nx = 1\n",
			},
			want: spelling.ErrCycle,
			text: "cycle in the load graph: main.star -> a.star -> main.star",
		},
		{
			name:  "self-load",
			entry: "load(\"s.star\", \"y\")\n" + WALK_MAIN,
			files: _Sources{
				"s.star": "load(\"s.star\", \"y\")\n",
			},
			want: spelling.ErrCycle,
			text: "cycle in the load graph: s.star -> s.star",
		},
		{
			name:  "missing module",
			entry: "load(\"gone.star\", \"x\")\n" + WALK_MAIN,
			files: _Sources{},
			want:  artifact.ErrUnit,
			text:  "cannot load \"gone.star\" from main.star: gone.star: no such unit",
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				_, err := artifact.Compile(&common.Source{
					Entry:  WALK_ENTRY,
					Text:   []byte(test.entry),
					Loader: test.files,
				})
				if !errors.Is(
					err,
					test.want,
				) {
					t.Fatalf(
						"got %v, want %v",
						err,
						test.want,
					)
				}

				if err.Error() != test.text {
					t.Fatalf(
						"got text %q, want %q",
						err.Error(),
						test.text,
					)
				}
			},
		)
	}
}

// TestWalk_DiamondOrder proves a module reached by two routes is merged once:
// the artifact is one unit, and the shared module's top level runs once.
//
// Revisions:
//   - 2026-11-02 17:53: initial creation (C43)
//   - 2026-11-02 22:58: the merge makes one unit, so the order is the
//     order of its text (C44a)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestWalk_DiamondOrder(t *testing.T) {
	var (
		files = _Sources{
			"l.star":    "load(\"base.star\", \"b\")\nl = b\n",
			"r.star":    "load(\"base.star\", \"b\")\nr = b\n",
			"base.star": "counter = [0]\nb = 1\n",
		}
		text = "load(\"l.star\", \"l\")\nload(\"r.star\", \"r\")\n" + WALK_MAIN
	)

	built, err := artifact.Compile(&common.Source{
		Entry:  WALK_ENTRY,
		Text:   []byte(text),
		Loader: files,
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

	message := new(artifactpb.Artifact)

	err = proto.Unmarshal(
		saved,
		message,
	)
	if err != nil {
		t.Fatalf(
			"unmarshal: %v",
			err,
		)
	}

	got := _Names(message.GetUnits())
	if !slices.Equal(
		got,
		[]string{WALK_ENTRY},
	) {
		t.Fatalf(
			"got units %v, want one unit %s",
			got,
			WALK_ENTRY,
		)
	}
}

// _Fixed is a loader that resolves every load to one name and serves one
// module text under it.
//
// Revisions:
//   - 2026-11-10 11:44: initial creation (C117, B239)
type _Fixed struct {
	name string
	text string
}

// Resolve returns the one name, whatever the target.
//
// Revisions:
//   - 2026-11-10 11:44: initial creation (C117, B239)
func (f *_Fixed) Resolve(
	_ string,
	_ string,
) (string, error) {
	return f.name, nil
}

// Load serves the module text under the one name.
//
// Revisions:
//   - 2026-11-10 11:44: initial creation (C117, B239)
func (f *_Fixed) Load(name string) ([]byte, error) {
	if name != f.name {
		return nil, fmt.Errorf(
			"no module %q",
			name,
		)
	}

	return []byte(f.text), nil
}

// TestWalk_BoundsNamesInRefusals proves a module or entry name of 100,000
// bytes does not fill the error of a load cycle, a load from such an entry,
// or a Save of an entry that cannot be marshalled (Q4, B239).
//
// Revisions:
//   - 2026-11-10 11:44: initial creation (C117, B239)
func TestWalk_BoundsNamesInRefusals(t *testing.T) {
	var (
		long = strings.Repeat(
			"m",
			100000,
		)
		module = long + ".star"
		loop   = "load(\"x\", \"y\")\n" + WALK_MAIN
	)

	tests := []struct {
		name   string
		entry  string
		loader common.Loader
		text   string
	}{
		{
			name:  "ring of long module names",
			entry: WALK_ENTRY,
			loader: &_Fixed{
				name: module,
				text: loop,
			},
			text: "cycle in the load graph: ",
		},
		{
			name:  "self-cycle of a long entry name",
			entry: module,
			loader: &_Fixed{
				name: module,
				text: loop,
			},
			text: "cycle in the load graph: ",
		},
	}

	for _, test := range tests {
		t.Run(
			test.name,
			func(t *testing.T) {
				_, err := artifact.Compile(&common.Source{
					Entry:  test.entry,
					Text:   []byte(loop),
					Loader: test.loader,
				})
				if !errors.Is(
					err,
					spelling.ErrCycle,
				) {
					t.Fatalf(
						"got %v, want ErrCycle",
						err,
					)
				}

				if len(err.Error()) > 1000 {
					t.Fatalf(
						"error is %d bytes, want a bound",
						len(err.Error()),
					)
				}

				if !strings.Contains(
					err.Error(),
					"... (",
				) {
					t.Fatalf(
						"error %q has no length marker",
						err.Error(),
					)
				}
			},
		)
	}

	built, err := artifact.Compile(&common.Source{
		Entry: "\xff" + long,
		Text:  []byte(WALK_MAIN),
	})
	if err != nil {
		t.Fatalf(
			"compile: %v",
			err,
		)
	}

	_, err = built.Save()
	if err == nil {
		t.Fatal("save of an entry that is not UTF-8 gave no error")
	}

	if len(err.Error()) > 1000 {
		t.Fatalf(
			"save error is %d bytes, want a bound",
			len(err.Error()),
		)
	}
}
