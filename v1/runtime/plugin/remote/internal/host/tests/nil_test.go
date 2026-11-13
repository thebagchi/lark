package tests

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"

	"google.golang.org/grpc"

	pluginpb "github.com/thebagchi/lark/proto/gen/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/remote/internal/host"
)

// _Announcer is a stream whose plugin announces itself once, with the empty
// token a zero Listener holds.
//
// Revisions:
//   - 2026-11-04 23:56: initial creation (C86 fix round 3)
//   - 2026-11-07 19:54: moved here with the Listener it exercises (C91w8c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Announcer struct {
	grpc.ServerStream
}

// Recv announces the plugin named p.
//
// Revisions:
//   - 2026-11-04 23:56: initial creation (C86 fix round 3)
//   - 2026-11-07 19:54: moved here with the Listener it exercises (C91w8c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (a *_Announcer) Recv() (*pluginpb.Request, error) {
	return &pluginpb.Request{
		Of: &pluginpb.Request_Register{
			Register: &pluginpb.Register{
				Name: "p",
			},
		},
	}, nil
}

// Send ends the stream, so Register returns after it accepts.
//
// Revisions:
//   - 2026-11-04 23:56: initial creation (C86 fix round 3)
//   - 2026-11-07 19:54: moved here with the Listener it exercises (C91w8c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (a *_Announcer) Send(*pluginpb.Response) error {
	return io.EOF
}

// TestNilZeroListener proves a nil or zero Listener does not panic, and
// answers what its doc says. A host holds a common.Listener and cannot reach a
// nil or zero concrete Listener, so the cases of TestNilSweep, TestNilSweepHelpers,
// TestNilZeroListener and TestNilZeroOwners in package runtime_test that built
// one are here, inside the package that owns the type.
//
// Revisions:
//   - 2026-11-04 23:56: initial creation (C86 fix round 3)
//   - 2026-11-07 19:54: gathers the Listener cases of four runtime tests (C91w8c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestNilZeroListener(t *testing.T) {
	var (
		dir         = t.TempDir()
		nilListener *host.Listener
	)

	err := os.WriteFile(
		filepath.Join(
			dir,
			"p.bin",
		),
		[]byte("#!/bin/sh\nexit 0\n"),
		0o700,
	)
	if err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		call func() error
		want error
	}{
		{
			"nil Listener Close",
			func() error {
				return nilListener.Close()
			},
			nil,
		},
		{
			"nil Listener Names Plugins Live",
			func() error {
				if nilListener.Names() != nil || nilListener.Plugins() != nil {
					return errors.New("want nil lists")
				}

				if nilListener.Live() != nil {
					return errors.New("want nil list")
				}

				return nil
			},
			nil,
		},
		{
			"zero Listener Close",
			func() error {
				return new(host.Listener).Close()
			},
			nil,
		},
		{
			"nil Listener Load with a match",
			func() error {
				_, err := nilListener.Load(
					dir,
					"*.bin",
				)

				return err
			},
			io.EOF,
		},
		{
			"zero Listener Load with a match",
			func() error {
				_, err := new(host.Listener).Load(
					dir,
					"*.bin",
				)

				return err
			},
			io.EOF,
		},
		{
			"Listener.Register nil stream",
			func() error {
				return nilListener.Register(nil)
			},
			io.EOF,
		},
		{
			"Listener.Register zero Listener",
			func() error {
				return new(host.Listener).Register(nil)
			},
			io.EOF,
		},
		{
			"zero Listener Register with an announcing stream",
			func() error {
				return new(host.Listener).Register(new(_Announcer))
			},
			io.EOF,
		},
	}

	for _, tc := range cases {
		t.Run(
			tc.name,
			func(t *testing.T) {
				var err error

				defer func() {
					r := recover()
					if r != nil {
						t.Fatalf(
							"panicked: %v",
							r,
						)
					}
				}()

				err = tc.call()

				if !errors.Is(
					err,
					tc.want,
				) && (tc.want != nil || err != nil) {
					t.Fatalf(
						"want %v, got %v",
						tc.want,
						err,
					)
				}
			},
		)
	}
}
