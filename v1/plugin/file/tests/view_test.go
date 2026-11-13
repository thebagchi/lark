package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unsafe"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

// TestWrite_KeepsNoViewOfItsData proves the view ends with the call, for the
// two writers that read their data through a view. The data is a bytes value
// over memory the test owns. The test calls the writer, changes that memory,
// and reads the file. The file must hold the bytes it was given. A writer that
// kept the view, and wrote it later, would write the changed bytes.
//
// Revisions:
//   - 2026-11-03 22:19: initial creation (C38b, plan D4)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestWrite_KeepsNoViewOfItsData(t *testing.T) {
	for _, name := range []string{"write", "append"} {
		for _, kind := range []string{"bytes", "str"} {
			t.Run(
				name+"/"+kind,
				func(t *testing.T) {
					var (
						want  = []byte("abc\x00\xffdef")
						owned = append(
							[]byte(nil),
							want...,
						)
						text = unsafe.String(
							unsafe.SliceData(owned),
							len(owned),
						)
						given starlark.Value = starlark.Bytes(text)
						named                = filepath.Join(
							t.TempDir(),
							"out.bin",
						)
					)

					if kind == "str" {
						given = starlark.String(text)
					}

					_, err := _Member(
						t,
						name,
					).CallInternal(
						&starlark.Thread{
							Name: "view",
						},
						starlark.Tuple{
							starlark.String(named),
							given,
						},
						nil,
					)
					if err != nil {
						t.Fatal(err)
					}

					for idx := range owned {
						owned[idx] ^= 0xff
					}

					got, err := os.ReadFile(named)
					if err != nil {
						t.Fatal(err)
					}

					if string(got) != string(want) {
						t.Fatalf(
							"file holds %q, want %q",
							got,
							want,
						)
					}
				},
			)
		}
	}
}

// TestWrite_RefusesWhatIsNeitherBytesNorString holds file.write and
// file.append to the refusal text of unpack, for data that is not bytes or a
// str, and proves no file is made.
//
// Revisions:
//   - 2026-11-03 22:59: initial creation (C38b)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestWrite_RefusesWhatIsNeitherBytesNorString(t *testing.T) {
	for _, name := range []string{"write", "append"} {
		for _, item := range []struct {
			name  string
			given starlark.Value
			want  string
		}{
			{
				"int",
				starlark.MakeInt(1),
				"got int",
			},
			{
				"none",
				starlark.None,
				"got NoneType",
			},
			{
				"list",
				starlark.NewList(nil),
				"got list",
			},
		} {
			t.Run(
				name+"/"+item.name,
				func(t *testing.T) {
					var (
						named = filepath.Join(
							t.TempDir(),
							"out.bin",
						)
						want = "file." + name + " " + item.want +
							": " + unpack.ErrData.Error()
					)

					_, err := _Member(
						t,
						name,
					).CallInternal(
						&starlark.Thread{
							Name: "view",
						},
						starlark.Tuple{
							starlark.String(named),
							item.given,
						},
						nil,
					)

					if err == nil || err.Error() != want {
						t.Fatalf(
							"got %v, want %q",
							err,
							want,
						)
					}

					_, statErr := os.Stat(named)
					if !os.IsNotExist(statErr) {
						t.Fatalf(
							"file made on refusal: %v",
							statErr,
						)
					}
				},
			)
		}
	}
}

// TestWrite_RefusesABadCall holds file.write and file.append to the text of
// starlark.UnpackArgs for a path that is not a str and for a wrong count of
// arguments, so the refusal in _Destination is reached.
//
// Revisions:
//   - 2026-11-03 23:44: initial creation (C38b)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestWrite_RefusesABadCall(t *testing.T) {
	for _, name := range []string{"write", "append"} {
		for _, item := range []struct {
			name string
			args starlark.Tuple
			want string
		}{
			{
				"path is an int",
				starlark.Tuple{starlark.MakeInt(5), starlark.Bytes("x")},
				"XX: for parameter path: got int, want string",
			},
			{
				"too few",
				starlark.Tuple{starlark.String("a")},
				"XX: missing argument for data",
			},
			{
				"too many",
				starlark.Tuple{
					starlark.String("a"),
					starlark.Bytes("x"),
					starlark.Bytes("y"),
				},
				"XX: got 3 arguments, want at most 2",
			},
		} {
			t.Run(
				name+"/"+item.name,
				func(t *testing.T) {
					var (
						fn = _Member(
							t,
							name,
						)
						want = strings.Replace(
							item.want,
							"XX",
							"file."+name,
							1,
						)
					)

					_, err := fn.CallInternal(
						&starlark.Thread{
							Name: "view",
						},
						item.args,
						nil,
					)

					if err == nil || err.Error() != want {
						t.Fatalf(
							"got %v, want %q",
							err,
							want,
						)
					}
				},
			)
		}
	}
}
