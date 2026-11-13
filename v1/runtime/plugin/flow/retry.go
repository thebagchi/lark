package flow

import (
	"github.com/thebagchi/lark/v1/runtime/plugin/flow/internal/attempt"
)

var (
	// ErrTimeout is attempt.ErrTimeout.
	//
	// Revisions:
	//   - 2026-11-10 01:37: initial creation (C120b, Q319)
	ErrTimeout = attempt.ErrTimeout
)
