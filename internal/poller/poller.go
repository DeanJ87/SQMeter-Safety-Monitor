package poller

import (
	"context"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"sqmeter-ascom-alpaca/internal/config"
	"sqmeter-ascom-alpaca/internal/safety"
	"sqmeter-ascom-alpaca/internal/sqmclient"
	"sqmeter-ascom-alpaca/internal/state"
)

// Poller periodically fetches SQMeter sensor data, evaluates safety rules,
// and writes the result into the state holder.
type Poller struct {
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

	lastPollNano atomic.Int64
}

// New creates a Poller wired to the given holders.
func New(cfgHolder *config.Holder, stateHolder *state.Holder, logger *slog.Logger) *Poller {
	cfg := cfgHolder.Get()
	return &Poller{
		cfgHolder:   cfgHolder,
		stateHolder: stateHolder,
		logger:      logger,
		client:      sqmclient.New(cfg.SQMeterBaseURL),
		currentURL:  cfg.SQMeterBaseURL,
	}
}

// Run performs an initial poll then re-polls on the configured interval.
// It blocks until ctx is cancelled.  The interval is read from the config
// holder on every tick so it can be changed at runtime without restarting.
func (p *Poller) Run(ctx context.Context) {
	p.PollNow(ctx)

	ticker := time.NewTicker(time.Second)
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
func (p *Poller) PollNow(ctx context.Context) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.doPoll(ctx)
}

func (p *Poller) doPoll(ctx context.Context) {
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
