//go:build windows
// +build windows

package socketprovider

import (
	"errors"
	"net"
	"time"

	"github.com/Microsoft/go-winio"
)

func dialSocket(socket Socket) (net.Conn, error) {
	if socket.Npipe == "" {
		return nil, errors.New("unsupported socket type")
	}

	dur := time.Second
	return winio.DialPipe(socket.Npipe, &dur)
}
