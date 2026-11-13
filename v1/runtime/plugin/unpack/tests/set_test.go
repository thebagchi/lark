package tests

// This file tests that Set names the key when the dict refuses it. The frozen
// dict that makes it fail is one no caller passes: FromProto always builds a
// fresh dict, so no exported call reaches the failure.
//
// Revisions:
//   - 2026-11-08 22:46: states the reason in the first comment (C65b)
//   - 2026-11-10 01:37: moved to tests/ from value_internal_test.go and rewrote its header
//     (C120b, Q311, Q322)

import (
	"errors"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack/internal/proto"
)

// TestSet_NamesTheKey checks that a failing SetKey names the key and keeps
// the cause. A frozen dict makes SetKey fail.
//
// Revisions:
//   - 2026-11-02 11:19: initial creation
//   - 2026-11-02 11:24: renamed ErrNotProtoValue
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses ErrConvert, formerly ErrNotProtoValue (C121, Q349)
func TestSet_NamesTheKey(t *testing.T) {
	into := starlark.NewDict(1)
	into.Freeze()

	err := proto.Set(
		into,
		"wanted-key",
		starlark.MakeInt(1),
	)
	if err == nil {
		t.Fatal("a frozen dict took a key")
	}

	if !strings.Contains(
		err.Error(),
		"wanted-key",
	) {
		t.Fatalf(
			"error does not name the key: %v",
			err,
		)
	}

	if !errors.Is(
		err,
		unpack.ErrConvert,
	) {
		t.Fatalf(
			"error is not ErrConvert: %v",
			err,
		)
	}

	if !strings.Contains(
		err.Error(),
		"frozen",
	) {
		t.Fatalf(
			"error lost its cause: %v",
			err,
		)
	}
}
