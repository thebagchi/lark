package state

import (
	"errors"
	"fmt"

	"go.starlark.net/syntax"

	"github.com/thebagchi/lark/v1/runtime/plugin/deep"
)

// ErrNotData is returned for something a store cannot usefully hold.
//
// A store exists so that threads pass data to each other. A function is code,
// and a frozen one read back by another thread is the same object the script
// already had; a handle names a thread, and a thread means nothing to whoever
// did not start it. Storing either is a mistake that reads as if it worked -
// the value goes in, comes back out, and does nothing.
var ErrNotData = errors.New("not data a store can hold")

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
func (s *_State) Check(tree *syntax.File) error {
	call, named := deep.ShowsCode(tree, NAME, SET)
	if call == nil {
		return nil
	}

	return fmt.Errorf("%s.%s at %s stores %s: %w", NAME, SET, call.Lparen, named, ErrNotData)
}
