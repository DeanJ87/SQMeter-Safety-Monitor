//go:build !windows

package discovery

import "net"

func listenUDP(port int) (*net.UDPConn, error) {
	return net.ListenUDP("udp4", &net.UDPAddr{Port: port})
}
