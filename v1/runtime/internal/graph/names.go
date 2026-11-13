package graph

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"go.starlark.net/syntax"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// _Identifiers checks that every name the generated script writes as an
// identifier is one: a function, a parameter, a constant, an argument, a
// called function (a path of identifiers joined by dots), a result, a keyword,
// an operand, a condition, a match expression, a loop count and a handle.
// Text such as "000", "a-b" or "a.b", and a keyword such as pass or class, is
// no identifier, so the script written from it would not parse (B259, Q300).
//
// The parser decides, not a list kept here: a name is an identifier when the
// parser reads the whole of it as one, and a keyword is not read as one.
//
// A result, a spawn binding and a join result may be unset. Every other name
// must be set.
//
// Returns ErrForm, naming the field and quoting the name (Q4). A nil flow has
// no names.
//
// Revisions:
//   - 2026-11-10 11:55: initial creation (C118, B259, Q300)
func _Identifiers(flow *workflowpb.Flow) error {
	err := _Declared(flow)
	if err != nil {
		return err
	}

	for _, call := range _Calls(flow) {
		err = _CallNames(call)
		if err != nil {
			return err
		}
	}

	visit := func(stmt *workflowpb.Statement) {
		if err == nil {
			err = _HeldNames(stmt)
		}
	}

	for _, fn := range flow.GetFunctions() {
		_Each(
			fn.GetStatements().GetStatement(),
			visit,
		)
	}

	_Each(
		flow.GetMain().GetStatement(),
		visit,
	)

	return err
}

// _Declared checks the names a flow declares at its top level: each function
// and its parameters, each constant and each argument.
//
// Revisions:
//   - 2026-11-10 11:55: initial creation (C118, B259, Q300)
func _Declared(flow *workflowpb.Flow) error {
	for _, fn := range flow.GetFunctions() {
		err := _Identified(
			"function",
			fn.GetName(),
		)
		if err != nil {
			return err
		}

		for _, param := range fn.GetParams() {
			err = _Identified(
				"parameter",
				param,
			)
			if err != nil {
				return err
			}
		}
	}

	for _, name := range slices.Sorted(maps.Keys(flow.GetConstants())) {
		err := _Identified(
			"constant",
			name,
		)
		if err != nil {
			return err
		}
	}

	for _, name := range slices.Sorted(maps.Keys(flow.GetArgs())) {
		err := _Identified(
			"argument",
			name,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

// _CallNames checks the names one call writes: the function it calls, which
// may be a path such as hex.to_bytes, the name its result is bound to, the
// name of each keyword, and each operand that is a name.
//
// Revisions:
//   - 2026-11-10 11:55: initial creation (C118, B259, Q300)
func _CallNames(call *workflowpb.Call) error {
	err := _Dotted(
		"call",
		call.GetFunction(),
	)
	if err != nil {
		return err
	}

	err = _Optional(
		"result",
		call.GetResult(),
	)
	if err != nil {
		return err
	}

	operands := slices.Clone(call.GetOperands())

	for _, keyword := range call.GetKeywords() {
		err = _Identified(
			"keyword",
			keyword.GetName(),
		)
		if err != nil {
			return err
		}

		operands = append(
			operands,
			keyword.GetValue(),
		)
	}

	for _, op := range operands {
		err = _OperandName(op)
		if err != nil {
			return err
		}
	}

	return nil
}

// _HeldNames checks the names one statement writes itself, not counting those
// of its calls or of the statements it holds: a spawn's binding, the handles
// a join or a cancel names and the join's result, the name an if tests, the
// name a match tests and the name a loop counts with.
//
// Revisions:
//   - 2026-11-10 11:55: initial creation (C118, B259, Q300)
func _HeldNames(stmt *workflowpb.Statement) error {
	err := _Optional(
		"handle",
		stmt.GetSpawn().GetBinding(),
	)
	if err != nil {
		return err
	}

	err = _Optional(
		"result",
		stmt.GetJoin().GetResult(),
	)
	if err != nil {
		return err
	}

	handles := slices.Concat(
		stmt.GetJoin().GetBindings(),
		stmt.GetCancel().GetBindings(),
	)

	for _, handle := range handles {
		err = _Identified(
			"handle",
			handle,
		)
		if err != nil {
			return err
		}
	}

	named, ok := stmt.GetIf().GetCondition().GetKind().(*workflowpb.Condition_Name)
	if ok {
		err = _Identified(
			"condition",
			named.Name,
		)
		if err != nil {
			return err
		}
	}

	tested, ok := stmt.GetMatch().GetExpression().GetKind().(*workflowpb.Expression_Name)
	if ok {
		err = _Identified(
			"match",
			tested.Name,
		)
		if err != nil {
			return err
		}
	}

	return _OperandName(stmt.GetLoop().GetTimes())
}

// _OperandName checks an operand that is a name.
//
// Revisions:
//   - 2026-11-10 11:55: initial creation (C118, B259, Q300)
func _OperandName(op *workflowpb.Operand) error {
	named, ok := op.GetSource().(*workflowpb.Operand_Name)
	if !ok {
		return nil
	}

	return _Identified(
		"operand",
		named.Name,
	)
}

// _Dotted is _Identified for a path of identifiers joined by dots, which is
// how a flow names a member of a predeclared name, such as hex.to_bytes or
// a.b.c. Every part must be an identifier.
//
// Revisions:
//   - 2026-11-10 11:55: initial creation (C118, B259, Q300)
func _Dotted(
	field string,
	name string,
) error {
	for part := range strings.SplitSeq(
		name,
		".",
	) {
		if !_Identifier(part) {
			return _Refused(
				field,
				name,
			)
		}
	}

	return nil
}

// _Optional is _Identified for a name a flow may leave unset.
//
// Revisions:
//   - 2026-11-10 11:55: initial creation (C118, B259, Q300)
func _Optional(
	field string,
	name string,
) error {
	if name == "" {
		return nil
	}

	return _Identified(
		field,
		name,
	)
}

// _Identified is nil when name is an identifier the script can bind or read,
// and ErrForm when it is not.
//
// Revisions:
//   - 2026-11-10 11:55: initial creation (C118, B259, Q300)
func _Identified(
	field string,
	name string,
) error {
	if _Identifier(name) {
		return nil
	}

	return _Refused(
		field,
		name,
	)
}

// _Refused is the refusal of a name that is not an identifier. The name is
// quoted and cut (Q4).
//
// Revisions:
//   - 2026-11-10 11:55: initial creation (C118, B259, Q300)
func _Refused(
	field string,
	name string,
) error {
	return fmt.Errorf(
		"%s: %s is not a Starlark identifier: %w",
		unpack.QuotePrefix(name),
		field,
		ErrForm,
	)
}

// _Identifier reports whether the parser reads all of name as one identifier.
// A keyword, a number, a name with a space, a dot or a dash, and the empty name
// are not.
//
// Revisions:
//   - 2026-11-10 11:55: initial creation (C118, B259, Q300)
func _Identifier(name string) bool {
	expr, err := common.OPTIONS.ParseExpr(
		EXPRESSION,
		name,
		0,
	)
	if err != nil {
		return false
	}

	ident, ok := expr.(*syntax.Ident)

	return ok && ident.Name == name
}
