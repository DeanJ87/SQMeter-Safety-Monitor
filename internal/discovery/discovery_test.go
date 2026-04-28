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

	started := make(chan struct{})
	go func() {
		// Signal after a short delay so the listener is up.
		time.Sleep(30 * time.Millisecond)
		close(started)
		resp.Run(ctx)
	}()
	<-started

	// Send discovery probe.
	conn, err := net.Dial("udp4", "127.0.0.1:32277")
	if err != nil {
		t.Fatalf("dial UDP: %v", err)
	}
	defer conn.Close()

	conn.SetDeadline(time.Now().Add(2 * time.Second))
	if _, err := conn.Write([]byte("alpacadiscovery1")); err != nil {
		t.Fatalf("write probe: %v", err)
	}

	buf := make([]byte, 256)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	var reply map[string]int
	if err := json.Unmarshal(buf[:n], &reply); err != nil {
		t.Fatalf("parse response %q: %v", buf[:n], err)
	}

	if got := reply["AlpacaPort"]; got != httpPort {
		t.Errorf("AlpacaPort: want %d, got %d", httpPort, got)
	}
}

func TestDiscovery_IgnoresUnrelatedPackets(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	const discPort = 32278

	resp := discovery.New(discPort, 11111, logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		time.Sleep(30 * time.Millisecond)
		resp.Run(ctx)
	}()
	time.Sleep(60 * time.Millisecond)

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
