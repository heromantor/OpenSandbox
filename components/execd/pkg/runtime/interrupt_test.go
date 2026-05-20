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

	time.Sleep(100 * time.Millisecond)
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

	time.Sleep(100 * time.Millisecond)
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

	time.Sleep(100 * time.Millisecond)
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

	time.Sleep(100 * time.Millisecond)
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
	case <-time.After(testKillTimeout):
		require.Fail(t, "killProcessOnly did not return within timeout")
	}
}

// --- waitForProcessExit tests ---

// TestWaitForProcessExit_ExitedProcess verifies that waitForProcessExit
// returns promptly for a process that has already exited.
// Note: process.Wait() can only be called once, so we cannot test this
// directly on an already-waited process. Instead we test with a process
// that exits on its own within the timeout.
func TestWaitForProcessExit_ExitedProcess(t *testing.T) {
	cmd := exec.Command("bash", "-c", "exit 0")
	require.NoError(t, cmd.Start())

	require.True(t, waitForProcessExit(cmd.Process, 3*time.Second),
		"expected true for a process that exits on its own")
}

// TestWaitForProcessExit_RunningProcessTimesOut verifies that waitForProcessExit
// returns false when the process does not exit within the timeout.
func TestWaitForProcessExit_RunningProcessTimesOut(t *testing.T) {
	cmd := exec.Command("sleep", "60")
	require.NoError(t, cmd.Start())
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
	})

	require.False(t, waitForProcessExit(cmd.Process, 50*time.Millisecond),
		"expected false for a running process that exceeds timeout")
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

var (
	testErrNoSuchProcess   = errors.New("os: process already finished: no such process")
	testErrAlreadyFinished = errors.New("os: process already finished")
	testErrOther           = errors.New("permission denied")
)

func TestIsProcessGone(t *testing.T) {
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
			name:   "[ok] other error returns false",
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
	require.EqualError(t, err, "no such session")
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
