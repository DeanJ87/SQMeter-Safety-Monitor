package poller_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"sqmeter-alpaca-safetymonitor/internal/config"
	"sqmeter-alpaca-safetymonitor/internal/poller"
	"sqmeter-alpaca-safetymonitor/internal/sqmclient"
	"sqmeter-alpaca-safetymonitor/internal/state"
)

func silentLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError}))
}

func clearSensors() sqmclient.SensorsResponse {
	return sqmclient.SensorsResponse{
		SkyQuality:  sqmclient.SkyQuality{SQM: 21.5, Bortle: 2.0},
		Environment: sqmclient.Environment{Temperature: 12.4, Humidity: 60.0, Dewpoint: 5.0},
		IRTemperature: sqmclient.IRTemperature{
			ObjectTemp: -15.0, AmbientTemp: 12.4, Status: 0,
		},
		LightSensor: sqmclient.LightSensor{Status: 0},
		CloudConditions: sqmclient.CloudConditions{
			CloudCoverPercent: 5.0, Description: "Clear",
		},
	}
}

func newFakeSQM(sensors sqmclient.SensorsResponse) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/sensors":
			json.NewEncoder(w).Encode(sensors)
		case "/api/status":
			json.NewEncoder(w).Encode(map[string]any{"uptime": 3600})
		default:
			http.NotFound(w, r)
		}
	}))
}

func newFakeSQMError() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "device error", http.StatusInternalServerError)
	}))
}

func defaultCfg(sqmURL string) *config.Config {
	cfg := config.Defaults()
	cfg.SQMeterBaseURL = sqmURL
	cfg.FailClosed = true
	cfg.ConnectedOnStartup = true
	return cfg
}

// ---------- PollNow: success path --------------------------------------------

func TestPollNow_UpdatesStateWithSensorData(t *testing.T) {
	srv := newFakeSQM(clearSensors())
	defer srv.Close()

	cfg := defaultCfg(srv.URL)
	cfgHolder := config.NewHolder(cfg, "")
	stateHolder := state.NewHolder(true)

	p := poller.New(cfgHolder, stateHolder, silentLogger())
	p.PollNow(context.Background())

	ev := stateHolder.Get()
	if !ev.IsSafe {
		t.Errorf("expected IsSafe=true after successful poll with clear sky, reasons: %v", ev.Reasons)
	}
	if ev.Values.SQM != 21.5 {
		t.Errorf("Values.SQM: want 21.5, got %v", ev.Values.SQM)
	}
	if ev.LastSuccessfulPollUTC.IsZero() {
		t.Error("LastSuccessfulPollUTC should be set after a successful poll")
	}
}

func TestPollNow_IsSafeTrue_WhenConditionsClear(t *testing.T) {
	sensors := clearSensors()
	sensors.CloudConditions.CloudCoverPercent = 3.0
	srv := newFakeSQM(sensors)
	defer srv.Close()

	cfgHolder := config.NewHolder(defaultCfg(srv.URL), "")
	stateHolder := state.NewHolder(true)

	p := poller.New(cfgHolder, stateHolder, silentLogger())
	p.PollNow(context.Background())

	if !stateHolder.Get().IsSafe {
		t.Errorf("expected safe with clear sky: %v", stateHolder.Get().Reasons)
	}
}

// ---------- PollNow: safety rule enforcement ----------------------------------

func TestPollNow_IsSafeFalse_WhenCloudCoverHigh(t *testing.T) {
	sensors := clearSensors()
	sensors.CloudConditions.CloudCoverPercent = 90.0
	srv := newFakeSQM(sensors)
	defer srv.Close()

	cfgHolder := config.NewHolder(defaultCfg(srv.URL), "")
	stateHolder := state.NewHolder(true)

	p := poller.New(cfgHolder, stateHolder, silentLogger())
	p.PollNow(context.Background())

	if stateHolder.Get().IsSafe {
		t.Error("expected unsafe: cloud cover 90% >= 80% threshold")
	}
}

func TestPollNow_IsSafeFalse_WhenDisconnected(t *testing.T) {
	srv := newFakeSQM(clearSensors())
	defer srv.Close()

	cfgHolder := config.NewHolder(defaultCfg(srv.URL), "")
	stateHolder := state.NewHolder(false) // disconnected

	p := poller.New(cfgHolder, stateHolder, silentLogger())
	p.PollNow(context.Background())

	if stateHolder.Get().IsSafe {
		t.Error("expected unsafe: device is disconnected")
	}
}

// ---------- PollNow: error handling ------------------------------------------

func TestPollNow_SetsLastError_WhenSQMUnreachable(t *testing.T) {
	cfgHolder := config.NewHolder(defaultCfg("http://127.0.0.1:1"), "")
	stateHolder := state.NewHolder(true)

	p := poller.New(cfgHolder, stateHolder, silentLogger())
	p.PollNow(context.Background())

	ev := stateHolder.Get()
	if ev.LastError == "" {
		t.Error("expected LastError to be set when SQMeter is unreachable")
	}
}

func TestPollNow_IsSafeFalse_WhenNoDataAndFailClosed(t *testing.T) {
	cfgHolder := config.NewHolder(defaultCfg("http://127.0.0.1:1"), "")
	stateHolder := state.NewHolder(true)

	p := poller.New(cfgHolder, stateHolder, silentLogger())
	p.PollNow(context.Background())

	if stateHolder.Get().IsSafe {
		t.Error("expected unsafe: no data yet and FAIL_CLOSED=true")
	}
}

func TestPollNow_IsSafeTrue_WhenNoDataAndFailOpen(t *testing.T) {
	cfg := defaultCfg("http://127.0.0.1:1")
	cfg.FailClosed = false
	cfgHolder := config.NewHolder(cfg, "")
	stateHolder := state.NewHolder(true)

	p := poller.New(cfgHolder, stateHolder, silentLogger())
	p.PollNow(context.Background())

	if !stateHolder.Get().IsSafe {
		t.Errorf("expected safe: no data with FAIL_CLOSED=false, reasons: %v", stateHolder.Get().Reasons)
	}
}

func TestPollNow_ServerError_SetsLastError(t *testing.T) {
	srv := newFakeSQMError()
	defer srv.Close()

	cfgHolder := config.NewHolder(defaultCfg(srv.URL), "")
	stateHolder := state.NewHolder(true)

	p := poller.New(cfgHolder, stateHolder, silentLogger())
	p.PollNow(context.Background())

	if stateHolder.Get().LastError == "" {
		t.Error("expected LastError when server returns 500")
	}
}

// ---------- hot-reload: SQMeter URL change -----------------------------------

func TestPollNow_HotReload_SwitchesURL(t *testing.T) {
	// Server A returns cloudy (unsafe), server B returns clear (safe).
	cloudy := clearSensors()
	cloudy.CloudConditions.CloudCoverPercent = 95.0
	srvA := newFakeSQM(cloudy)
	defer srvA.Close()

	srvB := newFakeSQM(clearSensors())
	defer srvB.Close()

	cfgHolder := config.NewHolder(defaultCfg(srvA.URL), "")
	stateHolder := state.NewHolder(true)

	p := poller.New(cfgHolder, stateHolder, silentLogger())

	// First poll — points at A, should be unsafe.
	p.PollNow(context.Background())
	if stateHolder.Get().IsSafe {
		t.Error("expected unsafe from server A (cloudy)")
	}

	// Update URL to B and poll again — should now be safe.
	updated := *cfgHolder.Get()
	updated.SQMeterBaseURL = srvB.URL
	cfgHolder.Update(&updated) //nolint:errcheck

	p.PollNow(context.Background())
	if !stateHolder.Get().IsSafe {
		t.Errorf("expected safe after switching to server B: %v", stateHolder.Get().Reasons)
	}
}

// ---------- Run: context cancellation ----------------------------------------

func TestRun_StopsOnContextCancel(t *testing.T) {
	srv := newFakeSQM(clearSensors())
	defer srv.Close()

	cfg := defaultCfg(srv.URL)
	cfg.PollIntervalSeconds = 1
	cfg.StaleAfterSeconds = 10
	cfgHolder := config.NewHolder(cfg, "")
	stateHolder := state.NewHolder(true)

	p := poller.New(cfgHolder, stateHolder, silentLogger())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		p.Run(ctx)
		close(done)
	}()

	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("Run() did not stop within 3s after context cancellation")
	}
}

func TestRun_PollsImmediatelyOnStart(t *testing.T) {
	srv := newFakeSQM(clearSensors())
	defer srv.Close()

	cfgHolder := config.NewHolder(defaultCfg(srv.URL), "")
	stateHolder := state.NewHolder(true)

	p := poller.New(cfgHolder, stateHolder, silentLogger())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan struct{})
	go func() {
		p.Run(ctx)
		close(done)
	}()

	// State should be populated very quickly after Run starts (initial poll).
	deadline := time.After(2 * time.Second)
	for {
		if !stateHolder.Get().LastPollUTC.IsZero() {
			break
		}
		select {
		case <-deadline:
			t.Fatal("poller did not perform initial poll within 2s")
		case <-time.After(20 * time.Millisecond):
		}
	}
	cancel()
}

// ---------- configuration-driven safety --------------------------------------

func TestPollNow_ConfigChange_BecomesUnsafe(t *testing.T) {
	// Sensor data: SQM=18, which is below a strict threshold.
	sensors := clearSensors()
	sensors.SkyQuality.SQM = 18.0
	srv := newFakeSQM(sensors)
	defer srv.Close()

	// Config A: no SQM minimum — safe.
	cfgA := defaultCfg(srv.URL)
	cfgHolder := config.NewHolder(cfgA, "")
	stateHolder := state.NewHolder(true)

	p := poller.New(cfgHolder, stateHolder, silentLogger())
	p.PollNow(context.Background())

	if !stateHolder.Get().IsSafe {
		t.Errorf("expected safe with no SQM minimum configured: %v", stateHolder.Get().Reasons)
	}

	// Config B: SQM minimum 20 — 18.0 is now unsafe.
	minSQM := 20.0
	updated := *cfgHolder.Get()
	updated.SQMMinSafe = &minSQM
	cfgHolder.Update(&updated) //nolint:errcheck

	p.PollNow(context.Background())
	if stateHolder.Get().IsSafe {
		t.Error("expected unsafe after config change raises SQM minimum above current value")
	}
}
