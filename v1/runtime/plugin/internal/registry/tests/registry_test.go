// This file tests the guarded form of a registered plugin and its cached values
// (TestGuarding_CheckForwards, TestValues_CachedValuesAreFrozen). Both tests
// register a plugin and then remove it, by cutting the registered list back to
// its old length. Registered returns a copy, so a plugin left behind would clash
// with itself on a second run (go test -count=2).
//
// Revisions:
//   - 2026-11-07 18:16: moved from package plugin, where they cut the exported DEFAULT (C91w1)
//   - 2026-11-10 00:25: moved to tests/ from registry_internal_test.go and rewrote its header
//     (C120a, Q311, Q322)
package tests

import (
	"errors"
	"strings"
	"sync"
	"testing"

	"go.starlark.net/starlark"
	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/internal/registry"
)

// _Plain is a plugin that is not a Checking.
//
// Revisions:
//   - 2026-11-07 18:16: initial creation (C91w1)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Plain struct{}

// Name is what this plugin is called.
//
// Revisions:
//   - 2026-11-07 18:16: initial creation (C91w1)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (p *_Plain) Name() string {
	return "plain"
}

// Values returns no names.
//
// Revisions:
//   - 2026-11-07 18:16: initial creation (C91w1)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (p *_Plain) Values() starlark.StringDict {
	return starlark.StringDict{}
}

// _Checking is a registered-style plugin that is also a Checking, and counts
// the trees it was asked about.
//
// Revisions:
//   - 2026-11-04 22:48: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Checking struct {
	guard sync.Mutex
	asked int
}

// Name is what this plugin is called.
//
// Revisions:
//   - 2026-11-04 22:35: initial creation (Q142)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (c *_Checking) Name() string {
	return "checking"
}

// Values returns one name.
//
// Revisions:
//   - 2026-11-04 22:35: initial creation (Q142)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (c *_Checking) Values() starlark.StringDict {
	return starlark.StringDict{
		"checking": starlark.MakeInt(1),
	}
}

// Check refuses every tree and counts the asks.
//
// Revisions:
//   - 2026-11-04 22:35: initial creation (Q142)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (c *_Checking) Check(*syntax.File) error {
	c.guard.Lock()
	defer c.guard.Unlock()

	c.asked++

	return ErrRefused
}

// TestGuarding_CheckForwards proves a registered plugin's own Check still runs
// through the wrapper, and a plugin with none is passed over.
//
// Revisions:
//   - 2026-11-04 22:35: initial creation (Q142)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestGuarding_CheckForwards(t *testing.T) {
	var (
		held   = new(_Checking)
		before = len(registry.DEFAULT)
		tree   = new(syntax.File)
	)

	registry.Register(held)

	defer func() {
		registry.DEFAULT = registry.DEFAULT[:before]
	}()

	checker, ok := registry.DEFAULT[before].(common.Checking)
	if !ok {
		t.Fatal("a registered plugin is not a Checking")
	}

	if checker.Check(tree) == nil || held.asked != 1 {
		t.Fatalf(
			"Check not forwarded: asked %d",
			held.asked,
		)
	}

	plain := registry.Wrapped(new(_Plain))
	if plain.Check(tree) != nil {
		t.Fatal("a plugin with no Check refused a tree")
	}
}

// _Shared is a registered plugin that hands out a list and a dict, which a
// script or a host could change if they were not frozen.
//
// Revisions:
//   - 2026-11-04 22:48: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Shared struct {
	list *starlark.List
	dict *starlark.Dict
}

// Name is what this plugin is called.
//
// Revisions:
//   - 2026-11-04 22:35: initial creation (Q142)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (s *_Shared) Name() string {
	return "shared"
}

// Values returns the list and the dict.
//
// Revisions:
//   - 2026-11-04 22:35: initial creation (Q142)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (s *_Shared) Values() starlark.StringDict {
	return starlark.StringDict{
		"xs": s.list,
		"ds": s.dict,
	}
}

// TestValues_CachedValuesAreFrozen proves what a registered plugin supplies is
// frozen once cached: a change from Go and a change from a script are both
// refused.
//
// Revisions:
//   - 2026-11-04 22:35: initial creation (Q142)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestValues_CachedValuesAreFrozen(t *testing.T) {
	var (
		held = &_Shared{
			list: starlark.NewList(nil),
			dict: starlark.NewDict(0),
		}
		before = len(registry.DEFAULT)
	)

	registry.Register(held)

	defer func() {
		registry.DEFAULT = registry.DEFAULT[:before]
	}()

	env, err := registry.Environment(registry.Registered())
	if err != nil {
		t.Fatalf(
			"environment: %v",
			err,
		)
	}

	list, ok := env["xs"].(*starlark.List)
	if !ok {
		t.Fatalf(
			"xs is %T",
			env["xs"],
		)
	}

	if list.Append(starlark.None) == nil {
		t.Fatal("Go appended to a cached list")
	}

	dict := env["ds"].(*starlark.Dict)
	if dict.SetKey(
		starlark.String("k"),
		starlark.None,
	) == nil {
		t.Fatal("Go set a key of a cached dict")
	}

	for _, source := range []string{"xs.append(1)\n", "ds[\"k\"] = 1\n"} {
		_, err = starlark.ExecFileOptions(
			common.OPTIONS,
			new(starlark.Thread),
			"frozen.star",
			[]byte(source),
			env,
		)
		if err == nil || !strings.Contains(
			err.Error(),
			"frozen",
		) {
			t.Fatalf(
				"script %q: err = %v, want a frozen refusal",
				source,
				err,
			)
		}
	}
}

// ErrRefused is what _Checking returns for every tree.
//
// Revisions:
//   - 2026-11-07 18:16: initial creation, beside _Checking (C91w1)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
var (
	ErrRefused = errors.New("refused")
)
