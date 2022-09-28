//go:build !windows
// +build !windows

package socketprovider

import (
	"errors"
	"net"
	"time"
)

func dialSocket(socket Socket) (net.Conn, error) {
	if socket.Unix == "" {
		return nil, errors.New("unsupported socket type")
	}

	return net.DialTimeout("unix", socket.Unix, time.Second)
}
