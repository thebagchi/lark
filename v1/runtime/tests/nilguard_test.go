package tests

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.starlark.net/starlark"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/observe"
)

// _NilGuardSource is a script every case below can run: main returns 1.
//
// Revisions:
//   - 2026-11-04 23:50: initial creation (C86)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _NilGuardSource() *common.Source {
	return &common.Source{
		Entry: "a.star",
		Text:  []byte("def main():\n    return 1\n"),
	}
}

// _Survives runs body and fails the test when it panics: a host must never be
// crashed by a nil it passed.
//
// Revisions:
//   - 2026-11-04 23:50: initial creation (C86)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Survives(
	t *testing.T,
	body func(),
) {
	t.Helper()

	defer func() {
		r := recover()
		if r != nil {
			t.Fatalf(
				"panicked: %v",
				r,
			)
		}
	}()

	body()
}

// TestNilInputs proves a nil at the public edge does not panic: Run, Compile,
// Merge and Load return an error for a nil source, and skip a nil option
// or a nil plugin.
//
// Revisions:
//   - 2026-11-04 23:50: initial creation (C86)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses runtime.Merge, formerly Amalgamate (C121, Q339)
func TestNilInputs(t *testing.T) {
	var (
		ctx   = context.Background()
		cases = []struct {
			name    string
			call    func() error
			wantErr bool
		}{
			{
				name: "Run with a nil source",
				call: func() error {
					_, err := runtime.Run(
						ctx,
						nil,
					)

					return err
				},
				wantErr: true,
			},
			{
				name: "Compile with a nil source",
				call: func() error {
					_, err := runtime.Compile(nil)

					return err
				},
				wantErr: true,
			},
			{
				name: "Merge with a nil source",
				call: func() error {
					_, err := runtime.Merge(nil)

					return err
				},
				wantErr: true,
			},
			{
				name: "Run with a nil run option",
				call: func() error {
					_, err := runtime.Run(
						ctx,
						_NilGuardSource(),
						nil,
					)

					return err
				},
			},
			{
				name: "Compile with a nil compile option",
				call: func() error {
					_, err := runtime.Compile(
						_NilGuardSource(),
						nil,
					)

					return err
				},
			},
			{
				name: "Merge with a nil compile option",
				call: func() error {
					_, err := runtime.Merge(
						_NilGuardSource(),
						nil,
					)

					return err
				},
			},
			{
				name: "Load with a nil compile option",
				call: func() error {
					_, err := runtime.Load(
						nil,
						nil,
					)

					return err
				},
				wantErr: true,
			},
			{
				name: "Compile with WithPlugins(nil)",
				call: func() error {
					_, err := runtime.Compile(
						_NilGuardSource(),
						runtime.WithPlugins(nil),
					)

					return err
				},
			},
			{
				name: "Compile with a nil plugin in the list",
				call: func() error {
					_, err := runtime.Compile(
						_NilGuardSource(),
						runtime.WithPlugins(
							nil,
							nil,
						),
					)

					return err
				},
			},
		}
	)

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

				if tc.wantErr && err == nil {
					t.Fatal("want an error, got nil")
				}

				if !tc.wantErr && err != nil {
					t.Fatalf(
						"want no error, got %v",
						err,
					)
				}
			},
		)
	}
}

// TestZeroExecution proves a zero Execution answers Status and Snapshot with
// an empty graph, never nil and never a panic.
//
// Revisions:
//   - 2026-11-04 23:50: initial creation (C86)
//   - 2026-11-07 23:23: tests the concrete Execution, the runtime one being an
//     interface now (C91w7)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestZeroExecution(t *testing.T) {
	cases := []struct {
		name string
		ask  func(e *observe.Execution) *workflowpb.Graph
	}{
		{
			name: "Status",
			ask: func(e *observe.Execution) *workflowpb.Graph {
				return e.Status()
			},
		},
		{
			name: "Snapshot",
			ask: func(e *observe.Execution) *workflowpb.Graph {
				return e.Snapshot()
			},
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var graph *workflowpb.Graph

				_Survives(
					t,
					func() {
						graph = tc.ask(new(observe.Execution))
					},
				)

				if graph == nil {
					t.Fatal("want an empty graph, got nil")
				}

				if len(graph.GetFunctions()) != 0 {
					t.Fatalf(
						"want no functions, got %d",
						len(graph.GetFunctions()),
					)
				}
			},
		)
	}
}

// TestErrSource proves a nil source is the one sentinel, whichever call
// refuses it.
//
// Revisions:
//   - 2026-11-04 23:50: initial creation (C86)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: renamed from TestErrNoSource; uses runtime.Merge and ErrSource, formerly
//     Amalgamate and ErrNoSource (C121, Q339, Q348)
func TestErrSource(t *testing.T) {
	cases := []struct {
		name string
		call func() error
	}{
		{
			name: "Run",
			call: func() error {
				_, err := runtime.Run(
					context.Background(),
					nil,
				)

				return err
			},
		},
		{
			name: "Compile",
			call: func() error {
				_, err := runtime.Compile(nil)

				return err
			},
		},
		{
			name: "Merge",
			call: func() error {
				_, err := runtime.Merge(nil)

				return err
			},
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				err := tc.call()
				if !errors.Is(
					err,
					runtime.ErrSource,
				) {
					t.Fatalf(
						"want ErrSource, got %v",
						err,
					)
				}
			},
		)
	}
}

// _NilPlugin is a plugin whose methods dereference the receiver, so a call to
// Name on a nil *_NilPlugin panics.
//
// Revisions:
//   - 2026-11-05 00:19: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
type _NilPlugin struct {
	name string
}

// Name is the plugin's name.
//
// Revisions:
//   - 2026-11-04 23:50: initial creation (C86 fix round 1)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func (p *_NilPlugin) Name() string {
	return p.name
}

// Values is the plugin's names.
//
// Revisions:
//   - 2026-11-04 23:50: initial creation (C86 fix round 1)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func (p *_NilPlugin) Values() starlark.StringDict {
	return starlark.StringDict{
		p.name: starlark.None,
	}
}

// TestTypedNilPlugin proves a plugin that is an interface holding a nil
// pointer is skipped like a nil plugin, and Name is never called on it.
//
// Revisions:
//   - 2026-11-04 23:50: initial creation (C86 fix round 1)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestTypedNilPlugin(t *testing.T) {
	var typed *_NilPlugin

	_Survives(
		t,
		func() {
			_, err := runtime.Compile(
				_NilGuardSource(),
				runtime.WithPlugins(typed),
			)
			if err != nil {
				t.Fatalf(
					"want no error, got %v",
					err,
				)
			}
		},
	)
}

// TestZeroExecutionWait proves Wait on a zero Execution returns an error at
// once, and Done is closed, so a select on it does not block.
//
// Revisions:
//   - 2026-11-04 23:50: initial creation (C86 fix round 1)
//   - 2026-11-07 23:23: tests the concrete Execution, the runtime one being an
//     interface now (C91w7)
//   - 2026-11-08 13:25: the error is a plain one, ErrNotStarted being dropped (C98, Q249)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestZeroExecutionWait(t *testing.T) {
	cases := []struct {
		name string
		call func(e *observe.Execution) error
	}{
		{
			name: "Wait",
			call: func(e *observe.Execution) error {
				_, err := e.Wait()

				return err
			},
		},
		{
			name: "Done in a select",
			call: func(e *observe.Execution) error {
				select {
				case <-e.Done():
					return errors.New("Done closed: the run was not started")
				case <-time.After(time.Second):
					return errors.New("Done blocked")
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var err error

				finished := make(chan struct{})

				go func() {
					defer close(finished)

					err = tc.call(new(observe.Execution))
				}()

				select {
				case <-finished:
				case <-time.After(5 * time.Second):
					t.Fatal("blocked")
				}

				if err == nil || !strings.Contains(
					err.Error(),
					"not started",
				) {
					t.Fatalf(
						"want a not-started error, got %v",
						err,
					)
				}
			},
		)
	}
}

// _ValuePlugin is a plugin of a kind that cannot be nil: a named string.
//
// Revisions:
//   - 2026-11-05 00:19: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
type _ValuePlugin string

// Name is the plugin's name.
//
// Revisions:
//   - 2026-11-04 23:50: initial creation (C86 fix round 2)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func (p _ValuePlugin) Name() string {
	return string(p)
}

// Values is one name, which a script can read.
//
// Revisions:
//   - 2026-11-04 23:50: initial creation (C86 fix round 2)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func (p _ValuePlugin) Values() starlark.StringDict {
	return starlark.StringDict{
		string(p): starlark.MakeInt(7),
	}
}

// TestValuePlugin proves a plugin of a kind that cannot be nil is kept, not
// skipped: the skip is for nothing behind the interface, and a named string
// always has something.
//
// Revisions:
//   - 2026-11-04 23:50: initial creation (C86 fix round 2)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses runtime.Eval, formerly Evaluate (C121, Q338)
func TestValuePlugin(t *testing.T) {
	art, err := runtime.Compile(
		&common.Source{
			Entry: "a.star",
			Text:  []byte("def main():\n    return seven\n"),
		},
		runtime.WithPlugins(_ValuePlugin("seven")),
	)
	if err != nil {
		t.Fatalf(
			"want no error, got %v",
			err,
		)
	}

	result, err := runtime.Eval(
		context.Background(),
		art,
	)
	if err != nil {
		t.Fatalf(
			"want no error, got %v",
			err,
		)
	}

	if result.Value.String() != "7" {
		t.Fatalf(
			"want 7, got %v",
			result.Value,
		)
	}
}
