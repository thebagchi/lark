// Probes for README.md and the package comment of v1/runtime (C66, bugs B109, B110,
// C83b): the Go code in them must not rot.
//
// The README holds fenced blocks. A fence marked "go" is a whole program, and
// it must pass go vet in a temporary module that points at this repository. A
// fence marked "go fragment" is a piece that needs the code around it. The
// test wraps it in a file and builds it, and fails on each name that does not
// resolve.
//
// The package comment holds indented blocks, with no mark. A block is Go when
// go/parser accepts it as a file, as declarations, or as statements that name a
// member of a package (_IsGo). A Go block that starts with "package " is a
// program, and any other is a fragment. Both get the checks above.
//
// Every Err name in the README and in the package comment must exist in the
// code.
//
// Revisions:
//   - 2026-11-04 21:42: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 22:36: the package comment too, and the parse rule (C83b, Q264)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
package tests

import (
	"fmt"
	"go/ast"
	"go/doc/comment"
	"go/parser"
	"go/token"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

const (
	// README is the file that holds the fences. ROOT_MODULE is the module of
	// this repository, and TEST_MODULE the module that holds each program.
	//
	// Revisions:
	//   - 2026-11-04 21:42: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	README      = "README.md"
	ROOT_MODULE = "github.com/thebagchi/lark"
	TEST_MODULE = "readmecheck"

	// PROGRAM marks a whole program, and FRAGMENT a piece of one. They are the
	// text after the three backticks of an opening fence.
	//
	// Revisions:
	//   - 2026-11-04 21:42: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	PROGRAM  = "go"
	FRAGMENT = "go fragment"
)

// _Fence is one fenced block of the README, or one Go block of the package
// comment of v1/runtime.
//
// Revisions:
//   - 2026-11-04 21:42: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 19:03: also a Go block of the runtime package comment, with from (C83b, Q264)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Fence struct {
	from string
	line int
	info string
	body string
}

var (
	// POSITION finds a build error that points into a fragment file.
	//
	// Revisions:
	//   - 2026-11-04 21:42: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	POSITION = regexp.MustCompile(`^f\d+/main\.go:\d+:\d+: `)

	// ALLOWED finds the errors that the wrapper causes. A value that the
	// fragment makes and does not use, and an import that it does not use.
	//
	// Revisions:
	//   - 2026-11-04 21:42: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	ALLOWED = regexp.MustCompile(`: (declared and not used: |.* imported and not used)`)

	// TYPED holds the names that another fence declares, with a type that this
	// test can write. _Wrapped declares each with its type, so the compiler
	// checks every use: ctx.Donex() and updates.Nosuch() fail. A fragment that
	// declares the name itself at the top of the file gets no declaration. A
	// name that is not here, not in ASSERTS and not a package of IMPORTS fails,
	// which finds a misspelled package (runtim.Start).
	//
	// Revisions:
	//   - 2026-11-04 21:42: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-07 23:23: run and _Supervise take runtime.Execution, an
	//     interface now (C91w7)
	//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	TYPED = map[string]string{
		"BUNDLE":      "string",
		"BUNDLE_MODE": "os.FileMode",
		"EVERY":       "time.Duration",
		"LIMIT":       "time.Duration",
		"LOG_FILE":    "string",
		"SCRIPT":      "string",
		"STEPS":       "string",
		"body":        "[]byte",
		"build":       "[]byte",
		"built":       "common.Artifact",
		"ctx":         "context.Context",
		"run":         "common.Execution",
		"sent":        "[]byte",
		"source":      "*common.Source",
		"src":         "[]byte",
		"steps":       "[]byte",
		"updates":     "chan []byte",
		"modules":     "common.Loader",
		"settings":    "common.Plugin",
		"_Supervise":  "func(common.Execution, context.CancelFunc)",
	}

	// ASSERTS holds, for each type that a fence of the README declares and
	// another fence uses, a line that proves the type has the methods of the
	// interface that the README says it is. The README is the only source of
	// the type: _Wrapped copies the declaring fence into each fragment that
	// uses the name, and adds this line once the type is in the file.
	//
	// Revisions:
	//   - 2026-11-04 21:42: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-07 23:42: types come from common and workflowpb (C91w9, Q103)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	ASSERTS = map[string]string{
		"_Memory":   "var _ common.Loader = (*_Memory)(nil)\n",
		"_Settings": "var _ common.Plugin = (*_Settings)(nil)\n",
	}

	// IMPORTS maps the name that a fragment writes before a dot to its package.
	//
	// Revisions:
	//   - 2026-11-04 21:42: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-07 23:42: common and workflowpb, which the README names now (C91w9)
	//   - 2026-11-08 19:03: path and filepath, for the Disk loader of the package doc (C83b)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	IMPORTS = map[string]string{
		"runtime":    "github.com/thebagchi/lark/v1/runtime",
		"common":     "github.com/thebagchi/lark/v1/runtime/common",
		"workflowpb": "github.com/thebagchi/lark/proto/gen/workflow",
		"remote":     "github.com/thebagchi/lark/v1/runtime/plugin/remote",
		"file":       "github.com/thebagchi/lark/v1/plugin/file",
		"starlark":   "go.starlark.net/starlark",
		"protojson":  "google.golang.org/protobuf/encoding/protojson",
		"log":        "log",
		"slog":       "log/slog",
		"time":       "time",
		"os":         "os",
		"errors":     "errors",
		"fmt":        "fmt",
		"context":    "context",
		"json":       "encoding/json",
		"io":         "io",
		"bytes":      "bytes",
		"fs":         "io/fs",
		"path":       "path",
		"filepath":   "path/filepath",
	}

	// UNCHECKED holds the first line of each fragment that cannot build, and the
	// reason. It is empty while every fragment builds.
	//
	// Revisions:
	//   - 2026-11-04 21:42: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	UNCHECKED = map[string]string{}

	// ERR_NAME finds a name that follows the rule for an exported error.
	//
	// Revisions:
	//   - 2026-11-04 21:42: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	ERR_NAME = regexp.MustCompile(`\bErr[A-Z][A-Za-z0-9]*\b`)

	// WRONG_ERR_NAME finds the old spelling of an error name.
	//
	// Revisions:
	//   - 2026-11-04 21:42: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	WRONG_ERR_NAME = regexp.MustCompile(`\bERR_[A-Z_]+\b`)
)

// _Root returns the directory that holds go.mod, found from the test directory.
//
// Revisions:
//   - 2026-11-04 19:56: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Root(t *testing.T) string {
	t.Helper()

	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	for {
		_, err = os.Stat(filepath.Join(
			dir,
			"go.mod",
		))
		if err == nil {
			return dir
		}

		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("no go.mod above the test directory")
		}

		dir = parent
	}
}

// _Readme reads the README of the repository.
//
// Revisions:
//   - 2026-11-04 19:56: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Readme(t *testing.T) string {
	t.Helper()

	text, err := os.ReadFile(filepath.Join(
		_Root(t),
		README,
	))
	if err != nil {
		t.Fatal(err)
	}

	return string(text)
}

// _Fences returns each fenced block whose opening fence starts with "go".
//
// A fence opens with three backticks and the info text, and closes with three
// backticks alone. The line is the one-based line of the opening fence.
//
// Revisions:
//   - 2026-11-04 19:56: initial creation
//   - 2026-11-08 22:36: the fence names its file, from README (C83b, Q264)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Fences(text string) []*_Fence {
	var (
		found []*_Fence
		open  *_Fence
		body  []string
	)

	for number, line := range strings.Split(
		text,
		"\n",
	) {
		switch {
		case open == nil && strings.HasPrefix(
			line,
			"```go",
		):
			open = &_Fence{
				from: README,
				line: number + 1,
				info: strings.TrimPrefix(
					line,
					"```",
				),
			}
			body = nil

		case open != nil && line == "```":
			open.body = strings.Join(
				body,
				"\n",
			) + "\n"
			found = append(
				found,
				open,
			)
			open = nil

		case open != nil:
			body = append(
				body,
				line,
			)
		}
	}

	return found
}

// RUNTIME_FILE is the file that holds the package comment of v1/runtime, whose
// Go blocks the tests check as they check the fences of the README. DOC_LINE
// is added to the number of a block of that comment, so no block shares a
// directory with a fence of the README.
//
// Revisions:
//   - 2026-11-08 19:03: initial creation (C83b, Q264)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
const (
	RUNTIME_FILE = "v1/runtime/runtime.go"
	DOC_LINE     = 100000
)

// _IsGo says whether a block of the package comment is Go. A block is Go when
// go/parser accepts it as a file, or as top-level declarations (after a package
// clause), or as statements (inside a function body) that name a member of
// another package, as in runtime.Run. Starlark, JSON, shell and log blocks fail
// the parse.
//
// A Starlark line such as host = arg("host", "localhost") parses as a Go
// statement. It names no member of a package, so the selector rule refuses it.
// A Starlark block that calls a plugin function (file.read) would pass the rule
// and fail the name check. Then write the block as a fence in the README, or
// change it so it does not parse as Go.
//
// Revisions:
//   - 2026-11-08 22:36: initial creation (C83b, Q264)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _IsGo(text string) bool {
	for _, wrap := range []struct {
		format   string
		selector bool
	}{
		{
			"%s",
			false,
		},
		{
			"package p\n%s",
			false,
		},
		{
			"package p\nfunc _() {\n%s\n}",
			true,
		},
	} {
		file, err := parser.ParseFile(
			token.NewFileSet(),
			"block.go",
			fmt.Sprintf(
				wrap.format,
				text,
			),
			parser.SkipObjectResolution,
		)
		if err != nil {
			continue
		}

		if !wrap.selector || _HasSelector(file) {
			return true
		}
	}

	return false
}

// _HasSelector says whether file holds a name of the form x.y, where x is a
// plain name.
//
// Revisions:
//   - 2026-11-08 22:36: initial creation (C83b, Q264)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _HasSelector(file *ast.File) bool {
	found := false

	ast.Inspect(
		file,
		func(node ast.Node) bool {
			selector, isSelector := node.(*ast.SelectorExpr)
			if isSelector {
				_, plain := selector.X.(*ast.Ident)
				found = found || plain
			}

			return !found
		},
	)

	return found
}

// TestReadme_IsGo proves _IsGo takes the Go blocks of a package comment that a
// start-of-block rule missed (a leading comment, an assignment, a bare call, an
// if), and leaves prose, shell, Starlark, JSON and log blocks alone.
//
// Revisions:
//   - 2026-11-08 22:36: initial creation (C83b, Q264)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestReadme_IsGo(t *testing.T) {
	cases := []struct {
		name string
		text string
		want bool
	}{
		{
			"comment then call",
			"// note\nruntime.Nosuch(ctx)\n",
			true,
		},
		{
			"assignment",
			"x = runtime.Nosuch2(ctx)\n",
			true,
		},
		{
			"bare call",
			"runtime.Nosuch3(ctx, built)\n",
			true,
		},
		{
			"if",
			"if errors.Is(e, runtime.Nosuch4) {\n\treturn e\n}\n",
			true,
		},
		{
			"var",
			"var y = runtime.Nosuch5()\n",
			true,
		},
		{
			"declarations",
			"const (\n\tA = 1\n)\n",
			true,
		},
		{
			"program",
			"package main\n\nfunc main() {}\n",
			true,
		},
		{
			"prose",
			"Run the build, then read the output.\n",
			false,
		},
		{
			"shell",
			"go build -o plugins/lark-clock.bin ./cmd/clock\n",
			false,
		},
		{
			"starlark def",
			"def main():\n    return 1\n",
			false,
		},
		{
			"starlark load",
			"load(\"steps.star\", \"fetch\")\n",
			false,
		},
		{
			"starlark arg",
			"host = arg(\"host\", \"localhost\")\n",
			false,
		},
		{
			"json",
			"{\"host\": \"x\"}\n",
			false,
		},
		{
			"log",
			"time=... level=INFO msg=\"fetching lark\"\n",
			false,
		},
		{
			"patch",
			"add /status \"STATUS_RUNNING\"\n",
			false,
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				if got := _IsGo(tc.text); got != tc.want {
					t.Fatalf(
						"_IsGo(%q) = %v, want %v",
						tc.text,
						got,
						tc.want,
					)
				}
			},
		)
	}
}

// _DocText returns the text of the package comment of v1/runtime, without its
// Revisions: an entry there names what the code was called when it was made,
// and an old entry is never rewritten (Q224).
//
// Revisions:
//   - 2026-11-08 21:14: initial creation (C78, Q265)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:26: stops at the Revisions of the comment, whose old entries name the
//     identifiers renamed by C121 (C121, Q347)
func _DocText(t *testing.T) string {
	t.Helper()

	file, err := parser.ParseFile(
		token.NewFileSet(),
		filepath.Join(
			_Root(t),
			RUNTIME_FILE,
		),
		nil,
		parser.ParseComments|parser.PackageClauseOnly,
	)
	if err != nil {
		t.Fatal(err)
	}

	if file.Doc == nil {
		t.Fatal("v1/runtime has no package comment")
	}

	guide, _, _ := strings.Cut(
		file.Doc.Text(),
		"\nRevisions:\n",
	)

	return guide
}

// _DocFences returns each Go block of the package comment of v1/runtime as a
// fence. A block that opens with "package " is a program, and any other is a
// fragment. The line of a fence is DOC_LINE and the number of the block.
//
// Revisions:
//   - 2026-11-08 19:03: initial creation (C83b, Q264)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _DocFences(t *testing.T) []*_Fence {
	t.Helper()

	doc := _DocText(t)

	var (
		cp     comment.Parser
		found  []*_Fence
		number int
	)

	for _, block := range cp.Parse(doc).Content {
		code, isCode := block.(*comment.Code)
		if !isCode {
			continue
		}

		number++

		if !_IsGo(code.Text) {
			continue
		}

		info := FRAGMENT
		if strings.HasPrefix(
			code.Text,
			"package ",
		) {
			info = PROGRAM
		}

		found = append(
			found,
			&_Fence{
				from: RUNTIME_FILE,
				line: DOC_LINE + number,
				info: info,
				body: code.Text,
			},
		)
	}

	return found
}

// _AllFences returns the Go fences of the README, then the Go blocks of the
// package comment of v1/runtime.
//
// Revisions:
//   - 2026-11-08 19:03: initial creation (C83b, Q264)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _AllFences(t *testing.T) []*_Fence {
	t.Helper()

	return append(
		_Fences(_Readme(t)),
		_DocFences(t)...,
	)
}

// _Module writes the temporary module that holds the given files, and returns
// its directory.
//
// The go.mod names this repository with a replace directive. It copies the
// requirements and the go.sum of the repository, so the check needs no network.
// A key of files is a path under the module, as f180/main.go.
//
// Revisions:
//   - 2026-11-04 19:56: initial creation
//   - 2026-11-04 19:56: takes many files, so one module holds every fence
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Module(
	t *testing.T,
	files map[string]string,
) string {
	t.Helper()

	var (
		root = _Root(t)
		dir  = t.TempDir()
	)

	mod, err := os.ReadFile(filepath.Join(
		root,
		"go.mod",
	))
	if err != nil {
		t.Fatal(err)
	}

	// The first line of go.mod names the module. The tool block is of no use here.
	text := strings.Replace(
		string(mod),
		"module "+ROOT_MODULE,
		"module "+TEST_MODULE,
		1,
	)
	text += "\nrequire " + ROOT_MODULE + " v0.0.0\n"
	text += "\nreplace " + ROOT_MODULE + " => " + strconv.Quote(root) + "\n"

	sum, err := os.ReadFile(filepath.Join(
		root,
		"go.sum",
	))
	if err != nil {
		t.Fatal(err)
	}

	files["go.mod"] = text
	files["go.sum"] = string(sum)

	for name, data := range files {
		path := filepath.Join(
			dir,
			name,
		)

		err = os.MkdirAll(
			filepath.Dir(path),
			0o700,
		)
		if err != nil {
			t.Fatal(err)
		}

		err = os.WriteFile(
			path,
			[]byte(data),
			0o600,
		)
		if err != nil {
			t.Fatal(err)
		}
	}

	return dir
}

// _Go runs the go command in dir, with the module cache and no network, and
// returns its output.
//
// -mod=mod lets go add the indirect requirements it finds in the module cache.
// The proxy is off, so nothing leaves this machine.
//
// Revisions:
//   - 2026-11-04 19:56: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Go(
	dir string,
	args ...string,
) (string, error) {
	cmd := exec.Command(
		"go",
		args...,
	)
	cmd.Dir = dir

	cmd.Env = append(
		os.Environ(),
		"GOFLAGS=-mod=mod",
		"GOPROXY=off",
	)

	out, err := cmd.CombinedOutput()

	return string(out), err
}

// _BuildFence builds one fence of the module with every error shown, and returns
// the output. The exit status is of no use: a fragment always fails to build.
//
// Revisions:
//   - 2026-11-04 19:56: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _BuildFence(
	dir string,
	fence *_Fence,
) string {
	out, _ := _Go(
		dir,
		"build",
		"-gcflags=-e",
		"-o",
		os.DevNull,
		"./"+_Dir(fence),
	)

	return out
}

// _Dir names the directory of a fence in the temporary module.
//
// Revisions:
//   - 2026-11-04 19:56: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Dir(fence *_Fence) string {
	return "f" + strconv.Itoa(fence.line)
}

// TestReadme_FencesAreMarked proves that each Go fence is a whole program or a
// marked fragment, and that a fragment hides no program.
//
// A fence marked "go" that lacks a package clause cannot compile, and a fence
// marked "go fragment" that has one would skip the check by mistake.
//
// Revisions:
//   - 2026-11-04 19:56: initial creation
//   - 2026-11-08 22:36: reads the fences of the package comment too (C83b, Q264)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestReadme_FencesAreMarked(t *testing.T) {
	fences := _AllFences(t)
	if len(fences) == 0 {
		t.Fatal("no Go fence found: the parser is wrong or the README lost them")
	}

	for _, fence := range fences {
		t.Run(
			"line_"+strconv.Itoa(fence.line),
			func(t *testing.T) {
				whole := strings.HasPrefix(
					fence.body,
					"package ",
				)

				switch {
				case fence.info != PROGRAM && fence.info != FRAGMENT:
					t.Fatalf(
						"info %q: write %q or %q",
						fence.info,
						PROGRAM,
						FRAGMENT,
					)

				case fence.info == PROGRAM && !whole:
					t.Fatal("no package clause: mark it go fragment")

				case fence.info == FRAGMENT && whole:
					t.Fatal("a program: mark it go")
				}
			},
		)
	}
}

// TestReadme_ProgramsVet proves that each whole program of the README passes
// go vet, in a temporary module that replaces this repository by its directory.
//
// Run: go test -run TestReadme_ProgramsVet ./v1/testing. A failure names the
// line of the opening fence and prints the output of go vet.
//
// Revisions:
//   - 2026-11-04 19:56: initial creation
//   - 2026-11-04 19:56: one module holds every program
//   - 2026-11-08 22:36: reads the fences of the package comment too (C83b, Q264)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestReadme_ProgramsVet(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a module for the programs")
	}

	var (
		files = map[string]string{}
		found []*_Fence
	)

	for _, fence := range _AllFences(t) {
		if fence.info == PROGRAM {
			found = append(
				found,
				fence,
			)
			files[_Dir(fence)+"/main.go"] = fence.body
		}
	}

	dir := _Module(
		t,
		files,
	)

	for _, fence := range found {
		t.Run(
			"line_"+strconv.Itoa(fence.line),
			func(t *testing.T) {
				out, err := _Go(
					dir,
					"vet",
					"./"+_Dir(fence),
				)
				if err != nil {
					t.Fatalf(
						"%s line %d does not pass go vet: %v\n%s",
						fence.from,
						fence.line,
						err,
						out,
					)
				}
			},
		)
	}
}

// TestReadme_FragmentNames proves that each name a fragment takes from a
// package resolves.
//
// A fragment cannot compile alone: it uses names that nearby fences define.
// So the test wraps the fragment in a file, builds the file with every error
// shown, and fails on every error except a short, named list that the wrapper
// causes (see ALLOWED and _Wrong). That finds a wrong function, type,
// constant or sentinel of a package, a wrong struct field, a wrong method of
// a value that the fragment makes or that TYPED declares (ctx.Donex() fails),
// and a type mismatch.
//
// Every name that another fence declares has a type here. A name of a package
// type comes from TYPED. A type that the README declares (_Memory, _Settings)
// comes from the README itself: _Wrapped copies the declaring fence into each
// fragment that uses it, and ASSERTS proves it still is a common.Loader or a
// common.Plugin. The README is the only source for those types.
//
// The remaining limit: the test writes the types of TYPED by hand, and the
// names modules and settings have the interface types common.Loader and
// common.Plugin, so in a fence that does not declare them, a method outside
// the interface fails even when the README type has it. A fence that needs one
// adds to TYPED.
//
// A fence of UNCHECKED does not build, and the map names why.
//
// Run: go test -run TestReadme_FragmentNames ./v1/testing.
//
// Revisions:
//   - 2026-11-04 19:56: initial creation
//   - 2026-11-04 19:56: fails on every error that ALLOWED does not name
//   - 2026-11-04 19:56: the wrapper returns error and TYPED gives
//     names their types, so a wrong method on them fails
//   - 2026-11-04 19:56: TYPED and STUBS type every name, so none goes unchecked
//   - 2026-11-04 19:56: the README declares _Memory and _Settings, with no copy in the test
//   - 2026-11-08 22:36: reads the fences of the package comment too (C83b, Q264)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestReadme_FragmentNames(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a module for the fragments")
	}

	var (
		files = map[string]string{}
		found []*_Fence
	)

	all := _AllFences(t)

	for _, fence := range all {
		if fence.info == FRAGMENT {
			found = append(
				found,
				fence,
			)
			files[_Dir(fence)+"/main.go"] = _Wrapped(
				fence,
				all,
			)
		}
	}

	dir := _Module(
		t,
		files,
	)

	for _, fence := range found {
		t.Run(
			"line_"+strconv.Itoa(fence.line),
			func(t *testing.T) {
				why, skipped := UNCHECKED[_FirstLine(fence.body)]
				if skipped {
					t.Skip(why)
				}

				bad := _Wrong(_BuildFence(
					dir,
					fence,
				))

				if len(bad) > 0 {
					t.Fatalf(
						"%s line %d: a name does not resolve:\n%s",
						fence.from,
						fence.line,
						strings.Join(
							bad,
							"\n",
						),
					)
				}
			},
		)
	}
}

// TestReadme_ErrNames proves that the README and the package comment of
// v1/runtime use no old error name, and that each ErrName in them is a name
// that the code declares.
//
// Names that begin with fs. belong to the standard library, so the test skips
// them.
//
// Revisions:
//   - 2026-11-04 19:56: initial creation
//   - 2026-11-08 21:14: the package comment of v1/runtime too (C78, Q265)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestReadme_ErrNames(t *testing.T) {
	declared := _Declared(t)

	for from, text := range map[string]string{
		README:       _Readme(t),
		RUNTIME_FILE: _DocText(t),
	} {
		_ErrNames(
			t,
			from,
			text,
			declared,
		)
	}
}

// _ErrNames reports each old error name in text, and each ErrName in it that
// the code does not declare. The name from tells which file the text is of.
//
// Revisions:
//   - 2026-11-08 21:14: initial creation (C78, Q265)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _ErrNames(
	t *testing.T,
	from string,
	text string,
	declared map[string]bool,
) {
	t.Helper()

	for _, wrong := range WRONG_ERR_NAME.FindAllString(
		text,
		-1,
	) {
		t.Errorf(
			"%s: %s: write %s as ErrName",
			from,
			wrong,
			wrong,
		)
	}

	for _, at := range ERR_NAME.FindAllStringIndex(
		text,
		-1,
	) {
		name := text[at[0]:at[1]]

		if strings.HasSuffix(
			text[:at[0]],
			"fs.",
		) {
			continue
		}

		if !declared[name] {
			t.Errorf(
				"%s names %s, and no Go file of the repository declares it",
				from,
				name,
			)
		}
	}
}

// _Wrapped puts a fragment in a file that go build reads.
//
// A fragment that starts with const, type or func (after any comment) is a
// set of declarations, and it goes at the top of the file. Any other fragment
// is statements, and it goes inside a function that returns error and ends with
// return nil, so a bare return err of the fragment is valid and the wrapper
// causes no return error. The file imports each package
// that the fragment names with a dot, from IMPORTS. An import that the fragment
// does not use is an error that ALLOWED lets pass.
//
// Revisions:
//   - 2026-11-04 19:56: initial creation
//   - 2026-11-04 19:56: the function returns error, so the wrapper causes no return error
//   - 2026-11-04 19:56: declares the names of TYPED with their types
//   - 2026-11-04 19:56: declares the types of STUBS too
//   - 2026-11-04 19:56: copies the declaring fence of a README type, not a stub of the test
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Wrapped(
	fence *_Fence,
	all []*_Fence,
) string {
	var (
		head    = "package main\n\nimport (\n"
		text    = fence.body
		wrapped = !_Declares(fence.body)
		top     = fence.body
	)

	if wrapped {
		text = "func _fragment() error {\n" + fence.body + "\treturn nil\n}\n"
		top = ""
	}

	// The declaring fences of the types the fragment uses, then the proof that
	// each type is what the README says it is.
	extra := _Declaring(
		fence,
		all,
		top,
	)
	top += extra

	for _, name := range _Sorted(ASSERTS) {
		if _Owns(
			top,
			name,
		) {
			extra += ASSERTS[name]
		}
	}

	text = _Prelude(top) + extra + text

	for name, path := range IMPORTS {
		if regexp.MustCompile(`\b` + name + `\.`).MatchString(text) {
			head += "\t" + name + " " + strconv.Quote(path) + "\n"
		}
	}

	return head + ")\n\n" + text
}

// _Declaring returns the text of each fence that declares a type of ASSERTS
// that the fence uses and does not declare itself.
//
// Revisions:
//   - 2026-11-04 19:56: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Declaring(
	fence *_Fence,
	all []*_Fence,
	top string,
) string {
	var out string

	for _, name := range _Sorted(ASSERTS) {
		if _Owns(
			top,
			name,
		) || !regexp.MustCompile(`\b`+name+`\b`).MatchString(fence.body) {
			continue
		}

		for _, other := range all {
			if other != fence && _Owns(
				other.body,
				name,
			) {
				out += other.body + "\n"

				break
			}
		}
	}

	return out
}

// _Prelude declares each name of TYPED with its type, in sorted order.
//
// The argument top is the text of the fragment when it declares names at the
// top of the file, and empty when it is statements. A name that top declares
// gets no line, found by a line that starts with the name and =, or by func,
// type, var or const. A statement fragment gets every name, because a name it
// declares with := lives in the function and hides the file name.
//
// Revisions:
//   - 2026-11-04 19:56: initial creation
//   - 2026-11-04 19:56: adds the types of STUBS
//   - 2026-11-04 19:56: the types of STUBS left; the README declares them
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Prelude(top string) string {
	var out string

	for _, name := range _Sorted(TYPED) {
		if !_Owns(
			top,
			name,
		) {
			out += "var " + name + " " + TYPED[name] + "\n"
		}
	}

	return out + "\n"
}

// _Owns tells whether top declares name: a line that starts with the name and
// =, or the keyword func, type, var or const before the name.
//
// Revisions:
//   - 2026-11-04 19:56: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Owns(
	top string,
	name string,
) bool {
	return regexp.MustCompile(
		`(?m)^\s*` + name + `\s*=|\b(func|type|var|const)\s+` + name + `\b`,
	).MatchString(top)
}

// _Sorted returns the keys of a map of strings in order, so the file that the
// wrapper writes is the same on each run.
//
// Revisions:
//   - 2026-11-04 19:56: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Sorted(held map[string]string) []string {
	keys := make(
		[]string,
		0,
		len(held),
	)
	for key := range held {
		keys = append(
			keys,
			key,
		)
	}

	slices.Sort(keys)

	return keys
}

// _FirstLine returns the first line of body, without space at its ends.
//
// Revisions:
//   - 2026-11-04 19:56: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _FirstLine(body string) string {
	first, _, _ := strings.Cut(
		body,
		"\n",
	)

	return strings.TrimSpace(first)
}

// _Declares tells whether the first line of code in body starts a declaration.
//
// Revisions:
//   - 2026-11-04 19:56: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Declares(body string) bool {
	for _, line := range strings.Split(
		body,
		"\n",
	) {
		if line == "" || strings.HasPrefix(
			line,
			"//",
		) {
			continue
		}

		for _, word := range []string{"const ", "type ", "func "} {
			if strings.HasPrefix(
				line,
				word,
			) {
				return true
			}
		}

		return false
	}

	return false
}

// _Wrong returns the errors of go build output that the fragment itself causes.
//
// The test takes the opposite path to a list of known mistakes: any error is a
// failure, except the errors that the wrapper or the missing context of a
// fragment cause (see ALLOWED). So an unknown field, a wrong method, a wrong
// constant, a wrong type and a type mismatch each fail, and so does an error
// text that this test has never seen. Only a line that names a position in a
// fragment file counts, so the header and the continuation lines of an error
// are not judged apart from its first line.
//
// Revisions:
//   - 2026-11-04 19:56: initial creation
//   - 2026-11-04 19:56: a name is allowed only if CONTEXT lists it, and a return is never allowed
//   - 2026-11-04 19:56: no undefined name is allowed, because TYPED and the README type each one
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Wrong(out string) []string {
	var found []string

	for _, line := range strings.Split(
		out,
		"\n",
	) {
		if !POSITION.MatchString(line) || ALLOWED.MatchString(line) {
			continue
		}

		found = append(
			found,
			line,
		)
	}

	return found
}

// _Declared returns each Err name that a non-test Go file of the repository
// declares or re-exports at the start of a line.
//
// Revisions:
//   - 2026-11-04 19:56: initial creation
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Declared(t *testing.T) map[string]bool {
	t.Helper()

	var (
		root  = _Root(t)
		found = map[string]bool{}
		start = regexp.MustCompile(`(?m)^\s*(Err[A-Z][A-Za-z0-9]*)\b\s*(=|error\b)`)
	)

	err := filepath.WalkDir(
		root,
		func(
			path string,
			entry os.DirEntry,
			err error,
		) error {
			if err != nil {
				return err
			}

			if entry.IsDir() && strings.HasPrefix(
				entry.Name(),
				".",
			) && path != root {
				return filepath.SkipDir
			}

			if entry.IsDir() || !strings.HasSuffix(
				path,
				".go",
			) || strings.HasSuffix(
				path,
				"_test.go",
			) {
				return nil
			}

			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}

			for _, match := range start.FindAllStringSubmatch(
				string(data),
				-1,
			) {
				found[match[1]] = true
			}

			return nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	return found
}
