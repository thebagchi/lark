package scheduler_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/dialect"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
	"github.com/thebagchi/lark/v1/runtime/spelling"
)

const (
	// PRINTING prints once from its top level and once from a function, so its
	// log has two callers to name.
	PRINTING = `
def greet():
    print("from greet")

print("from the top")
greet()
`

	// TOPLEVEL is what the interpreter calls a module's top level.
	TOPLEVEL = "<toplevel>"
)

// TestSettings_LogsWhenWhereAndWhoPrinted checks each printed line reaches the
// logger a run's settings name as one record: the line as its message, with
// the lane and the function that printed it, and the time its handler stamps.
//
// Revisions:
//   - 2026-10-02 13:12: initial creation, as
//     TestWithTranscript_WritesWhenWhereAndWhoPrinted
//   - 2026-10-02 16:21: reads the records a JSON handler writes
//   - 2026-10-03 08:34: names the logger in the run's settings, there being no
//     context carrying one
//   - 2026-10-03 20:54: checks what the run produced, which Begin's function answers with
func TestSettings_LogsWhenWhereAndWhoPrinted(t *testing.T) {
	var out bytes.Buffer

	thread := &starlark.Thread{Name: "printing"}

	settings := &scheduler.Settings{Logger: slog.New(slog.NewJSONHandler(&out, nil))}
	finish := scheduler.Begin(t.Context(), thread, spelling.ENTRY, settings)

	_, err := starlark.ExecFileOptions(dialect.OPTIONS, thread, "printing.star", PRINTING, nil)

	err = finish(err)

	if err != nil {
		t.Fatalf("run: %v", err)
	}

	want := [][]string{
		{"from the top", scheduler.SPINE, TOPLEVEL},
		{"from greet", scheduler.SPINE, "greet"},
	}

	lines := strings.Split(strings.TrimSuffix(out.String(), "\n"), "\n")
	if len(lines) != len(want) {
		t.Fatalf("got %q, want %d records", lines, len(want))
	}

	for idx, line := range lines {
		record := map[string]any{}

		err = json.Unmarshal([]byte(line), &record)
		if err != nil {
			t.Fatalf("record %q: %v", line, err)
		}

		got := []any{
			record[slog.MessageKey],
			record[scheduler.ATTR_THREAD],
			record[scheduler.ATTR_FUNCTION],
		}
		if fmt.Sprint(got) != fmt.Sprint(want[idx]) {
			t.Fatalf("got %v, want %v", got, want[idx])
		}

		_, err = time.Parse(time.RFC3339Nano, fmt.Sprint(record[slog.TimeKey]))
		if err != nil {
			t.Fatalf("want the handler's time in %q: %v", line, err)
		}
	}
}
