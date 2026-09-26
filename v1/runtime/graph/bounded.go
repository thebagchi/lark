package graph

import (
	"errors"
	"fmt"
	"strconv"

	"go.starlark.net/syntax"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
)

// ErrNoDelay is returned for a Repeat or a Retry asking for a pause between
// attempts.
//
// The schema declares delay_ms and the builtins take no delay, so a graph
// asking for one describes a workflow this runtime cannot run. Generating the
// call without it produces a script that runs, gives the right answer, and
// hammers whatever it talks to - a dropped delay is invisible until something
// downstream falls over.
var ErrNoDelay = errors.New("delay is not supported")

const (
	// The wrappers a script calls, and the pause.
	REPEAT  = "repeat"
	RETRY   = "retry"
	TIMEOUT = "timeout"
	SLEEP   = "sleep"

	// MILLIS is how many milliseconds a second holds. The schema counts
	// milliseconds and the builtins take seconds.
	MILLIS = 1000

	// WRAPPED is how many arguments a wrapper takes: how much, and what to do
	// that much of. A call with any other number is not one of these.
	WRAPPED = 2
)

// _Bounded is a repeat, retry or timeout as the line that builds it and calls
// it.
//
// Revisions:
//   - 2026-09-20 21:05: initial creation
func (g *_Gen) _Bounded(step *workflowpb.Step) (string, error) {
	switch {
	case step.GetRepeat() != nil:
		return g._Wrap(
			REPEAT,
			step.GetRepeat().GetCall(),
			strconv.Itoa(int(step.GetRepeat().GetCount())),
			step.GetRepeat().GetDelayMs(),
		)

	case step.GetRetry() != nil:
		return g._Wrap(
			RETRY,
			step.GetRetry().GetCall(),
			strconv.Itoa(int(step.GetRetry().GetAttempts())),
			step.GetRetry().GetDelayMs(),
		)
	}

	return g._Wrap(
		TIMEOUT,
		step.GetTimeout().GetCall(),
		_Seconds(step.GetTimeout().GetTimeoutMs()),
		0,
	)
}

// _Wrap is one wrapper as a script writes it, refusing a delay it cannot
// express.
//
// The site comes from _Site, so a wrapped call that passes arguments is a
// lambda and one that does not is the function itself - the same rule a spawn
// follows, from the same place, because both wrap a Call.
//
// Revisions:
//   - 2026-09-20 21:05: initial creation
//   - 2026-09-20 21:12: takes a Call rather than a name, so a wrapped site can
//     pass arguments
func (g *_Gen) _Wrap(
	who string,
	call *workflowpb.Call,
	bound string,
	delay int32,
) (string, error) {
	if delay != 0 {
		return "", fmt.Errorf("%s %s: %dms %w", who, call.GetFunction(), delay, ErrNoDelay)
	}

	site, err := g._Site(call)
	if err != nil {
		return "", err
	}

	return fmt.Sprintf("%s(%s%s%s)", who, bound, SEPARATOR, site), nil
}

// _Seconds is a duration in milliseconds as the number of seconds a builtin
// takes.
//
// A whole number of seconds is written as an integer, by the same rule a JSON
// argument follows: Starlark has two number types where the schema has one, and
// they differ under // and %.
//
// Revisions:
//   - 2026-09-20 21:05: initial creation
func _Seconds(ms int32) string {
	return _Number(float64(ms) / MILLIS)
}

// _Pause is a sleep, in the seconds the builtin takes.
//
// Revisions:
//   - 2026-09-20 21:05: initial creation
//   - 2026-09-21 01:32: Sleep carries milliseconds again, so this divides
func _Pause(sleep *workflowpb.Sleep) string {
	return fmt.Sprintf("%s(%s)", SLEEP, _Seconds(sleep.GetDurationMs()))
}

// _Wrapper is the bounded step a repeat, retry or timeout states, and whether
// it states one.
//
// The count or the budget comes first and the callable last, and the wrappers
// call straight away - there is no second call to read past. The callable
// takes the two spellings a spawned site takes, so a wrapped call carries
// arguments through a lambda.
//
// delay_ms is never set. No builtin can spell a delay, and the emitter already
// refuses a graph that asks for one, so this is the symmetric half.
//
// Revisions:
//   - 2026-09-21 01:32: initial creation
//   - 2026-09-21 08:09: a number only; repeat(True, f) used to derive as 0
func (r *_Reading) _Wrapper(name string, call *syntax.CallExpr) *workflowpb.Step {
	if len(call.Args) != WRAPPED {
		return nil
	}

	bound, ok := _Quantity(call.Args[0])
	if !ok {
		return nil
	}

	site, def := r._Target(call.Args[1])
	if def == nil {
		return nil
	}

	count := int32(bound)

	switch name {
	case REPEAT:
		return &workflowpb.Step{Action: &workflowpb.Step_Repeat{
			Repeat: &workflowpb.Repeat{Call: site, Count: count},
		}}

	case RETRY:
		return &workflowpb.Step{Action: &workflowpb.Step_Retry{
			Retry: &workflowpb.Retry{Call: site, Attempts: count},
		}}
	}

	return &workflowpb.Step{Action: &workflowpb.Step_Timeout{
		Timeout: &workflowpb.Timeout{
			Call:      site,
			TimeoutMs: int32(bound * MILLIS),
		},
	}}
}
