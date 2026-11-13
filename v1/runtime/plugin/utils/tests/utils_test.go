package tests

import (
	"regexp"
	"strings"
	"testing"

	"go.starlark.net/starlark"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin"
	_ "github.com/thebagchi/lark/v1/runtime/plugin/utils"
)

const (
	SCRIPT = "utils_test.star"

	// CALLED is the one function; ARGUED passes it what it does not take.
	//
	// Revisions:
	//   - 2026-09-21 23:56: initial creation
	//   - 2026-11-06 22:37: revision history added (C90)
	//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
	CALLED = `utils.datetime()`
	ARGUED = `utils.datetime(1)`
)

// STAMP is the brief's YYYY-MM-DD HH:MM:SS.uuuuuu, and nothing else.
//
// Revisions:
//   - 2026-09-21 23:56: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
var (
	STAMP = regexp.MustCompile(`^\d{4}-\d\d-\d\d \d\d:\d\d:\d\d\.\d{6}$`)
)

// _Eval runs one Starlark expression against the plugin environment.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation
//   - 2026-11-08 00:57: reads the moved names from common (Q244, C91w12d)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Eval(
	t *testing.T,
	expression string,
) (starlark.Value, error) {
	t.Helper()

	env, err := plugin.Environment(plugin.Registered())
	if err != nil {
		t.Fatalf(
			"environment: %v",
			err,
		)
	}

	return starlark.EvalOptions(
		common.OPTIONS,
		&starlark.Thread{
			Name: SCRIPT,
		},
		SCRIPT,
		expression,
		env,
	)
}

// TestDatetime_ReturnsTheBriefsShape is the whole of section 10.8 as this
// runtime spells it: the format, given back rather than printed.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
//   - 2026-09-21 15:25: reads the returned string rather than a printed line
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestDatetime_ReturnsTheBriefsShape(t *testing.T) {
	value, err := _Eval(
		t,
		CALLED,
	)
	if err != nil {
		t.Fatal(err)
	}

	text, ok := value.(starlark.String)
	if !ok {
		t.Fatalf(
			"want a string, got %s",
			value.Type(),
		)
	}

	if !STAMP.MatchString(string(text)) {
		t.Fatalf(
			"want YYYY-MM-DD HH:MM:SS.uuuuuu, got %q",
			string(text),
		)
	}
}

// TestDatetime_IsUsableAsAValue is why it returns rather than prints: the text
// goes wherever the script puts it.
//
// Revisions:
//   - 2026-09-21 15:25: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestDatetime_IsUsableAsAValue(t *testing.T) {
	value, err := _Eval(
		t,
		`"run-" + utils.datetime()`,
	)
	if err != nil {
		t.Fatal(err)
	}

	text, ok := value.(starlark.String)
	if !ok {
		t.Fatalf(
			"want a string, got %s",
			value.Type(),
		)
	}

	if !strings.HasPrefix(
		string(text),
		"run-",
	) {
		t.Fatalf(
			"want the text joined to another, got %q",
			string(text),
		)
	}
}

// TestDatetime_TakesNoArguments records that the brief gives it none.
//
// Revisions:
//   - 2026-09-21 10:35: initial creation
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestDatetime_TakesNoArguments(t *testing.T) {
	_, err := _Eval(
		t,
		ARGUED,
	)
	if err == nil || !strings.Contains(
		err.Error(),
		DATETIME_NAME,
	) {
		t.Fatalf(
			"want a refusal naming the function, got %v",
			err,
		)
	}
}

// DATETIME_NAME is how the refusal names it, module and member.
//
// Revisions:
//   - 2026-09-21 23:56: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
const (
	DATETIME_NAME = "utils.datetime"
)
