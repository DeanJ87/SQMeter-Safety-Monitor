package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

const discoveryMessage = "alpacadiscovery1"

// Status describes the current health of the discovery listener.
type Status struct {
	ConfiguredPort int        `json:"configured_port"`
	Running        bool       `json:"running"`
	Healthy        bool       `json:"healthy"`
	LastError      string     `json:"last_error,omitempty"`
	LastRequestAt  *time.Time `json:"last_request_at"`
	LastResponseAt *time.Time `json:"last_response_at"`
	ResponseCount  int64      `json:"response_count"`
}

// Responder listens on UDP and answers ASCOM Alpaca discovery broadcasts.
type Responder struct {
	discoveryPort int
	httpPort      int
	logger        *slog.Logger

	mu     sync.RWMutex
	status Status
}

// New creates a Responder that will advertise httpPort on discoveryPort.
func New(discoveryPort, httpPort int, logger *slog.Logger) *Responder {
	return &Responder{
		discoveryPort: discoveryPort,
		httpPort:      httpPort,
		logger:        logger,
		status:        Status{ConfiguredPort: discoveryPort},
	}
}

// GetStatus returns a snapshot of the current discovery listener health.
func (r *Responder) GetStatus() Status {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.status
}

// Run starts the UDP listener and blocks until ctx is cancelled.
func (r *Responder) Run(ctx context.Context) error {
	conn, err := listenUDP(r.discoveryPort)
	if err != nil {
		bindErr := fmt.Errorf("discovery: listen UDP :%d: %w", r.discoveryPort, err)
		r.mu.Lock()
		r.status.Running = false
		r.status.Healthy = false
		r.status.LastError = bindErr.Error()
		r.mu.Unlock()
		return bindErr
	}
	defer conn.Close()

	r.mu.Lock()
	r.status.Running = true
	r.status.Healthy = true
	r.status.LastError = ""
	r.mu.Unlock()

	r.logger.Info("alpaca discovery listening", "udp_port", r.discoveryPort)

	go func() {
		<-ctx.Done()
		conn.Close() /* #nosec G104 -- closing UDP socket on shutdown; error cannot be meaningfully handled here */ //nolint:errcheck
	}()

	reply, _ := json.Marshal(map[string]int{"AlpacaPort": r.httpPort})
	buf := make([]byte, 512)

	for {
		n, remote, err := conn.ReadFromUDP(buf)
		if err != nil {
			select {
			case <-ctx.Done():
				r.mu.Lock()
				r.status.Running = false
				r.mu.Unlock()
				return nil
			default:
				r.logger.Warn("discovery: read error", "error", err)
				continue
			}
		}

		msg := strings.TrimSpace(string(buf[:n]))
		if msg == discoveryMessage {
			now := time.Now().UTC()
			r.mu.Lock()
			r.status.LastRequestAt = &now
			r.mu.Unlock()

			r.logger.Debug("discovery: received probe", "from", remote)
			if _, werr := conn.WriteToUDP(reply, remote); werr != nil {
				r.logger.Warn("discovery: write error", "error", werr)
			}
			sent := time.Now().UTC()
			r.mu.Lock()
			r.status.LastResponseAt = &sent
			r.status.ResponseCount++
			r.mu.Unlock()
		}
	}
}
