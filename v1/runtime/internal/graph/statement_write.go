package graph

import (
	"fmt"

	"go.starlark.net/syntax"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// One direction of one subject lives here: writing a flow's statements back as
// lines. The other direction is in statement_read.go.

// Code is a function's body.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the statement nodes
//   - 2026-10-02 00:04: lifted into graph, taking the generated Function
//   - 2026-11-02 10:25: moved to statement_write.go
//   - 2026-11-06 00:16: quotes a name a flow can hold (C63, B128, Q208)
//   - 2026-11-08 10:35: a nil program is ErrForm (C108, B191)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 00:25: renamed from _Code, as a test in tests/ needs it (C120a, Q314)
func Code(
	prog *Program,
	fn *workflowpb.Function,
) ([]syntax.Stmt, error) {
	if prog == nil {
		return nil, fmt.Errorf(
			"program: %w",
			ErrForm,
		)
	}

	switch held := fn.GetCode().(type) {
	case *workflowpb.Function_Body:
		return _Parsed(
			prog,
			fn.GetName(),
			held.Body,
		)

	case *workflowpb.Function_Statements:
		return _ListBody(
			fn.GetName(),
			held.Statements.GetStatement(),
			prog,
		)

	default:
		return nil, fmt.Errorf(
			"%s: function has no body: %w",
			unpack.QuotePrefix(fn.GetName()),
			ErrForm,
		)
	}
}

// _Entry is main's body.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the statement nodes
//   - 2026-10-02 00:04: lifted into graph, taking the generated Flow
//   - 2026-11-02 10:25: moved to statement_write.go
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func _Entry(
	prog *Program,
	flow *workflowpb.Flow,
) ([]syntax.Stmt, error) {
	switch held := flow.GetSpine().(type) {
	case *workflowpb.Flow_Text:
		return _Parsed(
			prog,
			common.ENTRY,
			held.Text,
		)

	case *workflowpb.Flow_Main:
		return _ListBody(
			common.ENTRY,
			held.Main.GetStatement(),
			prog,
		)

	default:
		return nil, fmt.Errorf(
			"%s: function has no body: %w",
			common.ENTRY,
			ErrForm,
		)
	}
}

// _ListBody is a statement list. An empty list is not a def.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the statement nodes
//   - 2026-10-02 00:04: lifted into graph, taking the generated Statement
//   - 2026-11-02 10:25: moved to statement_write.go
//   - 2026-11-06 00:16: quotes a name a flow can hold (C63, B128, Q208)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func _ListBody(
	name string,
	list []*workflowpb.Statement,
	prog *Program,
) ([]syntax.Stmt, error) {
	if len(list) == 0 {
		return nil, fmt.Errorf(
			"%s: function has no body: %w",
			unpack.QuotePrefix(name),
			ErrForm,
		)
	}

	return _Statements(
		list,
		prog,
	)
}

// _Statements is a list of statements. A match is more than one.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the statement nodes
//   - 2026-10-02 00:04: lifted into graph, taking the generated Statement
//   - 2026-11-02 10:25: moved to statement_write.go
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func _Statements(
	list []*workflowpb.Statement,
	prog *Program,
) ([]syntax.Stmt, error) {
	var out []syntax.Stmt

	for _, stmt := range list {
		part, err := _Statement(
			stmt,
			prog,
		)
		if err != nil {
			return nil, err
		}

		out = append(
			out,
			part...,
		)
	}

	return out, nil
}

// _Statement is one statement. A match is the assignment and the comparisons.
//
// Revisions:
//   - 2026-10-01 11:57: initial creation
//   - 2026-10-01 13:08: returns the statement nodes
//   - 2026-10-02 00:04: lifted into graph, taking the generated Statement
//   - 2026-11-02 10:25: moved to statement_write.go
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func _Statement(
	stmt *workflowpb.Statement,
	prog *Program,
) ([]syntax.Stmt, error) {
	if stmt == nil || stmt.GetAction() == nil {
		return nil, fmt.Errorf(
			"statement: %w",
			ErrForm,
		)
	}

	switch stmt.GetAction().(type) {
	case *workflowpb.Statement_Call,
		*workflowpb.Statement_Spawn,
		*workflowpb.Statement_Join,
		*workflowpb.Statement_Cancel,
		*workflowpb.Statement_Sleep:
		return _Plain(stmt)

	default:
		return _Nested(
			stmt,
			prog,
		)
	}
}

// _Plain emits a statement that does not contain another statement.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-02 10:25: moved to statement_write.go
func _Plain(stmt *workflowpb.Statement) ([]syntax.Stmt, error) {
	switch stmt.GetAction().(type) {
	case *workflowpb.Statement_Call:
		return _Single(_Call(stmt.GetCall()))

	case *workflowpb.Statement_Spawn:
		return _Single(_Spawn(stmt.GetSpawn()))

	case *workflowpb.Statement_Join:
		return _Single(_Join(stmt.GetJoin()))

	case *workflowpb.Statement_Cancel:
		return _Single(_Cancel(stmt.GetCancel()))

	case *workflowpb.Statement_Sleep:
		return _Single(_Sleep(stmt.GetSleep()))

	default:
		return nil, fmt.Errorf(
			"statement: %w",
			ErrForm,
		)
	}
}

// _Nested emits a statement that contains another statement.
//
// Revisions:
//   - 2026-10-24 18:29: initial creation
//   - 2026-11-02 10:25: moved to statement_write.go
//   - 2026-11-10 01:00: now uses Program, were _Program (C120a, Q314)
func _Nested(
	stmt *workflowpb.Statement,
	prog *Program,
) ([]syntax.Stmt, error) {
	switch stmt.GetAction().(type) {
	case *workflowpb.Statement_Repeat:
		return _Single(_Repeat(stmt.GetRepeat()))

	case *workflowpb.Statement_Retry:
		return _Single(_Retry(stmt.GetRetry()))

	case *workflowpb.Statement_Timeout:
		return _Single(_Timeout(stmt.GetTimeout()))

	case *workflowpb.Statement_Loop:
		return _Single(_Loop(
			stmt.GetLoop(),
			prog,
		))

	case *workflowpb.Statement_If:
		return _Single(_If(
			stmt.GetIf(),
			prog,
		))

	case *workflowpb.Statement_Match:
		return _Match(
			stmt.GetMatch(),
			prog,
		)

	default:
		return nil, fmt.Errorf(
			"statement: %w",
			ErrForm,
		)
	}
}

// _Single is one statement as a list, so a match and a call share a caller.
//
// Revisions:
//   - 2026-10-01 13:08: initial creation
//   - 2026-11-02 10:25: moved to statement_write.go
func _Single(
	stmt syntax.Stmt,
	err error,
) ([]syntax.Stmt, error) {
	if err != nil {
		return nil, err
	}

	return []syntax.Stmt{stmt}, nil
}
