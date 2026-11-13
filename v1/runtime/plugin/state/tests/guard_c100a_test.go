package tests

// This file tests SetKeep, Kept and Entry.Apply against a nil builtin, a keep
// with no entry and a nil change. No script can build these inputs, so the
// tests call the functions directly (C100a).
//
// Revisions:
//   - 2026-11-08 22:46: states the reason in the first comment (C65b)
//   - 2026-11-10 01:37: moved to tests/ from guard_c100a_internal_test.go and rewrote its
//     header (C120b, Q311, Q322)

import (
	"errors"
	"sync"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/state/internal/store"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// GUARD_FN is a builtin for the keeps a test builds.
//
// Revisions:
//   - 2026-11-06 06:50: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
var (
	GUARD_FN = starlark.NewBuiltin(
		"set",
		nil,
	)
)

// TestGuard_C100a_StateNil proves each function refuses a nil it dereferenced.
//
// Cases from Go-level breaker, findings state-F2, F7 and F18 (bugs B172, B174
// and B177). The builtin and the callable are nil, and the keep lacks an
// entry or a value.
//
// Revisions:
//   - 2026-11-06 00:08: initial creation (C100a)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrRun and ErrConvert, formerly ErrNoRun and ErrNotProtoValue (C121,
//     Q349)
func TestGuard_C100a_StateNil(t *testing.T) {
	tests := []struct {
		name string
		call func() error
		want error
	}{
		{
			"Apply nil change",
			func() error {
				_, err := new(store.Entry).Apply(
					new(starlark.Thread),
					"k",
					nil,
				)

				return err
			},
			scheduler.ErrRun,
		},
		{
			"SetKeep nil builtin",
			func() error {
				_, err := store.SetKeep(
					new(starlark.Thread),
					nil,
					"k",
					starlark.String("v"),
				)

				return err
			},
			scheduler.ErrRun,
		},
		{
			"Kept nil keep",
			func() error {
				_, err := store.Kept(
					nil,
					nil,
				)

				return err
			},
			scheduler.ErrRun,
		},
		{
			"Kept keep with no builtin",
			func() error {
				_, err := store.Kept(
					nil,
					&store.Keep{
						Held:  new(store.Entry),
						Value: starlark.None,
					},
				)

				return err
			},
			scheduler.ErrRun,
		},
		{
			"Kept keep with no entry",
			func() error {
				_, err := store.Kept(
					nil,
					&store.Keep{
						Fn:    GUARD_FN,
						Value: starlark.None,
					},
				)

				return err
			},
			scheduler.ErrRun,
		},
		{
			"Kept keep with no value",
			func() error {
				_, err := store.Kept(
					nil,
					&store.Keep{
						Fn:   GUARD_FN,
						Held: new(store.Entry),
					},
				)

				return err
			},
			unpack.ErrConvert,
		},
		{
			"Kept nil evaluation",
			func() error {
				_, err := store.Kept(
					nil,
					&store.Keep{
						Fn:    GUARD_FN,
						Held:  new(store.Entry),
						Value: starlark.None,
					},
				)

				return err
			},
			scheduler.ErrRun,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				err := tt.call()
				if !errors.Is(
					err,
					tt.want,
				) {
					t.Fatalf(
						"got %v, want %v",
						err,
						tt.want,
					)
				}
			},
		)
	}
}

// TestGuard_C100a_EntryRace proves _Value reads while Keep writes, under
// -race, and that an entry nothing was kept in reads as None.
//
// Case from Go-level breaker, finding state-F20 (bug B178; Q212). The breaker
// wrote the field directly; the field is now an atomic pointer.
//
// Revisions:
//   - 2026-11-06 00:45: initial creation (C100a)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestGuard_C100a_EntryRace(t *testing.T) {
	var (
		entry = new(store.Entry)
		group sync.WaitGroup
	)

	if entry.Load() != starlark.None {
		t.Fatal("a new entry is not None")
	}

	group.Add(2)

	go func() {
		defer group.Done()

		for range 2000 {
			entry.Keep(starlark.String("v"))
		}
	}()

	go func() {
		defer group.Done()

		for range 2000 {
			if entry.Load() == nil {
				t.Error("got a nil value")
			}
		}
	}()

	group.Wait()
}
