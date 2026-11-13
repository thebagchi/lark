package tests

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/artifact"
)

const (
	// MODULES is the count of modules in the long chain of the benchmarks:
	// the main script loads the first, and each loads the next.
	//
	// Revisions:
	//   - 2026-11-02 00:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	MODULES = 12

	// DIAMOND_MAIN loads two modules that both load one, so the artifact holds
	// 4 units and the main script holds 2 loads, as the research scratch
	// benchmark of poc/research/artifact_observe_bench did.
	//
	// Revisions:
	//   - 2026-11-02 00:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	DIAMOND_MAIN = `
load("b.star", "fb")
load("c.star", "fc")

def main():
    return fb(1) + fc(2)
`

	// CHAIN_MAIN loads the first module of a chain, and returns what its
	// function returns, so every module of the chain is initialised.
	//
	// Revisions:
	//   - 2026-11-02 00:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	CHAIN_MAIN = `
load("m0.star", "f0")

def main():
    return f0(1)
`
)

var (
	// CHAINS are the module counts of BenchmarkRun_ManyUnits.
	//
	// Revisions:
	//   - 2026-11-02 00:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	CHAINS = []int{
		2,
		MODULES,
	}

	// DIAMOND is the modules the main script of the diamond loads, by name.
	//
	// Revisions:
	//   - 2026-11-02 00:17: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	DIAMOND = map[string]string{
		"a.star": "def fa(x):\n    return x\n",
		"b.star": "load(\"a.star\", \"fa\")\n\ndef fb(x):\n    return fa(x)\n",
		"c.star": "load(\"a.star\", \"fa\")\n\ndef fc(x):\n    return fa(x)\n",
	}
)

// _Memory is a Loader over modules held in memory, which resolves a name to
// itself.
//
// Revisions:
//   - 2026-11-02 00:17: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Memory struct {
	modules map[string]string
}

// Resolve returns target as it is.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (m *_Memory) Resolve(
	from string,
	target string,
) (string, error) {
	return target, nil
}

// Load returns the text of the named module, or an error when there is none.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (m *_Memory) Load(name string) ([]byte, error) {
	text, ok := m.modules[name]
	if !ok {
		return nil, fmt.Errorf(
			"no module %q",
			name,
		)
	}

	return []byte(text), nil
}

// BenchmarkSave_Deterministic measures Save of an artifact of 4 units whose
// main script holds 2 loads. The name is the one the plan gives for the change
// that makes Save deterministic; today it measures Save as it is.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation, from BenchmarkSave of
//     poc/research/artifact_observe_bench
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func BenchmarkSave_Deterministic(b *testing.B) {
	built, err := artifact.Compile(&common.Source{
		Entry: "main.star",
		Text:  []byte(DIAMOND_MAIN),
		Loader: &_Memory{
			modules: DIAMOND,
		},
	})
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		_, err = built.Save()
		if err != nil {
			b.Fatal(err)
		}
	}
}

// _Modules is the modules of a chain of count modules: module i loads module
// i+1, and the last loads nothing.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation, from the body of BenchmarkRun_ManyUnits
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Modules(count int) map[string]string {
	modules := map[string]string{}

	for idx := range count {
		var text strings.Builder

		if idx+1 < count {
			fmt.Fprintf(
				&text,
				"load(\"m%d.star\", \"f%d\")\n\n",
				idx+1,
				idx+1,
			)
		}

		fmt.Fprintf(
			&text,
			"def f%d(x):\n    return x\n",
			idx,
		)

		modules[fmt.Sprintf(
			"m%d.star",
			idx,
		)] = text.String()
	}

	return modules
}

// BenchmarkRun_ManyUnits measures a run through runtime.Start and Wait of a
// chain of each count of modules, whose entry point returns at once, so the
// cost is the setup of the run: the scheduler path, the recorder and the
// initialising of the units. This is the scenario plan item A10 measures.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation, through runtime.Start, which the plan's
//     figure of 46 allocations for 12 modules was measured with
//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func BenchmarkRun_ManyUnits(b *testing.B) {
	for _, count := range CHAINS {
		b.Run(
			strconv.Itoa(count),
			func(b *testing.B) {
				built, err := runtime.Compile(&common.Source{
					Entry: "main.star",
					Text:  []byte(CHAIN_MAIN),
					Loader: &_Memory{
						modules: _Modules(count),
					},
				})
				if err != nil {
					b.Fatal(err)
				}

				b.ReportAllocs()

				for b.Loop() {
					run, err := runtime.Start(
						b.Context(),
						built,
					)
					if err != nil {
						b.Fatal(err)
					}

					_, err = run.Wait()
					if err != nil {
						b.Fatal(err)
					}
				}
			},
		)
	}
}

// BenchmarkArtifactRun_ManyUnits measures artifact.Run, called directly, of a
// chain of MODULES modules. It leaves out the scheduler path of runtime.Start,
// so it is 16 allocations under BenchmarkRun_ManyUnits/12.
//
// Revisions:
//   - 2026-11-02 00:00: initial creation, as BenchmarkRun_ManyUnits
//   - 2026-11-02 00:00: renamed, to say it calls artifact.Run
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func BenchmarkArtifactRun_ManyUnits(b *testing.B) {
	built, err := artifact.Compile(&common.Source{
		Entry: "main.star",
		Text:  []byte(CHAIN_MAIN),
		Loader: &_Memory{
			modules: _Modules(MODULES),
		},
	})
	if err != nil {
		b.Fatal(err)
	}

	b.ReportAllocs()

	for b.Loop() {
		_, err = artifact.Run(
			b.Context(),
			built,
		)
		if err != nil {
			b.Fatal(err)
		}
	}
}
