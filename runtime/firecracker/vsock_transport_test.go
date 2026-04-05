package firecracker

import (
	"bufio"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestNewVsockHTTPClient_ReturnsNonNil(t *testing.T) {
	client := NewVsockHTTPClient("any.sock", 1234)
	if client == nil {
		t.Fatal("expected non-nil *http.Client, got nil")
	}
}

func TestNewVsockHTTPClient_TransportType(t *testing.T) {
	client := NewVsockHTTPClient("any.sock", 1234)
	if _, ok := client.Transport.(*http.Transport); !ok {
		t.Fatalf("expected *http.Transport, got %T", client.Transport)
	}
}

// startMockVsockUDS starts a Unix domain socket server that simulates the
// Firecracker vsock CONNECT protocol: reads "CONNECT <port>\n", writes
// "OK <port>\n", then serves a single HTTP response on the connection.
// Returns the UDS path. The listener is closed via t.Cleanup.
func startMockVsockUDS(t *testing.T, port uint32, handler func(conn net.Conn)) string {
	t.Helper()

	sockPath := filepath.Join(t.TempDir(), "test-vsock.sock")
	ln, err := net.Listen("unix", sockPath)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	t.Cleanup(func() { ln.Close() })

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed
			}
			go func(c net.Conn) {
				defer c.Close()

				// Read the CONNECT handshake from firecracker-go-sdk vsock.DialContext.
				reader := bufio.NewReader(c)
				line, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				expected := fmt.Sprintf("CONNECT %d\n", port)
				if line != expected {
					fmt.Fprintf(c, "ERR unexpected: %s", line)
					return
				}

				// Write the OK response.
				fmt.Fprintf(c, "OK %d\n", port)

				// Hand off to the test-specific handler.
				handler(c)
			}(conn)
		}
	}()

	return sockPath
}

func TestVsockHTTPClient_MockUDS_Success(t *testing.T) {
	sockPath := startMockVsockUDS(t, 44772, func(conn net.Conn) {
		// Read the HTTP request.
		reader := bufio.NewReader(conn)
		req, err := http.ReadRequest(reader)
		if err != nil {
			t.Errorf("read HTTP request: %v", err)
			return
		}
		_ = req.Body.Close()

		// Write a minimal HTTP response.
		resp := "HTTP/1.1 200 OK\r\nContent-Length: 2\r\n\r\nOK"
		fmt.Fprint(conn, resp)
	})

	client := NewVsockHTTPClient(sockPath, 44772)
	resp, err := client.Get("http://execd/health")
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestVsockHTTPClient_MockUDS_ConnectRefused(t *testing.T) {
	// Use a path that definitely does not exist.
	sockPath := filepath.Join(os.TempDir(), "nonexistent-vsock-test-12345.sock")

	client := NewVsockHTTPClient(sockPath, 44772)
	_, err := client.Get("http://execd/health")
	if err == nil {
		t.Fatal("expected error for non-existent UDS, got nil")
	}
	// The error should mention something about connection failure.
	errStr := strings.ToLower(err.Error())
	if !strings.Contains(errStr, "no such file") && !strings.Contains(errStr, "connect") &&
		!strings.Contains(errStr, "refused") && !strings.Contains(errStr, "dial") {
		t.Errorf("unexpected error text: %v", err)
	}
}
