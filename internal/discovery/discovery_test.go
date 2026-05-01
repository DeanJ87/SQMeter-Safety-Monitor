package discovery_test

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"runtime"
	"testing"
	"time"

	"sqmeter-alpaca-safetymonitor/internal/discovery"
)

func TestDiscovery_RespondsWithAlpacaPort(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))

	discPort := freeUDPPort(t)
	const httpPort = 11111

	resp := discovery.New(discPort, httpPort, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- resp.Run(ctx)
	}()

	reply := waitForDiscoveryReply(t, fmt.Sprintf("127.0.0.1:%d", discPort), errCh)

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
	discPort := freeUDPPort(t)

	resp := discovery.New(discPort, 11111, logger)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 1)
	go func() {
		errCh <- resp.Run(ctx)
	}()
	waitForDiscoveryReply(t, fmt.Sprintf("127.0.0.1:%d", discPort), errCh)

	conn, err := net.Dial("udp4", fmt.Sprintf("127.0.0.1:%d", discPort))
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

func TestDiscovery_AllowsSharedBindOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("shared Alpaca discovery bind is Windows-specific")
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	discPort := freeUDPPort(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	errCh := make(chan error, 2)
	for _, httpPort := range []int{32323, 11111} {
		resp := discovery.New(discPort, httpPort, logger)
		go func() {
			errCh <- resp.Run(ctx)
		}()
	}

	// Give both responders time to start listening before querying
	time.Sleep(100 * time.Millisecond)

	replies := waitForDiscoveryReplies(t, fmt.Sprintf("127.0.0.1:%d", discPort), errCh, 2)
	for _, want := range []int{32323, 11111} {
		if !replies[want] {
			t.Fatalf("missing AlpacaPort %d in replies: %v", want, replies)
		}
	}
}

func TestDiscovery_RejectsSecondBindOnNonWindows(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows permits shared Alpaca discovery binds")
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
	discPort := freeUDPPort(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	firstErrCh := make(chan error, 1)
	go func() {
		firstErrCh <- discovery.New(discPort, 11111, logger).Run(ctx)
	}()
	waitForDiscoveryReply(t, fmt.Sprintf("127.0.0.1:%d", discPort), firstErrCh)

	secondErrCh := make(chan error, 1)
	go func() {
		secondErrCh <- discovery.New(discPort, 32323, logger).Run(ctx)
	}()

	select {
	case err := <-secondErrCh:
		if err == nil {
			t.Fatal("expected second discovery responder to fail binding")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for second discovery responder bind failure")
	}
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

func freeUDPPort(t *testing.T) int {
	t.Helper()

	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatalf("reserve UDP port: %v", err)
	}
	defer conn.Close()

	return conn.LocalAddr().(*net.UDPAddr).Port
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

func waitForDiscoveryReplies(t *testing.T, addr string, errCh <-chan error, want int) map[int]bool {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	replies := make(map[int]bool)
	var lastErr error

	// With SO_REUSEADDR on Windows, multiple listeners on the same port means
	// each UDP packet goes to ONE listener (OS load balances). We need to send
	// multiple queries from different source ports to hit all responders.
	for time.Now().Before(deadline) {
		select {
		case err := <-errCh:
			t.Fatalf("discovery responder exited before it was ready: %v", err)
		default:
		}

		// Send a fresh discovery query - new connection = new source port
		// This gives OS a chance to route to a different responder
		reply, err := readDiscoveryReply(addr, 100*time.Millisecond)
		if err != nil {
			lastErr = err
		} else {
			replies[reply["AlpacaPort"]] = true
		}

		if len(replies) >= want {
			return replies
		}

		// Brief pause between query attempts
		time.Sleep(50 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for discovery replies: got %v, last error: %v", replies, lastErr)
	return nil
}

func readDiscoveryReplies(addr string, timeout time.Duration) (map[int]bool, error) {
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

	replies := make(map[int]bool)
	buf := make([]byte, 256)
	for {
		n, err := conn.Read(buf)
		if err != nil {
			if len(replies) > 0 {
				return replies, nil
			}
			return nil, err
		}

		var reply map[string]int
		if err := json.Unmarshal(buf[:n], &reply); err != nil {
			return nil, err
		}
		replies[reply["AlpacaPort"]] = true
	}
}
