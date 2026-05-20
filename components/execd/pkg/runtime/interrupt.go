// Copyright 2025 Alibaba Group Holding Ltd.
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
// +build !windows

package runtime

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/alibaba/opensandbox/execd/pkg/log"
)

// Interrupt stops execution in the specified session.
func (c *Controller) Interrupt(sessionID string) error {
	switch {
	case c.getJupyterKernel(sessionID) != nil:
		kernel := c.getJupyterKernel(sessionID)
		log.Info("Interrupting Jupyter kernel %s", kernel.kernelID)
		return kernel.client.InterruptKernel(kernel.kernelID)
	case c.getCommandKernel(sessionID) != nil:
		kernel := c.getCommandKernel(sessionID)
		return c.killPid(kernel.pid)
	case c.getBashSession(sessionID) != nil:
		return c.closeBashSession(sessionID)
	default:
		return errors.New("no such session")
	}
}

// killPid sends SIGTERM followed by SIGKILL to the process or its group.
// If the process is a group leader (PGID == PID), signals are sent to the
// entire group via syscall.Kill(-pid, sig). Otherwise signals target only
// the individual process via process.Signal(sig).
func (c *Controller) killPid(pid int) error {
	pgid, err := syscall.Getpgid(pid)
	if err != nil {
		if errors.Is(err, syscall.ESRCH) {
			return nil // process already exited
		}
		return fmt.Errorf("get process group: %w", err)
	}

	if pgid == pid {
		return c.killProcessGroup(pid)
	}
	return c.killProcessOnly(pid)
}

// killProcessGroup sends SIGTERM followed by SIGKILL to the entire process group.
func (c *Controller) killProcessGroup(pid int) error {
	return killProcessGroupGraceful(pid, 3*time.Second)
}

// killProcessOnly sends SIGTERM followed by SIGKILL to a single process.
func (c *Controller) killProcessOnly(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	log.Info("Attempting to terminate process %d", pid)

	if err := process.Signal(syscall.SIGTERM); err != nil {
		if isProcessGone(err) {
			return nil
		}

		log.Info("SIGTERM failed for pid %d: %v, trying SIGKILL", pid, err)
	} else {
		exited, waitErr := waitForProcessExit(process, 3*time.Second)
		if waitErr != nil {
			log.Warning("wait for process %d exit: %v", pid, waitErr)
		}
		if exited {
			log.Info("Process %d terminated gracefully", pid)
			return nil
		}

		log.Info("Process %d did not terminate after SIGTERM, using SIGKILL", pid)
	}

	if err := process.Signal(syscall.SIGKILL); err != nil {
		if isProcessGone(err) {
			return nil
		}

		return fmt.Errorf("failed to kill process %d: %w", pid, err)
	}

	if ok := confirmProcessTerminated(process, "process"); ok {
		return nil
	}

	return fmt.Errorf("process %d might still be running", pid)
}

// confirmProcessTerminated checks that a process has exited by sending
// signal 0 in a loop. Returns true if the process is confirmed dead or
// is a zombie (its parent hasn't called Wait yet).
func confirmProcessTerminated(process *os.Process, label string) bool {
	for range 3 {
		gone, _ := isProcessDeadOrZombie(process.Pid)
		if gone {
			log.Info("%s %d confirmed terminated", label, process.Pid)
			return true
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// isProcessDeadOrZombie checks whether a process is either dead or a zombie.
// Signal(0) succeeds for zombies, so we also check /proc/<pid>/stat state.
func isProcessDeadOrZombie(pid int) (bool, error) {
	process, err := os.FindProcess(pid)
	if err != nil {
		return false, fmt.Errorf("find process %d: %w", pid, err)
	}
	if err := process.Signal(syscall.Signal(0)); err != nil {
		if isProcessGone(err) {
			return true, nil
		}
		return false, fmt.Errorf("signal process %d: %w", pid, err)
	}
	// Signal(0) succeeded — process exists. Check if it's a zombie.
	return isZombie(pid)
}

// isZombie reads /proc/<pid>/stat and returns true if the process state is 'Z' (zombie).
func isZombie(pid int) (bool, error) {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/stat", pid))
	if err != nil {
		return false, fmt.Errorf("read /proc/%d/stat: %w", pid, err)
	}
	// Format: pid (comm) state ...
	// Find the closing ')' to skip the comm field which may contain spaces and ')' itself.
	closeParen := strings.LastIndex(string(data), ")")
	if closeParen < 0 || closeParen+2 >= len(data) {
		return false, fmt.Errorf("unexpected /proc/%d/stat format", pid)
	}
	state := data[closeParen+2]
	return state == 'Z', nil
}

// isProcessGone returns true if the error indicates that the target process
// has already exited. Uses typed checks (ESRCH, ErrProcessDone) first,
// with string matching as a fallback for wrapped errors.
func isProcessGone(err error) bool {
	if errors.Is(err, syscall.ESRCH) {
		return true
	}
	if errors.Is(err, os.ErrProcessDone) {
		return true
	}
	// Fallback for wrapped errors with non-standard formatting
	msg := err.Error()
	return strings.Contains(msg, "no such process") ||
		strings.Contains(msg, "already finished")
}

// waitForProcessExit waits for a process to exit within the given timeout.
// Uses Signal(0) polling instead of process.Wait() to avoid conflicting
// with cmd.Wait() callers. Returns true if the process exited within
// the timeout (including killed-by-signal and zombie cases).
func waitForProcessExit(process *os.Process, timeout time.Duration) (bool, error) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		gone, err := isProcessDeadOrZombie(process.Pid)
		if err != nil {
			return false, fmt.Errorf("check process %d liveness: %w", process.Pid, err)
		}
		if gone {
			return true, nil
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false, nil
}

// killProcessGroupImmediate sends SIGKILL to the process group.
// Expected errors (ESRCH, process already finished) are suppressed.
func killProcessGroupImmediate(pid int) {
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil && !isProcessGone(err) {
		log.Warning("kill process group %d: %v", pid, err)
	}
}

// killProcessGroupGraceful sends SIGTERM to the group, waits for gracePeriod,
// then sends SIGKILL. Returns nil if the group terminated.
func killProcessGroupGraceful(pid int, gracePeriod time.Duration) error {
	// SIGTERM to the group — bash forwards the signal to children and waits for them
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
		if isProcessGone(err) {
			return nil
		}
		log.Info("SIGTERM failed for process group %d: %v, trying SIGKILL", pid, err)
	} else {
		// Wait for the group to terminate after SIGTERM
		process, findErr := os.FindProcess(pid)
		if findErr != nil {
			return fmt.Errorf("find process %d: %w", pid, findErr)
		}
		exited, waitErr := waitForProcessExit(process, gracePeriod)
		if waitErr != nil {
			return fmt.Errorf("wait for process group %d exit: %w", pid, waitErr)
		}
		if exited {
			log.Info("Process group %d terminated gracefully", pid)
			return nil
		}
		log.Info("Process group %d did not terminate after SIGTERM, using SIGKILL", pid)
	}
	// SIGKILL to the group — guaranteed kill
	killProcessGroupImmediate(pid)
	return nil
}
