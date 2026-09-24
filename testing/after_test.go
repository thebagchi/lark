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
	"strings"
	"testing"
	"time"

	"go.starlark.net/starlark"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/runtime"
	"github.com/thebagchi/lark/runtime/graph"
	larkbase64 "github.com/thebagchi/lark/runtime/plugin/base64"
	_ "github.com/thebagchi/lark/runtime/plugin/flow"
	_ "github.com/thebagchi/lark/runtime/plugin/hash"
	_ "github.com/thebagchi/lark/runtime/plugin/math"
	_ "github.com/thebagchi/lark/runtime/plugin/path"
)

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

func TestJoin_UnderTheLockIsRefused(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	started := time.Now()
	_, err := _RunCtx(t, ctx, `
def rival():
    sleep(0.05)
    return state.update("k", lambda v: "rival")

def main():
    handle = spawn(rival)
    return state.update("k", lambda v: join(handle))
`)
	if time.Since(started) > 500*time.Millisecond {
		t.Fatalf("join under the lock ran for %s: %v", time.Since(started), err)
	}
	if !errors.Is(err, runtime.ErrNested) {
		t.Fatalf("got %v, want ErrNested", err)
	}
	if !strings.Contains(err.Error(), "join") {
		t.Fatalf("want the refusal at join, got %v", err)
	}
}

func TestSpawn_InsideAnUpdateCarriesTheBan(t *testing.T) {
	_, err := _Run(t, `
def child():
    sleep(0.05)
    state.set("b", 1)

def main():
    found = []
    def change(v):
        found.append(spawn(child))
        return "done"
    state.update("a", change)
    return join(found[0])
`)
	if !errors.Is(err, runtime.ErrNested) {
		t.Fatalf("child set after the update: %v", err)
	}
	if !strings.Contains(err.Error(), "started inside an update") {
		t.Fatalf("got %v", err)
	}
}

func TestUpdate_ReturnIsFrozenAndGetIsACopy(t *testing.T) {
	_, err := _Run(t, `
def main():
    def change(v):
        return [1]
    made = state.update("k", change)
    made.append(2)
    return made
`)
	if err == nil {
		t.Fatal("append to the value update returned was accepted")
	}

	value, err := _Run(t, `
def main():
    def change(v):
        return [1]
    state.update("k", change)
    copied = state.get("k")
    copied.append(2)
    return [copied, state.get("k")]
`)
	if err != nil {
		t.Fatal(err)
	}
	if value.String() != "[[1, 2], [1]]" {
		t.Fatalf("get copy: %s", value.String())
	}
}

func TestCheck_ASpineForkingItself(t *testing.T) {
	call := &workflowpb.Step{
		Action: &workflowpb.Step_Call{Call: &workflowpb.Call{Function: graph.ENTRY}},
	}
	fork := &workflowpb.Step{
		Action: &workflowpb.Step_Fork{Fork: &workflowpb.Fork{Thread: graph.SPINE}},
	}
	built := &workflowpb.Graph{
		Threads: []*workflowpb.Thread{{
			Id: graph.SPINE,
			State: &workflowpb.Thread_Static{Static: &workflowpb.Static{
				Steps: []*workflowpb.Step{call, fork},
			}},
		}},
	}
	fork.GetFork().Thread = "thread_1"
	if err := graph.Check(built); err != nil {
		t.Fatalf("spine forking thread_1: %v", err)
	}

	fork.GetFork().Thread = "thread_1_1"
	if !errors.Is(graph.Check(built), graph.ErrParentage) {
		t.Fatal("a grandchild id under the spine was not refused")
	}

	fork.GetFork().Thread = graph.SPINE
	if !errors.Is(graph.Check(built), graph.ErrParentage) {
		t.Fatalf("spine forking itself: %v", graph.Check(built))
	}
}

func TestCheck_ANestedOrdinalIsADecimal(t *testing.T) {
	if err := graph.Check(_Forking("thread_1", "thread_1_2")); err != nil {
		t.Fatalf("thread_1 forking thread_1_2: %v", err)
	}
	if !errors.Is(graph.Check(_Forking("thread_1", "thread_1_02")), graph.ErrParentage) {
		t.Fatalf("thread_1 forking thread_1_02: %v", graph.Check(_Forking("thread_1", "thread_1_02")))
	}
}

func TestCheck_AnOrdinalIsADecimal(t *testing.T) {
	for _, id := range []string{"thread_1", "thread_10"} {
		if err := graph.Check(_SpineForking(id)); err != nil {
			t.Fatalf("spine forking %s: %v", id, err)
		}
	}

	for _, id := range []string{"thread_01", "thread_00", "thread_1a"} {
		if !errors.Is(graph.Check(_SpineForking(id)), graph.ErrParentage) {
			t.Fatalf("spine forking %s: %v", id, graph.Check(_SpineForking(id)))
		}
	}
}

func TestCheck_AnEmptyOrdinalIsNotAChild(t *testing.T) {
	for _, id := range []string{"thread_", "thread_01", "thread_00", "thread_1a"} {
		t.Logf("spine forking %q -> %v", id, graph.Check(_SpineForking(id)))
	}

	if !errors.Is(graph.Check(_SpineForking("thread_")), graph.ErrParentage) {
		t.Fatalf("spine forking thread_: %v", graph.Check(_SpineForking("thread_")))
	}
}

func _SpineForking(id string) *workflowpb.Graph {
	return _Forking(graph.SPINE, id)
}

func _Forking(parent string, id string) *workflowpb.Graph {
	return &workflowpb.Graph{
		Threads: []*workflowpb.Thread{{
			Id: parent,
			State: &workflowpb.Thread_Static{Static: &workflowpb.Static{
				Steps: []*workflowpb.Step{
					{Action: &workflowpb.Step_Call{Call: &workflowpb.Call{Function: graph.ENTRY}}},
					{Action: &workflowpb.Step_Fork{Fork: &workflowpb.Fork{Thread: id}}},
				},
			}},
		}},
	}
}

func _GoHMAC(key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte("abc"))
	return hex.EncodeToString(mac.Sum(nil))
}

func _RunCtx(t *testing.T, ctx context.Context, src string) (starlark.Value, error) {
	t.Helper()
	built, err := runtime.NewCompiler().Compile("after.star", []byte(src))
	if err != nil {
		return nil, err
	}
	return built.Run(ctx)
}
