package tests

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/scheduler"
)

const (
	// PRINTING prints once from its top level and once from a function, so its
	// log has two callers to name.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	PRINTING = `
def greet():
    print("from greet")

print("from the top")
greet()
`

	// TOPLEVEL is what the interpreter calls a module's top level.
	//
	// Revisions:
	//   - 2026-10-04 01:25: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
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
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestSettings_LogsWhenWhereAndWhoPrinted(t *testing.T) {
	var out bytes.Buffer

	thread := &starlark.Thread{
		Name: "printing",
	}

	var settings = new(common.Settings)

	settings.Logger.Store(slog.New(slog.NewJSONHandler(
		&out,
		nil,
	)))

	var (
		finish = scheduler.Begin(
			t.Context(),
			thread,
			common.ENTRY,
			settings,
		)
	)

	_, err := starlark.ExecFileOptions(
		common.OPTIONS,
		thread,
		"printing.star",
		PRINTING,
		nil,
	)

	err = finish(err)
	if err != nil {
		t.Fatalf(
			"run: %v",
			err,
		)
	}

	var (
		want = [][]string{
			{"from the top", common.SPINE, TOPLEVEL},
			{"from greet", common.SPINE, "greet"},
		}
		lines = strings.Split(
			strings.TrimSuffix(
				out.String(),
				"\n",
			),
			"\n",
		)
	)
	if len(lines) != len(want) {
		t.Fatalf(
			"got %q, want %d records",
			lines,
			len(want),
		)
	}

	for idx, line := range lines {
		record := map[string]any{}

		err = json.Unmarshal(
			[]byte(line),
			&record,
		)
		if err != nil {
			t.Fatalf(
				"record %q: %v",
				line,
				err,
			)
		}

		got := []any{
			record[slog.MessageKey],
			record[common.ATTR_THREAD],
			record[common.ATTR_FUNCTION],
		}
		if fmt.Sprint(got) != fmt.Sprint(want[idx]) {
			t.Fatalf(
				"got %v, want %v",
				got,
				want[idx],
			)
		}

		_, err = time.Parse(
			time.RFC3339Nano,
			fmt.Sprint(record[slog.TimeKey]),
		)
		if err != nil {
			t.Fatalf(
				"want the handler's time in %q: %v",
				line,
				err,
			)
		}
	}
}
