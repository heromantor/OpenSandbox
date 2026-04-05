package firecracker

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// startMockExecd starts a Unix domain socket server that simulates the
// Firecracker vsock CONNECT protocol and serves HTTP responses via the
// provided handler. Returns the UDS path.
func startMockExecd(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()

	// Use os.CreateTemp in /tmp to keep the socket path under the 108-char
	// limit imposed by Unix domain sockets on macOS/Linux.
	f, err := os.CreateTemp("", "execd-test-*.sock")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	sockPath := f.Name()
	f.Close()
	os.Remove(sockPath) // net.Listen needs a free path

	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	t.Cleanup(func() {
		ln.Close()
		os.Remove(sockPath)
	})

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed
			}
			go func(c net.Conn) {
				defer c.Close()

				// Read the CONNECT handshake from fcvsock.DialContext.
				reader := bufio.NewReader(c)
				line, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				expected := fmt.Sprintf("CONNECT %d\n", ExecdPort)
				if line != expected {
					fmt.Fprintf(c, "ERR unexpected: %s", line)
					return
				}

				// Write the OK response (e.g., "OK 44772\n").
				fmt.Fprintf(c, "OK %d\n", ExecdPort)

				// Read the HTTP request and serve the response.
				req, err := http.ReadRequest(reader)
				if err != nil {
					return
				}
				// Create a minimal ResponseWriter.
				rw := &connResponseWriter{conn: c, header: make(http.Header)}
				handler.ServeHTTP(rw, req)
			}(conn)
		}
	}()

	return sockPath
}

// connResponseWriter is a minimal http.ResponseWriter over a raw net.Conn.
type connResponseWriter struct {
	conn       net.Conn
	header     http.Header
	statusCode int
	written    bool
}

func (w *connResponseWriter) Header() http.Header { return w.header }

func (w *connResponseWriter) WriteHeader(code int) {
	if w.written {
		return
	}
	w.statusCode = code
	w.written = true
	fmt.Fprintf(w.conn, "HTTP/1.1 %d %s\r\n", code, http.StatusText(code))
	for k, vs := range w.header {
		for _, v := range vs {
			fmt.Fprintf(w.conn, "%s: %s\r\n", k, v)
		}
	}
	fmt.Fprintf(w.conn, "\r\n")
}

func (w *connResponseWriter) Write(b []byte) (int, error) {
	if !w.written {
		w.WriteHeader(http.StatusOK)
	}
	return w.conn.Write(b)
}

func TestWaitForExecd_Success(t *testing.T) {
	sockPath := startMockExecd(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err := WaitForExecd(ctx, sockPath)
	if err != nil {
		t.Fatalf("expected nil error, got: %v", err)
	}
}

func TestWaitForExecd_RetryThenSuccess(t *testing.T) {
	var callCount int64

	sockPath := startMockExecd(t, func(w http.ResponseWriter, r *http.Request) {
		n := atomic.AddInt64(&callCount, 1)
		if n <= 2 {
			w.WriteHeader(http.StatusServiceUnavailable)
			w.Write([]byte("not ready"))
			return
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	err := WaitForExecd(ctx, sockPath)
	if err != nil {
		t.Fatalf("expected nil error after retries, got: %v", err)
	}

	finalCount := atomic.LoadInt64(&callCount)
	if finalCount < 3 {
		t.Errorf("expected at least 3 calls, got %d", finalCount)
	}
}

func TestWaitForExecd_ContextCanceled(t *testing.T) {
	// No mock server -- use a path with no listener.
	f, err := os.CreateTemp("", "execd-cancel-*.sock")
	if err != nil {
		t.Fatalf("create temp file: %v", err)
	}
	sockPath := f.Name()
	f.Close()
	os.Remove(sockPath)
	// No listener on this path -- connection attempts will fail.

	ctx, cancel := context.WithCancel(context.Background())
	// Cancel immediately in a goroutine to give WaitForExecd one tick.
	go func() {
		time.Sleep(100 * time.Millisecond)
		cancel()
	}()

	waitErr := WaitForExecd(ctx, sockPath)
	if waitErr == nil {
		t.Fatal("expected error, got nil")
	}
	errStr := strings.ToLower(waitErr.Error())
	if !strings.Contains(errStr, "context") {
		t.Errorf("expected error containing 'context', got: %v", waitErr)
	}
}

func TestWaitForExecd_ContextTimeout(t *testing.T) {
	// No mock server -- use a non-existent path.
	sockPath := "/tmp/nonexistent-execd-health-test-99999.sock"

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := WaitForExecd(ctx, sockPath)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "execd not ready") {
		t.Errorf("expected 'execd not ready' in error, got: %v", err)
	}
}

func TestWaitForExecd_Non200Status(t *testing.T) {
	sockPath := startMockExecd(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("error"))
	})

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	err := WaitForExecd(ctx, sockPath)
	if err == nil {
		t.Fatal("expected error for persistent 500, got nil")
	}
	errStr := err.Error()
	if !strings.Contains(errStr, "execd not ready") {
		t.Errorf("expected 'execd not ready' in error, got: %v", err)
	}
}
