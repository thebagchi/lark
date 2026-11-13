package tests

import (
	"os"
	"path/filepath"
	"testing"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/graph"
)

const (
	// PATTERN is the glob of the scripts every graph benchmark reads, inside
	// SAMPLES: the samples of the repository, so the input is real scripts.
	//
	// Revisions:
	//   - 2026-11-02 00:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	PATTERN = "*.star"
)

// _Sources are the sample scripts that Of accepts, read before the timer. A
// sample Of refuses (it has no main, for one) is left out, so every benchmark
// measures the same set.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation, from the research scratch benchmark
//     poc/research/graphbench/b_test.go
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Sources(b *testing.B) []*common.Source {
	b.Helper()

	names, err := filepath.Glob(filepath.Join(
		SAMPLES,
		PATTERN,
	))
	if err != nil {
		b.Fatal(err)
	}

	if len(names) == 0 {
		b.Fatalf(
			"no sample matches %s in %s",
			PATTERN,
			SAMPLES,
		)
	}

	var sources []*common.Source

	for _, name := range names {
		text, err := os.ReadFile(name)
		if err != nil {
			b.Fatal(err)
		}

		source := &common.Source{
			Entry: filepath.Base(name),
			Text:  text,
		}

		_, err = graph.Of(source)
		if err != nil {
			continue
		}

		sources = append(
			sources,
			source,
		)
	}

	if len(sources) == 0 {
		b.Fatal("Of accepts no sample")
	}

	return sources
}

// _Flows are the flows that Of derives from sources, made before the timer.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Flows(
	b *testing.B,
	sources []*common.Source,
) []*workflowpb.Flow {
	b.Helper()

	flows := make(
		[]*workflowpb.Flow,
		0,
		len(sources),
	)

	for _, source := range sources {
		flow, err := graph.Of(source)
		if err != nil {
			b.Fatal(err)
		}

		flows = append(
			flows,
			flow,
		)
	}

	return flows
}

// BenchmarkSamples measures Of and Emit over every sample, one after the other.
// It is the sum of BenchmarkOf and BenchmarkEmit.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func BenchmarkSamples(b *testing.B) {
	sources := _Sources(b)

	b.ReportAllocs()

	for b.Loop() {
		for _, source := range sources {
			flow, err := graph.Of(source)
			if err != nil {
				b.Fatal(err)
			}

			_, err = graph.Emit(flow)
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}

// BenchmarkOf measures Of alone, over every sample.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func BenchmarkOf(b *testing.B) {
	sources := _Sources(b)

	b.ReportAllocs()

	for b.Loop() {
		for _, source := range sources {
			_, err := graph.Of(source)
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}

// BenchmarkEmit measures Emit alone, over the flows of every sample.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func BenchmarkEmit(b *testing.B) {
	flows := _Flows(
		b,
		_Sources(b),
	)

	b.ReportAllocs()

	for b.Loop() {
		for _, flow := range flows {
			_, err := graph.Emit(flow)
			if err != nil {
				b.Fatal(err)
			}
		}
	}
}
