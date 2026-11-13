package run

import (
	"errors"
	"strconv"
	"strings"
	"sync"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// LocatedError is an error with the place it arose in front of its text: file:line: message.
//
// It wraps the error rather than copying it, so errors.Is and errors.As reach
// what they reached before, the interpreter's *starlark.EvalError among it.
//
// Revisions:
//   - 2026-11-04 06:39: initial creation (C69)
//   - 2026-11-08 19:23: guard covers says, so a write beside Error cannot race (C112, Q207)
//   - 2026-11-10 00:25: renamed from _LocatedError and the fields err, where, as a test in tests/
//     needs them (C120a, Q314)
type LocatedError struct {
	guard sync.Mutex
	Where string
	says  string
	Err   error
}

// Error is the text of the error it wraps with where put in front of the words
// the interpreter said, and after any words a wrapper put before them.
//
// One place sets the order (decision Q89): wrapper words, then the position,
// then the failing text. The error that was located may sit inside wrappers -
// "initialise main.star: " is one - and the position names the failing text, not
// the wrapper, so it goes where the text of the interpreter's error begins. An
// error that ends otherwise, as an assertion does, has no such text and gets the
// position first.
//
// Revisions:
//   - 2026-11-04 06:39: initial creation (C69)
//   - 2026-11-04 18:40: puts where before the interpreter's words, not before any
//     wrapper words (C69, fix round 2)
//   - 2026-11-08 19:23: a nil receiver or a nil err gives text and not a panic; says is loaded
//     once (C112, Q65, Q206, Q207)
//   - 2026-11-10 01:00: now uses LocatedError, Says, Err, Where, were _LocatedError, _Says, err,
//     where (C120a, Q314)
func (l *LocatedError) Error() string {
	if l == nil {
		return ""
	}

	var (
		says = l.Says()
		text string
	)

	if l.Err == nil {
		if says == "" {
			return l.Where
		}

		return l.Where + ": " + says
	}

	text = l.Err.Error()

	if says != "" && strings.HasSuffix(
		text,
		says,
	) {
		return text[:len(text)-len(says)] + l.Where + ": " + says
	}

	return l.Where + ": " + text
}

// Unwrap is the error that was located.
//
// Revisions:
//   - 2026-11-04 06:39: initial creation (C69)
//   - 2026-11-08 20:57: a nil receiver has nothing located, so it gives nil
//     (C112 fix round 1, B230, Q65)
//   - 2026-11-10 01:00: now uses LocatedError, Err, were _LocatedError, err (C120a, Q314)
func (l *LocatedError) Unwrap() error {
	if l == nil {
		return nil
	}

	return l.Err
}

// Says is the words the interpreter said, read under the guard Say writes under.
//
// Revisions:
//   - 2026-11-08 19:23: initial creation (C112, Q207)
//   - 2026-11-10 00:25: renamed from _Says, as a test in tests/ needs it (C120a, Q314)
func (l *LocatedError) Says() string {
	if l == nil {
		return ""
	}

	l.guard.Lock()
	defer l.guard.Unlock()

	return l.says
}

// Say sets the words the interpreter said, under the guard Says reads under.
//
// Revisions:
//   - 2026-11-08 19:23: initial creation (C112, Q207)
//   - 2026-11-10 00:25: renamed from _Say, as a test in tests/ needs it (C120a, Q314)
func (l *LocatedError) Say(says string) {
	if l == nil {
		return
	}

	l.guard.Lock()
	l.says = says
	l.guard.Unlock()
}

// _Locate returns err with the file and line of the innermost
// Starlark frame in front of its text (decision Q80: the position, with no
// stack).
//
// The frames come from the deepest *starlark.EvalError in the chain of err: that
// is the error as the thread where it arose saw it. Where the chain holds none,
// as with an assertion, which is raised from a builtin and is no EvalError yet,
// stack is used, and a caller with no stack to give passes nil.
//
// Nothing is added when there is no place to name: a builtin the host called
// from Go has a stack of one builtin frame, which has no line. Nothing is added
// twice either: an error already located, however deep in the chain, is
// returned as it is, so a join that re-raises a thread's failure keeps the
// place in that thread and does not add the place of the join.
//
// The innermost Starlark frame, not the builtin above it, because the builtin
// is Go code: the line a person can open is the one that called it.
//
// Revisions:
//   - 2026-11-04 06:39: initial creation (C69)
//   - 2026-11-04 18:40: keeps the interpreter's words, so the text puts the position
//     after the wrapper words (C69, fix round 2)
//   - 2026-11-08 19:23: sets says through _Say (C112, Q207)
//   - 2026-11-10 01:00: now uses LocatedError, Where, Err, Say, were _LocatedError, where, err,
//     _Say (C120a, Q314)
func _Locate(
	err error,
	stack starlark.CallStack,
) error {
	if err == nil {
		return nil
	}

	var located *LocatedError
	if errors.As(
		err,
		&located,
	) {
		return err
	}

	var says string

	evaluated := _Deepest(err)
	if evaluated != nil {
		stack = evaluated.CallStack
		says = evaluated.Msg
	}

	where := _Innermost(stack)
	if where == "" {
		return err
	}

	located = &LocatedError{
		Where: where,
		Err:   err,
	}
	located.Say(says)

	return located
}

// _Deepest is the *starlark.EvalError nearest the cause in the chain of err, or
// nil when the chain holds none.
//
// A builtin that calls a function and wraps its failure raises a new error, so
// the chain can hold an EvalError of the caller around one of the callee. The
// callee's is the deeper stack, and the one the failure arose in.
//
// Revisions:
//   - 2026-11-04 06:39: initial creation (C69)
func _Deepest(err error) *starlark.EvalError {
	var deepest *starlark.EvalError

	for err != nil {
		var found *starlark.EvalError
		if !errors.As(
			err,
			&found,
		) {
			break
		}

		deepest = found
		err = found.Unwrap()
	}

	return deepest
}

// _Innermost is the position of the innermost frame of stack that has a line,
// as file:line, or the empty string when no frame has one.
//
// File and line, with no column (decision Q89). The file name is cut as C21
// cuts a file name in an error (decision Q91), so a long path cannot make a
// long message; a name of common.QUOTE_LIMIT bytes or fewer prints unchanged.
//
// Revisions:
//   - 2026-11-04 06:39: initial creation (C69)
//   - 2026-11-04 18:12: file:line with no column, and the file name cut (Q89, Q91)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 20:07: moved to package run; its names are used unqualified (C91w5, Q116)
//   - 2026-11-08 00:59: names read from common (C91w12c, Q244)
func _Innermost(stack starlark.CallStack) string {
	for index := range stack {
		frame := stack.At(index)

		if frame.Pos.Line > 0 && frame.Pos.Filename() != BUILTIN_FILE {
			return unpack.ShortText(frame.Pos.Filename()) + ":" +
				strconv.Itoa(int(frame.Pos.Line))
		}
	}

	return ""
}

// Locate returns err with the position of the innermost Starlark frame of
// thread in front of its text, for a builtin to raise.
//
// Only for a builtin running on the interpreter goroutine of thread, where the
// stack is its own to read. Fail cannot do this itself: the file plugin's
// walker calls Fail with the thread it was made for, from a goroutine that may
// not be the one running it, and reading that thread's stack is a data race.
//
// Revisions:
//   - 2026-11-04 07:10: initial creation (C69)
func Locate(
	thread *starlark.Thread,
	err error,
) error {
	if thread == nil {
		return _Locate(
			err,
			nil,
		)
	}

	return _Locate(
		err,
		thread.CallStack(),
	)
}
