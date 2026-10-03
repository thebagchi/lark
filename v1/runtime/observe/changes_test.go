package observe_test

import (
	"encoding/json"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"

	"google.golang.org/protobuf/encoding/protojson"

	patchpb "github.com/thebagchi/lark/proto/gen/patch"
	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/artifact"
	"github.com/thebagchi/lark/v1/runtime/observe"
)

const (
	// WATCHED is a run wide enough to change from several goroutines at once,
	// with a retry so one function reaches running more than once.
	WATCHED = "testdata/watched.star"

	// REMOVE is the one operation a change makes that takes something away,
	// and APPEND the index RFC 6901 gives the end of an array.
	REMOVE = "remove"
	APPEND = "-"
)

// TestChanges_PatchAnEmptyGraphIntoTheRunsOwn checks what a user interface
// does: start from {}, apply every change a started run sends, in order, and
// hold exactly the graph Status answers with once the run is over.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
func TestChanges_PatchAnEmptyGraphIntoTheRunsOwn(t *testing.T) {
	var (
		guard   sync.Mutex
		changes []*workflowpb.Change
	)

	watch := func(change *workflowpb.Change) {
		guard.Lock()
		defer guard.Unlock()

		changes = append(changes, change)
	}

	run := observe.Start(t.Context(), _Compile(t, WATCHED), artifact.WithChanges(watch))

	_, err := run.Wait()
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	guard.Lock()
	defer guard.Unlock()

	var held any = map[string]any{}

	for _, change := range changes {
		held = _Folded(t, held, change)
	}

	want := _Decoded(t, run.Status())
	if !reflect.DeepEqual(held, want) {
		t.Fatalf("patched into\n%v\nwant\n%v", held, want)
	}
}

// TestStatus_FromInsideAChangeIsTheGraphItMade checks that a watcher asking for
// the whole graph while it is told a change is answered with the graph that
// change made: no later step has folded in, and nothing it made is missing.
//
// The last change is the one that can go wrong. It says the run ended, a moment
// before the run closes Done, and a graph that waited for Done would still say
// running.
//
// Revisions:
//   - 2026-10-02 16:45: initial creation
func TestStatus_FromInsideAChangeIsTheGraphItMade(t *testing.T) {
	var (
		run     *observe.Execution
		ready   = make(chan struct{})
		guard   sync.Mutex
		changes []*workflowpb.Change
		asked   []*workflowpb.Graph
	)

	watch := func(change *workflowpb.Change) {
		guard.Lock()
		defer guard.Unlock()

		changes = append(changes, change)

		// The first is told inside Start, before there is a run to ask.
		if len(changes) == 1 {
			asked = append(asked, nil)

			return
		}

		<-ready

		asked = append(asked, run.Status())
	}

	run = observe.Start(t.Context(), _Compile(t, WATCHED), artifact.WithChanges(watch))
	close(ready)

	_, err := run.Wait()
	if err != nil {
		t.Fatalf("run: %v", err)
	}

	guard.Lock()
	defer guard.Unlock()

	var held any = map[string]any{}

	for idx, change := range changes {
		held = _Folded(t, held, change)

		if asked[idx] == nil {
			continue
		}

		want := _Decoded(t, asked[idx])
		if !reflect.DeepEqual(held, want) {
			t.Fatalf("change %d patched into\n%v\nStatus said\n%v", idx, held, want)
		}
	}
}

// _Folded is doc with every operation of change applied, in order.
//
// Revisions:
//   - 2026-10-02 16:45: initial creation
func _Folded(t *testing.T, doc any, change *workflowpb.Change) any {
	t.Helper()

	for _, op := range change.GetOperations() {
		parts := strings.Split(op.GetPath(), "/")[1:]
		doc = _Applied(t, doc, parts, op)
	}

	return doc
}

// _Decoded is graph as a JSON decoder holds the JSON protojson writes for it,
// which is what a patched copy is compared with.
//
// Revisions:
//   - 2026-10-02 16:45: initial creation
func _Decoded(t *testing.T, graph *workflowpb.Graph) any {
	t.Helper()

	raw, err := protojson.Marshal(graph)
	if err != nil {
		t.Fatal(err)
	}

	var decoded any

	err = json.Unmarshal(raw, &decoded)
	if err != nil {
		t.Fatal(err)
	}

	return decoded
}

// _Applied is doc with op applied at the pointer parts, as a JSON Patch library
// applies one: an add sets a member or inserts into a list, and a remove takes
// one away.
//
// Revisions:
//   - 2026-10-02 16:33: initial creation
func _Applied(t *testing.T, doc any, parts []string, op *patchpb.Operation) any {
	t.Helper()

	last := len(parts) == 1

	switch held := doc.(type) {
	case map[string]any:
		if !last {
			held[parts[0]] = _Applied(t, held[parts[0]], parts[1:], op)

			return held
		}

		if op.GetOp() == REMOVE {
			delete(held, parts[0])

			return held
		}

		held[parts[0]] = op.GetValue().AsInterface()

		return held
	case []any:
		if last && parts[0] == APPEND {
			return append(held, op.GetValue().AsInterface())
		}

		idx, err := strconv.Atoi(parts[0])
		if err != nil {
			t.Fatalf("path %s: %v", op.GetPath(), err)
		}

		if !last {
			held[idx] = _Applied(t, held[idx], parts[1:], op)

			return held
		}

		if op.GetOp() == REMOVE {
			return slices.Delete(held, idx, idx+1)
		}

		return slices.Insert(held, idx, op.GetValue().AsInterface())
	}

	t.Fatalf("path %s: nothing to apply it to at %s", op.GetPath(), parts[0])

	return nil
}
