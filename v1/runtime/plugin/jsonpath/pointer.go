// Package jsonpath gives a script RFC 6901 pointers and RFC 6902 patch over
// Starlark values. Importing it is what enables it.
//
// It works on Starlark dicts and lists directly rather than on decoded JSON, so
// nothing round-trips through text on the way.
//
// Revisions:
//   - 2026-09-20 00:29: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
package jsonpath

import (
	"github.com/thebagchi/lark/v1/runtime/plugin/jsonpath/internal/patch"
)

var (
	// ErrPointer is patch.ErrPointer.
	//
	// Revisions:
	//   - 2026-11-10 01:37: initial creation (C120b, Q319)
	ErrPointer = patch.ErrPointer

	// ErrMissing is patch.ErrMissing.
	//
	// Revisions:
	//   - 2026-11-10 01:37: initial creation (C120b, Q319)
	ErrMissing = patch.ErrMissing

	// ErrKind is patch.ErrKind.
	//
	// Revisions:
	//   - 2026-11-10 01:37: initial creation (C120b, Q319)
	ErrKind = patch.ErrKind
)
