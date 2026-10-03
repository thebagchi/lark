package artifact

import (
	"context"
	"io"
	"log/slog"
)

// _Trailing is a slog handler that writes a record's message after its
// attributes, so a line reads as where it came from - the thread, the
// function - and then what it said.
//
// slog's own handlers write the message before every attribute. This moves it
// into the last attribute, under the same key, and has the handler it wraps
// drop the emptied message it would otherwise write first.
type _Trailing struct {
	inner slog.Handler
}

// _NewTrailing is a text handler writing to out, its message last.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
//   - 2026-10-03 16:31: moved here from cmd/lark, so WithLog writes as lark does
func _NewTrailing(out io.Writer) *_Trailing {
	options := &slog.HandlerOptions{
		ReplaceAttr: _Unspoken,
	}

	return &_Trailing{inner: slog.NewTextHandler(out, options)}
}

// Enabled is whether the handler wrapped writes a record of level.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
//   - 2026-10-03 16:31: moved here from cmd/lark, so WithLog writes as lark does
func (t *_Trailing) Enabled(ctx context.Context, level slog.Level) bool {
	return t.inner.Enabled(ctx, level)
}

// Handle writes record with its message moved after its attributes.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
//   - 2026-10-03 16:31: moved here from cmd/lark, so WithLog writes as lark does
func (t *_Trailing) Handle(ctx context.Context, record slog.Record) error {
	moved := slog.NewRecord(record.Time, record.Level, "", record.PC)

	record.Attrs(func(attr slog.Attr) bool {
		moved.AddAttrs(attr)

		return true
	})

	moved.AddAttrs(slog.String(slog.MessageKey, record.Message))

	return t.inner.Handle(ctx, moved)
}

// WithAttrs is this handler over the wrapped one with attrs added.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
//   - 2026-10-03 16:31: moved here from cmd/lark, so WithLog writes as lark does
func (t *_Trailing) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &_Trailing{inner: t.inner.WithAttrs(attrs)}
}

// WithGroup is this handler over the wrapped one inside the group name.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
//   - 2026-10-03 16:31: moved here from cmd/lark, so WithLog writes as lark does
func (t *_Trailing) WithGroup(name string) slog.Handler {
	return &_Trailing{inner: t.inner.WithGroup(name)}
}

// _Unspoken drops the message slog writes first, which Handle emptied after
// moving it last. A line the script printed empty is dropped with it: the two
// look alike to a ReplaceAttr, and a record without its message still says
// where it came from.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
//   - 2026-10-03 16:31: moved here from cmd/lark, so WithLog writes as lark does
func _Unspoken(groups []string, attr slog.Attr) slog.Attr {
	emptied := len(groups) == 0 && attr.Key == slog.MessageKey && attr.Value.String() == ""
	if emptied {
		return slog.Attr{}
	}

	return attr
}
