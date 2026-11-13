package tests

import (
	"errors"
	"io"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/structpb"

	pluginpb "github.com/thebagchi/lark/proto/gen/plugin"
	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/plugin/remote/internal/host"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
)

const (
	// LONG is the length of a name or a failure text a test makes, far past
	// the 100 bytes a message may echo (Q4).
	//
	// Revisions:
	//   - 2026-11-11 00:41: initial creation (C122, B269)
	LONG = 100000

	// SHORT is the most an error of the host may hold when the text it was
	// given is LONG: a few names cut to 100 bytes, their markers, and the
	// words round them.
	//
	// Revisions:
	//   - 2026-11-11 00:41: initial creation (C122, B269)
	SHORT = 2000

	// TOKEN is the secret a test listener is given.
	//
	// Revisions:
	//   - 2026-11-11 00:41: initial creation (C122, B269)
	TOKEN = "0123456789abcdef"
)

// _Scripted is a stream whose plugin announces name and names, and then
// answers every ask with the failure text, or with null when there is none.
//
// Revisions:
//   - 2026-11-11 00:41: initial creation (C122, B269)
type _Scripted struct {
	grpc.ServerStream
	name    string
	names   []string
	failed  string
	answers chan *pluginpb.Request
	started bool
}

// Recv announces the plugin once, then hands back each answer until answers
// closes, which ends the stream.
//
// Revisions:
//   - 2026-11-11 00:41: initial creation (C122, B269)
func (s *_Scripted) Recv() (*pluginpb.Request, error) {
	if !s.started {
		s.started = true

		return &pluginpb.Request{
			Of: &pluginpb.Request_Register{
				Register: &pluginpb.Register{
					Name:  s.name,
					Token: TOKEN,
					Names: s.names,
				},
			},
		}, nil
	}

	held, open := <-s.answers
	if !open {
		return nil, io.EOF
	}

	return held, nil
}

// Send turns each ask into an answer.
//
// Revisions:
//   - 2026-11-11 00:41: initial creation (C122, B269)
func (s *_Scripted) Send(sent *pluginpb.Response) error {
	ask := sent.GetAsk()
	if ask == nil {
		return nil
	}

	s.answers <- &pluginpb.Request{
		Of: &pluginpb.Request_Answer{
			Answer: &pluginpb.Answer{
				Id:     ask.GetId(),
				Result: structpb.NewNullValue(),
				Failed: s.failed,
			},
		},
	}

	return nil
}

// _Listening is a listener that closes when the test ends.
//
// Revisions:
//   - 2026-11-11 00:41: initial creation (C122, B269)
func _Listening(t *testing.T) *host.Listener {
	t.Helper()

	listener, err := host.Listen(
		filepath.Join(
			t.TempDir(),
			"s",
		),
		TOKEN,
	)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_ = listener.Close()
	})

	return listener
}

// _Joined registers a scripted plugin in the background. The channel it
// returns gets what Register returns, once it returns; the plugin stays until
// the test closes its answers.
//
// Revisions:
//   - 2026-11-11 00:41: initial creation (C122, B269)
func _Joined(
	t *testing.T,
	listener *host.Listener,
	stream *_Scripted,
) chan error {
	t.Helper()

	stream.answers = make(
		chan *pluginpb.Request,
		1,
	)

	done := make(
		chan error,
		1,
	)

	go func() {
		done <- listener.Register(stream)
	}()

	return done
}

// _Waited waits until count plugins are live.
//
// Revisions:
//   - 2026-11-11 00:41: initial creation (C122, B269)
func _Waited(
	t *testing.T,
	listener *host.Listener,
	count int,
) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)

	for len(listener.Live()) != count {
		if time.Now().After(deadline) {
			t.Fatalf(
				"%d plugins are live, want %d",
				len(listener.Live()),
				count,
			)
		}

		time.Sleep(time.Millisecond)
	}
}

// _Cut proves err is short and holds the marker of a cut text of LONG bytes.
//
// Revisions:
//   - 2026-11-11 00:41: initial creation (C122, B269)
func _Cut(
	t *testing.T,
	err error,
	marker string,
) {
	t.Helper()

	if err == nil {
		t.Fatal("no error")
	}

	if len(err.Error()) > SHORT || !strings.Contains(
		err.Error(),
		marker,
	) {
		t.Fatalf(
			"error is %d bytes, want at most %d with %q: %.300q",
			len(err.Error()),
			SHORT,
			marker,
			err,
		)
	}
}

// TestCall_CutsTheNameAndTheFailure proves a call that fails through a
// remote plugin cuts the builtin's name and the plugin's own failure text to
// 100 bytes in the error, and the builtin keeps its whole name (Q346, Q4,
// B269).
//
// Revisions:
//   - 2026-11-11 00:41: initial creation (C122, B269)
func TestCall_CutsTheNameAndTheFailure(t *testing.T) {
	var (
		member = strings.Repeat(
			"m",
			LONG,
		)
		failure = strings.Repeat(
			"f",
			LONG,
		)
		listener = _Listening(t)
		stream   = &_Scripted{
			name:   "p",
			names:  []string{"mod." + member},
			failed: failure,
		}
		done = _Joined(
			t,
			listener,
			stream,
		)
	)

	t.Cleanup(func() {
		close(stream.answers)
		<-done
	})

	_Waited(
		t,
		listener,
		1,
	)

	module, ok := listener.Plugins()[0].Values()["mod"].(*starlarkstruct.Module)
	if !ok {
		t.Fatal("the plugin supplies no mod module")
	}

	builtin, ok := module.Members[member].(*starlark.Builtin)
	if !ok {
		t.Fatal("the module holds no builtin of the long name")
	}

	if builtin.Name() != "mod."+member {
		t.Fatalf(
			"the builtin is called %d bytes long, want its whole name",
			len(builtin.Name()),
		)
	}

	_, err := starlark.Call(
		new(starlark.Thread),
		builtin,
		nil,
		nil,
	)
	if !errors.Is(
		err,
		host.ErrRemote,
	) {
		t.Fatalf(
			"got %v, want ErrRemote",
			err,
		)
	}

	_Cut(
		t,
		err,
		"... ("+strconv.Itoa(LONG)+" bytes)",
	)

	if strings.Count(
		err.Error(),
		"... (",
	) < 2 {
		t.Fatalf(
			"want the name and the failure both cut: %.300q",
			err,
		)
	}
}

// TestCall_CutsTheNameOfAPluginThatLeft proves the error of a call to a plugin
// that has gone cuts the builtin's name. The call reports the name where it
// offers the ask and where it waits for the answer, and either may be the one
// that sees the plugin gone (Q346, B269).
//
// Revisions:
//   - 2026-11-11 00:41: initial creation (C122, B269)
func TestCall_CutsTheNameOfAPluginThatLeft(t *testing.T) {
	var (
		member = strings.Repeat(
			"m",
			LONG,
		)
		listener = _Listening(t)
		stream   = &_Scripted{
			name:  "p",
			names: []string{"mod." + member},
		}
		done = _Joined(
			t,
			listener,
			stream,
		)
	)

	_Waited(
		t,
		listener,
		1,
	)

	module, ok := listener.Plugins()[0].Values()["mod"].(*starlarkstruct.Module)
	if !ok {
		t.Fatal("the plugin supplies no mod module")
	}

	close(stream.answers)
	<-done

	_Waited(
		t,
		listener,
		0,
	)

	for range 20 {
		_, err := starlark.Call(
			new(starlark.Thread),
			module.Members[member],
			nil,
			nil,
		)
		if !errors.Is(
			err,
			host.ErrGone,
		) {
			t.Fatalf(
				"got %v, want ErrGone",
				err,
			)
		}

		_Cut(
			t,
			err,
			"... ("+strconv.Itoa(LONG+len("mod."))+" bytes)",
		)
	}
}

// TestRegister_CutsThePluginAndItsNames proves the refusal of a second plugin
// of one name, and of a returning plugin that announces other names, cuts the
// plugin's name and the names it announced to 100 bytes (Q346, B269).
//
// Revisions:
//   - 2026-11-11 00:41: initial creation (C122, B269)
func TestRegister_CutsThePluginAndItsNames(t *testing.T) {
	var (
		name = strings.Repeat(
			"p",
			LONG,
		)
		first = strings.Repeat(
			"a",
			LONG,
		)
		second = strings.Repeat(
			"b",
			LONG,
		)
		listener = _Listening(t)
		one      = &_Scripted{
			name:  name,
			names: []string{first},
		}
		done = _Joined(
			t,
			listener,
			one,
		)
	)

	_Waited(
		t,
		listener,
		1,
	)

	var (
		again = &_Scripted{
			name:  name,
			names: []string{first},
		}
		err = <-_Joined(
			t,
			listener,
			again,
		)
	)
	if !errors.Is(
		err,
		host.ErrAttached,
	) {
		t.Fatalf(
			"got %v, want ErrAttached",
			err,
		)
	}

	_Cut(
		t,
		err,
		"... ("+strconv.Itoa(LONG)+" bytes)",
	)

	close(one.answers)
	<-done

	_Waited(
		t,
		listener,
		0,
	)

	other := &_Scripted{
		name:  name,
		names: []string{second},
	}

	err = <-_Joined(
		t,
		listener,
		other,
	)
	if !errors.Is(
		err,
		host.ErrRenamed,
	) {
		t.Fatalf(
			"got %v, want ErrRenamed",
			err,
		)
	}

	_Cut(
		t,
		err,
		"... ("+strconv.Itoa(LONG)+" bytes)",
	)
}

// TestListen_CutsTheAddressTheSystemRefused proves the error of a socket path
// that cannot be listened on holds the path cut to 100 bytes once, and not
// again inside the error of the system, which names the address whole (B269).
//
// Revisions:
//   - 2026-11-11 00:41: initial creation (C122, B269)
func TestListen_CutsTheAddressTheSystemRefused(t *testing.T) {
	var (
		part = strings.Repeat(
			"d",
			200,
		)
		socket = filepath.Join(
			t.TempDir(),
			part,
			part,
			part,
			part,
			part,
			part,
			part,
			part,
			part,
			part,
			part,
			part,
			part,
			part,
			part,
		)
	)

	_, err := host.Listen(
		socket,
		TOKEN,
	)
	if err == nil {
		t.Fatal("no error for a path too long to listen on")
	}

	if len(err.Error()) > SHORT || strings.Count(
		err.Error(),
		part,
	) > 2 {
		t.Fatalf(
			"error is %d bytes, want the address cut: %.400q",
			len(err.Error()),
			err,
		)
	}
}

// TestCheck_CutsThePositionAndTheFunctionName proves the refusal of a call
// that hands a function to a remote plugin cuts the name of the file the call
// is in, and the name of the function, to 100 bytes (Q4, B273).
//
// Revisions:
//   - 2026-11-11 06:41: initial creation (C122, B273)
func TestCheck_CutsThePositionAndTheFunctionName(t *testing.T) {
	var (
		long = strings.Repeat(
			"g",
			LONG,
		)
		listener = _Listening(t)
		stream   = &_Scripted{
			name:  "p",
			names: []string{"mod.f"},
		}
		done = _Joined(
			t,
			listener,
			stream,
		)
	)

	t.Cleanup(func() {
		close(stream.answers)
		<-done
	})

	_Waited(
		t,
		listener,
		1,
	)

	tree, err := common.OPTIONS.Parse(
		long+".star",
		"def "+long+"():\n    pass\n\nx = mod.f("+long+")\n",
		0,
	)
	if err != nil {
		t.Fatal(err)
	}

	checking, ok := listener.Plugins()[0].(common.Checking)
	if !ok {
		t.Fatal("the plugin checks nothing")
	}

	err = checking.Check(tree)
	if !errors.Is(
		err,
		unpack.ErrConvert,
	) {
		t.Fatalf(
			"got %v, want ErrConvert",
			err,
		)
	}

	_Cut(
		t,
		err,
		"... ("+strconv.Itoa(LONG+len(".star"))+" bytes)",
	)

	if strings.Count(
		err.Error(),
		"... (",
	) < 2 {
		t.Fatalf(
			"want the file and the function both cut: %.300q",
			err,
		)
	}
}
