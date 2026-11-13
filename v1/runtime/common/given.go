package common

import (
	"log/slog"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
)

// RunOption sets one thing on a run of an artifact: the arguments its script
// is handed, the logger what it prints goes to, or where its changes go.
//
// An option rather than a parameter of Run, so a run can be told more without
// every caller changing. What travels on the context is only when to stop.
//
// Revisions:
//   - 2026-10-04 01:25: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 23:04: moved from artifact (C91w6, Q120)
type RunOption func(given *Given)

// Given is what a run's options told it: the arguments its script is supplied
// with (Args), the logger what it prints goes to (Logger), the function its
// graph's changes go to (Watch), where its lines are reported (Into), whether
// its graph is kept live (Live), and whether the memory watch is on (Memory),
// each zero when nothing was. Refused is why an option could not be taken: an
// argument no JSON holds, or an option given twice. An option cannot return it,
// so the run does, before anything runs.
//
// Exported fields, because the package observe reads the logger and the
// watcher before a run starts and adds what watches it. It applies the options
// once, with artifact.Apply, and hands the result to artifact.Perform. Make
// one only through those, and set a field only in an option: a field set
// anywhere else is a setting no other run of the options would see. A nil
// *Given has no fields to read, so a caller checks it before it reads one.
//
// Revisions:
//   - 2026-10-04 01:25: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-07 19:16: uses the names moved to common or unexported (C91w4)
//   - 2026-11-07 23:04: moved from artifact; the fields are exported and the
//     accessors Changes, Live and Logger are field reads (C91w6, Q120)
//   - 2026-11-08 12:50: Refused holds every option refusal, not the arguments' alone (C75, Q256)
//   - 2026-11-10 17:15: Refused no longer names WithLog beside WithLogger, which is dropped (C121,
//     Q340)
//   - 2026-11-13 12:48: Args is a workflow.Map, the lark values, in place of a
//     google.protobuf.Struct (C132, Q437)
type Given struct {
	Args    *workflowpb.Map
	Refused error
	Logger  *slog.Logger
	Watch   func(change *workflowpb.Change)
	Into    Reporter
	Live    bool
	Memory  *MemoryWatch
}
