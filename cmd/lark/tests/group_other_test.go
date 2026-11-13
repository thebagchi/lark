//go:build !unix

package tests

import "os/exec"

// _Grouped is the form for a system without process groups: the kill of cmd is
// the kill of the child alone.
//
// Revisions:
//   - 2026-11-02 23:47: initial creation (C27)
//   - 2026-11-10 01:37: moved to tests/ (C120b, Q311)
func _Grouped(cmd *exec.Cmd) {
	cmd.WaitDelay = KILL_WAIT
}
