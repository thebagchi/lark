package tests

import (
	"errors"
	"io"
	"path/filepath"
	"testing"

	"google.golang.org/grpc"

	pluginpb "github.com/thebagchi/lark/proto/gen/plugin"
	"github.com/thebagchi/lark/v1/runtime/plugin/remote/internal/host"
)

// _Presenter is a stream whose plugin announces itself once with token.
//
// Revisions:
//   - 2026-11-08 10:43: initial creation (C109, Q254)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
type _Presenter struct {
	grpc.ServerStream
	token string
}

// Recv announces the plugin named p, with the token of the presenter.
//
// Revisions:
//   - 2026-11-08 10:43: initial creation (C109, Q254)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (p *_Presenter) Recv() (*pluginpb.Request, error) {
	return &pluginpb.Request{
		Of: &pluginpb.Request_Register{
			Register: &pluginpb.Register{
				Name:  "p",
				Token: p.token,
			},
		},
	}, nil
}

// Send ends the stream, so Register returns once it has accepted.
//
// Revisions:
//   - 2026-11-08 10:43: initial creation (C109, Q254)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func (p *_Presenter) Send(*pluginpb.Response) error {
	return io.EOF
}

// TestRegister_Token proves Register refuses every token but the listener's,
// and accepts that one. An accepted plugin gets as far as Send, which fails
// with io.EOF; a refused one gets ErrToken before any Send.
//
// Mutations (apply, run, revert): accepting any token fails the three refusals;
// refusing every token fails the accepted case.
//
// Revisions:
//   - 2026-11-08 10:43: initial creation (C109, Q254)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestRegister_Token(t *testing.T) {
	const SECRET = "0123456789abcdef"

	tests := []struct {
		name  string
		token string
		want  error
	}{
		{
			name:  "the right token is accepted",
			token: SECRET,
			want:  io.EOF,
		},
		{
			name:  "a wrong token of the same length is refused",
			token: "0123456789abcdeX",
			want:  host.ErrToken,
		},
		{
			name:  "a prefix of the token is refused",
			token: SECRET[:8],
			want:  host.ErrToken,
		},
		{
			name:  "the empty token is refused",
			token: "",
			want:  host.ErrToken,
		},
		{
			name:  "the token with more after it is refused",
			token: SECRET + "0",
			want:  host.ErrToken,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
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

				t.Cleanup(func() {
					_ = listener.Close()
				})

				err = listener.Register(&_Presenter{
					token: tt.token,
				})
				if !errors.Is(
					err,
					tt.want,
				) {
					t.Fatalf(
						"Register error %v, want %v",
						err,
						tt.want,
					)
				}
			},
		)
	}
}
