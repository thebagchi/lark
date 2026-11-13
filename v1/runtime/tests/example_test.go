package tests

// Runnable examples of the host API, from outside the package. Each example
// has an Output block, so go test runs it and checks it, and pkg.go.dev shows
// it beside the item it names. These examples show what a host does (Q171).

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"

	"github.com/thebagchi/lark/v1/plugin/file"
	"github.com/thebagchi/lark/v1/runtime"
	"github.com/thebagchi/lark/v1/runtime/common"
)

// ExampleRun shows the one-call path: a host gives a source, and gets the
// value main returned.
//
// Revisions:
//   - 2026-11-08 13:17: initial creation (C74, Q171)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func ExampleRun() {
	value, err := runtime.Run(
		context.Background(),
		&common.Source{
			Entry: "greet.star",
			Text:  []byte("def main():\n    return \"hello \" + \"world\"\n"),
		},
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(value)
	// Output: "hello world"
}

// ExampleCompile shows Compile checking a script once and giving an artifact,
// which Eval then runs once on the caller's goroutine.
//
// Revisions:
//   - 2026-11-08 19:17: initial creation (C74, Q273)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses runtime.Eval, formerly Evaluate (C121, Q338)
func ExampleCompile() {
	built, err := runtime.Compile(&common.Source{
		Entry: "double.star",
		Text:  []byte("def main():\n    return 21 * 2\n"),
	})
	if err != nil {
		log.Fatal(err)
	}

	result, err := runtime.Eval(
		context.Background(),
		built,
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(result.Value)
	// Output: 42
}

// ExampleStart shows the two-step path. Compile checks the script once, and
// Start begins a run that the host collects with Wait.
//
// Revisions:
//   - 2026-11-08 13:17: initial creation (C74, Q171)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func ExampleStart() {
	built, err := runtime.Compile(&common.Source{
		Entry: "sum.star",
		Text:  []byte("def main():\n    return 1 + 2\n"),
	})
	if err != nil {
		log.Fatal(err)
	}

	run, err := runtime.Start(
		context.Background(),
		built,
	)
	if err != nil {
		log.Fatal(err)
	}

	// Start returned at once. The host collects the run when it wants it.
	value, err := run.Wait()
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(value)
	// Output: 3
}

// ExampleStart_cancel shows that cancelling the context given to Start is how
// a host stops a run: Wait then returns ErrCancelled.
//
// Revisions:
//   - 2026-11-08 13:17: initial creation (C74, Q171)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func ExampleStart_cancel() {
	built, err := runtime.Compile(&common.Source{
		Entry: "slow.star",
		Text:  []byte("def main():\n    sleep(3600000)\n"),
	})
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	run, err := runtime.Start(
		ctx,
		built,
	)
	if err != nil {
		log.Fatal(err)
	}

	stop()

	_, err = run.Wait()

	fmt.Println(errors.Is(
		err,
		runtime.ErrCancelled,
	))
	// Output: true
}

// ExampleEval shows a host that wants the value, the graph and the prints
// together, at the end of the run.
//
// Revisions:
//   - 2026-11-08 13:17: initial creation (C74, Q171)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: renamed from ExampleEvaluate, with runtime.Evaluate now runtime.Eval
//     (C121, Q338)
func ExampleEval() {
	built, err := runtime.Compile(&common.Source{
		Entry: "answer.star",
		Text:  []byte("def main():\n    print(\"working\")\n    return 42\n"),
	})
	if err != nil {
		log.Fatal(err)
	}

	result, err := runtime.Eval(
		context.Background(),
		built,
		runtime.WithLog(io.Discard),
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(result.Value)
	fmt.Println(result.Prints[0].Text)
	fmt.Println(result.Graph.GetStatus())
	// Output:
	// 42
	// working
	// STATUS_SUCCEEDED
}

// ExampleWithPlugins shows a host giving a script the file plugin, which is
// never one of the runtime's own. The script writes a file and reads it back.
//
// Revisions:
//   - 2026-11-08 13:17: initial creation (C74, Q171)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
//   - 2026-11-10 17:15: uses runtime.Eval, formerly Evaluate (C121, Q338)
func ExampleWithPlugins() {
	dir, err := os.MkdirTemp(
		"",
		"lark-example-",
	)
	if err != nil {
		log.Fatal(err)
	}
	defer func() {
		err := os.RemoveAll(dir)
		if err != nil {
			log.Print(err)
		}
	}()

	var (
		note = filepath.Join(
			dir,
			"note.txt",
		)
		text = fmt.Sprintf(
			"def main():\n    file.write(%q, \"saved\")\n    return file.read(%q)\n",
			note,
			note,
		)
	)

	built, err := runtime.Compile(
		&common.Source{
			Entry: "note.star",
			Text:  []byte(text),
		},
		runtime.WithPlugins(file.New()),
	)
	if err != nil {
		log.Fatal(err)
	}

	result, err := runtime.Eval(
		context.Background(),
		built,
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(result.Value)
	// Output: "saved"
}

// ExampleExport shows a host reading what a run returned as a protobuf value,
// with no type switch on go.starlark.net.
//
// Revisions:
//   - 2026-11-08 13:17: initial creation (C74, Q171)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func ExampleExport() {
	value, err := runtime.Run(
		context.Background(),
		&common.Source{
			Entry: "report.star",
			Text: []byte(
				"def main():\n" +
					"    return {\"ok\": True, \"n\": [1, 2]}\n",
			),
		},
	)
	if err != nil {
		log.Fatal(err)
	}

	exported, err := runtime.Export(
		context.Background(),
		value,
	)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Println(exported.AsInterface())
	// Output: map[n:[1 2] ok:true]
}
