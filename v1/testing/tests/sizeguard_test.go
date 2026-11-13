// Probes for the size guard (C46, bug B053 and B013): a script-given size that
// does not fit the free memory is refused with an error, before any
// allocation, at every builtin that makes a buffer of that size.
//
// Revisions:
//   - 2026-11-03 22:33: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/plugin/file"
	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/memwatch"
)

const (
	// FITS is a size that passes the guard on any machine a test runs on, and
	// is above the size under which the guard reads no memory (1 MiB), so the
	// read path runs.
	//
	// Revisions:
	//   - 2026-11-03 22:33: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	FITS = 2 << 20

	// UNFIT_FALLBACK is the size used when the machine's memory cannot be
	// read: above the 1 GiB ceiling.
	//
	// Revisions:
	//   - 2026-11-03 22:33: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	UNFIT_FALLBACK = 4 << 30

	// ROOM_TEXT is the end of the refusal.
	//
	// Revisions:
	//   - 2026-11-03 22:33: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	ROOM_TEXT = "not enough free memory"
)

// _Unfit is a size no allocation of this machine can meet: four times the
// total memory. Where the total cannot be read, it is 4 GiB, above the fixed
// ceiling.
//
// Revisions:
//   - 2026-11-03 00:50: initial creation (C46)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Unfit() uint64 {
	reading, err := memwatch.System()()
	if err != nil {
		return UNFIT_FALLBACK
	}

	return 4 * reading.Total
}

// TestSizeGuard_RefusesAtEverySite runs each builtin that makes a buffer of a
// script-given size with a size four times the machine's memory, and wants an
// error that names the builtin, the size and "not enough free memory". The
// script has to end with that error, not with the process dying.
//
// Success: every row ends with the error text in want. File rows read a sparse
// file of that size; they are skipped where the file system cannot hold one.
//
// Revisions:
//   - 2026-11-03 00:50: initial creation (C46)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestSizeGuard_RefusesAtEverySite(t *testing.T) {
	t.Parallel()

	var (
		size   = _Unfit()
		sparse = filepath.Join(
			t.TempDir(),
			"sparse.bin",
		)
	)

	made, err := os.Create(sparse)
	if err != nil {
		t.Fatal(err)
	}

	defer func() {
		_ = made.Close()
	}()

	err = made.Truncate(int64(size))
	if err != nil {
		t.Skipf(
			"no sparse file of %d bytes: %v",
			size,
			err,
		)
	}

	tests := map[string]struct {
		body string
		who  string
	}{
		"hex.from_int": {
			body: "hex.from_int(255, SIZE)",
			who:  "hex.from_int",
		},
		"binary.from_int": {
			body: "binary.from_int(255, SIZE)",
			who:  "binary.from_int",
		},
		"integer.to_bytes": {
			body: "integer.to_bytes(255, SIZE)",
			who:  "integer.to_bytes",
		},
		"integer.to_bytes signed": {
			body: "integer.to_bytes(-1, SIZE, 'big', True)",
			who:  "integer.to_bytes",
		},
		"buffer write_uint": {
			body: "buf.new().write_uint(1, SIZE)",
			who:  "write_uint",
		},
		"buffer write_int": {
			body: "buf.new().write_int(1, SIZE)",
			who:  "write_int",
		},
		"random.bytes": {
			body: "random.bytes(SIZE)",
			who:  "random.bytes",
		},
		"file.read": {
			body: "file.read('" + sparse + "')",
			who:  "file.read",
		},
		"file.bytes": {
			body: "file.bytes('" + sparse + "')",
			who:  "file.bytes",
		},
	}

	for name, test := range tests {
		t.Run(
			name,
			func(t *testing.T) {
				t.Parallel()

				_, err := _GuardRun(
					t,
					"def main():\n    return "+strings.ReplaceAll(
						test.body,
						"SIZE",
						fmt.Sprint(size),
					)+"\n",
				)
				if err == nil {
					t.Fatalf(
						"no error for %q",
						test.body,
					)
				}

				if !strings.Contains(
					err.Error(),
					test.who,
				) || !strings.Contains(
					err.Error(),
					ROOM_TEXT,
				) {
					t.Fatalf(
						"err = %q, want %q and %q",
						err.Error(),
						test.who,
						ROOM_TEXT,
					)
				}
			},
		)
	}
}

// TestSizeGuard_SizesThatFitWork runs the same builtins with a size that fits,
// above the 1 MiB under which the guard reads nothing, and wants the same
// answer as before the guard: a value of that length.
//
// Revisions:
//   - 2026-11-03 00:50: initial creation (C46)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestSizeGuard_SizesThatFitWork(t *testing.T) {
	t.Parallel()

	var (
		small = filepath.Join(
			t.TempDir(),
			"small.bin",
		)
		err = os.WriteFile(
			small,
			make(
				[]byte,
				FITS,
			),
			0o640,
		)
	)
	if err != nil {
		t.Fatal(err)
	}

	tests := map[string]string{
		"hex.from_int":     "len(hex.from_int(255, %d))",
		"binary.from_int":  "len(binary.from_int(255, %d))",
		"integer.to_bytes": "len(integer.to_bytes(255, %d))",
		"write_uint":       "len([b.write_uint(1, %d), b.bytes()][1])",
		"random.bytes":     "len(random.bytes(%d))",
		"file.read":        "len(file.read('" + small + "')) + 0 * %d",
		"file.bytes":       "len(file.bytes('" + small + "')) + 0 * %d",
	}

	for name, expression := range tests {
		t.Run(
			name,
			func(t *testing.T) {
				t.Parallel()

				got, err := _GuardRun(
					t,
					"def main():\n    b = buf.new()\n    return "+fmt.Sprintf(
						expression,
						FITS,
					)+"\n",
				)
				if err != nil {
					t.Fatalf(
						"run: %v",
						err,
					)
				}

				if got.String() != fmt.Sprint(FITS) {
					t.Fatalf(
						"got %s, want %d",
						got.String(),
						FITS,
					)
				}
			},
		)
	}
}

// _GuardRun compiles text with the file plugin and runs it.
//
// Revisions:
//   - 2026-11-03 00:50: initial creation (C46)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _GuardRun(
	t *testing.T,
	text string,
) (starlark.Value, error) {
	t.Helper()

	built, err := runtime.Compile(
		&common.Source{
			Entry: "probe.star",
			Text:  []byte(text),
		},
		runtime.WithPlugins(file.New()),
	)
	if err != nil {
		t.Fatalf(
			"compile: %v",
			err,
		)
	}

	run, err := runtime.Start(
		t.Context(),
		built,
	)
	if err != nil {
		t.Fatal(err)
	}

	return run.Wait()
}
