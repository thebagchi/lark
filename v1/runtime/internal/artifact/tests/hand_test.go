package tests

import (
	"bytes"
	"testing"

	"go.starlark.net/starlark"

	artifactpb "github.com/thebagchi/lark/proto/gen/artifact"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/script"
	"github.com/thebagchi/lark/v1/runtime/plugin"
)

// _Hand builds a bundle of many units by hand, one unit per file, dependencies
// first.
//
// Compile merges every load into one unit, so it can no longer make such a
// bundle; the format still carries many, and Load and Save must keep reading
// and writing them.
//
// Revisions:
//   - 2026-11-03 06:29: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Hand struct {
	t      *testing.T
	walk   *script.Walk
	known  func(string) bool
	bundle *artifactpb.Artifact
}

// _Handmade is the bundle of the named fixture and every file it loads, one
// unit per file.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Handmade(
	t *testing.T,
	entry string,
) *artifactpb.Artifact {
	t.Helper()

	env, err := plugin.Environment(plugin.Registered())
	if err != nil {
		t.Fatalf(
			"environment: %v",
			err,
		)
	}

	env[common.CALL] = nil

	hand := &_Hand{
		t: t,
		walk: script.NewWalk(
			_Loader(),
			entry,
		),
		known: env.Has,
		bundle: &artifactpb.Artifact{
			Entry: entry,
		},
	}

	err = hand._Add(
		entry,
		_Fixture(
			t,
			entry,
		),
	)
	if err != nil {
		t.Fatalf(
			"add %s: %v",
			entry,
			err,
		)
	}

	return hand.bundle
}

// _Add compiles the file as a unit, after the units it loads.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (h *_Hand) _Add(
	name string,
	src []byte,
) error {
	_, code, err := starlark.SourceProgramOptions(
		common.OPTIONS,
		name,
		src,
		h.known,
	)
	if err != nil {
		h.t.Fatalf(
			"compile %s: %v",
			name,
			err,
		)
	}

	unit := &artifactpb.Unit{
		Name:  name,
		Loads: map[string]string{},
	}

	for index := range code.NumLoads() {
		var (
			module, _ = code.Load(index)
			got, err  = h.walk.Reach(
				name,
				module,
				h._Add,
			)
		)
		if err != nil {
			h.t.Fatalf(
				"reach %s: %v",
				module,
				err,
			)
		}

		unit.Loads[module] = got
	}

	var encoded bytes.Buffer

	err = code.Write(&encoded)
	if err != nil {
		h.t.Fatalf(
			"write %s: %v",
			name,
			err,
		)
	}

	unit.Code = encoded.Bytes()
	h.bundle.Units = append(
		h.bundle.Units,
		unit,
	)

	return nil
}
