package state

import (
	"fmt"

	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/plugin/deep"
	"github.com/thebagchi/lark/v1/runtime/plugin/state/internal/spec"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// Check refuses a set of something the source already shows is not data.
//
// Only what is visible. state.set("k", helper) names a function this file
// declares, and a lambda is one written in place - both are certain before
// anything runs, and a script author would rather hear it then. Everything
// else is caught when it runs, by the same rule: a call's result, a value
// read from somewhere, a function an update returns.
//
// This is a Checking, which the compiler asks every plugin for. The compiler
// does not know what set means, and does not have to - and it does not ask at
// all about a file that has taken the name state for itself, so nothing here
// has to remember that a global shadows a predeclared one.
//
// Revisions:
//   - 2026-09-24 20:10: initial creation
//   - 2026-09-30 22:41: reads the source through deep.ShowsCode, which event
//     asks the same question of
//   - 2026-11-08 00:27: names read from internal/spec (C91w12a)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 17:15: uses ErrConvert, formerly ErrNotProtoValue (C121, Q349)
func (s *_State) Check(tree *syntax.File) error {
	call, named, err := deep.ShowsCode(
		tree,
		spec.NAME,
		spec.SET,
	)
	if err != nil {
		return err
	}

	if call == nil {
		return nil
	}

	return fmt.Errorf(
		"%s.%s at %s stores %s: not data a store can hold: %w",
		spec.NAME,
		spec.SET,
		call.Lparen,
		named,
		unpack.ErrConvert,
	)
}
