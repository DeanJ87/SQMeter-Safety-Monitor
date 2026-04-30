package discovery_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net"
	"os"
	"testing"
	"time"

	"sqmeter-alpaca-safetymonitor/internal/discovery"
)

func TestDiscovery_RespondsWithAlpacaPort(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	// Port 0 lets the OS pick a free port, but Responder doesn't expose that.
	// Use a fixed high port unlikely to conflict in tests.
	const discPort = 32277
	const httpPort = 11111

	resp := discovery.New(discPort, httpPort, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- resp.Run(ctx)
	}()

	reply := waitForDiscoveryReply(t, "127.0.0.1:32277", errCh)

	if got := reply["AlpacaPort"]; got != httpPort {
		t.Errorf("AlpacaPort: want %d, got %d", httpPort, got)
	}
}

func TestDiscovery_GetStatus_HealthyAfterBind(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	const discPort = 32279

	resp := discovery.New(discPort, 11111, logger)

	// Before Run: not running, not healthy.
	s := resp.GetStatus()
	if s.Running || s.Healthy {
		t.Errorf("before Run: want running=false healthy=false, got running=%v healthy=%v", s.Running, s.Healthy)
	}
	if s.ConfiguredPort != discPort {
		t.Errorf("ConfiguredPort: want %d, got %d", discPort, s.ConfiguredPort)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() { errCh <- resp.Run(ctx) }()

	// Wait until listener is up by sending a probe.
	waitForDiscoveryReply(t, "127.0.0.1:32279", errCh)

	s = resp.GetStatus()
	if !s.Running {
		t.Error("after bind: want running=true")
	}
	if !s.Healthy {
		t.Error("after bind: want healthy=true")
	}
	if s.LastError != "" {
		t.Errorf("after bind: want no error, got %q", s.LastError)
	}
	if s.ResponseCount < 1 {
		t.Errorf("after probe: want ResponseCount >= 1, got %d", s.ResponseCount)
	}
	if s.LastRequestAt == nil {
		t.Error("after probe: want LastRequestAt set")
	}
	if s.LastResponseAt == nil {
		t.Error("after probe: want LastResponseAt set")
	}
}

func TestDiscovery_GetStatus_UnhealthyOnBindFailure(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	const discPort = 32280

	// Occupy the port so the responder cannot bind.
	occupier, err := net.ListenUDP("udp4", &net.UDPAddr{Port: discPort})
	if err != nil {
		t.Fatalf("could not occupy port %d for test: %v", discPort, err)
	}
	defer occupier.Close()

	resp := discovery.New(discPort, 11111, logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	runErr := resp.Run(ctx)
	if runErr == nil {
		t.Fatal("expected Run to return an error when port is occupied")
	}

	s := resp.GetStatus()
	if s.Running {
		t.Error("after bind failure: want running=false")
	}
	if s.Healthy {
		t.Error("after bind failure: want healthy=false")
	}
	if s.LastError == "" {
		t.Error("after bind failure: want LastError set")
	}
}

func TestDiscovery_GetStatus_RunningFalseAfterStop(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	const discPort = 32281

	resp := discovery.New(discPort, 11111, logger)
	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() { errCh <- resp.Run(ctx) }()

	waitForDiscoveryReply(t, "127.0.0.1:32281", errCh)

	cancel()
	select {
	case <-errCh:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not return after context cancel")
	}

	s := resp.GetStatus()
	if s.Running {
		t.Error("after stop: want running=false")
	}
	// Healthy stays true — the listener was healthy, just stopped cleanly.
	if !s.Healthy {
		t.Error("after clean stop: want healthy=true (was healthy, stopped cleanly)")
	}
}

func TestDiscovery_IgnoresUnrelatedPackets(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	const discPort = 32278

	resp := discovery.New(discPort, 11111, logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- resp.Run(ctx)
	}()
	waitForDiscoveryReply(t, "127.0.0.1:32278", errCh)

	conn, err := net.Dial("udp4", "127.0.0.1:32278")
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(300 * time.Millisecond))
	conn.Write([]byte("notadiscoverypacket"))

	buf := make([]byte, 256)
	_, err = conn.Read(buf)
	if err == nil {
		t.Error("expected no response to unrelated packet, but got one")
	}
	// A deadline/timeout error is expected and correct here.
}

func waitForDiscoveryReply(t *testing.T, addr string, errCh <-chan error) map[string]int {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	var lastErr error
	for time.Now().Before(deadline) {
		select {
		case err := <-errCh:
			t.Fatalf("discovery responder exited before it was ready: %v", err)
		default:
		}

		reply, err := readDiscoveryReply(addr, 100*time.Millisecond)
		if err == nil {
			return reply
		}
		lastErr = err
	}

	t.Fatalf("timed out waiting for discovery reply: %v", lastErr)
	return nil
}

func readDiscoveryReply(addr string, timeout time.Duration) (map[string]int, error) {
	conn, err := net.Dial("udp4", addr)
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if err := conn.SetDeadline(time.Now().Add(timeout)); err != nil {
		return nil, err
	}
	if _, err := conn.Write([]byte("alpacadiscovery1")); err != nil {
		return nil, err
	}

	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil {
		return nil, err
	}

	var reply map[string]int
	if err := json.Unmarshal(buf[:n], &reply); err != nil {
		return nil, err
	}
	return reply, nil
}
