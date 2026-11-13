package tests

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"unsafe"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
)

const (
	// HANDOVER_SIZE is how many bytes each file in the handover tests holds.
	//
	// Revisions:
	//   - 2026-11-03 22:07: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	HANDOVER_SIZE = 4096

	// HANDOVER_FILES is how many files each handover test reads.
	//
	// Revisions:
	//   - 2026-11-03 22:07: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	HANDOVER_FILES = 3
)

// _Member is the builtin of the file module that a script calls by name.
//
// Revisions:
//   - 2026-11-03 00:30: initial creation (C37)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Member(
	tb testing.TB,
	name string,
) *starlark.Builtin {
	tb.Helper()

	module, ok := _Environment(tb)["file"].(*starlarkstruct.Module)
	if !ok {
		tb.Fatal("file is not a module")
	}

	fn, ok := module.Members[name].(*starlark.Builtin)
	if !ok {
		tb.Fatalf(
			"file.%s is not a builtin",
			name,
		)
	}

	return fn
}

// _Written is a directory that holds HANDOVER_FILES files of different text,
// and the text of each.
//
// Revisions:
//   - 2026-11-03 00:30: initial creation (C37)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Written(tb testing.TB) ([]string, []string) {
	tb.Helper()

	var (
		root  = tb.TempDir()
		paths []string
		texts []string
	)

	for idx := range HANDOVER_FILES {
		var (
			text = strings.Repeat(
				string(rune('a'+idx)),
				HANDOVER_SIZE,
			)
			path = filepath.Join(
				root,
				"f"+strconv.Itoa(idx),
			)
		)

		err := os.WriteFile(
			path,
			[]byte(text),
			0o600,
		)
		if err != nil {
			tb.Fatal(err)
		}

		paths = append(
			paths,
			path,
		)
		texts = append(
			texts,
			text,
		)
	}

	return paths, texts
}

// TestHandover_ReadNeverWritesAHandedOverBuffer proves, for file.read and
// file.bytes, the rule that makes the handover safe: nothing writes the buffer
// after the call. The files are read, then written again with other text, then
// read again. The first values must keep the text they had, and no two values
// may share bytes.
//
// Revisions:
//   - 2026-11-03 00:30: initial creation (C37)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestHandover_ReadNeverWritesAHandedOverBuffer(t *testing.T) {
	for _, name := range []string{"read", "bytes"} {
		t.Run(
			name,
			func(t *testing.T) {
				var (
					fn = _Member(
						t,
						name,
					)
					paths, texts = _Written(t)
					thread       = &starlark.Thread{
						Name: SCRIPT,
					}
					first, second []starlark.Value
				)

				readAll := func() []starlark.Value {
					var out []starlark.Value

					for _, path := range paths {
						value, err := fn.CallInternal(
							thread,
							starlark.Tuple{starlark.String(path)},
							nil,
						)
						if err != nil {
							t.Fatal(err)
						}

						out = append(
							out,
							value,
						)
					}

					return out
				}

				first = readAll()

				for _, path := range paths {
					err := os.WriteFile(
						path,
						[]byte(strings.Repeat(
							"Z",
							HANDOVER_SIZE,
						)),
						0o600,
					)
					if err != nil {
						t.Fatal(err)
					}
				}

				second = readAll()

				all := append(
					first,
					second...,
				)

				for idx, value := range all {
					want := texts[idx%HANDOVER_FILES]
					if idx >= HANDOVER_FILES {
						want = strings.Repeat(
							"Z",
							HANDOVER_SIZE,
						)
					}

					got := _Chars(
						t,
						value,
					)
					if got != want {
						t.Fatalf(
							"value %d: wrong text (%d bytes)",
							idx,
							len(got),
						)
					}

					for other := range idx {
						if _Shares(
							got,
							_Chars(
								t,
								all[other],
							),
						) {
							t.Fatalf(
								"values %d and %d share a buffer",
								other,
								idx,
							)
						}
					}
				}
			},
		)
	}
}

// _Shares reports whether two texts start at the same byte, so one buffer
// holds both.
//
// Revisions:
//   - 2026-11-03 00:30: initial creation (C37)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Shares(
	left string,
	right string,
) bool {
	return unsafe.StringData(left) == unsafe.StringData(right)
}

// _Chars is the text of a string or bytes value.
//
// Revisions:
//   - 2026-11-03 00:30: initial creation (C37)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Chars(
	tb testing.TB,
	value starlark.Value,
) string {
	tb.Helper()

	switch held := value.(type) {
	case starlark.String:
		return string(held)

	case starlark.Bytes:
		return string(held)
	}

	tb.Fatalf(
		"got %s, want a string or bytes",
		value.Type(),
	)

	return ""
}

// TestHandover_ReadMakesOneBufferPerCall holds file.read and file.bytes to the
// allocations of the handover: the read buffer is handed over, not copied.
//
// Revisions:
//   - 2026-11-03 00:30: initial creation (C37)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestHandover_ReadMakesOneBufferPerCall(t *testing.T) {
	for _, name := range []string{"read", "bytes"} {
		t.Run(
			name,
			func(t *testing.T) {
				var (
					fn = _Member(
						t,
						name,
					)
					paths, _ = _Written(t)
					thread   = &starlark.Thread{
						Name: SCRIPT,
					}
					args = starlark.Tuple{starlark.String(paths[0])}
				)

				got := testing.AllocsPerRun(
					20,
					func() {
						_, err := fn.CallInternal(
							thread,
							args,
							nil,
						)
						if err != nil {
							t.Fatal(err)
						}
					},
				)
				if got > MAX_READ_ALLOCS {
					t.Fatalf(
						"%v allocations, want at most %d",
						got,
						MAX_READ_ALLOCS,
					)
				}
			},
		)
	}
}

// MAX_READ_ALLOCS is the most allocations one read of a 4 KiB file may make.
//
// Revisions:
//   - 2026-11-03 22:07: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
const (
	MAX_READ_ALLOCS = 9
)

// BenchmarkFile_Read is file.read and file.bytes on a file of 64 KiB.
//
// Revisions:
//   - 2026-11-03 00:30: initial creation (C37)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func BenchmarkFile_Read(b *testing.B) {
	for _, name := range []string{"read", "bytes"} {
		b.Run(
			name,
			func(b *testing.B) {
				var (
					fn = _Member(
						b,
						name,
					)
					path = filepath.Join(
						b.TempDir(),
						"big",
					)
					thread = &starlark.Thread{
						Name: SCRIPT,
					}
				)

				err := os.WriteFile(
					path,
					make(
						[]byte,
						65536,
					),
					0o600,
				)
				if err != nil {
					b.Fatal(err)
				}

				args := starlark.Tuple{starlark.String(path)}

				b.ReportAllocs()

				for b.Loop() {
					_, err = fn.CallInternal(
						thread,
						args,
						nil,
					)
					if err != nil {
						b.Fatal(err)
					}
				}
			},
		)
	}
}
