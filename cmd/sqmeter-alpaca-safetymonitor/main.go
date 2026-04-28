package main

import (
	"context"
	"crypto/rand"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"sqmeter-alpaca-safetymonitor/internal/alpaca"
	"sqmeter-alpaca-safetymonitor/internal/config"
	"sqmeter-alpaca-safetymonitor/internal/discovery"
	"sqmeter-alpaca-safetymonitor/internal/safety"
	"sqmeter-alpaca-safetymonitor/internal/sqmclient"
	"sqmeter-alpaca-safetymonitor/internal/state"
	"sqmeter-alpaca-safetymonitor/internal/web"
)

// Injected at build time via -ldflags.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

func main() {
	var (
		configPath         = flag.String("config", "config.json", "path to JSON config file (created if absent)")
		uuidPath           = flag.String("uuid-file", "device-uuid.txt", "path to persist stable device UUID")
		showVersion        = flag.Bool("version", false, "print version and exit")
		writeDefaultConfig = flag.Bool("write-default-config", false, "write default config to --config path and exit")
		checkConfig        = flag.Bool("check-config", false, "validate config and exit")
	)
	flag.Parse()

	if *showVersion {
		fmt.Printf("sqmeter-alpaca-safetymonitor %s (commit %s, built %s)\n", version, commit, date)
		return
	}

	if *writeDefaultConfig {
		if err := config.SaveDefault(*configPath); err != nil {
			fmt.Fprintf(os.Stderr, "error writing default config: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("default config written to %s\n", *configPath)
		return
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "config error: %v\n", err)
		os.Exit(1)
	}

	if *checkConfig {
		fmt.Printf("config OK (SQMeter=%s, HTTP port=%d, discovery port=%d)\n",
			cfg.SQMeterBaseURL, cfg.AlpacaHTTPPort, cfg.AlpacaDiscoveryPort)
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

	cfgHolder := config.NewHolder(cfg, *configPath)

	deviceUUID, err := loadOrCreateUUID(*uuidPath)
	if err != nil {
		logger.Warn("could not persist device UUID, using ephemeral UUID", "error", err)
	}
	logger.Info("device UUID", "uuid", deviceUUID)

	stateHolder := state.NewHolder(cfg.ConnectedOnStartup)

	pol := newPoller(cfgHolder, stateHolder, logger)

	webHandler, err := web.New(cfgHolder, stateHolder)
	if err != nil {
		logger.Error("failed to initialise web handler", "error", err)
		os.Exit(1)
	}

	alpacaHandler := alpaca.New(cfgHolder, stateHolder, deviceUUID, version, func(ctx context.Context) {
		pol.PollNow(ctx)
	})

	mux := http.NewServeMux()
	alpacaHandler.Register(mux)
	webHandler.Register(mux)

	srv := &http.Server{
		Addr:         fmt.Sprintf("%s:%d", cfg.AlpacaHTTPBind, cfg.AlpacaHTTPPort),
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	disc := discovery.New(cfg.AlpacaDiscoveryPort, cfg.AlpacaHTTPPort, logger)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sig := make(chan os.Signal, 1)
	signal.Notify(sig, syscall.SIGINT, syscall.SIGTERM)

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
		logger.Info("alpaca HTTP listening", "addr", srv.Addr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP server error", "error", err)
			cancel()
		}
	}()

	select {
	case s := <-sig:
		logger.Info("received signal, shutting down", "signal", s)
	case <-ctx.Done():
	}
	cancel()

	shutCtx, shutCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer shutCancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		logger.Warn("HTTP shutdown error", "error", err)
	}

	wg.Wait()
	logger.Info("stopped")
}

// ---------- poller -----------------------------------------------------------

type poller struct {
	cfgHolder   *config.Holder
	stateHolder *state.Holder
	logger      *slog.Logger

	mu                    sync.Mutex
	client                *sqmclient.Client
	currentURL            string
	hasData               bool
	lastSensors           *sqmclient.SensorsResponse
	lastSuccessfulPollUTC time.Time
	lastPollUTC           time.Time
	lastError             string

	lastPollNano atomic.Int64 // unix nano of last poll start, for interval check
}

func newPoller(cfgHolder *config.Holder, stateHolder *state.Holder, logger *slog.Logger) *poller {
	cfg := cfgHolder.Get()
	return &poller{
		cfgHolder:   cfgHolder,
		stateHolder: stateHolder,
		logger:      logger,
		client:      sqmclient.New(cfg.SQMeterBaseURL),
		currentURL:  cfg.SQMeterBaseURL,
	}
}

// Run performs an initial poll then re-polls on the configured interval.
// The interval is read from the config holder on every tick, so it can be
// changed at runtime without restarting.
func (p *poller) Run(ctx context.Context) {
	p.PollNow(ctx)

	ticker := time.NewTicker(time.Second) // 1-second base tick
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			cfg := p.cfgHolder.Get()
			interval := time.Duration(cfg.PollIntervalSeconds) * time.Second
			lastNano := p.lastPollNano.Load()
			if lastNano == 0 || time.Since(time.Unix(0, lastNano)) >= interval {
				p.PollNow(ctx)
			}
		case <-ctx.Done():
			return
		}
	}
}

// PollNow executes one poll cycle immediately (safe for concurrent callers).
func (p *poller) PollNow(ctx context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.doPoll(ctx)
}

func (p *poller) doPoll(ctx context.Context) {
	cfg := p.cfgHolder.Get()

	// Rebuild client if SQMeter URL changed (hot-reload).
	if cfg.SQMeterBaseURL != p.currentURL {
		p.client = sqmclient.New(cfg.SQMeterBaseURL)
		p.currentURL = cfg.SQMeterBaseURL
		p.logger.Info("SQMeter URL updated", "url", cfg.SQMeterBaseURL)
	}

	now := time.Now().UTC()
	p.lastPollUTC = now
	p.lastPollNano.Store(now.UnixNano())

	sensors, err := p.client.FetchSensors(ctx)
	if err != nil {
		p.lastError = err.Error()
		p.logger.Warn("SQMeter poll failed", "error", err)
	} else {
		p.lastError = ""
		p.hasData = true
		p.lastSensors = sensors
		p.lastSuccessfulPollUTC = now
		p.logger.Debug("SQMeter poll ok",
			"sqm", sensors.SkyQuality.SQM,
			"cloud_pct", sensors.CloudConditions.CloudCoverPercent,
		)
	}

	if cfg.ManualOverride == "force_safe" {
		p.logger.Warn("force_safe override active — reporting safe regardless of conditions")
	}

	evaluated := safety.Evaluate(cfg, safety.Input{
		Connected:             p.stateHolder.IsConnected(),
		HasData:               p.hasData,
		Sensors:               p.lastSensors,
		LastSuccessfulPollUTC: p.lastSuccessfulPollUTC,
		LastPollUTC:           now,
		LastError:             p.lastError,
	})

	p.stateHolder.Update(evaluated)
	p.logger.Debug("safety evaluated", "isSafe", evaluated.IsSafe, "reasons", len(evaluated.Reasons))
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
