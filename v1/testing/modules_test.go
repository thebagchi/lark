// Regression probes for the modules a script is given: the encodings, the
// numbers, the filesystem, the patches and the random source.
package testing_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/base32"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/base64"
	larkbase64 "github.com/thebagchi/lark/v1/runtime/plugin/base64"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/codec"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/file"
	larkfile "github.com/thebagchi/lark/v1/runtime/plugin/file"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/flow"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/hash"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/jsonpath"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/math"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/path"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/random"
	larkrandom "github.com/thebagchi/lark/v1/runtime/plugin/random"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/regexp"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/state"
)

func TestScript_ExistsCRCCopyAndSetTogether(t *testing.T) {
	dir := t.TempDir()
	secret := filepath.Join(dir, "secret")
	if err := os.Mkdir(secret, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(secret, "note"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(secret, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(secret, 0o755) })

	note := filepath.Join(secret, "note")
	src := fmt.Sprintf(`
def main():
    hidden = file.exists(%q)
    seed = crc32(b"abc", 4294967296)
    plain = crc32(b"abc", 0)
    doc = {"a": [1]}
    out = patch_json(doc, [{"op": "copy", "from": "/a", "path": "/b"}])
    out["b"].append(2)

    def slow(v):
        sleep(0.05)
        return "from-update"

    def writer():
        sleep(0.01)
        state.set("n", "from-set")

    handle = spawn(writer)
    state.update("n", slow)
    join(handle)
    return [hidden, seed == plain, doc["a"], state.get("n")]
`, note)

	built, err := runtime.NewCompiler().Compile("probe.star", []byte(src))
	if err != nil {
		t.Fatal(err)
	}
	value, err := built.Run(context.Background())
	if err != nil {
		// file.exists is first. After the fix it refuses an unsearchable
		// directory, so this script stops there. That refusal is the pass.
		if !errors.Is(err, os.ErrPermission) {
			t.Fatal(err)
		}

		return
	}

	// Seen twice on 2026-09-24, before the fix: [False, True, [1, 2], "from-update"].
	const seen = `[False, True, [1, 2], "from-update"]`
	if value.String() == seen {
		t.Errorf("reproduced file.exists, crc32, patch copy, and state.set: %s", seen)
	}
}

func TestExists_BehindAClosedDirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root can search a mode 000 directory")
	}

	dir := t.TempDir()
	secret := filepath.Join(dir, "secret")
	if err := os.Mkdir(secret, 0o755); err != nil {
		t.Fatal(err)
	}
	note := filepath.Join(secret, "note")
	if err := os.WriteFile(note, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(secret, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(secret, 0o755) })

	_, err := _Run(t, `
def main():
    file.exists("`+note+`")
    return "reached"
`)
	if err == nil {
		t.Fatal("exists on an unsearchable path returned instead of refusing")
	}
	if !errors.Is(err, os.ErrPermission) {
		t.Fatalf("got %v, want a permission refusal", err)
	}
}

func TestScript_CRCAndCopyAndSet(t *testing.T) {
	value, err := _Run(t, `
def main():
    wide = "refused"
    caught = ""
    # A failing builtin is a failing script, so the wide seed is its own run.

    doc = {"a": [1]}
    out = patch_json(doc, [{"op": "copy", "from": "/a", "path": "/b"}])
    out["b"].append(2)

    def slow(v):
        sleep(0.05)
        return "from-update"

    def writer():
        sleep(0.01)
        state.set("n", "from-set")

    handle = spawn(writer)
    state.update("n", slow)
    join(handle)
    return [doc["a"], state.get("n"), crc32(b"abc", 4294967295) != crc32(b"abc", 0)]
`)
	if err != nil {
		t.Fatal(err)
	}
	if value.String() != `[[1], "from-set", True]` {
		t.Fatalf("got %s", value.String())
	}

	_, err = _Run(t, `
def main():
    return crc32(b"abc", 4294967296)
`)
	if err == nil {
		t.Fatal("a 2**32 seed was accepted")
	}
}

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
	if len(got) != 1600 || !_SlicesEqual(got, want) {
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

func _SlicesEqual(left []int, right []int) bool {
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

func TestPatch_MovesInsideOneList(t *testing.T) {
	cases := []struct{ name, expression, want string }{
		{
			"low index to a high one",
			`patch_json({"a": ["a", "b", "c", "d"]}, [{"op": "move", "from": "/a/0", "path": "/a/3"}])`,
			`{"a": ["b", "c", "d", "a"]}`,
		},
		{
			"high index to a low one",
			`patch_json({"a": ["a", "b", "c", "d"]}, [{"op": "move", "from": "/a/3", "path": "/a/0"}])`,
			`{"a": ["d", "a", "b", "c"]}`,
		},
		{
			"add at the length",
			`patch_json({"a": ["x"]}, [{"op": "add", "path": "/a/1", "value": "y"}])`,
			`{"a": ["x", "y"]}`,
		},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			value, err := _Run(t, "def main():\n    return "+item.expression+"\n")
			if err != nil {
				t.Fatal(err)
			}
			if value.String() != item.want {
				t.Fatalf("got %s, want %s", value.String(), item.want)
			}
		})
	}

	_, err := _Run(t, `
def main():
    return patch_json({"a": ["x"]}, [{"op": "add", "path": "/a/2", "value": "y"}])
`)
	if err == nil {
		t.Fatal("add one past the end was accepted")
	}

	_, err = _Run(t, `
def main():
    return patch_json({"a": ["x"]}, [{"op": "replace", "path": "/a/-", "value": "y"}])
`)
	if err == nil {
		t.Fatal("replace of - was accepted")
	}
}

func TestNumbers_AtTheEdge(t *testing.T) {
	value, err := _Run(t, `
def main():
    huge = 1208925819614629174706176
    return [math.isnan(math.log(-1)), math.isinf(math.log(0)), math.ceil(huge) == huge, math.floor(huge) == huge]
`)
	if err != nil {
		t.Fatal(err)
	}
	if value.String() != "[True, True, True, True]" {
		t.Fatalf("got %s", value.String())
	}

	started := time.Now()
	value, err = _Run(t, `
def main():
    sleep(1e-10)
    return "done"
`)
	if err != nil {
		t.Fatal(err)
	}
	if value.String() != `"done"` {
		t.Fatalf("sleep: %s", value.String())
	}
	if time.Since(started) > time.Second {
		t.Fatalf("sleep(1e-10) took %s", time.Since(started))
	}

	wins, timeouts := 0, 0
	script := `
def ok():
    return 1

def main():
    return timeout(0, ok)
`
	for range 20 {
		_, err := _Run(t, script)
		if err == nil {
			wins++
			continue
		}
		timeouts++
	}
	t.Logf("timeout(0): %d returned 1, %d timed out", wins, timeouts)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	started = time.Now()
	_, err = _RunCtx(t, ctx, `
def step():
    return 1

def main():
    return repeat(1099511627776, step)
`)
	elapsed := time.Since(started)
	if err == nil {
		t.Fatal("repeat of 2**40 returned a value")
	}
	if elapsed > 500*time.Millisecond {
		t.Fatalf("repeat of 2**40 ran for %s before %v", elapsed, err)
	}
	t.Logf("repeat(2**40) after %s: %v", elapsed, err)
}

func TestHMAC_EmptyAndLongKeyMatchGo(t *testing.T) {
	empty := _GoHMAC("")
	long := _GoHMAC(strings.Repeat("k", 80))
	value, err := _Run(t, fmt.Sprintf(`
def main():
    return [hash.hmac("sha256", "", "abc"), hash.hmac("sha256", %q, "abc")]
`, strings.Repeat("k", 80)))
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf(`["%s", "%s"]`, empty, long)
	if value.String() != want {
		t.Fatalf("got %s, want %s", value.String(), want)
	}
}

func TestBase64_RefusesTheOtherAlphabet(t *testing.T) {
	raw := []byte{0xfb, 0xef, 0xff}
	standard := base64.StdEncoding.EncodeToString(raw)
	url := base64.RawURLEncoding.EncodeToString(raw)
	if standard == url {
		t.Fatalf("picked bytes whose alphabets match: %s", standard)
	}

	_, err := _Run(t, fmt.Sprintf(`
def main():
    return base64.decode(%q)
`, url))
	if !errors.Is(err, larkbase64.ErrEncoded) {
		t.Fatalf("standard decode of url text: %v", err)
	}

	_, err = _Run(t, fmt.Sprintf(`
def main():
    return base64.urldecode(%q)
`, standard))
	if !errors.Is(err, larkbase64.ErrEncoded) {
		t.Fatalf("url decode of standard text: %v", err)
	}

	padded := base64.URLEncoding.EncodeToString([]byte("a"))
	value, err := _Run(t, fmt.Sprintf(`
def main():
    return base64.urldecode(%q)
`, padded))
	if err != nil {
		t.Fatal(err)
	}
	if value.String() != `b"a"` {
		t.Fatalf("padded url decode: %s", value.String())
	}
}

func TestPath_JoinClimbsAndDirectoriesList(t *testing.T) {
	value, err := _Run(t, `
def main():
    return path.join("/tmp", "..", "etc")
`)
	if err != nil {
		t.Fatal(err)
	}
	if value.String() != `"/etc"` {
		t.Fatalf("join climbed to %s", value.String())
	}

	dir := t.TempDir()
	value, err = _Run(t, fmt.Sprintf(`
def main():
    return [file.size(%q), file.list(%q)]
`, dir, dir))
	if err != nil {
		t.Fatal(err)
	}
	text := value.String()
	if !strings.HasSuffix(text, ", []]") && !strings.Contains(text, ", []") {
		t.Fatalf("empty directory: %s", text)
	}

	held := filepath.Join(dir, "held")
	if err := os.Mkdir(held, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(held, "a.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "link")
	if err := os.Symlink(held, link); err != nil {
		t.Fatal(err)
	}
	value, err = _Run(t, fmt.Sprintf(`
def main():
    return file.list(%q)
`, link))
	if err != nil {
		t.Fatal(err)
	}
	if value.String() != `["a.txt"]` {
		t.Fatalf("list through a symlink: %s", value.String())
	}
}

func _GoHMAC(key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte("abc"))
	return hex.EncodeToString(mac.Sum(nil))
}
