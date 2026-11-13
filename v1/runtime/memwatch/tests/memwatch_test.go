package tests

import (
	"errors"
	"os"
	"os/exec"
	"testing"
	"time"

	"github.com/thebagchi/lark/v1/runtime/common"
	"github.com/thebagchi/lark/v1/runtime/memwatch"
)

// TestStart_Forwards proves the top package forwards: Start trips with the
// package's own ErrLimit, and a nil watch is no watch.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrLimit, formerly ErrLimitReached (C121, Q349)
func TestStart_Forwards(t *testing.T) {
	var (
		tripped = make(
			chan error,
			1,
		)
		stop = memwatch.Start(
			t.Context(),
			&common.MemoryWatch{
				Interval: time.Millisecond,
				Source: func() (*common.MemoryReading, error) {
					return &common.MemoryReading{
						Used:  99,
						Total: 100,
					}, nil
				},
			},
			func(err error) {
				tripped <- err
			},
		)
	)
	defer stop()

	select {
	case err := <-tripped:
		if !errors.Is(
			err,
			memwatch.ErrLimit,
		) {
			t.Fatalf(
				"got %v, want ErrLimit",
				err,
			)
		}

	case <-time.After(5 * time.Second):
		t.Fatal("did not trip")
	}

	memwatch.Start(
		t.Context(),
		nil,
		nil,
	)()
}

// TestCheck_OnThisSystem proves Check says ErrUnavailable, or nothing, on the real
// system. It passes on either kind of system.
//
// Revisions:
//   - 2026-11-02 22:32: initial creation (C27)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
func TestCheck_OnThisSystem(t *testing.T) {
	checked := memwatch.Check()

	if checked != nil && !errors.Is(
		checked,
		memwatch.ErrUnavailable,
	) {
		t.Fatalf(
			"Check failed with %v, want ErrUnavailable",
			checked,
		)
	}
}

// TestRoom_Forwards proves the top package forwards Room and Fit with its own
// ErrRoom, and System with a working source.
//
// Revisions:
//   - 2026-11-03 00:40: initial creation (C46)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrRoom, formerly ErrNoRoom (C121, Q349)
func TestRoom_Forwards(t *testing.T) {
	err := memwatch.Fit(
		"who",
		common.ROOM_CEILING+1,
		func() (*common.MemoryReading, error) {
			return nil, memwatch.ErrUnavailable
		},
	)
	if !errors.Is(
		err,
		memwatch.ErrRoom,
	) {
		t.Fatalf(
			"err = %v, want ErrRoom",
			err,
		)
	}

	err = memwatch.Room(
		"who",
		1<<62,
	)
	if !errors.Is(
		err,
		memwatch.ErrRoom,
	) {
		t.Fatalf(
			"Room(1<<62) = %v, want ErrRoom",
			err,
		)
	}

	if memwatch.System() == nil {
		t.Fatal("System() is nil")
	}
}

// TestRoom_CountsTheAddressSpaceLimit proves Room reads the address-space limit
// (RLIMIT_AS, ulimit -v) as a third source. The test runs itself as a child
// under ulimit -v 2000000 (about 2 GB). The child asks Room for 4 GiB: the
// machine has that free, so only the address-space limit can refuse it. Success:
// the child exits 0 after seeing ErrRoom for 4 GiB and no error for 2 MiB.
// Without the Address source in watch.Free the child fails. The test is skipped
// where the machine has under 8 GiB free, where the host would refuse anyway.
//
// Revisions:
//   - 2026-11-03 22:21: initial creation (C46 rebase)
//   - 2026-11-10 00:25: moved to tests/ (C120a, Q311)
//   - 2026-11-10 17:15: uses ErrRoom, formerly ErrNoRoom (C121, Q349)
func TestRoom_CountsTheAddressSpaceLimit(t *testing.T) {
	const (
		CHILD = "LARK_ROOM_CHILD"
		FOUR  = 4 << 30
	)

	if os.Getenv(CHILD) != "" {
		err := memwatch.Room(
			"who",
			FOUR,
		)
		if !errors.Is(
			err,
			memwatch.ErrRoom,
		) {
			t.Fatalf(
				"4 GiB under ulimit: err = %v, want ErrRoom",
				err,
			)
		}

		err = memwatch.Room(
			"who",
			2<<20,
		)
		if err != nil {
			t.Fatalf(
				"2 MiB under ulimit: err = %v, want nil",
				err,
			)
		}

		return
	}

	reading, err := memwatch.System()()
	if err != nil || reading.Total-reading.Used < 2*FOUR {
		t.Skip("needs 8 GiB free")
	}

	cmd := exec.Command(
		"sh",
		"-c",
		"ulimit -v 2000000 && exec \"$0\" -test.run '^TestRoom_CountsTheAddress'",
		os.Args[0],
	)

	cmd.Env = append(
		os.Environ(),
		CHILD+"=1",
	)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf(
			"child: %v\n%.600s",
			err,
			out,
		)
	}
}
