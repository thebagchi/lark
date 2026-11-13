package graph

import (
	"fmt"

	"go.starlark.net/syntax"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
)

// One direction of one subject lives here: writing a loop back as
// for _ in range(times). The other direction is in loop_read.go.

// BLANK is the loop variable a Loop is written with. The flow does not store
// the name, because the body does not read it.
//
// Revisions:
//   - 2026-10-04 01:25: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
const (
	BLANK = "_"
)

// Loop is for _ in range(times) with one statement as the body.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the for node
//   - 2026-10-02 00:04: lifted into graph, taking the generated Loop
//   - 2026-11-02 10:25: moved to loop_write.go
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:00: now uses Loop, Program, were _Loop, _Program (C120a, Q314)
func _Loop(
	loop *workflowpb.Loop,
	prog *Program,
) (syntax.Stmt, error) {
	if loop == nil || loop.GetBody() == nil {
		return nil, fmt.Errorf(
			"loop: function has no body: %w",
			ErrForm,
		)
	}

	times, err := _Times(loop.GetTimes())
	if err != nil {
		return nil, err
	}

	body, err := _Statement(
		loop.GetBody(),
		prog,
	)
	if err != nil {
		return nil, err
	}

	return &syntax.ForStmt{
		Vars: _Name(BLANK),
		X: _Bare(
			RANGE,
			[]syntax.Expr{times},
		),
		Body: body,
	}, nil
}

// Times is a loop count: a name, or a whole number.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the expression node
//   - 2026-10-02 00:04: lifted into graph, taking the generated Operand
//   - 2026-11-02 10:25: moved to loop_write.go
//   - 2026-11-10 01:00: comment now names Times, were _Times (C120a, Q314)
func _Times(op *workflowpb.Operand) (syntax.Expr, error) {
	if op == nil {
		return nil, fmt.Errorf(
			"loop: %w",
			ErrForm,
		)
	}

	switch src := op.GetSource().(type) {
	case *workflowpb.Operand_Name:
		if src.Name == "" {
			return nil, fmt.Errorf(
				"loop: %w",
				ErrForm,
			)
		}

		return _Name(src.Name), nil

	case *workflowpb.Operand_Literal:
		text, err := _Whole(src.Literal)
		if err != nil {
			return nil, err
		}

		return _Num(text), nil

	default:
		return nil, fmt.Errorf(
			"loop: %w",
			ErrForm,
		)
	}
}
