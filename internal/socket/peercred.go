//go:build linux

package socket

import (
	"fmt"
	"net"

	"golang.org/x/sys/unix"
)

// PeerCred holds the credentials of the connecting process.
type PeerCred struct {
	UID uint32
	GID uint32
	PID int32
}

// GetPeerCred extracts the UID, GID, and PID of the peer from a Unix socket connection.
// This uses SO_PEERCRED which is Linux-specific.
func GetPeerCred(conn net.Conn) (*PeerCred, error) {
	unixConn, ok := conn.(*net.UnixConn)
	if !ok {
		return nil, fmt.Errorf("connection is not a Unix socket")
	}

	raw, err := unixConn.SyscallConn()
	if err != nil {
		return nil, fmt.Errorf("failed to get raw connection: %w", err)
	}

	var cred *unix.Ucred
	var credErr error

	err = raw.Control(func(fd uintptr) {
		cred, credErr = unix.GetsockoptUcred(int(fd), unix.SOL_SOCKET, unix.SO_PEERCRED)
	})
	if err != nil {
		return nil, fmt.Errorf("failed to control socket: %w", err)
	}
	if credErr != nil {
		return nil, fmt.Errorf("failed to get peer credentials: %w", credErr)
	}

	return &PeerCred{
		UID: cred.Uid,
		GID: cred.Gid,
		PID: cred.Pid,
	}, nil
}
