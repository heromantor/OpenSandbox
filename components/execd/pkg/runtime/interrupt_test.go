// Copyright 2026 Alibaba Group Holding Ltd.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build !windows

package runtime

import (
	"errors"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

const testKillTimeout = 10 * time.Second

// --- killPid routing tests ---

// TestKillPid_RoutesToProcessGroup verifies that killPid routes to killProcessGroup
// when the process is a group leader (PGID == PID, i.e. Setpgid: true).
func TestKillPid_RoutesToProcessGroup(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found in PATH")
	}

	c := NewController("", "")

	cmd := exec.Command("bash", "-c", "sleep 60 & sleep 60 & wait")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	})

	pid := cmd.Process.Pid
	require.NoError(t, c.killPid(pid))

	// Wait for the process to reap the zombie, then verify it's gone.
	_ = cmd.Wait()
	require.Error(t, cmd.Process.Signal(syscall.Signal(0)), "expected process leader to be terminated")

	// Verify child processes in the group are also gone.
	assertNoOrphanChildren(t, pid)
}

// TestKillPid_RoutesToProcessOnly verifies that killPid targets only the
// individual process (not the group) when the process is not a group leader.
func TestKillPid_RoutesToProcessOnly(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found in PATH")
	}

	c := NewController("", "")

	// Start a process WITHOUT Setpgid — it inherits the parent's group.
	cmd := exec.Command("bash", "-c", "sleep 60")
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
	})

	require.NoError(t, c.killPid(cmd.Process.Pid))

	// Wait for the process to reap the zombie, then verify it's gone.
	_ = cmd.Wait()
	require.Error(t, cmd.Process.Signal(syscall.Signal(0)), "expected process to be terminated after killPid")
}

// TestKillPid_AlreadyFinished verifies that killPid returns nil when the
// target process has already exited (ESRCH from Getpgid).
func TestKillPid_AlreadyFinished(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found in PATH")
	}

	c := NewController("", "")

	cmd := exec.Command("bash", "-c", "true")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start())
	pid := cmd.Process.Pid
	_, _ = cmd.Process.Wait()

	require.NoError(t, c.killPid(pid))
}

// --- killProcessGroup tests ---

// TestKillProcessGroup_TerminatesGracefully verifies that a process group
// that responds to SIGTERM is terminated gracefully without SIGKILL.
func TestKillProcessGroup_TerminatesGracefully(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found in PATH")
	}

	c := NewController("", "")

	// A process that handles SIGTERM and exits quickly.
	cmd := exec.Command("bash", "-c", "trap 'exit 0' TERM; sleep 60")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	})

	require.NoError(t, c.killProcessGroup(cmd.Process.Pid))

	// Wait for the process to reap the zombie, then verify it's gone.
	_ = cmd.Wait()
	require.Error(t, cmd.Process.Signal(syscall.Signal(0)), "expected process to be terminated")
}

// TestKillProcessGroup_KillsUnresponsiveGroup verifies that a process group
// ignoring SIGTERM is eventually killed with SIGKILL.
func TestKillProcessGroup_KillsUnresponsiveGroup(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found in PATH")
	}

	c := NewController("", "")

	// A process that ignores SIGTERM — requires SIGKILL.
	cmd := exec.Command("bash", "-c", "trap '' TERM; sleep 60")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	})

	done := make(chan error, 1)
	go func() {
		done <- c.killProcessGroup(cmd.Process.Pid)
	}()

	select {
	case err := <-done:
		require.NoError(t, err, "killProcessGroup should succeed even for unresponsive processes")
		_ = cmd.Wait()
	case <-time.After(testKillTimeout):
		require.Fail(t, "killProcessGroup did not return within timeout")
	}
}

// --- killProcessOnly tests ---

// TestKillProcessOnly_TerminatesGracefully verifies that a single process
// that responds to SIGTERM is terminated gracefully.
func TestKillProcessOnly_TerminatesGracefully(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found in PATH")
	}

	c := NewController("", "")

	cmd := exec.Command("bash", "-c", "trap 'exit 0' TERM; sleep 60")
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
	})

	require.NoError(t, c.killProcessOnly(cmd.Process.Pid))

	// Wait for the process to reap the zombie, then verify it's gone.
	_ = cmd.Wait()
	require.Error(t, cmd.Process.Signal(syscall.Signal(0)), "expected process to be terminated")
}

// TestKillProcessOnly_KillsUnresponsiveProcess verifies that a single process
// ignoring SIGTERM is eventually killed with SIGKILL.
func TestKillProcessOnly_KillsUnresponsiveProcess(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found in PATH")
	}

	c := NewController("", "")

	cmd := exec.Command("bash", "-c", "trap '' TERM; sleep 60")
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
	})

	done := make(chan error, 1)
	go func() {
		done <- c.killProcessOnly(cmd.Process.Pid)
	}()

	select {
	case err := <-done:
		require.NoError(t, err, "killProcessOnly should succeed even for unresponsive processes")
		_ = cmd.Wait()
	case <-time.After(testKillTimeout):
		require.Fail(t, "killProcessOnly did not return within timeout")
	}
}

// --- waitForProcessExit tests ---

// TestWaitForProcessExit_ExitedProcess verifies that waitForProcessExit
// returns promptly for a process that has exited on its own.
func TestWaitForProcessExit_ExitedProcess(t *testing.T) {
	cmd := exec.Command("bash", "-c", "exit 0")
	require.NoError(t, cmd.Start())

	exited, err := waitForProcessExit(cmd.Process, 3*time.Second)
	require.NoError(t, err)
	require.True(t, exited, "expected true for a process that exits on its own")
	_ = cmd.Wait()
}

// TestWaitForProcessExit_KilledBySIGTERM verifies that waitForProcessExit
// returns true for a process killed by SIGTERM (ExitError case).
func TestWaitForProcessExit_KilledBySIGTERM(t *testing.T) {
	cmd := exec.Command("sleep", "60")
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
	})

	require.NoError(t, cmd.Process.Signal(syscall.SIGTERM))
	exited, err := waitForProcessExit(cmd.Process, 3*time.Second)
	require.NoError(t, err)
	require.True(t, exited, "expected true for a process killed by SIGTERM")
	_ = cmd.Wait()
}

// TestWaitForProcessExit_NoConflictWithCmdWait verifies that waitForProcessExit
// does not conflict with cmd.Wait() — both can be called concurrently.
func TestWaitForProcessExit_NoConflictWithCmdWait(t *testing.T) {
	cmd := exec.Command("bash", "-c", "exit 0")
	require.NoError(t, cmd.Start())

	// Call cmd.Wait() first, then waitForProcessExit — should not panic or deadlock.
	_ = cmd.Wait()
	exited, err := waitForProcessExit(cmd.Process, 3*time.Second)
	require.NoError(t, err)
	require.True(t, exited, "expected true after cmd.Wait() completed")
}

// TestWaitForProcessExit_RunningProcessTimesOut verifies that waitForProcessExit
// returns false when the process does not exit within the timeout.
func TestWaitForProcessExit_RunningProcessTimesOut(t *testing.T) {
	cmd := exec.Command("sleep", "60")
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
	})

	exited, err := waitForProcessExit(cmd.Process, 50*time.Millisecond)
	require.NoError(t, err)
	require.False(t, exited, "expected false for a running process that exceeds timeout")
}

// --- confirmProcessTerminated tests ---

// TestConfirmProcessTerminated_DeadProcess verifies that confirmProcessTerminated
// returns true for a process that has already exited.
func TestConfirmProcessTerminated_DeadProcess(t *testing.T) {
	cmd := exec.Command("true")
	require.NoError(t, cmd.Start())
	_, _ = cmd.Process.Wait()

	require.True(t, confirmProcessTerminated(cmd.Process, "test"),
		"expected true for a dead process")
}

// TestConfirmProcessTerminated_LiveProcess verifies that confirmProcessTerminated
// returns false for a process that is still running.
func TestConfirmProcessTerminated_LiveProcess(t *testing.T) {
	cmd := exec.Command("sleep", "60")
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
	})

	require.False(t, confirmProcessTerminated(cmd.Process, "test"),
		"expected false for a running process")
}

// --- isProcessGone tests ---

func TestIsProcessGone(t *testing.T) {
	var (
		testErrNoSuchProcess   = errors.New("os: process already finished: no such process")
		testErrAlreadyFinished = errors.New("os: process already finished")
		testErrOther           = errors.New("permission denied")
	)

	tests := []struct {
		name   string
		err    error
		wantOk bool
	}{
		{
			name:   "[ok] no such process",
			err:    testErrNoSuchProcess,
			wantOk: true,
		},
		{
			name:   "[ok] already finished",
			err:    testErrAlreadyFinished,
			wantOk: true,
		},
		{
			name:   "[ok] syscall ESRCH",
			err:    syscall.ESRCH,
			wantOk: true,
		},
		{
			name:   "[ok] os ErrProcessDone",
			err:    os.ErrProcessDone,
			wantOk: true,
		},
		{
			name:   "[err] other error returns false",
			err:    testErrOther,
			wantOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isProcessGone(tt.err)
			require.Equal(t, tt.wantOk, got)
		})
	}
}

// --- Interrupt routing tests ---

func TestInterrupt_NoSuchSession(t *testing.T) {
	c := NewController("", "")

	err := c.Interrupt("nonexistent-session")
	require.ErrorIs(t, err, ErrNoSuchSession)
}

// --- killProcessGroup edge case ---

// TestKillProcessGroup_AlreadyExited verifies that killProcessGroup returns nil
// when called for a process group that has already exited.
func TestKillProcessGroup_AlreadyExited(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found in PATH")
	}

	c := NewController("", "")

	cmd := exec.Command("bash", "-c", "true")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start())
	pid := cmd.Process.Pid
	_, _ = cmd.Process.Wait()

	require.NoError(t, c.killProcessGroup(pid))
}

// --- killProcessOnly edge case ---

// TestKillProcessOnly_AlreadyFinished verifies that killProcessOnly returns nil
// when called for a process that has already exited.
func TestKillProcessOnly_AlreadyFinished(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found in PATH")
	}

	c := NewController("", "")

	cmd := exec.Command("bash", "-c", "true")
	require.NoError(t, cmd.Start())
	pid := cmd.Process.Pid
	_, _ = cmd.Process.Wait()

	require.NoError(t, c.killProcessOnly(pid))
}

// --- helper ---

// assertNoOrphanChildren checks that no child processes with the given PGID
// are still running, which would indicate the process group was not fully killed.
func assertNoOrphanChildren(t *testing.T, pgid int) {
	t.Helper()

	// Read /proc to find any remaining processes in the group.
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return // not Linux, skip check
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		pid, err := strconv.Atoi(entry.Name())
		if err != nil || pid <= 0 {
			continue
		}

		pgidOfEntry, err := syscall.Getpgid(pid)
		if err != nil {
			continue
		}
		require.NotEqual(t, pgid, pgidOfEntry,
			"found orphan process %d still in process group %d", pid, pgid)
	}
}

// --- killProcessGroupGraceful tests ---

// TestKillProcessGroupGraceful_TerminatesWithTrap verifies that a process
// with a SIGTERM trap handler terminates gracefully without SIGKILL.
func TestKillProcessGroupGraceful_TerminatesWithTrap(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found in PATH")
	}

	cmd := exec.Command("bash", "-c", "trap 'exit 0' TERM; sleep 60")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
	})

	require.NoError(t, killProcessGroupGraceful(cmd.Process.Pid, 2*time.Second))
	_ = cmd.Wait()
}

// TestKillProcessGroupGraceful_IgnoresSIGTERM verifies that a process
// ignoring SIGTERM is killed with SIGKILL after the grace period.
func TestKillProcessGroupGraceful_IgnoresSIGTERM(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found in PATH")
	}

	cmd := exec.Command("bash", "-c", "trap '' TERM; sleep 60")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
	})

	done := make(chan error, 1)
	go func() {
		done <- killProcessGroupGraceful(cmd.Process.Pid, 1*time.Second)
	}()

	select {
	case err := <-done:
		require.NoError(t, err, "killProcessGroupGraceful should succeed even for unresponsive processes")
		_ = cmd.Wait()
	case <-time.After(testKillTimeout):
		require.Fail(t, "killProcessGroupGraceful did not return within timeout")
	}
}

// TestKillProcessGroupGraceful_AlreadyExited verifies that killProcessGroupGraceful
// returns nil when called for a process group that has already exited.
func TestKillProcessGroupGraceful_AlreadyExited(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found in PATH")
	}

	cmd := exec.Command("bash", "-c", "true")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start())
	pid := cmd.Process.Pid
	_, _ = cmd.Process.Wait()

	require.NoError(t, killProcessGroupGraceful(pid, 2*time.Second))
}

// --- isZombie tests ---

// TestIsZombie_RunningProcess verifies that isZombie returns false for a running process.
func TestIsZombie_RunningProcess(t *testing.T) {
	cmd := exec.Command("sleep", "60")
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	zombie, err := isZombie(cmd.Process.Pid)
	require.NoError(t, err)
	require.False(t, zombie, "expected false for a running process")
}

// TestIsZombie_ZombieProcess verifies that isZombie returns true for a zombie
// process (exited but not yet reaped by Wait).
func TestIsZombie_ZombieProcess(t *testing.T) {
	cmd := exec.Command("bash", "-c", "exit 0")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start())

	// Wait for the process to exit but do NOT call cmd.Wait() yet —
	// the process becomes a zombie until reaped.
	require.Eventually(t, func() bool {
		z, err := isZombie(cmd.Process.Pid)
		return err == nil && z
	}, 2*time.Second, 50*time.Millisecond, "expected process to become zombie")

	_ = cmd.Wait()
}

// TestIsZombie_NonexistentProcess verifies that isZombie returns an error
// for a PID that does not exist.
func TestIsZombie_NonexistentProcess(t *testing.T) {
	_, err := isZombie(999999999)
	require.Error(t, err)
}

// --- isProcessDeadOrZombie tests ---

// TestIsProcessDeadOrZombie_DeadProcess verifies that isProcessDeadOrZombie
// returns true for a fully reaped (dead) process.
func TestIsProcessDeadOrZombie_DeadProcess(t *testing.T) {
	cmd := exec.Command("bash", "-c", "true")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start())
	_, _ = cmd.Process.Wait()

	gone, err := isProcessDeadOrZombie(cmd.Process)
	require.NoError(t, err)
	require.True(t, gone, "expected true for a dead process")
}

// TestIsProcessDeadOrZombie_RunningProcess verifies that isProcessDeadOrZombie
// returns false for a running process.
func TestIsProcessDeadOrZombie_RunningProcess(t *testing.T) {
	cmd := exec.Command("sleep", "60")
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	gone, err := isProcessDeadOrZombie(cmd.Process)
	require.NoError(t, err)
	require.False(t, gone, "expected false for a running process")
}

// TestIsProcessDeadOrZombie_KilledButNotReaped verifies that isProcessDeadOrZombie
// returns true for a process killed by SIGTERM but not yet reaped by Wait.
func TestIsProcessDeadOrZombie_KilledButNotReaped(t *testing.T) {
	cmd := exec.Command("sleep", "60")
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Wait()
	})

	require.NoError(t, cmd.Process.Signal(syscall.SIGTERM))
	// Process is dead or zombie — either way, isProcessDeadOrZombie should return true.
	require.Eventually(t, func() bool {
		gone, _ := isProcessDeadOrZombie(cmd.Process)
		return gone
	}, 2*time.Second, 50*time.Millisecond, "expected killed process to be dead or zombie")
}

// --- killProcessGroupImmediate tests ---

// TestKillProcessGroupImmediate_KillsRunningGroup verifies that killProcessGroupImmediate
// kills a running process group.
func TestKillProcessGroupImmediate_KillsRunningGroup(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found in PATH")
	}

	cmd := exec.Command("bash", "-c", "sleep 60 & sleep 60 & wait")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		_ = cmd.Wait()
	})

	pid := cmd.Process.Pid
	killProcessGroupImmediate(pid)

	_ = cmd.Wait()
	require.Error(t, cmd.Process.Signal(syscall.Signal(0)), "expected process leader to be terminated")
	assertNoOrphanChildren(t, pid)
}

// TestKillProcessGroupImmediate_AlreadyExited verifies that killProcessGroupImmediate
// does not return an error for an already-exited process group.
func TestKillProcessGroupImmediate_AlreadyExited(t *testing.T) {
	if _, err := exec.LookPath("bash"); err != nil {
		t.Skip("bash not found in PATH")
	}

	cmd := exec.Command("bash", "-c", "true")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	require.NoError(t, cmd.Start())
	pid := cmd.Process.Pid
	_, _ = cmd.Process.Wait()

	// Should not panic or log warnings — ESRCH is suppressed.
	killProcessGroupImmediate(pid)
}
