package tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/merge"
	"github.com/thebagchi/lark/v1/runtime/plugin"
)

// BenchmarkMerge_Samples measures Merge, alone and with the text rendered,
// over every sample that runs alone. One op merges all of them; two load a
// module. The environment is built once, outside the timing.
//
// Revisions:
//   - 2026-11-05 18:00: initial creation (C44b)
//   - 2026-11-07 18:50: the samples path gains one level after the move (C91w3)
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 00:25: moved to tests/, with one more ".." in the samples path (C120a, Q311)
//   - 2026-11-13 17:40: uses package merge, formerly amalgam (C134, Q446)
func BenchmarkMerge_Samples(b *testing.B) {
	env, err := plugin.Environment(plugin.Registered())
	if err != nil {
		b.Fatal(err)
	}

	env[common.CALL] = env["spawn"]

	var (
		files, _ = filepath.Glob(filepath.Join(
			"..",
			"..",
			"..",
			"..",
			"..",
			"samples",
			"*.star",
		))
		sources = make(
			[]*common.Source,
			0,
			len(files),
		)
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

		sources = append(
			sources,
			&common.Source{
				Entry: file,
				Text:  text,
			},
		)
	}

	b.Run(
		"Merge",
		func(b *testing.B) {
			b.ReportAllocs()

			for b.Loop() {
				for _, source := range sources {
					_, err := merge.Merge(
						source,
						env,
					)
					if err != nil {
						b.Fatal(err)
					}
				}
			}
		},
	)
	b.Run(
		"MergeAndText",
		func(b *testing.B) {
			b.ReportAllocs()

			for b.Loop() {
				for _, source := range sources {
					merged, err := merge.Merge(
						source,
						env,
					)
					if err != nil {
						b.Fatal(err)
					}

					_, err = merged.Source()
					if err != nil {
						b.Fatal(err)
					}
				}
			}
		},
	)
}
