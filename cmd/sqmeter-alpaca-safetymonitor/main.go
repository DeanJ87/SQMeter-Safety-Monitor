package main

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/kardianos/service"
	"sqmeter-alpaca-safetymonitor/internal/alpaca"
	"sqmeter-alpaca-safetymonitor/internal/config"
	"sqmeter-alpaca-safetymonitor/internal/discovery"
	"sqmeter-alpaca-safetymonitor/internal/poller"
	"sqmeter-alpaca-safetymonitor/internal/state"
	"sqmeter-alpaca-safetymonitor/internal/web"
)

// Injected at build time via -ldflags.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

// ---------- service wrapper --------------------------------------------------

type program struct {
	cfgPath  string
	uuidPath string

	cancel  context.CancelFunc
	stopped chan struct{}
}

func (p *program) Start(s service.Service) error {
	ctx, cancel := context.WithCancel(context.Background())
	p.cancel = cancel
	p.stopped = make(chan struct{})
	go p.run(ctx, service.Interactive())
	return nil
}

func (p *program) Stop(_ service.Service) error {
	if p.cancel != nil {
		p.cancel()
	}
	if p.stopped == nil {
		return nil
	}
	select {
	case <-p.stopped:
	case <-time.After(10 * time.Second):
		return fmt.Errorf("service did not stop within timeout")
	}
	return nil
}

func (p *program) run(ctx context.Context, interactive bool) {
	defer close(p.stopped)

	_, statErr := os.Stat(p.cfgPath)
	isFirstRun := os.IsNotExist(statErr)

	cfg, err := config.Load(p.cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		return
	}

	logger := newLogger(cfg.LogLevel)
	logger.Info("starting sqmeter-alpaca-safetymonitor",
		"version", version,
		"sqmeter_url", cfg.SQMeterBaseURL,
		"http_port", cfg.AlpacaHTTPPort,
		"discovery_port", cfg.AlpacaDiscoveryPort,
		"bind", cfg.AlpacaHTTPBind,
	)

	if config.IsWideOpen(cfg.AlpacaHTTPBind) {
		logger.Warn("service is bound to a non-loopback address; reachable from the network",
			"bind", cfg.AlpacaHTTPBind,
			"note", "do not expose this port to the internet")
	}
	if cfg.ManualOverride == "force_safe" {
		logger.Warn("MANUAL_OVERRIDE=force_safe — all safety rules are bypassed")
	}

	cfgHolder := config.NewHolder(cfg, p.cfgPath)

	deviceUUID, err := loadOrCreateUUID(p.uuidPath)
	if err != nil {
		logger.Warn("could not persist device UUID, using ephemeral UUID", "error", err)
	}
	logger.Info("device UUID", "uuid", deviceUUID)

	stateHolder := state.NewHolder(cfg.ConnectedOnStartup)
	pol := poller.New(cfgHolder, stateHolder, logger)

	webHandler, err := web.New(cfgHolder, stateHolder)
	if err != nil {
		logger.Error("failed to initialise web handler", "error", err)
		return
	}

	alpacaHandler := alpaca.New(cfgHolder, stateHolder, deviceUUID, version, pol.PollNow)

	disc := discovery.New(cfg.AlpacaDiscoveryPort, cfg.AlpacaHTTPPort, logger)
	webHandler.WithDiscovery(disc.GetStatus)
	webHandler.WithVersion(fmt.Sprintf("%s (commit %s, built %s)", version, commit, date))

	mux := http.NewServeMux()
	alpacaHandler.Register(mux)
	webHandler.Register(mux)

	srv := &http.Server{
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	listenAddr := fmt.Sprintf("%s:%d", cfg.AlpacaHTTPBind, cfg.AlpacaHTTPPort)
	ln, err := net.Listen("tcp", listenAddr)
	if err != nil {
		logger.Error("failed to bind HTTP port", "addr", listenAddr, "error", err)
		return
	}

	var wg sync.WaitGroup

	wg.Add(1)
	go func() {
		defer wg.Done()
		pol.Run(ctx)
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := disc.Run(ctx); err != nil {
			logger.Error("discovery fatal error", "error", err)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		logger.Info("alpaca HTTP listening", "addr", ln.Addr())
		if err := srv.Serve(ln); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server error", "error", err)
		}
	}()

	if interactive && isFirstRun {
		setupURL := fmt.Sprintf("http://127.0.0.1:%d/setup", cfg.AlpacaHTTPPort)
		logger.Info("first run detected, opening setup page", "url", setupURL)
		openBrowser(setupURL)
	}

	<-ctx.Done()
	logger.Info("shutting down")

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		logger.Warn("HTTP shutdown error", "error", err)
	}

	wg.Wait()
	logger.Info("stopped")
}

// ---------- entry point ------------------------------------------------------

func main() {
	// Default config paths to the directory containing the executable so the
	// service always finds its config regardless of working directory.
	exe, _ := os.Executable()
	exeDir := filepath.Dir(exe)

	var (
		cfgPath            = flag.String("config", filepath.Join(exeDir, "config.json"), "path to JSON config file")
		uuidPath           = flag.String("uuid-file", filepath.Join(exeDir, "device-uuid.txt"), "path to persist stable device UUID")
		svcCmd             = flag.String("service", "", "manage the system service: install|uninstall|start|stop|status")
		showVersion        = flag.Bool("version", false, "print version and exit")
		writeDefaultConfig = flag.Bool("write-default-config", false, "write default config to --config path and exit")
		checkConfig        = flag.Bool("check-config", false, "validate config and exit")
		runDiagnostics     = flag.Bool("diagnostics", false, "print service diagnostics and exit (service must be running)")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("sqmeter-alpaca-safetymonitor %s (commit %s, built %s)\n", version, commit, date)
		return
	}

	if *writeDefaultConfig {
		if err := config.SaveDefault(*cfgPath); err != nil {
			fmt.Fprintf(os.Stderr, "error writing default config: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("default config written to %s\n", *cfgPath)
		return
	}

	if *checkConfig {
		cfg, err := config.Load(*cfgPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "config error: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("config OK (SQMeter=%s, HTTP port=%d, discovery port=%d)\n",
			cfg.SQMeterBaseURL, cfg.AlpacaHTTPPort, cfg.AlpacaDiscoveryPort)
		return
	}

	if *runDiagnostics {
		cfg, err := config.Load(*cfgPath)
		if err != nil {
			fmt.Fprintf(os.Stderr, "config error: %v\n", err)
			os.Exit(1)
		}
		diagAddr := fmt.Sprintf("http://127.0.0.1:%d/api/diagnostics", cfg.AlpacaHTTPPort)
		client := &http.Client{Timeout: 5 * time.Second}
		resp, err := client.Get(diagAddr) //nolint:noctx
		if err != nil {
			fmt.Fprintf(os.Stderr, "service unreachable at %s: %v\n", diagAddr, err)
			fmt.Fprintf(os.Stderr, "Start the service first, or use --check-config for config-only validation.\n")
			os.Exit(1)
		}
		defer resp.Body.Close()
		var report web.DiagnosticsReport
		if err := json.NewDecoder(resp.Body).Decode(&report); err != nil {
			fmt.Fprintf(os.Stderr, "failed to decode diagnostics response: %v\n", err)
			os.Exit(1)
		}
		printDiagnosticsReport(report)
		return
	}

	prg := &program{
		cfgPath:  *cfgPath,
		uuidPath: *uuidPath,
	}

	svcConfig := &service.Config{
		Name:        "SQMeterAlpacaSafetyMonitor",
		DisplayName: "SQMeter Alpaca SafetyMonitor",
		Description: "ASCOM Alpaca SafetyMonitor bridge for SQMeter ESP32. Responds to Alpaca discovery on UDP port 32227.",
	}

	s, err := service.New(prg, svcConfig)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	if *svcCmd != "" {
		if err := service.Control(s, *svcCmd); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		fmt.Printf("service %s: OK\n", *svcCmd)
		return
	}

	// In interactive mode (terminal), forward OS signals to service Stop so
	// Ctrl-C triggers a clean shutdown.
	if service.Interactive() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			sig := <-sigCh
			fmt.Fprintf(os.Stderr, "\nreceived %s, stopping...\n", sig)
			s.Stop() //nolint:errcheck
		}()
	}

	if err := s.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// ---------- helpers ----------------------------------------------------------

func loadOrCreateUUID(path string) (string, error) {
	if data, err := os.ReadFile(path); err == nil {
		u := strings.TrimSpace(string(data))
		if len(u) == 36 {
			return u, nil
		}
	}
	u, err := newUUID()
	if err != nil {
		return "00000000-0000-4000-8000-000000000001", err
	}
	_ = os.WriteFile(path, []byte(u+"\n"), 0600)
	return u, nil
}

func newUUID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func newLogger(level string) *slog.Logger {
	var l slog.Level
	switch strings.ToLower(level) {
	case "debug":
		l = slog.LevelDebug
	case "warn", "warning":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: l}))
}

func openBrowser(url string) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	_ = cmd.Start()
}

func printDiagnosticsReport(r web.DiagnosticsReport) {
	none := func(s string) string {
		if s == "" {
			return "(none)"
		}
		return s
	}
	strSlice := func(ss []string) string {
		if len(ss) == 0 {
			return "(none)"
		}
		out := ""
		for _, s := range ss {
			out += "\n      - " + s
		}
		return out
	}

	fmt.Println("sqmeter-alpaca-safetymonitor diagnostics")
	fmt.Println("=========================================")
	fmt.Printf("version:    %s\n", none(r.Version))
	fmt.Printf("timestamp:  %s\n", r.Timestamp)
	fmt.Printf("uptime:     %s\n", r.Uptime)
	fmt.Println()

	fmt.Println("Config:")
	fmt.Printf("  config path:    %s\n", none(r.Config.Path))
	fmt.Printf("  sqmeter url:    %s\n", none(r.Config.SQMeterURL))
	fmt.Printf("  http bind:      %s:%d\n", r.Config.HTTPBind, r.Config.HTTPPort)
	fmt.Printf("  discovery port: %d\n", r.Config.DiscoveryPort)
	fmt.Printf("  wide open:      %v\n", r.Config.WideOpen)
	fmt.Println()

	fmt.Println("Discovery:")
	if r.Discovery == nil {
		fmt.Println("  (status not available)")
	} else {
		d := r.Discovery
		fmt.Printf("  running:        %v\n", d.Running)
		fmt.Printf("  healthy:        %v\n", d.Healthy)
		fmt.Printf("  last error:     %s\n", none(d.LastError))
		fmt.Printf("  response count: %d\n", d.ResponseCount)
		if d.LastRequestAt != nil {
			fmt.Printf("  last request:   %s\n", d.LastRequestAt.UTC().Format("2006-01-02 15:04:05 UTC"))
		}
		if !d.Healthy {
			fmt.Println()
			fmt.Println("  ! Discovery is not healthy. Another process may own UDP port", d.ConfiguredPort)
			fmt.Println("    Check for conflicting Alpaca software (e.g. ASCOM Simulators).")
		}
	}
	fmt.Println()

	fmt.Println("Poller:")
	fmt.Printf("  last poll:      %s\n", none(r.Poller.LastPollUTC))
	fmt.Printf("  last success:   %s\n", none(r.Poller.LastSuccessUTC))
	lastErr := "(none)"
	if r.Poller.LastError != nil {
		lastErr = *r.Poller.LastError
	}
	fmt.Printf("  last error:     %s\n", lastErr)
	fmt.Println()

	fmt.Println("Safety:")
	fmt.Printf("  connected:      %v\n", r.Safety.Connected)
	fmt.Printf("  is_safe:        %v\n", r.Safety.IsSafe)
	fmt.Printf("  override:       %s\n", r.Safety.ManualOverride)
	fmt.Printf("  reasons:        %s\n", strSlice(r.Safety.Reasons))
	fmt.Printf("  warnings:       %s\n", strSlice(r.Safety.Warnings))
}
