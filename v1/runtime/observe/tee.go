package observe

import (
	"context"
	"log/slog"

	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

// _Tee is a slog handler that keeps each line a run prints in a transcript,
// and then hands the record to the handler of the logger the run was given.
//
// A handler rather than a second path out of the scheduler, so a run prints in
// one way whoever reads it. The handler it wraps is told exactly what it was
// told before: the same record, and only when it is enabled for its level.
type _Tee struct {
	inner      slog.Handler
	transcript *_Transcript
}

// _NewTee is a handler that keeps each line in transcript and logs it to
// logger, or to slog's default when logger is nil, as a run with no logger
// does.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation
func _NewTee(
	logger *slog.Logger,
	transcript *_Transcript,
) *_Tee {
	if logger == nil {
		logger = slog.Default()
	}

	return &_Tee{
		inner:      logger.Handler(),
		transcript: transcript,
	}
}

// Enabled is true for every level, since a line a script printed is kept
// whatever level the host logs at.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation
func (t *_Tee) Enabled(
	ctx context.Context,
	level slog.Level,
) bool {
	return true
}

// Handle keeps record as a line of the transcript, and then hands it to the
// wrapped handler when that handler is enabled for its level, as a logger
// would have.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation
func (t *_Tee) Handle(
	ctx context.Context,
	record slog.Record,
) error {
	t.transcript._Add(_Printed(&record))

	if !t.inner.Enabled(
		ctx,
		record.Level,
	) {
		return nil
	}

	return t.inner.Handle(
		ctx,
		record,
	)
}

// WithAttrs is this handler over the wrapped one with attrs added, keeping into
// the same transcript.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation
func (t *_Tee) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &_Tee{
		inner:      t.inner.WithAttrs(attrs),
		transcript: t.transcript,
	}
}

// WithGroup is this handler over the wrapped one inside the group name,
// keeping into the same transcript.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation
func (t *_Tee) WithGroup(name string) slog.Handler {
	return &_Tee{
		inner:      t.inner.WithGroup(name),
		transcript: t.transcript,
	}
}

// _Printed is the line record says was printed: its message, and the thread
// and the function the scheduler logs it with.
//
// Revisions:
//   - 2026-10-28 23:50: initial creation
func _Printed(record *slog.Record) *Print {
	line := &Print{
		Text: record.Message,
	}

	record.Attrs(func(attr slog.Attr) bool {
		switch attr.Key {
		case scheduler.ATTR_THREAD:
			line.Thread = attr.Value.String()
		case scheduler.ATTR_FUNCTION:
			line.Function = attr.Value.String()
		}

		return true
	})

	return line
}
