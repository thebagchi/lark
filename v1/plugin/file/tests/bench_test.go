package tests

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
)

// BenchmarkFile_List is file.list on a directory of 100 files.
//
// Revisions:
//   - 2026-11-02 08:27: initial creation
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func BenchmarkFile_List(b *testing.B) {
	root := b.TempDir()

	for index := range 100 {
		err := os.WriteFile(
			filepath.Join(
				root,
				fmt.Sprintf(
					"file-%03d.txt",
					index,
				),
			),
			nil,
			0o600,
		)
		if err != nil {
			b.Fatal(err)
		}
	}

	var (
		env    = _Environment(b)
		thread = &starlark.Thread{
			Name: SCRIPT,
		}
	)

	env["root"] = starlark.String(root)

	b.ReportAllocs()

	for b.Loop() {
		_, err := starlark.EvalOptions(
			common.OPTIONS,
			thread,
			SCRIPT,
			`file.list(root)`,
			env,
		)
		if err != nil {
			b.Fatal(err)
		}
	}
}
