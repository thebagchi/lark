package core

import (
	"github.com/thebagchi/lark/v1/runtime/plugin/core/internal/builtin"
)

var (
	// ErrAssert is builtin.ErrAssert.
	//
	// Revisions:
	//   - 2026-11-10 01:37: initial creation (C120b, Q319)
	ErrAssert = builtin.ErrAssert

	// ErrCondition is builtin.ErrCondition.
	//
	// Revisions:
	//   - 2026-11-10 01:37: initial creation (C120b, Q319)
	//   - 2026-11-10 17:15: renamed from ErrNotACondition (C121, Q348)
	ErrCondition = builtin.ErrCondition
)
