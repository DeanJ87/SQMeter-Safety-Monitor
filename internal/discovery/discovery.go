package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"strings"
)

const discoveryMessage = "alpacadiscovery1"

// Responder listens on UDP and answers ASCOM Alpaca discovery broadcasts.
type Responder struct {
	discoveryPort int
	httpPort      int
	logger        *slog.Logger
}

// New creates a Responder that will advertise httpPort on discoveryPort.
func New(discoveryPort, httpPort int, logger *slog.Logger) *Responder {
	return &Responder{
		discoveryPort: discoveryPort,
		httpPort:      httpPort,
		logger:        logger,
	}
}

// Run starts the UDP listener and blocks until ctx is cancelled.
func (r *Responder) Run(ctx context.Context) error {
	addr := &net.UDPAddr{Port: r.discoveryPort}
	conn, err := net.ListenUDP("udp4", addr)
	if err != nil {
		return fmt.Errorf("discovery: listen UDP :%d: %w", r.discoveryPort, err)
	}
	defer conn.Close()

	r.logger.Info("alpaca discovery listening", "udp_port", r.discoveryPort)

	go func() {
		<-ctx.Done()
		conn.Close()
	}()

	reply, _ := json.Marshal(map[string]int{"AlpacaPort": r.httpPort})
	buf := make([]byte, 512)

	for {
		n, remote, err := conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-ctx.Done():
				return nil
			default:
				r.logger.Warn("discovery: read error", "error", err)
				continue
			}
		}

		msg := strings.TrimSpace(string(buf[:n]))
		if msg == discoveryMessage {
			r.logger.Debug("discovery: received probe", "from", remote)
			if _, werr := conn.WriteToUDP(reply, remote); werr != nil {
				r.logger.Warn("discovery: write error", "error", werr)
			}
		}
	}
}
