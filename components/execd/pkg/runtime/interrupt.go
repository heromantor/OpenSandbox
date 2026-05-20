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

	"github.com/alibaba/opensandbox/internal/safego"

	"github.com/alibaba/opensandbox/execd/pkg/log"
)

// Interrupt stops execution in the specified session.
func (c *Controller) Interrupt(sessionID string) error {
	switch {
	case c.getJupyterKernel(sessionID) != nil:
		kernel := c.getJupyterKernel(sessionID)
		log.Warning("Interrupting Jupyter kernel %s", kernel.kernelID)
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
	log.Warning("Attempting to terminate process group %d", pid)

	// Send SIGTERM to the whole process group.
	if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
		if isProcessGone(err) {
			return nil
		}

		log.Warning("SIGTERM failed for process group %d: %v, trying SIGKILL", pid, err)
	} else {
		// Wait for the process leader to exit after SIGTERM.
		// On Unix, FindProcess always succeeds; check existence via Signal(0).
		process, err := os.FindProcess(pid)
		switch {
		case err != nil:
			log.Warning("Process group %d did not terminate after SIGTERM, using SIGKILL: %v", pid, err)
		default:
			if waitForProcessExit(process, 3*time.Second) {
				log.Info("Process group %d terminated gracefully", pid)
				return nil
			}
		}
	}

	// Send SIGKILL to the whole process group.
	if err := syscall.Kill(-pid, syscall.SIGKILL); err != nil {
		if isProcessGone(err) {
			log.Info("Process group %d confirmed terminated", pid)
			return nil
		}

		return fmt.Errorf("failed to kill process group %d: %w", pid, err)
	}

	// Verify the process leader has exited.
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("os.FindProcess for group %d returned unexpected error: %w", pid, err)
	}
	if ok := confirmProcessTerminated(process, "process group"); ok {
		return nil
	}

	return fmt.Errorf("process group %d might still be running", pid)
}

// killProcessOnly sends SIGTERM followed by SIGKILL to a single process.
func (c *Controller) killProcessOnly(pid int) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	log.Warning("Attempting to terminate process %d", pid)

	if err := process.Signal(syscall.SIGTERM); err != nil {
		if isProcessGone(err) {
			return nil
		}

		log.Warning("SIGTERM failed for pid %d: %v, trying SIGKILL", pid, err)
	} else {
		if waitForProcessExit(process, 3*time.Second) {
			log.Info("Process %d terminated gracefully", pid)
			return nil
		}

		log.Warning("Process %d did not terminate after SIGTERM, using SIGKILL", pid)
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
// signal 0 in a loop. Returns true if the process is confirmed dead.
func confirmProcessTerminated(process *os.Process, label string) bool {
	for range 3 {
		if err := process.Signal(syscall.Signal(0)); err != nil {
			if isProcessGone(err) {
				log.Info("%s %d confirmed terminated", label, process.Pid)
				return true
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return false
}

// isProcessGone returns true if the error indicates that the target process
// has already exited ("no such process" or "already finished").
func isProcessGone(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "no such process") ||
		strings.Contains(msg, "already finished")
}

// waitForProcessExit waits for a process to exit within the given timeout.
// Returns true if the process exited cleanly (Wait returned nil error).
func waitForProcessExit(process *os.Process, timeout time.Duration) bool {
	done := make(chan error, 1)
	safego.Go(func() {
		_, err := process.Wait()
		done <- err
	})

	select {
	case err := <-done:
		return err == nil
	case <-time.After(timeout):
		return false
	}
}
