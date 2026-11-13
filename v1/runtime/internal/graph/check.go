package graph

import (
	"errors"
	"fmt"
	"maps"
	"slices"
	"strconv"

	"go.starlark.net/syntax"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/script"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

var (
	// ErrFork is returned for a join or a cancel naming a binding that
	// no spawn earlier on its statement list holds, and for two spawns given
	// one binding on one list.
	//
	// A generated script names a thread by the binding its spawn wrote, so a
	// join anywhere else - in the other branch of an if, or before the spawn -
	// names a variable that does not exist, and a second spawn of one name
	// hides the first.
	//
	// Revisions:
	//   - 2026-09-30 20:58: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
	//   - 2026-11-10 17:15: renamed from ErrNotForked (C121, Q348)
	ErrFork = errors.New("a thread is joined or cancelled where it was not spawned")

	// ErrUnresolved is returned for a name that resolves to nothing: not a
	// parameter of the function holding it, a result or a binding earlier on
	// its list, or a function, constant or argument the flow declares. A
	// constant sees only functions and constants, since it is computed before
	// anything else a name could hold is bound. It is also returned for a
	// load that names no module, which the parser does not produce: it exists
	// so the assertion has a sentinel rather than borrowing one that means
	// something else.
	//
	// Revisions:
	//   - 2026-09-30 20:58: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
	ErrUnresolved = errors.New("a name resolves to nothing")
)

const (
	// SOUND names a text that Check parses, where a parse needs a name for it. The name is
	// not in the words of a refusal.
	//
	// Revisions:
	//   - 2026-11-13 19:29: initial creation (C133, B295, Q449)
	SOUND = "body"
)

// Check is what a flow must satisfy before anything generates from it.
//
// Exported because a host taking a flow from a user interface needs it, and
// that host is not this package. Emit does not call it: generating is the
// caller's decision and this is the caller's check, so a host that has already
// validated does not pay twice.
//
// _Distinct is part of it, so a host validating a whole flow asks once.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-09-21 16:25: includes Distinct, which its own first line had always
//     claimed
//   - 2026-09-30 00:44: refuses a join or a cancel of a thread not forked
//     there, a fork whose call is not its thread's, and a parameter its call
//     cannot see
//   - 2026-10-02 00:13: checks a Flow: bodies, the arity of every call, a
//     call carrying both lists, constants, and each statement list's names
//   - 2026-11-10 11:55: refuses a name that is no identifier, or is a keyword (C118, B259, Q300)
//   - 2026-11-11 22:57: refuses a spawn or a timeout in init (C127, B274, Q383, Q388, Q389)
//   - 2026-11-13 10:15: refuses a value that is none, an int that is no integer, a float
//     that is not finite, and a value nested too deep (C132, Q2, Q407, Q421)
//   - 2026-11-13 19:29: refuses a text or a body that is no Starlark, and a constant, an
//     argument, an operand or a statement that states nothing (C133, B295, B296, Q449, Q453)
//   - 2026-11-13 23:55: calls _Args, formerly _Arity (C136, Q475)
func Check(flow *workflowpb.Flow) error {
	err := _Identifiers(flow)
	if err != nil {
		return err
	}

	err = _InitStarts(flow)
	if err != nil {
		return err
	}

	err = _Distinct(flow)
	if err != nil {
		return err
	}

	err = _Bodies(flow)
	if err != nil {
		return err
	}

	err = _Sound(flow)
	if err != nil {
		return err
	}

	err = _Formed(flow)
	if err != nil {
		return err
	}

	err = _Args(flow)
	if err != nil {
		return err
	}

	err = _Computed(flow)
	if err != nil {
		return err
	}

	err = _Literals(flow)
	if err != nil {
		return err
	}

	return _Resolved(flow)
}

// _Bodies checks that every function, and main, has something to write.
//
// Code unset, an empty statement list, and text that is empty or only comments
// are each nothing: the closer a generator writes is not a body.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
//   - 2026-11-04 19:26: an init entry with no call and no text is nothing
//     (C44c, B054)
//   - 2026-11-05 23:52: quotes or bounds a name a script chose (C63, B128, Q4)
//   - 2026-11-06 00:16: quotes a name a flow can hold (C63, B128, Q208)
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
func _Bodies(flow *workflowpb.Flow) error {
	for _, fn := range flow.GetFunctions() {
		var empty bool

		switch held := fn.GetCode().(type) {
		case *workflowpb.Function_Statements:
			empty = len(held.Statements.GetStatement()) == 0

		case *workflowpb.Function_Body:
			empty = _NotesOnly(held.Body)

		default:
			empty = true
		}

		if empty {
			return fmt.Errorf(
				"%s: function has no body: %w",
				unpack.QuotePrefix(fn.GetName()),
				ErrForm,
			)
		}
	}

	err := _InitBodies(flow)
	if err != nil {
		return err
	}

	var empty bool

	switch held := flow.GetSpine().(type) {
	case *workflowpb.Flow_Main:
		empty = len(held.Main.GetStatement()) == 0

	case *workflowpb.Flow_Text:
		empty = _NotesOnly(held.Text)

	default:
		empty = true
	}

	if empty {
		return fmt.Errorf(
			"%s: function has no body: %w",
			common.ENTRY,
			ErrForm,
		)
	}

	return nil
}

// _Sound checks that every text a flow carries is Starlark: a function body and the
// text of main parse as a suite, and the text of an init entry parses as statements at
// the margin, as Emit reads each of them. What does not parse is ErrForm, with the line
// of the text and the parser's words cut to 100 bytes (Q4). A name that a text reads is
// not resolved here.
//
// Revisions:
//   - 2026-11-13 19:29: initial creation (C133, B295, Q449)
func _Sound(flow *workflowpb.Flow) error {
	for _, fn := range flow.GetFunctions() {
		held, ok := fn.GetCode().(*workflowpb.Function_Body)
		if !ok {
			continue
		}

		err := _Unsound(
			unpack.QuotePrefix(fn.GetName())+": body",
			held.Body,
			_Wrapping,
			1,
		)
		if err != nil {
			return err
		}
	}

	for number, held := range flow.GetInit() {
		form, ok := held.GetForm().(*workflowpb.Init_Text)
		if !ok {
			continue
		}

		err := _Unsound(
			INIT+" "+strconv.Itoa(number+1)+": text",
			form.Text,
			_Flat,
			0,
		)
		if err != nil {
			return err
		}
	}

	text, ok := flow.GetSpine().(*workflowpb.Flow_Text)
	if !ok {
		return nil
	}

	return _Unsound(
		common.ENTRY+": body",
		text.Text,
		_Wrapping,
		1,
	)
}

// _Formed checks that what a flow declares states something Emit can write: a constant
// has a value or a call, an argument has a name, and every operand of every call has a
// source, a keyword's value included. What is refused is ErrForm, with the name cut to
// 100 bytes (Q4). An argument's name need not be its key: the key is the variable the
// script binds, the name is the one the host supplies.
//
// Revisions:
//   - 2026-11-13 20:19: initial creation (C133, B296, Q4, Q453)
func _Formed(flow *workflowpb.Flow) error {
	for _, name := range slices.Sorted(maps.Keys(flow.GetConstants())) {
		if flow.GetConstants()[name].GetKind() == nil {
			return fmt.Errorf(
				"%s: constant has no value and no call: %w",
				unpack.QuotePrefix(name),
				ErrForm,
			)
		}
	}

	for _, name := range slices.Sorted(maps.Keys(flow.GetArgs())) {
		if flow.GetArgs()[name].GetName() == "" {
			return fmt.Errorf(
				"%s: argument has no name: %w",
				unpack.QuotePrefix(name),
				ErrForm,
			)
		}
	}

	for _, call := range _Calls(flow) {
		err := _Sourced(call)
		if err != nil {
			return err
		}
	}

	for _, name := range slices.Sorted(maps.Keys(flow.GetConstants())) {
		err := _Resultless(
			flow.GetConstants()[name].GetCall(),
			"",
		)
		if err != nil {
			return err
		}
	}

	var found error

	visit := func(stmt *workflowpb.Statement) {
		if found == nil {
			found = _Shaped(stmt)
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

	return found
}

// _Shaped checks that one statement has what Emit writes it from: the call of a spawn,
// a repeat, a retry and a timeout, a count, a delay, a duration and a timeout that are
// not below zero, a match with cases and an expression, and no result bound by a call
// that the form writes as a value (a condition, an expression of a match, the call of
// a spawn, a repeat, a retry and a timeout). A statement that holds another is judged
// by the visit that reaches the one inside.
//
// Revisions:
//   - 2026-11-13 20:24: initial creation (C133, B296, Q453)
func _Shaped(stmt *workflowpb.Statement) error {
	switch held := stmt.GetAction().(type) {
	case *workflowpb.Statement_Spawn:
		return _Resultless(
			held.Spawn.GetCall(),
			common.SPAWN,
		)

	case *workflowpb.Statement_Repeat:
		return _Counted(
			common.REPEAT,
			held.Repeat.GetCall(),
			held.Repeat.GetCount(),
			held.Repeat.GetDelayMs(),
		)

	case *workflowpb.Statement_Retry:
		return _Counted(
			common.RETRY,
			held.Retry.GetCall(),
			held.Retry.GetAttempts(),
			held.Retry.GetDelayMs(),
		)

	case *workflowpb.Statement_Timeout:
		return _Counted(
			common.TIMEOUT,
			held.Timeout.GetCall(),
			held.Timeout.GetTimeoutMs(),
			0,
		)

	case *workflowpb.Statement_Sleep:
		if held.Sleep.GetDurationMs() < 0 {
			return fmt.Errorf(
				"%s: a duration is below zero: %w",
				common.SLEEP,
				ErrForm,
			)
		}

	case *workflowpb.Statement_If:
		return _Conditioned(held.If.GetCondition())

	case *workflowpb.Statement_Loop:
		return _Counting(held.Loop.GetTimes())

	case *workflowpb.Statement_Match:
		return _Matched(held.Match)
	}

	return nil
}

// _Conditioned checks that the condition of an if has a kind, and that the call of a
// condition binds no result. A name of a condition is a name that _Identifiers has judged.
//
// Revisions:
//   - 2026-11-13 21:21: initial creation (C133, B296, Q453)
func _Conditioned(cond *workflowpb.Condition) error {
	if cond.GetKind() == nil {
		return fmt.Errorf(
			"if: the condition states nothing: %w",
			ErrForm,
		)
	}

	return _Resultless(
		cond.GetCall(),
		"",
	)
}

// _Counting checks that the count of a loop is a literal or a name. Emit writes no other
// source, an expression included. A name of a count is a name that _Identifiers has
// judged.
//
// Revisions:
//   - 2026-11-13 21:21: initial creation (C133, B296, Q453)
func _Counting(times *workflowpb.Operand) error {
	switch times.GetSource().(type) {
	case *workflowpb.Operand_Literal, *workflowpb.Operand_Name:
		return nil

	default:
		return fmt.Errorf(
			"loop: the count is not a literal or a name: %w",
			ErrForm,
		)
	}
}

// _Counted checks the call of a statement that runs a call a number of times or for a
// time, and that neither number is below zero.
//
// Revisions:
//   - 2026-11-13 20:24: initial creation (C133, B296, Q453)
func _Counted(
	word string,
	call *workflowpb.Call,
	count,
	delay int32,
) error {
	if count < 0 || delay < 0 {
		return fmt.Errorf(
			"%s: a count, a delay or a timeout is below zero: %w",
			word,
			ErrForm,
		)
	}

	return _Resultless(
		call,
		word,
	)
}

// _Matched checks that a match has cases and an expression of a kind, and that the call
// of the expression binds no result.
//
// Revisions:
//   - 2026-11-13 20:24: initial creation (C133, B296, Q453)
func _Matched(match *workflowpb.Match) error {
	if len(match.GetCases()) == 0 || match.GetExpression().GetKind() == nil {
		return fmt.Errorf(
			"%s: no cases or no expression: %w",
			"match",
			ErrForm,
		)
	}

	return _Resultless(
		match.GetExpression().GetCall(),
		"",
	)
}

// _Resultless checks a call that the script writes as a value, not as a statement: it binds no
// result, and when word is not empty the call is there.
//
// Revisions:
//   - 2026-11-13 20:24: initial creation (C133, B296, Q453)
func _Resultless(
	call *workflowpb.Call,
	word string,
) error {
	if call == nil && word != "" {
		return fmt.Errorf(
			"%s: the call is missing: %w",
			word,
			ErrForm,
		)
	}

	if call.GetResult() != "" {
		return fmt.Errorf(
			"call of %s: a call here binds no result: %w",
			unpack.QuotePrefix(call.GetFunction()),
			ErrForm,
		)
	}

	return nil
}

// _Sourced checks that every operand of call, and the value of every keyword, has a
// source.
//
// Revisions:
//   - 2026-11-13 20:19: initial creation (C133, B296, Q453)
func _Sourced(call *workflowpb.Call) error {
	var operands = slices.Clone(call.GetOperands())

	for _, keyword := range call.GetKeywords() {
		operands = append(
			operands,
			keyword.GetValue(),
		)
	}

	for _, operand := range operands {
		if operand.GetSource() == nil {
			return fmt.Errorf(
				"call of %s: operand has no source: %w",
				unpack.QuotePrefix(call.GetFunction()),
				ErrForm,
			)
		}
	}

	return nil
}

// _Unsound is nil when text parses by source, and otherwise the refusal that begins with
// lead (the name and the noun: "f": body, init 2: text), then the line of the text, where
// the parser's line is shifted up by lines, the def a suite is wrapped in, the parser's
// words cut to 100 bytes, and ErrForm.
//
// Revisions:
//   - 2026-11-13 19:29: initial creation (C133, B295, Q4, Q449)
func _Unsound(
	lead,
	text string,
	source func(
		string,
		string,
	) (*_Parse, error),
	lines int32,
) error {
	_, err := source(
		SOUND,
		text,
	)
	if err == nil {
		return nil
	}

	var (
		found syntax.Error
		words = err.Error()
	)

	if errors.As(
		err,
		&found,
	) {
		words = fmt.Sprintf(
			"line %d: %s",
			max(
				found.Pos.Line-lines,
				1,
			),
			found.Msg,
		)
	}

	return fmt.Errorf(
		"%s is not Starlark (%s): %w",
		lead,
		unpack.ShortText(words),
		ErrForm,
	)
}

// _InitStarts checks that no init entry starts a thread: init is the top level
// of the script, and spawn and timeout are refused while the top level runs
// (Q383, Q389).
//
// An init entry is a call, or text. The text is read as statements and searched
// for a call of a builtin script.Starts names, at any depth. A flow that
// declares a function of that name is left alone for it, as the name then means
// that function.
//
// Revisions:
//   - 2026-11-11 22:57: initial creation, as _InitSpawn; takes every builtin script.Starts
//     names, in place of spawn alone (C127, B274, Q383, Q389)
func _InitStarts(flow *workflowpb.Flow) error {
	declared := make(map[string]bool)

	for _, fn := range flow.GetFunctions() {
		declared[fn.GetName()] = true
	}

	builtin := func(name string) bool {
		return !declared[name]
	}

	for _, held := range flow.GetInit() {
		var starting string

		switch form := held.GetForm().(type) {
		case *workflowpb.Init_Call:
			name := form.Call.GetFunction()
			if script.Starts(name) && builtin(name) {
				starting = name
			}

		case *workflowpb.Init_Text:
			file, err := common.OPTIONS.Parse(
				INIT,
				form.Text,
				0,
			)
			if err == nil {
				starting = _Starting(
					file,
					builtin,
				)
			}
		}

		if starting != "" {
			return _Barring(
				INIT+": "+ErrForm.Error(),
				ErrForm,
				starting,
			)
		}
	}

	return nil
}

// _BarredError is the refusal of a spawn or a timeout in init: its text, and the
// two sentinels it matches under errors.Is, kind and script.ErrInit (Q388).
//
// Revisions:
//   - 2026-11-12 06:29: initial creation (C127, B274, Q388)
type _BarredError struct {
	text string
	kind error
}

// Error is the text.
//
// Revisions:
//   - 2026-11-12 06:29: initial creation (C127, B274, Q388)
func (r *_BarredError) Error() string {
	return r.text
}

// Unwrap is the sentinels the refusal matches: ErrForm or ErrCarry, and
// script.ErrInit.
//
// Revisions:
//   - 2026-11-12 06:29: initial creation (C127, B274, Q388)
func (r *_BarredError) Unwrap() []error {
	return []error{
		r.kind,
		script.ErrInit,
	}
}

// _Barring is a refusal of the builtin name in init that starts with lead,
// matches kind and script.ErrInit, and ends with the hint that names the
// builtin.
//
// Revisions:
//   - 2026-11-12 06:29: initial creation (C127, B274, Q388, Q389)
func _Barring(
	lead string,
	kind error,
	name string,
) error {
	return &_BarredError{
		text: fmt.Sprintf(
			"%s (%s cannot start a thread while the module loads; "+
				"call %s from main or a function main calls)",
			lead,
			name,
			name,
		),
		kind: kind,
	}
}

// _Starting is the name of the first builtin script.Starts names that node
// calls, at any depth, and is the builtin by the test builtin; or "".
//
// Revisions:
//   - 2026-11-11 22:57: initial creation, as _Spawns; finds every builtin script.Starts names,
//     where it found spawn alone (C127, B274, Q383, Q389)
func _Starting(
	node syntax.Node,
	builtin func(string) bool,
) string {
	var found string

	syntax.Walk(
		node,
		func(next syntax.Node) bool {
			if found != "" {
				return false
			}

			call, ok := next.(*syntax.CallExpr)
			if !ok {
				return true
			}

			name, ok := call.Fn.(*syntax.Ident)
			if ok && script.Starts(name.Name) && builtin(name.Name) {
				found = name.Name
			}

			return found == ""
		},
	)

	return found
}

// _InitBodies checks that every init entry has a call or text to write.
//
// Revisions:
//   - 2026-11-04 19:26: initial creation (C44c, B054)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
func _InitBodies(flow *workflowpb.Flow) error {
	for _, held := range flow.GetInit() {
		var empty bool

		switch form := held.GetForm().(type) {
		case *workflowpb.Init_Call:
			empty = form.Call == nil

		case *workflowpb.Init_Text:
			empty = _NotesOnly(form.Text)

		default:
			empty = true
		}

		if empty {
			return fmt.Errorf(
				"%s: function has no body: %w",
				INIT,
				ErrForm,
			)
		}
	}

	return nil
}

// _Args checks every call the flow makes against the function it names: in
// a statement, a condition, a match, and a constant.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-09-21 23:53: the thread's own call is among its steps
//   - 2026-10-02 00:13: every call in a Flow, refusing too few as well as too
//     many, a function the flow does not declare, and a call carrying both
//     lists
//   - 2026-11-13 23:55: renamed from _Arity (C136, Q475)
func _Args(flow *workflowpb.Flow) error {
	takes := make(map[string]int)

	for _, fn := range flow.GetFunctions() {
		takes[fn.GetName()] = len(fn.GetParams())
	}

	for _, call := range _Calls(flow) {
		err := _Fits(
			takes,
			call,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

// _Fits checks one call against the parameters of the function it names.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-10-02 00:13: refuses too few, a function the flow does not
//     declare, and both lists at once
//   - 2026-11-04 19:26: counts keywords, checks what a call writes out, and
//     leaves a builtin's count unchecked (C44c, B054)
//   - 2026-11-05 23:52: quotes or bounds a name a script chose (C63, B128, Q4)
//   - 2026-11-06 00:16: quotes a name a flow can hold (C63, B128, Q208)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-13 23:55: uses spelling.ErrArgs, formerly ErrArity (C136, Q475)
func _Fits(
	takes map[string]int,
	call *workflowpb.Call,
) error {
	if _Both(call) {
		return fmt.Errorf(
			"%s: a call carries both args and operands: %w",
			unpack.QuotePrefix(call.GetFunction()),
			ErrForm,
		)
	}

	err := _Phrased(call)
	if err != nil {
		return err
	}

	params, known := takes[call.GetFunction()]
	if call.GetBuiltin() && known {
		return fmt.Errorf(
			"builtin %s is declared by the flow: %w",
			unpack.QuotePrefix(call.GetFunction()),
			ErrUnresolved,
		)
	}

	if call.GetBuiltin() {
		return nil
	}

	if !known {
		return fmt.Errorf(
			"call of %s: %w",
			unpack.QuotePrefix(call.GetFunction()),
			ErrUnresolved,
		)
	}

	passed := len(call.GetArgs()) + len(call.GetOperands()) +
		len(call.GetKeywords())
	if passed == params {
		return nil
	}

	return fmt.Errorf(
		"%s takes %d and is passed %d: %w",
		unpack.QuotePrefix(call.GetFunction()),
		params,
		passed,
		spelling.ErrArgs,
	)
}

// _Phrased checks what a call passes that is written out: every keyword has a
// name, no name is passed twice, and every expression is one expression.
//
// Revisions:
//   - 2026-11-04 19:26: initial creation (C44c, B054)
//   - 2026-11-05 23:52: quotes or bounds a name a script chose (C63, B128, Q4)
//   - 2026-11-06 00:16: quotes a name a flow can hold (C63, B128, Q208)
func _Phrased(call *workflowpb.Call) error {
	seen := make(map[string]bool)

	for _, keyword := range call.GetKeywords() {
		name := keyword.GetName()
		if name == "" || seen[name] {
			return fmt.Errorf(
				"%s: keyword: %w",
				unpack.QuotePrefix(call.GetFunction()),
				ErrForm,
			)
		}

		seen[name] = true
	}

	operands := slices.Clone(call.GetOperands())

	for _, keyword := range call.GetKeywords() {
		operands = append(
			operands,
			keyword.GetValue(),
		)
	}

	for _, op := range operands {
		if _, ok := op.GetSource().(*workflowpb.Operand_Expression); !ok {
			continue
		}

		if !_Alone(op.GetExpression()) {
			return fmt.Errorf(
				"%s: expression: %w",
				unpack.QuotePrefix(call.GetFunction()),
				ErrForm,
			)
		}
	}

	return nil
}

// _Literals checks every value the flow states: a constant, the default of an
// argument, an argument of a call, an operand that is a literal and the count of
// a loop, which is an int. A value is checked by writing it, which is the rule that refuses it
// later, so Check and Emit cannot differ. A flow is untrusted input, so what the
// writer refuses is refused here before anything generates from it (Q2, Q4).
//
// Revisions:
//   - 2026-11-13 10:15: initial creation (C132, Q2, Q407, Q421)
func _Literals(flow *workflowpb.Flow) error {
	for _, name := range slices.Sorted(maps.Keys(flow.GetConstants())) {
		if _, ok := flow.GetConstants()[name].GetKind().(*workflowpb.Constant_Value); !ok {
			continue
		}

		err := _Stated(
			unpack.QuotePrefix(name),
			flow.GetConstants()[name].GetValue(),
		)
		if err != nil {
			return err
		}
	}

	for _, name := range slices.Sorted(maps.Keys(flow.GetArgs())) {
		if flow.GetArgs()[name].GetDefault() == nil {
			continue
		}

		err := _Stated(
			unpack.QuotePrefix(name),
			flow.GetArgs()[name].GetDefault(),
		)
		if err != nil {
			return err
		}
	}

	var found error

	visit := func(stmt *workflowpb.Statement) {
		if found == nil {
			found = _Looped(stmt.GetLoop().GetTimes())
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

	if found != nil {
		return found
	}

	for _, call := range _Calls(flow) {
		err := _Passes(call)
		if err != nil {
			return err
		}
	}

	return nil
}

// _Passes checks the values one call passes: its args, and the literals among its
// operands and its keywords.
//
// Revisions:
//   - 2026-11-13 10:15: initial creation (C132, Q2, Q407, Q421)
func _Passes(call *workflowpb.Call) error {
	for _, arg := range call.GetArgs() {
		err := _Stated(
			unpack.QuotePrefix(call.GetFunction()),
			arg,
		)
		if err != nil {
			return err
		}
	}

	operands := slices.Clone(call.GetOperands())

	for _, keyword := range call.GetKeywords() {
		operands = append(
			operands,
			keyword.GetValue(),
		)
	}

	for _, op := range operands {
		err := _Operated(op)
		if err != nil {
			return err
		}
	}

	return nil
}

// _Looped checks the count of a loop when it is a literal: an int, which range
// takes, and not a float or a string. It passes a name.
//
// Revisions:
//   - 2026-11-13 10:15: initial creation (C132, Q407, Q421)
func _Looped(op *workflowpb.Operand) error {
	if _, ok := op.GetSource().(*workflowpb.Operand_Literal); !ok {
		return nil
	}

	_, err := _Whole(op.GetLiteral())

	return err
}

// _Operated checks an operand when it is a literal, and passes any other.
//
// Revisions:
//   - 2026-11-13 10:15: initial creation (C132, Q2, Q407, Q421)
func _Operated(op *workflowpb.Operand) error {
	if _, ok := op.GetSource().(*workflowpb.Operand_Literal); !ok {
		return nil
	}

	return _Stated(
		"operand",
		op.GetLiteral(),
	)
}

// _Stated checks one value, and names what holds it when it is refused.
//
// Revisions:
//   - 2026-11-13 10:15: initial creation (C132, Q2, Q407, Q421)
func _Stated(
	holder string,
	value *workflowpb.Value,
) error {
	_, err := _Value(value)
	if err != nil {
		return fmt.Errorf(
			"%s: %w",
			holder,
			err,
		)
	}

	return nil
}

// _Calls is every call a flow makes, constants first, in sorted order, so the
// refusal a flow earns is the same on every check.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
//   - 2026-11-04 19:26: includes the calls of init (C44c, B054)
func _Calls(flow *workflowpb.Flow) []*workflowpb.Call {
	var calls []*workflowpb.Call

	for _, name := range slices.Sorted(maps.Keys(flow.GetConstants())) {
		if call := flow.GetConstants()[name].GetCall(); call != nil {
			calls = append(
				calls,
				call,
			)
		}
	}

	visit := func(stmt *workflowpb.Statement) {
		calls = append(
			calls,
			_Made(stmt)...,
		)
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

	for _, held := range flow.GetInit() {
		if call := held.GetCall(); call != nil {
			calls = append(
				calls,
				call,
			)
		}
	}

	return calls
}

// _Made is the calls one statement makes itself, not counting those of the
// statements it holds.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
func _Made(stmt *workflowpb.Statement) []*workflowpb.Call {
	made := []*workflowpb.Call{
		stmt.GetCall(),
		stmt.GetSpawn().GetCall(),
		stmt.GetRepeat().GetCall(),
		stmt.GetRetry().GetCall(),
		stmt.GetTimeout().GetCall(),
		stmt.GetIf().GetCondition().GetCall(),
		stmt.GetMatch().GetExpression().GetCall(),
	}

	var calls []*workflowpb.Call

	for _, call := range made {
		if call != nil {
			calls = append(
				calls,
				call,
			)
		}
	}

	return calls
}

// _Each visits every statement of a list and every statement those hold, in
// order.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
func _Each(
	list []*workflowpb.Statement,
	visit func(*workflowpb.Statement),
) {
	for _, stmt := range list {
		_Visit(
			stmt,
			visit,
		)
	}
}

// _Visit visits one statement and the statements it holds.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
func _Visit(
	stmt *workflowpb.Statement,
	visit func(*workflowpb.Statement),
) {
	if stmt == nil {
		return
	}

	visit(stmt)

	_Visit(
		stmt.GetIf().GetThen(),
		visit,
	)
	_Visit(
		stmt.GetIf().GetElse(),
		visit,
	)
	_Visit(
		stmt.GetLoop().GetBody(),
		visit,
	)

	for _, arm := range stmt.GetMatch().GetCases() {
		_Visit(
			arm.GetStatement(),
			visit,
		)
	}

	_Visit(
		stmt.GetMatch().GetDefault(),
		visit,
	)
}

// _Computed checks that a constant's call names only functions and constants
// declared in the flow, and that no constants name each other in a cycle.
//
// A generated file binds its constants at the top, before anything else a
// name could hold is in reach, and writes each after the constants it names -
// which a cycle makes impossible.
//
// Revisions:
//   - 2026-09-30 00:44: initial creation
//   - 2026-10-02 00:13: a constant may name another constant or a function,
//     and a cycle is ErrConstant
//   - 2026-11-05 23:52: quotes or bounds a name a script chose (C63, B128, Q4)
//   - 2026-11-06 00:16: quotes a name a flow can hold (C63, B128, Q208)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-10 17:15: uses ErrCarry, formerly ErrNotCarried (C121, Q348)
func _Computed(flow *workflowpb.Flow) error {
	functions := make(map[string]bool)

	for _, fn := range flow.GetFunctions() {
		functions[fn.GetName()] = true
	}

	held := flow.GetConstants()

	for _, name := range slices.Sorted(maps.Keys(held)) {
		for _, op := range held[name].GetCall().GetOperands() {
			named := op.GetName()
			if named == "" || functions[named] {
				continue
			}

			if _, constant := held[named]; !constant {
				return fmt.Errorf(
					"constant %s names %s: %w",
					unpack.QuotePrefix(name),
					unpack.QuotePrefix(named),
					ErrUnresolved,
				)
			}
		}
	}

	_, err := _Order(flow)
	if err != nil {
		return fmt.Errorf(
			"constants name each other: constant %w",
			ErrCarry,
		)
	}

	return nil
}

// _Resolved checks each statement list of every function, and of main: every
// name resolves, and every join and cancel names a spawn the list can see.
//
// Revisions:
//   - 2026-10-02 00:13: initial creation
//   - 2026-11-04 19:26: checks the calls of init as main's are (C44c, B054)
//   - 2026-11-05 23:52: quotes or bounds a name a script chose (C63, B128, Q4)
//   - 2026-11-06 00:16: quotes a name a flow can hold (C63, B128, Q208)
//   - 2026-11-08 02:11: reads the names from common (C107, Q247)
func _Resolved(flow *workflowpb.Flow) error {
	globals := make(map[string]bool)

	for _, fn := range flow.GetFunctions() {
		globals[fn.GetName()] = true
	}

	for name := range flow.GetConstants() {
		globals[name] = true
	}

	for name := range flow.GetArgs() {
		globals[name] = true
	}

	for _, fn := range flow.GetFunctions() {
		var seen = _Opening(
			globals,
			fn.GetParams(),
		)

		err := seen._List(fn.GetStatements().GetStatement())
		if err != nil {
			return fmt.Errorf(
				"%s: %w",
				unpack.QuotePrefix(fn.GetName()),
				err,
			)
		}
	}

	err := _Opening(
		globals,
		nil,
	)._List(_Called(flow.GetInit()))
	if err != nil {
		return fmt.Errorf(
			"%s: %w",
			INIT,
			err,
		)
	}

	err = _Opening(
		globals,
		nil,
	)._List(flow.GetMain().GetStatement())
	if err != nil {
		return fmt.Errorf(
			"%s: %w",
			common.ENTRY,
			err,
		)
	}

	return nil
}

// _Called is the calls of an init as statements, which are checked as a
// statement of main is. An init entry of text has none.
//
// Revisions:
//   - 2026-11-04 19:26: initial creation (C44c, B054)
func _Called(list []*workflowpb.Init) []*workflowpb.Statement {
	var stmts []*workflowpb.Statement

	for _, held := range list {
		call := held.GetCall()
		if call == nil {
			continue
		}

		stmts = append(
			stmts,
			&workflowpb.Statement{
				Action: &workflowpb.Statement_Call{
					Call: call,
				},
			},
		)
	}

	return stmts
}
