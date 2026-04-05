package firecracker

import (
	"context"
	"net"
	"net/http"

	fcvsock "github.com/firecracker-microvm/firecracker-go-sdk/vsock"
)

// NewVsockHTTPClient creates an HTTP client that routes all requests through
// a Firecracker vsock Unix domain socket. The client performs the CONNECT
// handshake to the specified guest port on every connection, then speaks
// standard HTTP over the resulting net.Conn.
//
// The returned client is safe for concurrent use. Each request opens a new
// vsock connection (no persistent connection pooling across requests).
//
// Usage:
//
//	client := NewVsockHTTPClient("/tmp/firecracker-vsock-abc.sock", 44772)
//	resp, err := client.Get("http://execd/health")
func NewVsockHTTPClient(udsPath string, guestPort uint32) *http.Client {
	return &http.Client{
		Transport: &http.Transport{
			DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
				return fcvsock.DialContext(ctx, udsPath, guestPort)
			},
		},
	}
}
