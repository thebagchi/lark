//go:build unix

package tests

import (
	"bufio"
	"context"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// _Grouped puts cmd in a process group of its own and makes the kill of cmd
// the kill of the whole group, so a grandchild of the command cannot outlive a
// test. It uses os/exec's Cancel hook, which exec.CommandContext calls when the
// context ends, and Setpgid, so the group id is the child's pid.
//
// Revisions:
//   - 2026-11-02 23:47: initial creation (C27)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Grouped(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	cmd.Cancel = func() error {
		return syscall.Kill(
			-cmd.Process.Pid,
			syscall.SIGKILL,
		)
	}
}

// TestGrouped_KillsTheGrandchild proves the kill reaches the whole group: a
// shell starts a sleeping grandchild and prints its pid; the context ends; the
// grandchild must be gone. Without the group kill only the shell dies and the
// grandchild stays.
//
// Revisions:
//   - 2026-11-02 23:48: initial creation (C27)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func TestGrouped_KillsTheGrandchild(t *testing.T) {
	var (
		ctx, cancel = context.WithCancel(t.Context())
		cmd         = exec.CommandContext(
			ctx,
			"sh",
			"-c",
			"sleep 300 & echo $!; wait",
		)
	)

	defer cancel()

	_Grouped(cmd)

	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}

	err = cmd.Start()
	if err != nil {
		t.Fatal(err)
	}

	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil {
		t.Fatal(err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(line))
	if err != nil {
		t.Fatal(err)
	}

	cancel()

	_ = cmd.Wait()

	for range 50 {
		if syscall.Kill(
			pid,
			0,
		) != nil {
			return
		}

		time.Sleep(20 * time.Millisecond)
	}

	_ = syscall.Kill(
		pid,
		syscall.SIGKILL,
	)

	t.Fatalf(
		"grandchild %d outlived the kill of its parent",
		pid,
	)
}
