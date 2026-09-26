// Command clock is a plugin that lives in its own process, and the worked
// example of how to write one.
//
// It is started by a host rather than run by hand. lark finds it in the
// directory it was pointed at, starts it, and tells it where to dial and what
// to present through the environment: LARK_PLUGIN_SOCKET and
// LARK_PLUGIN_TOKEN. Run without those it says so and stops.
//
// The host starts it but still listens, and this dials back - so nothing has
// to reach here, and there is no port to open and no address anyone needs to
// know. The token is in the environment rather than in an argument because an
// argument is in the process table, readable by anyone on the machine, and the
// token is the only thing between a local process and putting names into every
// script the host compiles.
//
// Read it for the shape rather than for what it does. It supplies two names,
// clock.now and clock.add, and the interesting part is what it does not do: it
// imports this module's generated schema and nothing else. It holds no
// starlark.StringDict, it never mentions the runtime, and it is never handed a
// function or a thread handle, because the host refuses those before they
// reach a socket. That is what lets a plugin be written in a language this
// repository does not speak.
package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"os"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/structpb"

	pluginpb "github.com/thebagchi/lark/proto/gen/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/remote"
)

const (
	NAME_FLAG  = "name"
	NAME_USAGE = "what this plugin calls itself, for a conflict report"

	// NAME is what this plugin is called, and NOW and ADD what it supplies.
	// Dotted, so a script reaches them through a clock module as it does every
	// other plugin's names.
	NAME = "clock"
	NOW  = "clock.now"
	ADD  = "clock.add"

	// LAYOUT is how now renders the time: the one format that sorts as text.
	LAYOUT = time.RFC3339

	// NO_HOST is the exit code for a plugin nobody started, and FAILED for one
	// that could not reach the host that did.
	NO_HOST = 2
	FAILED  = 1

	// UNIX is the only network a host listens on.
	UNIX = "unix"
)

// main dials the host and answers until the stream closes.
//
// Revisions:
//   - 2026-09-25 06:58: initial creation
func main() {
	name := flag.String(NAME_FLAG, NAME, NAME_USAGE)

	flag.Parse()

	socket := os.Getenv(remote.SOCKET_ENV)
	token := os.Getenv(remote.TOKEN_ENV)

	if socket == "" {
		fmt.Fprintf(os.Stderr,
			"clock: %s is not set, so nothing started this. A host does.\n",
			remote.SOCKET_ENV)
		os.Exit(NO_HOST)
	}

	err := _Serve(socket, token, *name)
	if err != nil {
		fmt.Fprintf(os.Stderr, "clock: %v\n", err)
		os.Exit(FAILED)
	}
}

// _Serve dials, announces what this supplies, and answers until the host goes.
//
// Returns nil when the host closed the stream, which is how this ends rather
// than a failure. A plugin outliving its host would be a process nobody is
// waiting for.
//
// Revisions:
//   - 2026-09-25 06:58: initial creation
func _Serve(socket string, token string, name string) error {
	held, err := grpc.NewClient(
		"unix:"+socket,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			var dialer net.Dialer

			return dialer.DialContext(ctx, UNIX, socket)
		}),
	)
	if err != nil {
		return fmt.Errorf("dialing %s: %w", socket, err)
	}

	defer func() {
		_ = held.Close()
	}()

	stream, err := pluginpb.NewServiceClient(held).Register(context.Background())
	if err != nil {
		return fmt.Errorf("opening a stream: %w", err)
	}

	// The announcement, which has to be the first thing said. A host that does
	// not recognise the token closes the stream before it takes any name, so
	// a wrong one reads as the host going away.
	err = stream.Send(&pluginpb.Request{
		Of: &pluginpb.Request_Register{
			Register: &pluginpb.Register{
				Token: token,
				Name:  name,
				Names: []string{NOW, ADD},
			},
		},
	})
	if err != nil {
		return fmt.Errorf("announcing %s: %w", name, err)
	}

	return _Answer(stream)
}

// _Answer answers every ask until the stream closes.
//
// Revisions:
//   - 2026-09-25 06:58: initial creation
func _Answer(stream pluginpb.Service_RegisterClient) error {
	for {
		held, err := stream.Recv()
		if err != nil {
			// The host went away, which is how this ends.
			return nil
		}

		ask := held.GetAsk()
		if ask == nil {
			continue
		}

		err = stream.Send(&pluginpb.Request{
			Of: &pluginpb.Request_Answer{Answer: _Worked(ask)},
		})
		if err != nil {
			return nil
		}
	}
}

// _Worked is the answer to one ask.
//
// A name this plugin does not supply is a defect in the host rather than a
// script's typo, because the host filtered the name before it asked - so it
// comes back as a failure rather than as an empty value that would read like
// a result.
//
// Revisions:
//   - 2026-09-25 06:58: initial creation
func _Worked(ask *pluginpb.Ask) *pluginpb.Answer {
	switch ask.GetName() {
	case NOW:
		return &pluginpb.Answer{
			Id:     ask.GetId(),
			Result: structpb.NewStringValue(time.Now().Format(LAYOUT)),
		}

	case ADD:
		total := float64(0)

		for _, given := range ask.GetArgs() {
			total += given.GetNumberValue()
		}

		return &pluginpb.Answer{
			Id:     ask.GetId(),
			Result: structpb.NewNumberValue(total),
		}
	}

	return &pluginpb.Answer{
		Id:     ask.GetId(),
		Failed: "no such name: " + ask.GetName(),
	}
}
