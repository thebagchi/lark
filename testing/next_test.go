package testing_test

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	_ "github.com/thebagchi/lark/runtime/plugin/base32"
	_ "github.com/thebagchi/lark/runtime/plugin/base64"
	larkfile "github.com/thebagchi/lark/runtime/plugin/file"
	_ "github.com/thebagchi/lark/runtime/plugin/hash"
	_ "github.com/thebagchi/lark/runtime/plugin/math"
	_ "github.com/thebagchi/lark/runtime/plugin/path"
	larkrandom "github.com/thebagchi/lark/runtime/plugin/random"
	_ "github.com/thebagchi/lark/runtime/plugin/regexp"
	"github.com/thebagchi/lark/runtime/scheduler"
)

func TestRandomInt_FullRange(t *testing.T) {
	value, err := _Run(t, `
def main():
    random.seed(1)
    return random.int(0, 9223372036854775807)
`)
	if err != nil {
		if strings.Contains(err.Error(), "recovered") {
			t.Fatalf("recovered panic, want a number or ErrRange: %v", err)
		}
		if !errors.Is(err, larkrandom.ErrRange) {
			t.Fatal(err)
		}
		return
	}

	number, ok := value.(starlark.Int)
	if !ok {
		t.Fatalf("got %T", value)
	}
	held, exact := number.Int64()
	if !exact || held < 0 {
		t.Fatalf("got %s, want a number inside the range", value.String())
	}

	_, err = _Run(t, `
def main():
    random.seed(1)
    return random.int(-9223372036854775808, 9223372036854775807)
`)
	if err != nil {
		t.Fatalf("the whole int64 range: %v", err)
	}
}

func TestRandomInt_Edges(t *testing.T) {
	value, err := _Run(t, `
def main():
    random.seed(1)
    return random.int(0, 0)
`)
	if err != nil {
		t.Fatal(err)
	}
	if value.String() != "0" {
		t.Fatalf("got %s, want 0", value.String())
	}

	_, err = _Run(t, `
def main():
    random.seed(1)
    return random.int(3, 1)
`)
	if !errors.Is(err, larkrandom.ErrRange) {
		t.Fatalf("got %v, want ErrRange", err)
	}
}

func TestRandomInt_ThreadsShareOneSequence(t *testing.T) {
	parallel, err := _Run(t, `
def draw():
    out = []
    for _ in range(200):
        out.append(random.int(0, 9))
    return out

def main():
    random.seed(1)
    handles = [spawn(draw) for _ in range(8)]
    parts = join(*handles)
    bag = []
    for part in parts:
        for n in part:
            bag.append(n)
    return bag
`)
	if err != nil {
		t.Fatal(err)
	}

	alone, err := _Run(t, `
def main():
    random.seed(1)
    out = []
    for _ in range(1600):
        out.append(random.int(0, 9))
    return out
`)
	if err != nil {
		t.Fatal(err)
	}

	got := _Ints(t, parallel)
	want := _Ints(t, alone)
	sort.Ints(got)
	sort.Ints(want)
	if len(got) != 1600 || !slicesEqual(got, want) {
		t.Fatalf("parallel bag differs from one thread drawing 1600 times, len %d", len(got))
	}
}

func TestFile_SymlinkAndDirectory(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.txt")
	if err := os.WriteFile(target, []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}

	value, err := _Run(t, fmt.Sprintf(`
def main():
    return [file.exists(%q), file.read(%q)]
`, link, link))
	if err != nil {
		t.Fatal(err)
	}
	if value.String() != `[True, "hello"]` {
		t.Fatalf("symlink: %s", value.String())
	}

	_, err = _Run(t, fmt.Sprintf(`
def main():
    file.remove(%q)
    return None
`, link))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("link still there: %v", err)
	}
	body, err := os.ReadFile(target)
	if err != nil || string(body) != "hello" {
		t.Fatalf("target after removing the link: %q %v", body, err)
	}

	broken := filepath.Join(dir, "broken")
	if err := os.Symlink(filepath.Join(dir, "missing"), broken); err != nil {
		t.Fatal(err)
	}
	value, err = _Run(t, fmt.Sprintf(`
def main():
    return file.exists(%q)
`, broken))
	if err != nil {
		t.Fatal(err)
	}
	if value.String() != "False" {
		t.Fatalf("broken symlink exists: %s", value.String())
	}

	_, err = _Run(t, fmt.Sprintf(`
def main():
    return file.read(%q)
`, dir))
	if !errors.Is(err, larkfile.ErrFile) {
		t.Fatalf("read directory: %v", err)
	}

	parent := filepath.Join(dir, "not-a-dir")
	if err := os.WriteFile(parent, []byte("stay"), 0o644); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(parent, "child.txt")
	_, err = _Run(t, fmt.Sprintf(`
def main():
    file.write(%q, "x")
    return None
`, child))
	if !errors.Is(err, larkfile.ErrFile) {
		t.Fatalf("write through a file: %v", err)
	}
	body, err = os.ReadFile(parent)
	if err != nil || string(body) != "stay" {
		t.Fatalf("parent changed: %q %v", body, err)
	}
}

func TestFile_AppendsStayWhole(t *testing.T) {
	dir := t.TempDir()
	named := filepath.Join(dir, "log.txt")
	_, err := _Run(t, fmt.Sprintf(`
def left():
    for _ in range(80):
        file.append(%q, "AAAAAAAAAA\n")

def right():
    for _ in range(80):
        file.append(%q, "BBBBBBBBBB\n")

def main():
    join(spawn(left), spawn(right))
    return file.read(%q)
`, named, named, named))
	if err != nil {
		t.Fatal(err)
	}

	text, err := os.ReadFile(named)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(string(text), "\n"), "\n")
	if len(lines) != 160 {
		t.Fatalf("got %d lines", len(lines))
	}
	for _, line := range lines {
		if line != "AAAAAAAAAA" && line != "BBBBBBBBBB" {
			t.Fatalf("interleaved line %q", line)
		}
	}
}

func TestRegexp_LogLineOffsetsSplitAndCount(t *testing.T) {
	value, err := _Run(t, `
def main():
    line = '127.0.0.1 - frank [10/Oct/2000:13:55:36 -0700] "GET /apache_pb.gif HTTP/1.0" 200 2326'
    found = regexp.search(r'^(\S+) (\S+) (\S+) \[([^\]]+)\] "(\S+) (\S+) [^"]+" (\d+) (\S+)', line)
    groups = found.groups
    letter = regexp.search(r"é", "café")
    return [
        groups[0], groups[4], groups[5], groups[6],
        letter.text, letter.start, letter.end, "café"[letter.start:letter.end],
        regexp.split(r"(\d)", "a1b2c"),
        regexp.sub(r"a", "-", "banana", count = 0),
        regexp.sub(r"a", "-", "banana", count = -1),
    ]
`)
	if err != nil {
		t.Fatal(err)
	}
	want := `["127.0.0.1", "GET", "/apache_pb.gif", "200", "é", 3, 5, "é", ["a", "b", "c"], "b-n-n-", "b-n-n-"]`
	if value.String() != want {
		t.Fatalf("got %s\nwant %s", value.String(), want)
	}
}

func TestState_SetOfAnotherNameInsideUpdateIsRefused(t *testing.T) {
	_, err := _Run(t, `
def main():
    def change(v):
        state.set("b", 1)
        return v
    state.update("a", change)
    return "reached"
`)
	if !errors.Is(err, scheduler.ErrNested) {
		t.Fatalf("got %v, want ErrNested", err)
	}
}

func TestAdd_GivesTheWrittenListItsOwnContainers(t *testing.T) {
	value, err := _Run(t, `
def main():
    doc = {"a": [1]}
    out = patch_json(doc, [{"op": "add", "path": "/b", "value": doc["a"]}])
    out["b"].append(2)
    return [doc["a"], out["b"]]
`)
	if err != nil {
		t.Fatal(err)
	}
	if value.String() != "[[1], [1, 2]]" {
		t.Fatalf("got %s, want the original left at [1] and the added list its own", value.String())
	}
}

func TestRoundTrip_NewModulesPrintTheSame(t *testing.T) {
	root := _ModuleRoot(t)
	dir := t.TempDir()
	script := filepath.Join(dir, "round.star")
	body := []byte(`
def main():
    random.seed(7)
    nums = []
    for _ in range(20):
        nums.append(random.int(0, 9))
    print(math.log2(1024))
    print(math.gcd(12, 18))
    print(regexp.sub(r"(\w+)@(\w+)", "$2/$1", "bob@corp"))
    print(hash.sha256("abc"))
    print(base64.encode("foobar"))
    print(base32.encode("foobar"))
    print(path.base(path.join("a", "b.txt")))
    print(nums)
`)
	if err := os.WriteFile(script, body, 0o644); err != nil {
		t.Fatal(err)
	}
	graph := filepath.Join(dir, "round.json")

	fromScript := _Lark(t, root, "-s", script)
	written := _Lark(t, root, "-s", script, "-t")
	if err := os.WriteFile(graph, written, 0o644); err != nil {
		t.Fatal(err)
	}
	fromGraph := _Lark(t, root, "-g", graph)
	if !bytes.Equal(fromScript, fromGraph) {
		t.Fatalf("script:\n%s\ngraph:\n%s", fromScript, fromGraph)
	}
}

func _Ints(t *testing.T, value starlark.Value) []int {
	t.Helper()
	list, ok := value.(*starlark.List)
	if !ok {
		t.Fatalf("got %T", value)
	}
	out := make([]int, 0, list.Len())
	for i := range list.Len() {
		number, ok := list.Index(i).(starlark.Int)
		if !ok {
			t.Fatalf("element %d is %s", i, list.Index(i).Type())
		}
		held, err := starlark.AsInt32(number)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, held)
	}
	return out
}

func slicesEqual(left []int, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func _ModuleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func _Lark(t *testing.T, root string, args ...string) []byte {
	t.Helper()
	cmd := exec.Command("go", append([]string{"run", "./cmd/lark"}, args...)...)
	cmd.Dir = root
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("lark %s: %v\n%s", strings.Join(args, " "), err, out)
	}
	return out
}
