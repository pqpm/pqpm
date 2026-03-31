//go:build !linux

package socket

import (
	"fmt"
	"net"
)

// PeerCred holds the credentials of the connecting process.
type PeerCred struct {
	UID uint32
	GID uint32
	PID int32
}

// GetPeerCred is not supported on non-Linux platforms.
func GetPeerCred(conn net.Conn) (*PeerCred, error) {
	return nil, fmt.Errorf("peer credential extraction is only supported on Linux")
}
