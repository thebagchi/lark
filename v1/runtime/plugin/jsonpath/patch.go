package jsonpath

import (
	"github.com/thebagchi/lark/v1/runtime/plugin/jsonpath/internal/patch"
)

var (
	// ErrOperation is patch.ErrOperation.
	//
	// Revisions:
	//   - 2026-11-10 01:37: initial creation (C120b, Q319)
	ErrOperation = patch.ErrOperation

	// ErrTest is patch.ErrTest.
	//
	// Revisions:
	//   - 2026-11-10 01:37: initial creation (C120b, Q319)
	ErrTest = patch.ErrTest

	// ErrInto is patch.ErrInto.
	//
	// Revisions:
	//   - 2026-11-10 01:37: initial creation (C120b, Q319)
	ErrInto = patch.ErrInto
)
