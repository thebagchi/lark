package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
)

// BenchmarkCompile_Samples measures a compile of every sample that runs alone.
// One op compiles all of them, two of which load a module.
//
// Revisions:
//   - 2026-11-02 22:58: initial creation (C44a)
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func BenchmarkCompile_Samples(b *testing.B) {
	var (
		files, _ = filepath.Glob(filepath.Join(
			"..",
			"..",
			"..",
			"samples",
			"*.star",
		))
		texts = map[string][]byte{}
	)

	for _, file := range files {
		name := filepath.Base(file)
		if name == "clock.star" || name == "strings.star" {
			continue
		}

		text, err := os.ReadFile(file)
		if err != nil {
			b.Fatal(err)
		}

		texts[file] = text
	}

	b.ReportAllocs()

	for b.Loop() {
		for file, text := range texts {
			_, err := runtime.Compile(&common.Source{
				Entry: file,
				Text:  text,
			})
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}
