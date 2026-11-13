package tests

import (
	"errors"
	"io"
	"path/filepath"
	"testing"
	"time"

	"go.starlark.net/starlark"
	"go.starlark.net/starlarkstruct"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/structpb"

	pluginpb "github.com/thebagchi/lark/proto/gen/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/remote/internal/host"
	"github.com/thebagchi/lark/v1/runtime/plugin/unpack"
	"github.com/thebagchi/lark/v1/runtime/trail"
)

// _Alien is a value kind the shared converter does not know. It embeds a known
// kind to satisfy the unexported kind interface of structpb, and the type switch
// of unpack.FromProto does not match it. No decoded protobuf value is one, so
// only a stream that holds Go values, as _Replier does, can send it.
//
// Revisions:
//   - 2026-11-04 22:48: initial creation
//   - 2026-11-06 22:37: revision history added (C90)
//   - 2026-11-08 13:25: merges duplicate sentinels (C98, Q249-Q252)
//   - 2026-11-08 22:46: moved to the external package (C65b)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Alien struct {
	*structpb.Value_BoolValue
}

// _Replier is a stream whose plugin announces "clock.add", and then answers
// every ask with result.
//
// Revisions:
//   - 2026-11-08 22:46: initial creation (C65b)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Replier struct {
	grpc.ServerStream
	result  *structpb.Value
	answers chan *pluginpb.Request
	token   string
	started bool
}

// Recv announces the plugin once, then hands back each answer until answers
// closes, which ends the stream.
//
// Revisions:
//   - 2026-11-08 22:46: initial creation (C65b)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (r *_Replier) Recv() (*pluginpb.Request, error) {
	if !r.started {
		r.started = true

		return &pluginpb.Request{
			Of: &pluginpb.Request_Register{
				Register: &pluginpb.Register{
					Name:  "p",
					Token: r.token,
					Names: []string{"clock.add"},
				},
			},
		}, nil
	}

	held, open := <-r.answers
	if !open {
		return nil, io.EOF
	}

	return held, nil
}

// Send takes the acceptance, and turns each ask into an answer that carries
// the result of the replier.
//
// Revisions:
//   - 2026-11-08 22:46: initial creation (C65b)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (r *_Replier) Send(sent *pluginpb.Response) error {
	ask := sent.GetAsk()
	if ask == nil {
		return nil
	}

	r.answers <- &pluginpb.Request{
		Of: &pluginpb.Request_Answer{
			Answer: &pluginpb.Answer{
				Id:     ask.GetId(),
				Result: r.result,
			},
		},
	}

	return nil
}

// TestCall_RefusesWhatItCannotCarry proves a call through a remote plugin
// fails with the right sentinel when an argument cannot go out and when the
// answer cannot come in. A cycle in an argument reads "who: argument N: cycle
// in value": the call named once, then the sentence of the walker.
//
// Revisions:
//   - 2026-11-02 11:34: initial creation
//   - 2026-11-02 12:28: initial creation (C26)
//   - 2026-11-04 21:49: calls _Starlark, _Refused being gone (C59)
//   - 2026-11-08 10:46: converts with wire.Value, the converter having moved (C73, Q257)
//   - 2026-11-08 13:25: one sentinel, the remote one being merged into it (C98, Q251)
//   - 2026-11-08 22:46: merged with TestSendable_NamesTheCallAndTheArgument, and moved to
//     the external package (C65b)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrConvert and ErrCyclic, formerly ErrNotProtoValue and
//     ErrSelfContaining (C121, Q349)
func TestCall_RefusesWhatItCannotCarry(t *testing.T) {
	var (
		cycle = starlark.NewList(nil)
		err   = cycle.Append(cycle)
	)
	if err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name   string
		args   starlark.Tuple
		result *structpb.Value
		want   error
		text   string
	}{
		{
			name: "an argument that holds itself",
			args: starlark.Tuple{
				starlark.MakeInt(1),
				cycle,
			},
			result: structpb.NewNullValue(),
			want:   trail.ErrCyclic,
			text:   "clock.add: argument 1: cycle in value",
		},
		{
			name: "an answer of a kind the converter does not know",
			args: starlark.Tuple{
				starlark.MakeInt(1),
			},
			result: &structpb.Value{
				Kind: _Alien{},
			},
			want: unpack.ErrConvert,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				_, err := starlark.Call(
					new(starlark.Thread),
					_Connected(
						t,
						tt.result,
					),
					tt.args,
					nil,
				)
				if !errors.Is(
					err,
					tt.want,
				) {
					t.Fatalf(
						"got %v, want %v",
						err,
						tt.want,
					)
				}

				if tt.text != "" && err.Error() != tt.text {
					t.Fatalf(
						"text %q, want %q",
						err,
						tt.text,
					)
				}
			},
		)
	}
}

// _Connected registers a plugin that answers every ask with result, and returns
// its clock.add builtin. The plugin leaves when the test ends.
//
// Revisions:
//   - 2026-11-08 22:46: initial creation (C65b)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func _Connected(
	t *testing.T,
	result *structpb.Value,
) starlark.Value {
	t.Helper()

	const SECRET = "0123456789abcdef"

	listener, err := host.Listen(
		filepath.Join(
			t.TempDir(),
			"s",
		),
		SECRET,
	)
	if err != nil {
		t.Fatal(err)
	}

	var (
		stream = &_Replier{
			result: result,
			answers: make(
				chan *pluginpb.Request,
				1,
			),
			token: SECRET,
		}
		done = make(chan struct{})
	)

	t.Cleanup(func() {
		close(stream.answers)
		<-done

		_ = listener.Close()
	})

	go func() {
		defer close(done)

		_ = listener.Register(stream)
	}()

	deadline := time.Now().Add(5 * time.Second)

	for len(listener.Live()) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("the plugin never attached")
		}

		time.Sleep(time.Millisecond)
	}

	module, ok := listener.Plugins()[0].Values()["clock"].(*starlarkstruct.Module)
	if !ok {
		t.Fatal("the plugin supplies no clock module")
	}

	return module.Members["add"]
}
