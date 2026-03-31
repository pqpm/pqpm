package socket

import (
	"encoding/json"
	"fmt"
	"net"
	"os"

	"github.com/pqpm/pqpm/internal/types"
)

const SocketPath = "/var/run/pqpm/pqpmd.sock"

// --- Server (Daemon) Side ---

// Listen creates and listens on the Unix domain socket.
func Listen() (net.Listener, error) {
	// Remove stale socket file if it exists
	if err := os.RemoveAll(SocketPath); err != nil {
		return nil, fmt.Errorf("failed to remove stale socket: %w", err)
	}

	// Ensure the directory exists
	if err := os.MkdirAll("/var/run/pqpm", 0755); err != nil {
		return nil, fmt.Errorf("failed to create socket directory: %w", err)
	}

	listener, err := net.Listen("unix", SocketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to listen on socket: %w", err)
	}

	// Allow all users to connect to the socket
	if err := os.Chmod(SocketPath, 0666); err != nil {
		listener.Close()
		return nil, fmt.Errorf("failed to set socket permissions: %w", err)
	}

	return listener, nil
}

// ReadRequest reads a DaemonRequest from a connection.
func ReadRequest(conn net.Conn) (*types.DaemonRequest, error) {
	decoder := json.NewDecoder(conn)
	var req types.DaemonRequest
	if err := decoder.Decode(&req); err != nil {
		return nil, fmt.Errorf("failed to decode request: %w", err)
	}
	return &req, nil
}

// WriteResponse writes a DaemonResponse to a connection.
func WriteResponse(conn net.Conn, resp *types.DaemonResponse) error {
	encoder := json.NewEncoder(conn)
	return encoder.Encode(resp)
}

// --- Client (CLI) Side ---

// Connect establishes a connection to the daemon's Unix socket.
func Connect() (net.Conn, error) {
	conn, err := net.Dial("unix", SocketPath)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to daemon (is pqpmd running?): %w", err)
	}
	return conn, nil
}

// SendRequest sends a request to the daemon and returns the response.
func SendRequest(req *types.DaemonRequest) (*types.DaemonResponse, error) {
	conn, err := Connect()
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	encoder := json.NewEncoder(conn)
	if err := encoder.Encode(req); err != nil {
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	decoder := json.NewDecoder(conn)
	var resp types.DaemonResponse
	if err := decoder.Decode(&resp); err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}
	return &resp, nil
}
