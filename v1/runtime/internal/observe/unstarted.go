package observe

import (
	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/artifact"
)

// Written is the spawns of art a run that ended with status may have been
// kept from making, or nil.
//
// A run that succeeded was kept from nothing: a spawn it did not make sat in a
// branch the script did not take. So only a run that failed or was cancelled
// asks the artifact, and a run that succeeded pays nothing for it.
//
// Revisions:
//   - 2026-11-05 00:27: initial creation (C45, Q18)
//   - 2026-11-10 00:25: renamed from _Written, as a test in tests/ needs it (C120a, Q314)
func Written(
	art *artifact.Artifact,
	status workflowpb.Status,
) []common.Spawn {
	if status == workflowpb.Status_STATUS_SUCCEEDED {
		return nil
	}

	return art.Spawns()
}

// _Prevented is the sites among sites that a stopped run did not make: the
// spawns written in a function the run called, of a function the run did not.
// has says whether the run called a function.
//
// One site once, in the order given. It reads the run as it stands and adds
// nothing, so a function it names whose own spawns are written elsewhere does
// not bring those spawns in: only functions that ran are scanned. The recorder
// and the ledger both ask it, which is what keeps their graphs equal.
//
// Revisions:
//   - 2026-11-05 00:27: initial creation (C45, Q18)
func _Prevented(
	sites []common.Spawn,
	has func(name string) bool,
) []common.Spawn {
	var (
		prevented []common.Spawn
		seen      = make(map[common.Spawn]struct{})
	)

	for _, site := range sites {
		if !has(site.Caller) || has(site.Target) {
			continue
		}

		_, again := seen[site]
		if again {
			continue
		}

		seen[site] = struct{}{}
		prevented = append(
			prevented,
			site,
		)
	}

	return prevented
}
