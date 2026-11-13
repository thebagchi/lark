package tests

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/artifact"
	"github.com/thebagchi/lark/v1/runtime/internal/merge"
	"github.com/thebagchi/lark/v1/runtime/internal/observe"
	"github.com/thebagchi/lark/v1/runtime/internal/script"
	"github.com/thebagchi/lark/v1/runtime/plugin"
)

// TestNilSweep proves each exported method or function that a nil receiver or
// a nil argument reached does not panic, and answers what its doc says. The
// table is the sweep of C86 fix round 2; the report lists what the sweep left
// out and why.
//
// Revisions:
//   - 2026-11-04 23:50: initial creation (C86 fix round 2)
//   - 2026-11-07 19:54: the Listener cases moved to remote/internal/host (C91w8c)
//   - 2026-11-07 20:07: updated for the move of Handle, Evaluation and Hold to common (C91w5)
//   - 2026-11-07 23:04: the Given getters are field reads now, so their case is gone; a nil
//     interface gives ErrNotCompiled and a typed nil keeps ErrNoUnit (C91w6)
//   - 2026-11-07 23:23: tests the concrete Execution, the runtime one being an
//     interface now (C91w7)
//   - 2026-11-08 13:25: Wait's error is plain, ErrNotStarted being dropped (C98, Q249)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrSource and ErrUnit and ErrArtifact, formerly ErrNoSource and
//     ErrNoUnit and ErrNotCompiled (C121, Q348)
//   - 2026-11-13 17:40: uses package merge, formerly amalgam; the row name follows (C134, Q446)
func TestNilSweep(t *testing.T) {
	var (
		ctx       = context.Background()
		execution *observe.Execution
		art       *artifact.Artifact
		merged    *merge.Merged
		walk      *script.Walk
	)

	cases := []struct {
		name string
		call func() error
		want error
	}{
		{
			name: "nil Execution Wait",
			call: func() error {
				_, err := execution.Wait()
				if err == nil || !strings.Contains(
					err.Error(),
					"not started",
				) {
					return errors.New("want a not-started error")
				}

				return nil
			},
		},
		{
			name: "nil Execution Done is closed",
			call: func() error {
				select {
				case <-execution.Done():
					return nil
				case <-time.After(time.Second):
					return errors.New("Done blocked")
				}
			},
		},
		{
			name: "nil Execution Status",
			call: func() error {
				if execution.Status() == nil {
					return errors.New("nil graph")
				}

				return nil
			},
		},
		{
			name: "nil Execution Snapshot",
			call: func() error {
				if execution.Snapshot() == nil {
					return errors.New("nil graph")
				}

				return nil
			},
		},
		{
			name: "nil Artifact Save",
			call: func() error {
				_, err := art.Save()

				return err
			},
			want: runtime.ErrUnit,
		},
		{
			name: "zero Artifact Save",
			call: func() error {
				_, err := new(artifact.Artifact).Save()

				return err
			},
			want: runtime.ErrUnit,
		},
		{
			name: "artifact.Run nil Artifact",
			call: func() error {
				_, err := artifact.Run(
					ctx,
					art,
				)

				return err
			},
			want: runtime.ErrUnit,
		},
		{
			name: "artifact.Perform nil interface",
			call: func() error {
				_, err := artifact.Perform(
					ctx,
					nil,
					nil,
				)

				return err
			},
			want: runtime.ErrArtifact,
		},
		{
			name: "artifact.Perform zero Artifact",
			call: func() error {
				_, err := artifact.Perform(
					ctx,
					new(artifact.Artifact),
					nil,
				)

				return err
			},
			want: runtime.ErrUnit,
		},
		{
			name: "artifact.Supplied nil thread",
			call: func() error {
				_, ok := artifact.Supplied(nil)
				if ok {
					return errors.New("want false")
				}

				return nil
			},
		},
		{
			name: "script.LoaderFor nil Source",
			call: func() error {
				if script.LoaderFor(nil) == nil {
					return errors.New("nil loader")
				}

				return nil
			},
		},
		{
			name: "script.NewWalk nil loader",
			call: func() error {
				_, err := script.NewWalk(
					nil,
					"a.star",
				).Reach(
					"a.star",
					"b.star",
					nil,
				)
				if err == nil {
					return errors.New("want an error for a missing file")
				}

				return nil
			},
		},
		{
			name: "nil Walk Reach",
			call: func() error {
				_, err := walk.Reach(
					"a",
					"b",
					nil,
				)

				return err
			},
			want: script.ErrSource,
		},
		{
			name: "merge.Merge nil Source",
			call: func() error {
				_, err := merge.Merge(
					nil,
					nil,
				)

				return err
			},
			want: script.ErrSource,
		},
		{
			name: "nil Merged Invalid",
			call: func() error {
				_, bad := merged.Invalid()
				if bad {
					return errors.New("want false")
				}

				return nil
			},
		},
		{
			name: "nil Merged Source",
			call: func() error {
				_, err := merged.Source()

				return err
			},
			want: script.ErrSource,
		},
		{
			name: "plugin.Environment nil plugin",
			call: func() error {
				_, err := plugin.Environment([]common.Plugin{nil})

				return err
			},
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var err error

				_Survives(
					t,
					func() {
						err = tc.call()
					},
				)

				if tc.want == nil && err != nil {
					t.Fatalf(
						"want no error, got %v",
						err,
					)
				}

				if tc.want != nil && !errors.Is(
					err,
					tc.want,
				) {
					t.Fatalf(
						"want %v, got %v",
						tc.want,
						err,
					)
				}
			},
		)
	}
}
