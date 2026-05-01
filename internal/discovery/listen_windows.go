//go:build windows

package discovery

import (
	"context"
	"fmt"
	"net"
	"syscall"

	"golang.org/x/sys/windows"
)

func listenUDP(port int) (*net.UDPConn, error) {
	lc := net.ListenConfig{
		Control: func(network, address string, conn syscall.RawConn) error {
			var sockOptErr error
			if err := conn.Control(func(fd uintptr) {
				// Alpaca discovery is a shared UDP port; this lets compatible
				// Windows responders bind alongside each other.
				sockOptErr = windows.SetsockoptInt(windows.Handle(fd), windows.SOL_SOCKET, windows.SO_REUSEADDR, 1)
			}); err != nil {
				return err
			}
			return sockOptErr
		},
	}

	packetConn, err := lc.ListenPacket(context.Background(), "udp4", fmt.Sprintf(":%d", port))
	if err != nil {
		return nil, err
	}

	udpConn, ok := packetConn.(*net.UDPConn)
	if !ok {
		packetConn.Close()
		return nil, fmt.Errorf("unexpected UDP listener type %T", packetConn)
	}
	return udpConn, nil
}
