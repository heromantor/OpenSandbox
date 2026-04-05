package firecracker

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

// WaitForExecd blocks until the execd agent inside the guest responds to a
// health check over vsock, or the context is canceled/timed out.
//
// It creates an HTTP client routed through the vsock UDS and polls
// GET http://execd/health every 200ms. The health check succeeds
// when execd returns HTTP 200.
//
// The caller should pass a context with timeout to bound the wait:
//
//	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
//	defer cancel()
//	if err := WaitForExecd(ctx, "/tmp/firecracker-vsock-abc.sock"); err != nil {
//	    // execd not reachable
//	}
func WaitForExecd(ctx context.Context, vsockUDSPath string) error {
	client := NewVsockHTTPClient(vsockUDSPath, ExecdPort)

	const pollInterval = 200 * time.Millisecond
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	var lastErr error
	for {
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return fmt.Errorf("execd not ready: %w (last: %v)", ctx.Err(), lastErr)
			}
			return fmt.Errorf("execd not ready: %w", ctx.Err())
		case <-ticker.C:
			if err := pingExecd(ctx, client); err != nil {
				lastErr = err
				continue
			}
			return nil
		}
	}
}

// pingExecd sends a single HTTP GET /health request to execd and returns nil
// if the response is 200 OK.
func pingExecd(ctx context.Context, client *http.Client) error {
	reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, "GET", "http://execd/health", nil)
	if err != nil {
		return fmt.Errorf("create health request: %w", err)
	}

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("health check request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check: unexpected status %d", resp.StatusCode)
	}
	return nil
}
