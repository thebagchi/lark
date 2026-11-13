package tests

import (
	"errors"
	"fmt"
	"path"
	"slices"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/internal/script"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

// _Map is a Loader over module names, counting fetches.
//
// Revisions:
//   - 2026-11-02 20:49: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Map struct {
	files map[string]bool
	loads []string
}

// Resolve reads target as a file beside from.
//
// Revisions:
//   - 2026-11-02 17:53: initial creation (C43)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (m *_Map) Resolve(
	from string,
	target string,
) (string, error) {
	return path.Join(
		path.Dir(from),
		target,
	), nil
}

// Load records the fetch, or fails for a name not in the map.
//
// Revisions:
//   - 2026-11-02 17:53: initial creation (C43)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (m *_Map) Load(name string) ([]byte, error) {
	m.loads = append(
		m.loads,
		name,
	)

	if !m.files[name] {
		return nil, fmt.Errorf(
			"no module %s",
			name,
		)
	}

	return nil, nil
}

// TestWalk_Stops proves each stage of a walk stops with its sentinel and its
// facts, and a module already read costs no second fetch.
//
// Revisions:
//   - 2026-11-02 17:53: initial creation (C43)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestWalk_Stops(t *testing.T) {
	var (
		loader = &_Map{
			files: map[string]bool{
				"a.star": true,
				"b.star": true,
			},
		}
		walk = script.NewWalk(
			loader,
			"main.star",
		)
		reader func(
			name string,
			src []byte,
		) error
		got *script.StopError
	)

	reader = func(
		name string,
		src []byte,
	) error {
		switch name {
		case "a.star":
			_, err := walk.Reach(
				name,
				"b.star",
				reader,
			)

			return err
		case "b.star":
			_, err := walk.Reach(
				name,
				"a.star",
				reader,
			)

			return err
		}

		return nil
	}

	_, err := walk.Reach(
		"main.star",
		"a.star",
		reader,
	)
	if !errors.As(
		err,
		&got,
	) {
		t.Fatalf(
			"got %v, want a stop",
			err,
		)
	}

	if got.Stage != script.STAGE_CYCLE || !errors.Is(
		err,
		spelling.ErrCycle,
	) {
		t.Fatalf(
			"got stage %d err %v, want a cycle",
			got.Stage,
			got.Err,
		)
	}

	want := []string{"a.star", "b.star", "a.star"}
	if !slices.Equal(
		got.Ring,
		want,
	) {
		t.Fatalf(
			"got ring %v, want %v",
			got.Ring,
			want,
		)
	}

	_, err = walk.Reach(
		"main.star",
		"gone.star",
		reader,
	)
	if !errors.As(
		err,
		&got,
	) || got.Stage != script.STAGE_LOAD {
		t.Fatalf(
			"got %v, want a load stop",
			err,
		)
	}

	_, err = walk.Reach(
		"main.star",
		"b.star",
		reader,
	)
	if err != nil {
		t.Fatalf(
			"a module already read: %v",
			err,
		)
	}

	wantLoads := []string{"a.star", "b.star", "gone.star"}
	if !slices.Equal(
		loader.loads,
		wantLoads,
	) {
		t.Fatalf(
			"got fetches %v, want %v",
			loader.loads,
			wantLoads,
		)
	}
}
