package tests

import (
	"math"
	"testing"

	"github.com/thebagchi/lark/v1/runtime/plugin/remote/internal/host"
)

// TestLimits_MatchGRPCDefaults pins each message size limit to the value grpc
// v1.84.0 uses when no option is given.
//
// The grpc constants are unexported, so the values are written here with their
// source: google.golang.org/grpc@v1.84.0/server.go:61-62. A change to either
// constant, to 8 MiB for example, fails this test. A grpc upgrade that changes
// its default is a decision to take, not a drift to accept.
//
// Revisions:
//   - 2026-11-02 10:54: initial creation
//   - 2026-11-02 11:18: constants renamed to MAX_RECEIVE and MAX_SEND
//   - 2026-11-07 19:54: moved inside remote/internal/host with the constants (Q228, C91w8c)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestLimits_MatchGRPCDefaults(t *testing.T) {
	tests := []struct {
		name string
		got  int
		want int
	}{
		{
			name: "MAX_RECEIVE is defaultServerMaxReceiveMessageSize (4 MiB)",
			got:  host.MAX_RECEIVE,
			want: 1024 * 1024 * 4,
		},
		{
			name: "MAX_SEND is defaultServerMaxSendMessageSize (MaxInt32)",
			got:  host.MAX_SEND,
			want: math.MaxInt32,
		},
	}

	for _, tt := range tests {
		t.Run(
			tt.name,
			func(t *testing.T) {
				if tt.got != tt.want {
					t.Fatalf(
						"got %d, want %d",
						tt.got,
						tt.want,
					)
				}
			},
		)
	}
}
