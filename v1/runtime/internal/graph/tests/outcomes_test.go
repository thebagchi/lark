package tests

import (
	"regexp"
	"strings"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/internal/graph"
)

const (
	// OUTCOMES_SCRIPT calls outcomes from main and from a helper, and prints
	// what it returns, so a round trip has something to compare.
	//
	// Revisions:
	//   - 2026-11-02 17:03: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	OUTCOMES_SCRIPT = `
def good():
    return 7

def bad():
    fail("boom")

def tally(handles):
    rs = outcomes(*handles)
    return [r["kind"] for r in rs]

def main():
    a = spawn(bad)
    b = spawn(good)
    rs = outcomes(a, b)
    print(rs[0]["ok"], rs[0]["kind"], rs[0]["error"])
    print(rs[1]["ok"], rs[1]["value"])
    print(tally([spawn(good)]))
`

	// OUTCOMES_NAME is the entry the script is derived under.
	//
	// Revisions:
	//   - 2026-11-02 17:03: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
	OUTCOMES_NAME = "outcomes.star"
)

// TestOutcomes_TheFlowKeepsAScriptThatUsesItAsText proves derive keeps a main
// that calls outcomes as text (no proto change, decision of 2026-11-02), that
// the flow passes Check, and that the round trip keeps the call and behaves
// the same.
//
// Starting condition: no plugins beyond the defaults.
// Success: the flow has no main statements and its text holds the call; the
// regenerated script holds the call, prints what the original printed, and a
// second pass changes nothing.
//
// Revisions:
//   - 2026-11-02 16:29: initial creation (C28)
//   - 2026-11-04 06:39: masks the line of a position, which the
//     regenerated script lays out differently (C69)
//   - 2026-11-04 18:12: the position is file:line, with no column (Q89)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestOutcomes_TheFlowKeepsAScriptThatUsesItAsText(t *testing.T) {
	var (
		src       = []byte(OUTCOMES_SCRIPT)
		flow, err = graph.Of(&common.Source{
			Entry: OUTCOMES_NAME,
			Text:  src,
		})
	)
	if err != nil {
		t.Fatalf(
			"derive: %v",
			err,
		)
	}

	if flow.GetMain() != nil {
		t.Fatalf(
			"main is modelled as statements, want text: %v",
			flow,
		)
	}

	if !strings.Contains(
		flow.GetText(),
		"outcomes(a, b)",
	) {
		t.Fatalf(
			"the text does not keep the call: %q",
			flow.GetText(),
		)
	}

	err = graph.Check(flow)
	if err != nil {
		t.Fatalf(
			"check: %v",
			err,
		)
	}

	once, err := graph.Emit(flow)
	if err != nil {
		t.Fatalf(
			"emit: %v",
			err,
		)
	}

	if !strings.Contains(
		string(once),
		"outcomes(a, b)",
	) {
		t.Fatalf(
			"the regenerated script lost the call:\n%s",
			once,
		)
	}

	var (
		was = _Run(
			t,
			OUTCOMES_NAME,
			src,
		)
		now = _Run(
			t,
			OUTCOMES_NAME,
			once,
		)
	)

	// A position names a line of the script that ran, and the regenerated
	// script lays its lines out differently, so only the file is compared.
	var (
		place   = regexp.MustCompile(`:\d+: `)
		wasText = place.ReplaceAllString(
			was.printed,
			": ",
		)
		nowText = place.ReplaceAllString(
			now.printed,
			": ",
		)
	)

	if was.printed == "" || wasText != nowText ||
		was.failure != now.failure {
		t.Fatalf(
			"was %q (%s), now %q (%s)",
			was.printed,
			was.failure,
			now.printed,
			now.failure,
		)
	}

	twice := _Emitted(
		t,
		once,
		OUTCOMES_NAME,
	)
	if string(once) != string(twice) {
		t.Fatalf(
			"a second pass differs\n--- once\n%s\n--- twice\n%s",
			once,
			twice,
		)
	}
}
