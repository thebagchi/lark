package graph_test

import (
	"errors"
	"strings"
	"testing"

	"google.golang.org/protobuf/proto"

	workflowpb "github.com/thebagchi/lark/proto/gen/workflow"
	"github.com/thebagchi/lark/v1/runtime/graph"
)

// DECLARING is a script declaring every shape an argument can take, with the
// two names disagreeing on purpose: what a caller passes is "host", what the
// script calls it is "db".
const DECLARING = `
db = arg("host", "db.internal")
port = arg("port", 5432)
ratio = arg("ratio", 0.25)
tls = arg("tls", True)
spare = arg("spare", None)
tags = arg("tags", ["a", "b"])
labels = arg("labels", {"tier": "gold"})
keyword = arg("keyword", default = "set either way round")
token = arg("token")
limit = 3

def main():
    print(db)
`

// SHADOWING is a script defining a function of its own called arg, which is
// what it then calls.
const SHADOWING = `
def arg():
    return "mine"

x = arg()

def main():
    print(x)
`

// DECLARED is how many arguments DECLARING declares. The tenth binding is a
// constant, which is the point of it being there.
const DECLARED = 9

// TestOf_AnArgumentCarriesBothNames is the answer to the question the shape
// turned on: a script binds an argument to a name of its own choosing, so the
// graph carries the bound name as its key and the supplied name as a field.
//
// Revisions:
//   - 2026-09-22 22:48: initial creation
func TestOf_AnArgumentCarriesBothNames(t *testing.T) {
	args := _Derived(t, DECLARING).Graph.GetArgs()

	if len(args) != DECLARED {
		t.Fatalf("derived %d arguments, want %d", len(args), DECLARED)
	}

	if args["db"].GetName() != "host" {
		t.Fatalf("db is supplied as %q, want host", args["db"].GetName())
	}
}

// TestOf_AnArgumentWithNoDefaultCarriesNone checks that a run-must-supply
// argument carries nothing rather than null, since a script asking for None is
// a different declaration.
//
// Revisions:
//   - 2026-09-22 22:48: initial creation
func TestOf_AnArgumentWithNoDefaultCarriesNone(t *testing.T) {
	args := _Derived(t, DECLARING).Graph.GetArgs()

	if args["token"].GetDefault() != nil {
		t.Fatalf("token defaults to %v, want nothing", args["token"].GetDefault())
	}

	if args["spare"].GetDefault() == nil {
		t.Fatal("spare carries no default, want null")
	}
}

// TestOf_AConstantIsNotAnArgument checks that a module-level binding which is
// not an arg() call is still carried as a constant.
//
// Revisions:
//   - 2026-09-22 22:48: initial creation
func TestOf_AConstantIsNotAnArgument(t *testing.T) {
	derived := _Derived(t, DECLARING).Graph

	if derived.GetConstants()["limit"].GetValue().GetNumberValue() != 3 {
		t.Fatalf("limit was carried as %v, want the constant 3", derived.GetConstants()["limit"])
	}

	if derived.GetArgs()["limit"] != nil {
		t.Fatal("limit was carried as an argument, want a constant")
	}
}

// TestOf_AMalformedDeclarationIsRefused checks that an arg() call a graph
// cannot carry is refused as a broken declaration rather than passed on as a
// constant that happens to fail, which would send a reader looking for the
// wrong thing.
//
// Revisions:
//   - 2026-09-22 22:48: initial creation
func TestOf_AMalformedDeclarationIsRefused(t *testing.T) {
	cases := map[string]string{
		"no name":     "x = arg()\n",
		"too many":    "x = arg(\"a\", \"b\", \"c\")\n",
		"computed":    "name = \"host\"\nx = arg(name)\n",
		"not a value": "x = arg(\"a\", [1, 2][0])\n",
	}

	for name, declaring := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := graph.Of([]byte(declaring+"\ndef main():\n    pass\n"), SOURCED, nil)
			if !errors.Is(err, graph.ErrNotCarried) {
				t.Fatalf("got %v, want ErrNotCarried", err)
			}
		})
	}
}

// TestEmit_DeclarationsSurviveTheRoundTrip is what the two directions owe each
// other: every shape a declaration can take goes out as Starlark and comes
// back as the argument it was.
//
// Revisions:
//   - 2026-09-22 22:48: initial creation
func TestEmit_DeclarationsSurviveTheRoundTrip(t *testing.T) {
	first := _Derived(t, DECLARING).Graph

	out, err := graph.Emit(first)
	if err != nil {
		t.Fatalf("emit: %v", err)
	}

	again := _Derived(t, string(out)).Graph

	for bound, declared := range first.GetArgs() {
		if !proto.Equal(declared, again.GetArgs()[bound]) {
			t.Fatalf("%s went out as %v and came back as %v", bound, declared, again.GetArgs()[bound])
		}
	}

	if len(again.GetArgs()) != len(first.GetArgs()) {
		t.Fatalf("emitted %d arguments and read back %d", len(first.GetArgs()), len(again.GetArgs()))
	}
}

// TestEmit_WritesTheDeclarationAScriptWrote checks the generated line itself,
// since a round trip would pass on two functions that agreed with each other
// and not with Starlark.
//
// Revisions:
//   - 2026-09-22 22:48: initial creation
func TestEmit_WritesTheDeclarationAScriptWrote(t *testing.T) {
	out, err := graph.Emit(&workflowpb.Graph{
		Functions: []*workflowpb.Function{{Name: "main", Body: "print(db)"}},
		Args: map[string]*workflowpb.Arg{
			"db":    {Name: "host", Default: _Text("db.internal")},
			"token": {Name: "token"},
		},
	})
	if err != nil {
		t.Fatalf("emit: %v", err)
	}

	for _, want := range []string{`db = arg("host", "db.internal")`, `token = arg("token")`} {
		if !strings.Contains(string(out), want) {
			t.Fatalf("emitted:\n%s\nwant it to contain %q", string(out), want)
		}
	}
}

// TestOf_AScriptsOwnArgIsNotADeclaration checks that a script defining a
// function called arg is read as calling that function.
//
// A global shadows a predeclared name, so such a script runs its own arg and
// never reaches the builtin. A derivation that read the call as a declaration
// would refuse a script this runtime runs, which is the one thing the two
// forms may never disagree about.
//
// Revisions:
//   - 2026-09-22 23:12: initial creation
func TestOf_AScriptsOwnArgIsNotADeclaration(t *testing.T) {
	derived := _Derived(t, SHADOWING).Graph

	if len(derived.GetArgs()) != 0 {
		t.Fatalf("derived %v as arguments, want none", derived.GetArgs())
	}

	called := derived.GetConstants()["x"].GetCall()
	if called.GetFunction() != "arg" {
		t.Fatalf("x was carried as %v, want a call of the script's own arg", called)
	}

	out, err := graph.Emit(derived)
	if err != nil {
		t.Fatalf("emit: %v", err)
	}

	if !strings.Contains(string(out), "x = arg()") {
		t.Fatalf("emitted:\n%s\nwant it to contain x = arg()", string(out))
	}
}

// TestEmit_AHostileNameStaysAString pins the one place this feature puts text
// somebody else wrote into source that is then compiled and run.
//
// A graph arrives from a user interface, and emitting it writes each argument's
// supplied name and default into a generated script. A name carrying a quote
// and a newline would close the literal and open a statement, so it is written
// as a quoted literal and this checks that it stayed one - the payload appears
// in the output only as the string it is.
//
// Revisions:
//   - 2026-09-22 23:20: initial creation
func TestEmit_AHostileNameStaysAString(t *testing.T) {
	payload := "x\")\nprint(\"broken out\nevil = arg(\"y"

	out, err := graph.Emit(&workflowpb.Graph{
		Functions: []*workflowpb.Function{{Name: "main", Body: "print(taken)"}},
		Args:      map[string]*workflowpb.Arg{"taken": {Name: payload}},
	})
	if err != nil {
		t.Fatalf("emit: %v", err)
	}

	// The payload tries to close the literal and bind a second argument. What
	// the generated script actually declares is the test: one argument, named
	// by the whole payload, and no sign of the one it tried to smuggle in.
	again := _Derived(t, string(out)).Graph

	if len(again.GetArgs()) != 1 || again.GetConstants()["evil"] != nil {
		t.Fatalf("emitted:\n%s\nwhich declared %v", out, again.GetArgs())
	}

	if again.GetArgs()["taken"].GetName() != payload {
		t.Fatalf("came back as %q, want the payload unchanged", again.GetArgs()["taken"].GetName())
	}
}
