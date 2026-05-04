package web_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"sqmeter-ascom-alpaca/internal/config"
	"sqmeter-ascom-alpaca/internal/discovery"
	"sqmeter-ascom-alpaca/internal/sqmclient"
	"sqmeter-ascom-alpaca/internal/state"
	"sqmeter-ascom-alpaca/internal/web"
)

func newTestWebHandler(t *testing.T, connected bool, ev state.EvaluatedState) (*web.Handler, *config.Holder, *state.Holder) {
	t.Helper()
	cfgHolder := config.NewHolder(config.Defaults(), "")
	holder := state.NewHolder(connected)
	holder.Update(ev)
	h, err := web.New(cfgHolder, holder)
	if err != nil {
		t.Fatalf("web.New: %v", err)
	}
	return h, cfgHolder, holder
}

func safeEv() state.EvaluatedState {
	now := time.Now().UTC()
	sensors := &sqmclient.SensorsResponse{
		LightSensor:     sqmclient.LightSensor{Lux: 0.2, Status: 0},
		SkyQuality:      sqmclient.SkyQuality{SQM: 21.35, Bortle: 3.0},
		Environment:     sqmclient.Environment{Temperature: 8.5, Humidity: 62.0, Pressure: 1012.3, Dewpoint: 4.1, Status: 0},
		IRTemperature:   sqmclient.IRTemperature{ObjectTemp: -10.2, AmbientTemp: 8.4, Status: 0},
		CloudConditions: sqmclient.CloudConditions{CloudCoverPercent: 12.0, TemperatureDelta: -18.6, CorrectedDelta: -17.0, Description: "Clear"},
	}
	return state.EvaluatedState{
		IsSafe:                true,
		Reasons:               []string{},
		Warnings:              []string{},
		LastPollUTC:           now,
		LastSuccessfulPollUTC: now,
		RawSensors:            sensors,
		Values: state.Values{
			SQM:               sensors.SkyQuality.SQM,
			Bortle:            sensors.SkyQuality.Bortle,
			Temperature:       sensors.Environment.Temperature,
			Humidity:          sensors.Environment.Humidity,
			Dewpoint:          sensors.Environment.Dewpoint,
			CloudCoverPercent: sensors.CloudConditions.CloudCoverPercent,
			CloudCondition:    sensors.CloudConditions.Description,
			TemperatureDelta:  sensors.CloudConditions.TemperatureDelta,
			CorrectedDelta:    sensors.CloudConditions.CorrectedDelta,
		},
	}
}

func unsafeEv() state.EvaluatedState {
	ev := safeEv()
	ev.IsSafe = false
	ev.Reasons = []string{"cloud cover 90% >= unsafe threshold 80%"}
	return ev
}

func serve(t *testing.T, h *web.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	h.Register(mux)
	var req *http.Request
	if body != "" {
		req = httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

type failingResponseWriter struct {
	header http.Header
	code   int
}

func (w *failingResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *failingResponseWriter) Write([]byte) (int, error) {
	return 0, errors.New("forced write failure")
}

func (w *failingResponseWriter) WriteHeader(code int) {
	w.code = code
}

// ---------- /health ----------------------------------------------------------

func TestHealth_OKWhenSafe(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	w := serve(t, h, http.MethodGet, "/health", "")
	if w.Code != http.StatusOK {
		t.Errorf("health: want 200, got %d", w.Code)
	}
	var body map[string]any
	json.NewDecoder(w.Body).Decode(&body)
	if body["status"] != "ok" {
		t.Errorf("health status: want ok, got %q", body["status"])
	}
}

func TestHealth_UnsafeWhenIsSafeFalse(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, unsafeEv())
	w := serve(t, h, http.MethodGet, "/health", "")
	if w.Code != http.StatusOK {
		t.Errorf("health: want 200 even when unsafe, got %d", w.Code)
	}
	var body map[string]any
	json.NewDecoder(w.Body).Decode(&body)
	if body["status"] != "unsafe" {
		t.Errorf("health status: want unsafe, got %q", body["status"])
	}
}

func TestHealth_ServiceUnavailableWhenDisconnected(t *testing.T) {
	h, _, _ := newTestWebHandler(t, false, safeEv())
	w := serve(t, h, http.MethodGet, "/health", "")
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("health: want 503 when disconnected, got %d", w.Code)
	}
	var body map[string]any
	json.NewDecoder(w.Body).Decode(&body)
	if body["status"] != "disconnected" {
		t.Errorf("health status: want disconnected, got %q", body["status"])
	}
}

// ---------- /status.json -----------------------------------------------------

func TestStatusJSON_ValidJSON(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	w := serve(t, h, http.MethodGet, "/status.json", "")
	if w.Code != http.StatusOK {
		t.Errorf("status.json: want 200, got %d", w.Code)
	}
	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("status.json: invalid JSON: %v", err)
	}
}

func TestStatusJSON_IsSafeField(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	w := serve(t, h, http.MethodGet, "/status.json", "")
	var body map[string]any
	json.NewDecoder(w.Body).Decode(&body)
	if v, ok := body["isSafe"]; !ok || v != true {
		t.Errorf("status.json: expected isSafe=true, got %v (present=%v)", v, ok)
	}
}

func TestStatusJSON_ReflectsUnsafeState(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, unsafeEv())
	w := serve(t, h, http.MethodGet, "/status.json", "")
	var body map[string]any
	json.NewDecoder(w.Body).Decode(&body)
	if body["isSafe"] != false {
		t.Errorf("status.json: expected isSafe=false for unsafe state, got %v", body["isSafe"])
	}
}

// ---------- /  (dashboard) ---------------------------------------------------

func TestDashboard_Renders200(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	w := serve(t, h, http.MethodGet, "/", "")
	if w.Code != http.StatusOK {
		t.Errorf("dashboard: want 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/html") {
		t.Errorf("dashboard Content-Type: want text/html, got %q", ct)
	}
}

func TestDashboard_WriteFailureReturnsWithoutPanic(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := &failingResponseWriter{}

	h.Dashboard(w, req)

	if ct := w.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Fatalf("dashboard Content-Type: want text/html before write failure, got %q", ct)
	}
}

func TestDashboard_WithDiscoveryStatus_Healthy(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	h.WithDiscovery(func() discovery.Status {
		return discovery.Status{ConfiguredPort: 32227, Running: true, Healthy: true}
	})
	w := serve(t, h, http.MethodGet, "/", "")
	if w.Code != http.StatusOK {
		t.Fatalf("dashboard with discovery: want 200, got %d", w.Code)
	}
}

func TestDashboard_WithDiscoveryStatus_Unhealthy(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	h.WithDiscovery(func() discovery.Status {
		return discovery.Status{
			ConfiguredPort: 32227,
			Running:        false,
			Healthy:        false,
			LastError:      "listen udp :32227: bind: address already in use",
		}
	})
	w := serve(t, h, http.MethodGet, "/", "")
	if w.Code != http.StatusOK {
		t.Fatalf("dashboard unhealthy discovery: want 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "not running") {
		t.Error("dashboard: expected 'not running' text for unhealthy discovery")
	}
}

func TestDashboard_AliasedFromStatus(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	w := serve(t, h, http.MethodGet, "/status", "")
	if w.Code != http.StatusOK {
		t.Errorf("/status: want 200, got %d", w.Code)
	}
}

func TestDashboard_ReferenceShellStructure(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, unsafeEv())
	w := serve(t, h, http.MethodGet, "/", "")
	if w.Code != http.StatusOK {
		t.Fatalf("dashboard: want 200, got %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{
		"ASCOM SQMeter Bridge",
		`href="/static/app.css"`,
		`href="#overview"`,
		`href="#configuration"`,
		`href="#safety-monitor"`,
		`href="#observing-conditions"`,
		`href="#diagnostics"`,
		"Current Readings",
		"Bridge Status",
		"Poll Timing",
		"Exposed Alpaca Devices",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("dashboard: expected %q in rendered shell", want)
		}
	}
}

func TestDashboard_NoRuntimeExternalCSSOrFonts(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	w := serve(t, h, http.MethodGet, "/", "")
	body := w.Body.String()
	for _, forbidden := range []string{
		"fonts." + "googleapis",
		"fonts." + "gstatic",
		"cdn." + "tailwindcss",
		"<" + "style>",
		"Twe" + "aks",
	} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("dashboard HTML contains forbidden runtime asset/debug marker %q", forbidden)
		}
	}
}

func TestDashboard_HashNavigationScriptServed(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	w := serve(t, h, http.MethodGet, "/static/app.js", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /static/app.js: want 200, got %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{"hashchange", "safety-monitor", "observing-conditions", "overview"} {
		if !strings.Contains(body, want) {
			t.Fatalf("app.js: expected hash navigation marker %q", want)
		}
	}
}

func TestDashboard_StaticCSSAndFontsServed(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	css := serve(t, h, http.MethodGet, "/static/app.css", "")
	if css.Code != http.StatusOK {
		t.Fatalf("GET /static/app.css: want 200, got %d", css.Code)
	}
	cssBody := css.Body.String()
	for _, want := range []string{"IBM Plex Sans", "IBMPlexSans-Regular.woff2", "bridge-shell", "warn-banner"} {
		if !strings.Contains(cssBody, want) {
			t.Fatalf("app.css: expected %q", want)
		}
	}
	for _, forbidden := range []string{"fonts." + "googleapis", "fonts." + "gstatic", "cdn." + "tailwindcss"} {
		if strings.Contains(cssBody, forbidden) {
			t.Fatalf("app.css contains forbidden external reference %q", forbidden)
		}
	}

	font := serve(t, h, http.MethodGet, "/static/fonts/ibm-plex/IBMPlexSans-Regular.woff2", "")
	if font.Code != http.StatusOK {
		t.Fatalf("GET IBM Plex font: want 200, got %d", font.Code)
	}
	if font.Body.Len() == 0 {
		t.Fatal("GET IBM Plex font: expected embedded font bytes")
	}
}

func TestDashboard_SafetyMonitorTableStructure(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, unsafeEv())
	w := serve(t, h, http.MethodGet, "/", "")
	if w.Code != http.StatusOK {
		t.Fatalf("dashboard: want 200, got %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{
		"Safety Rules",
		"<th>Rule</th>",
		"<th>Threshold</th>",
		"<th>Current Value</th>",
		"<th>Result</th>",
		"<th>Enabled</th>",
		"pill pass",
		"pill fail",
		"pill enabled",
		"Unsafe Reasons",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("safety monitor: expected %q", want)
		}
	}
}

func TestDashboard_WideOpenUsesSoftGoldWarning(t *testing.T) {
	h, cfgHolder, _ := newTestWebHandler(t, true, safeEv())
	cfg := cfgHolder.Get()
	cfg.AlpacaHTTPBind = "0.0.0.0"
	cfgHolder.Update(cfg)

	w := serve(t, h, http.MethodGet, "/", "")
	body := w.Body.String()
	if !strings.Contains(body, "warn-banner") || !strings.Contains(body, "ALPACA_HTTP_BIND=0.0.0.0") {
		t.Fatalf("dashboard: expected soft warning banner for wide-open bind")
	}
}

// ---------- /setup -----------------------------------------------------------

func TestGetSetup_Renders200(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	w := serve(t, h, http.MethodGet, "/setup", "")
	if w.Code != http.StatusOK {
		t.Errorf("GET /setup: want 200, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if !strings.HasPrefix(ct, "text/html") {
		t.Errorf("GET /setup Content-Type: want text/html, got %q", ct)
	}
}

func TestGetSetup_RendersConfigurationShellFeedback(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	w := serve(t, h, http.MethodGet, "/setup?saved=1&restart=1&error=bad+config", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /setup: want 200, got %d", w.Code)
	}
	body := w.Body.String()
	for _, want := range []string{
		`data-initial-page="configuration"`,
		"Configuration saved.",
		"Saved changes require restart.",
		"Error: bad config",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("GET /setup: expected %q in configuration shell", want)
		}
	}
}

func TestPostSetup_ValidSubmit_Redirects(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	form := url.Values{}
	form.Set("SQMETER_BASE_URL", "http://newsqm.local")
	form.Set("ALPACA_HTTP_BIND", "127.0.0.1")
	form.Set("ALPACA_HTTP_PORT", "11111")
	form.Set("ALPACA_DISCOVERY_PORT", "32227")
	form.Set("POLL_INTERVAL_SECONDS", "5")
	form.Set("STALE_AFTER_SECONDS", "30")
	form.Set("FAIL_CLOSED", "true")
	form.Set("CONNECTED_ON_STARTUP", "true")
	form.Set("CLOUD_COVER_UNSAFE_PERCENT", "80")
	form.Set("CLOUD_COVER_CAUTION_PERCENT", "50")
	form.Set("MANUAL_OVERRIDE", "auto")
	form.Set("LOG_LEVEL", "info")

	w := serve(t, h, http.MethodPost, "/setup", form.Encode())
	if w.Code != http.StatusSeeOther {
		t.Errorf("POST /setup: want 303 redirect, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "/setup") {
		t.Errorf("POST /setup redirect: expected /setup, got %q", loc)
	}
}

func TestPostSetup_UpdatesConfig(t *testing.T) {
	h, cfgHolder, _ := newTestWebHandler(t, true, safeEv())
	form := url.Values{}
	// Only set the fields that differ from defaults
	form.Set("SQMETER_BASE_URL", "http://changed.local")
	form.Set("ALPACA_HTTP_BIND", "127.0.0.1")
	form.Set("ALPACA_HTTP_PORT", "11111")
	form.Set("ALPACA_DISCOVERY_PORT", "32227")
	form.Set("POLL_INTERVAL_SECONDS", "5")
	form.Set("STALE_AFTER_SECONDS", "30")
	form.Set("FAIL_CLOSED", "true")
	form.Set("CONNECTED_ON_STARTUP", "true")
	form.Set("CLOUD_COVER_UNSAFE_PERCENT", "80")
	form.Set("CLOUD_COVER_CAUTION_PERCENT", "50")
	form.Set("MANUAL_OVERRIDE", "auto")
	form.Set("LOG_LEVEL", "info")

	serve(t, h, http.MethodPost, "/setup", form.Encode())

	if cfgHolder.Get().SQMeterBaseURL != "http://changed.local" {
		t.Errorf("config not updated: SQMeterBaseURL = %q", cfgHolder.Get().SQMeterBaseURL)
	}
}

func TestPostSetup_RestartRequiredFieldsInRedirect(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	form := url.Values{}
	form.Set("SQMETER_BASE_URL", "http://sqmeter.local")
	form.Set("ALPACA_HTTP_BIND", "0.0.0.0")
	form.Set("ALPACA_HTTP_PORT", "22222")
	form.Set("ALPACA_DISCOVERY_PORT", "32227")
	form.Set("POLL_INTERVAL_SECONDS", "5")
	form.Set("STALE_AFTER_SECONDS", "30")
	form.Set("FAIL_CLOSED", "true")
	form.Set("CONNECTED_ON_STARTUP", "true")
	form.Set("CLOUD_COVER_UNSAFE_PERCENT", "80")
	form.Set("CLOUD_COVER_CAUTION_PERCENT", "50")
	form.Set("MANUAL_OVERRIDE", "auto")
	form.Set("LOG_LEVEL", "info")

	w := serve(t, h, http.MethodPost, "/setup", form.Encode())
	if w.Code != http.StatusSeeOther {
		t.Fatalf("POST /setup: want 303 redirect, got %d", w.Code)
	}

	loc := w.Header().Get("Location")
	if !strings.Contains(loc, "restart=1") {
		t.Fatalf("POST /setup redirect: expected restart flag, got %q", loc)
	}
	if !strings.Contains(loc, "restart_required_fields=ALPACA_HTTP_BIND") {
		t.Fatalf("POST /setup redirect: expected ALPACA_HTTP_BIND field, got %q", loc)
	}
	if !strings.Contains(loc, "restart_required_fields=ALPACA_HTTP_PORT") {
		t.Fatalf("POST /setup redirect: expected ALPACA_HTTP_PORT field, got %q", loc)
	}
}

func TestGetSetup_RestartWarningListsFields(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	path := "/setup?saved=1&restart_required_fields=ALPACA_HTTP_BIND&restart_required_fields=ALPACA_HTTP_PORT"

	w := serve(t, h, http.MethodGet, path, "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /setup: want 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "ALPACA_HTTP_BIND") || !strings.Contains(body, "ALPACA_HTTP_PORT") {
		t.Fatalf("GET /setup warning did not list restart-required fields: %s", body)
	}
}

// ---------- /config.json API -------------------------------------------------

func TestGetConfigJSON_ValidJSON(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	w := serve(t, h, http.MethodGet, "/config.json", "")
	if w.Code != http.StatusOK {
		t.Errorf("GET /config.json: want 200, got %d", w.Code)
	}
	var cfg map[string]any
	if err := json.NewDecoder(w.Body).Decode(&cfg); err != nil {
		t.Fatalf("GET /config.json: invalid JSON: %v", err)
	}
	if _, ok := cfg["SQMETER_BASE_URL"]; !ok {
		t.Error("GET /config.json: expected SQMETER_BASE_URL field")
	}
}

func TestPutConfigJSON_UpdatesConfig(t *testing.T) {
	h, cfgHolder, _ := newTestWebHandler(t, true, safeEv())
	body := `{"SQMETER_BASE_URL":"http://put-updated.local","LOG_LEVEL":"debug"}`

	w := serve(t, h, http.MethodPut, "/config.json", body)
	if w.Code != http.StatusOK {
		t.Errorf("PUT /config.json: want 200, got %d — body: %s", w.Code, w.Body.String())
	}

	if cfgHolder.Get().SQMeterBaseURL != "http://put-updated.local" {
		t.Errorf("config not updated via PUT: SQMeterBaseURL = %q", cfgHolder.Get().SQMeterBaseURL)
	}
}

func TestPutConfigJSON_InvalidJSON_Returns400(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	w := serve(t, h, http.MethodPut, "/config.json", "{not valid json{{")
	if w.Code != http.StatusBadRequest {
		t.Errorf("PUT /config.json invalid JSON: want 400, got %d", w.Code)
	}
}

func TestPutConfigJSON_InvalidConfig_Returns422(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	body := `{"MANUAL_OVERRIDE":"completely_wrong"}`
	w := serve(t, h, http.MethodPut, "/config.json", body)
	if w.Code != http.StatusUnprocessableEntity {
		t.Errorf("PUT /config.json invalid config: want 422, got %d", w.Code)
	}
}

func TestPutConfigJSON_NeedsRestart_FlaggedInResponse(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	body := `{"ALPACA_HTTP_PORT":22222}`
	w := serve(t, h, http.MethodPut, "/config.json", body)

	var resp struct {
		RestartRequired       bool     `json:"restart_required"`
		RestartRequiredFields []string `json:"restart_required_fields"`
		NeedsRestart          bool     `json:"needsRestart"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if !resp.RestartRequired {
		t.Error("expected restart_required=true when HTTP port changes")
	}
	if !resp.NeedsRestart {
		t.Error("expected legacy needsRestart=true when HTTP port changes")
	}
	if len(resp.RestartRequiredFields) != 1 || resp.RestartRequiredFields[0] != "ALPACA_HTTP_PORT" {
		t.Fatalf("restart_required_fields = %#v, want [ALPACA_HTTP_PORT]", resp.RestartRequiredFields)
	}
}

func TestPutConfigJSON_NoRestart_ReportsFalseAndEmptyFields(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	body := `{"SQMETER_BASE_URL":"http://put-updated.local"}`
	w := serve(t, h, http.MethodPut, "/config.json", body)

	if w.Code != http.StatusOK {
		t.Fatalf("PUT /config.json: want 200, got %d - body: %s", w.Code, w.Body.String())
	}

	var resp struct {
		RestartRequired       bool     `json:"restart_required"`
		RestartRequiredFields []string `json:"restart_required_fields"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("PUT /config.json: invalid JSON: %v", err)
	}
	if resp.RestartRequired {
		t.Error("expected restart_required=false for hot-reloadable field")
	}
	if len(resp.RestartRequiredFields) != 0 {
		t.Fatalf("restart_required_fields = %#v, want empty", resp.RestartRequiredFields)
	}
}

// ---------- /status.json discovery field ------------------------------------

func TestStatusJSON_DiscoveryField_WhenHealthy(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	h.WithDiscovery(func() discovery.Status {
		return discovery.Status{
			ConfiguredPort: 32227,
			Running:        true,
			Healthy:        true,
			ResponseCount:  3,
		}
	})

	w := serve(t, h, http.MethodGet, "/status.json", "")
	if w.Code != http.StatusOK {
		t.Fatalf("status.json: want 200, got %d", w.Code)
	}

	var body map[string]any
	if err := json.NewDecoder(w.Body).Decode(&body); err != nil {
		t.Fatalf("status.json: invalid JSON: %v", err)
	}

	disc, ok := body["discovery"].(map[string]any)
	if !ok {
		t.Fatalf("status.json: expected discovery field, got %T", body["discovery"])
	}
	if disc["running"] != true {
		t.Errorf("discovery.running: want true, got %v", disc["running"])
	}
	if disc["healthy"] != true {
		t.Errorf("discovery.healthy: want true, got %v", disc["healthy"])
	}
	if port, _ := disc["configured_port"].(float64); int(port) != 32227 {
		t.Errorf("discovery.configured_port: want 32227, got %v", disc["configured_port"])
	}
}

func TestStatusJSON_DiscoveryField_WhenUnhealthy(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	h.WithDiscovery(func() discovery.Status {
		return discovery.Status{
			ConfiguredPort: 32227,
			Running:        false,
			Healthy:        false,
			LastError:      "listen udp :32227: bind: address already in use",
		}
	})

	w := serve(t, h, http.MethodGet, "/status.json", "")
	var body map[string]any
	json.NewDecoder(w.Body).Decode(&body)

	disc, ok := body["discovery"].(map[string]any)
	if !ok {
		t.Fatalf("status.json: expected discovery field")
	}
	if disc["running"] != false {
		t.Errorf("discovery.running: want false, got %v", disc["running"])
	}
	if disc["healthy"] != false {
		t.Errorf("discovery.healthy: want false, got %v", disc["healthy"])
	}
	if disc["last_error"] == "" || disc["last_error"] == nil {
		t.Error("discovery.last_error: expected non-empty error string")
	}
}

func TestStatusJSON_NoDiscoveryField_WhenNotWired(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	// no WithDiscovery call

	w := serve(t, h, http.MethodGet, "/status.json", "")
	var body map[string]any
	json.NewDecoder(w.Body).Decode(&body)

	if _, present := body["discovery"]; present {
		t.Error("status.json: discovery field should be absent when no getter is wired")
	}
}

// ---------- POST /api/test-sqmeter ------------------------------------------

func TestTestSQMeter_ReachableURLInBody(t *testing.T) {
	// httptest.NewServer opens a real TCP listener, so a TCP-dial test succeeds.
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	h, _, _ := newTestWebHandler(t, true, safeEv())
	body := `{"url":"` + backend.URL + `"}`
	w := serve(t, h, http.MethodPost, "/api/test-sqmeter", body)

	if w.Code != http.StatusOK {
		t.Fatalf("test-sqmeter: want 200, got %d — body: %s", w.Code, w.Body.String())
	}
	var resp struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("test-sqmeter: invalid JSON: %v", err)
	}
	if !resp.OK {
		t.Errorf("test-sqmeter: want ok=true for reachable URL, got false; message: %s", resp.Message)
	}
}

func TestTestSQMeter_UnreachableURLInBody(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	body := `{"url":"http://127.0.0.1:19999"}`
	w := serve(t, h, http.MethodPost, "/api/test-sqmeter", body)

	if w.Code != http.StatusOK {
		t.Fatalf("test-sqmeter: want 200 envelope, got %d", w.Code)
	}
	var resp struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.OK {
		t.Error("test-sqmeter: want ok=false for unreachable URL, got true")
	}
	if resp.Message == "" {
		t.Error("test-sqmeter: want non-empty failure message")
	}
}

func TestTestSQMeter_FallsBackToConfiguredURL(t *testing.T) {
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer backend.Close()

	h, cfgHolder, _ := newTestWebHandler(t, true, safeEv())
	cfg := *cfgHolder.Get()
	cfg.SQMeterBaseURL = backend.URL
	cfgHolder.Update(&cfg) //nolint:errcheck

	// Empty body — handler uses configured URL.
	w := serve(t, h, http.MethodPost, "/api/test-sqmeter", "")
	var resp struct {
		OK bool `json:"ok"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if !resp.OK {
		t.Error("test-sqmeter: want ok=true when falling back to configured URL")
	}
}

func TestTestSQMeter_NoURLAnywhere(t *testing.T) {
	h, cfgHolder, _ := newTestWebHandler(t, true, safeEv())
	cfg := *cfgHolder.Get()
	cfg.SQMeterBaseURL = ""
	cfgHolder.Update(&cfg) //nolint:errcheck

	w := serve(t, h, http.MethodPost, "/api/test-sqmeter", "")
	var resp struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.OK {
		t.Error("test-sqmeter: want ok=false when no URL configured and none in body")
	}
	if resp.Message == "" {
		t.Error("test-sqmeter: want non-empty message")
	}
}

func TestTestSQMeter_InvalidScheme(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	body := `{"url":"ftp://192.168.1.1"}`
	w := serve(t, h, http.MethodPost, "/api/test-sqmeter", body)

	var resp struct {
		OK bool `json:"ok"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.OK {
		t.Error("test-sqmeter: want ok=false for non-http(s) scheme")
	}
}

func TestTestSQMeter_InvalidJSON(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	w := serve(t, h, http.MethodPost, "/api/test-sqmeter", "{not valid")

	var resp struct {
		OK bool `json:"ok"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.OK {
		t.Error("test-sqmeter: want ok=false for malformed JSON body")
	}
}

// ---------- GET /api/diagnostics ---------------------------------------------

func TestDiagnostics_BasicFields(t *testing.T) {
	h, cfgHolder, _ := newTestWebHandler(t, true, safeEv())
	h.WithVersion("test-version")
	cfg := *cfgHolder.Get()
	cfg.SQMeterBaseURL = "http://192.168.1.100"
	cfg.AlpacaHTTPPort = 11111
	cfg.AlpacaDiscoveryPort = 32227
	cfgHolder.Update(&cfg) //nolint:errcheck

	w := serve(t, h, http.MethodGet, "/api/diagnostics", "")

	if w.Code != http.StatusOK {
		t.Fatalf("diagnostics: want 200, got %d", w.Code)
	}

	var report web.DiagnosticsReport
	if err := json.NewDecoder(w.Body).Decode(&report); err != nil {
		t.Fatalf("diagnostics: decode failed: %v", err)
	}

	if report.Version != "test-version" {
		t.Errorf("diagnostics.version: want %q, got %q", "test-version", report.Version)
	}
	if report.Timestamp == "" {
		t.Error("diagnostics.timestamp: want non-empty")
	}
	if report.Uptime == "" {
		t.Error("diagnostics.uptime: want non-empty")
	}
	if report.Config.SQMeterURL != "http://192.168.1.100" {
		t.Errorf("diagnostics.config.sqmeterUrl: want %q, got %q", "http://192.168.1.100", report.Config.SQMeterURL)
	}
	if report.Config.HTTPPort != 11111 {
		t.Errorf("diagnostics.config.httpPort: want 11111, got %d", report.Config.HTTPPort)
	}
	if report.Config.DiscoveryPort != 32227 {
		t.Errorf("diagnostics.config.discoveryPort: want 32227, got %d", report.Config.DiscoveryPort)
	}
	if !report.Safety.Connected {
		t.Error("diagnostics.safety.connected: want true")
	}
	if !report.Safety.IsSafe {
		t.Error("diagnostics.safety.isSafe: want true")
	}
	if report.Safety.Reasons == nil {
		t.Error("diagnostics.safety.reasons: want non-nil slice")
	}
}

func TestDiagnostics_UnsafeState(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, unsafeEv())

	w := serve(t, h, http.MethodGet, "/api/diagnostics", "")

	var report web.DiagnosticsReport
	json.NewDecoder(w.Body).Decode(&report) //nolint:errcheck

	if report.Safety.IsSafe {
		t.Error("diagnostics.safety.isSafe: want false for unsafe state")
	}
	if len(report.Safety.Reasons) == 0 {
		t.Error("diagnostics.safety.reasons: want at least one reason for unsafe state")
	}
}

func TestDiagnostics_WithDiscovery(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	ds := discovery.Status{
		ConfiguredPort: 32227,
		Running:        true,
		Healthy:        true,
		ResponseCount:  3,
	}
	h.WithDiscovery(func() discovery.Status { return ds })

	w := serve(t, h, http.MethodGet, "/api/diagnostics", "")

	var report web.DiagnosticsReport
	json.NewDecoder(w.Body).Decode(&report) //nolint:errcheck

	if report.Discovery == nil {
		t.Fatal("diagnostics.discovery: want non-nil when getter is wired")
	}
	if !report.Discovery.Running {
		t.Error("diagnostics.discovery.running: want true")
	}
	if !report.Discovery.Healthy {
		t.Error("diagnostics.discovery.healthy: want true")
	}
	if report.Discovery.ResponseCount != 3 {
		t.Errorf("diagnostics.discovery.responseCount: want 3, got %d", report.Discovery.ResponseCount)
	}
}

func TestDiagnostics_WithoutDiscovery(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	// no WithDiscovery call

	w := serve(t, h, http.MethodGet, "/api/diagnostics", "")

	var report web.DiagnosticsReport
	json.NewDecoder(w.Body).Decode(&report) //nolint:errcheck

	if report.Discovery != nil {
		t.Error("diagnostics.discovery: want nil when no getter is wired")
	}
}

func TestDiagnostics_PollerTimestamps(t *testing.T) {
	ev := safeEv()
	ev.LastError = "timeout connecting to device"
	h, _, _ := newTestWebHandler(t, true, ev)

	w := serve(t, h, http.MethodGet, "/api/diagnostics", "")

	var report web.DiagnosticsReport
	json.NewDecoder(w.Body).Decode(&report) //nolint:errcheck

	if report.Poller.LastPollUTC == "" {
		t.Error("diagnostics.poller.lastPollUtc: want non-empty")
	}
	if report.Poller.LastError == nil {
		t.Error("diagnostics.poller.lastError: want non-nil when state has error")
	} else if *report.Poller.LastError != "timeout connecting to device" {
		t.Errorf("diagnostics.poller.lastError: want %q, got %q", "timeout connecting to device", *report.Poller.LastError)
	}
}

// ---------- optional float fields coverage ----------------------------------

func TestPostSetup_WithOptionalFloats(t *testing.T) {
	h, cfgHolder, _ := newTestWebHandler(t, true, safeEv())
	form := url.Values{}
	form.Set("SQMETER_BASE_URL", "http://sqmeter.local")
	form.Set("ALPACA_HTTP_BIND", "127.0.0.1")
	form.Set("ALPACA_HTTP_PORT", "11111")
	form.Set("ALPACA_DISCOVERY_PORT", "32227")
	form.Set("POLL_INTERVAL_SECONDS", "5")
	form.Set("STALE_AFTER_SECONDS", "30")
	form.Set("FAIL_CLOSED", "true")
	form.Set("CONNECTED_ON_STARTUP", "true")
	form.Set("CLOUD_COVER_UNSAFE_PERCENT", "80")
	form.Set("CLOUD_COVER_CAUTION_PERCENT", "50")
	form.Set("MANUAL_OVERRIDE", "auto")
	form.Set("LOG_LEVEL", "info")
	form.Set("SQM_MIN_SAFE", "18.5")
	form.Set("HUMIDITY_MAX_SAFE", "85.0")
	form.Set("DEWPOINT_MARGIN_MIN_C", "2.0")

	w := serve(t, h, http.MethodPost, "/setup", form.Encode())
	if w.Code != http.StatusSeeOther {
		t.Fatalf("POST /setup: want 303, got %d", w.Code)
	}

	cfg := cfgHolder.Get()
	if cfg.SQMMinSafe == nil || *cfg.SQMMinSafe != 18.5 {
		t.Errorf("SQMMinSafe: want 18.5, got %v", cfg.SQMMinSafe)
	}
	if cfg.HumidityMaxSafe == nil || *cfg.HumidityMaxSafe != 85.0 {
		t.Errorf("HumidityMaxSafe: want 85.0, got %v", cfg.HumidityMaxSafe)
	}
	if cfg.DewpointMarginMinC == nil || *cfg.DewpointMarginMinC != 2.0 {
		t.Errorf("DewpointMarginMinC: want 2.0, got %v", cfg.DewpointMarginMinC)
	}
}

func TestPostSetup_InvalidOptionalFloat_Ignores(t *testing.T) {
	h, cfgHolder, _ := newTestWebHandler(t, true, safeEv())

	// Set valid optional floats first
	cfg := cfgHolder.Get()
	sqmVal := 18.5
	cfg.SQMMinSafe = &sqmVal
	cfgHolder.Update(cfg)

	form := url.Values{}
	form.Set("SQMETER_BASE_URL", "http://sqmeter.local")
	form.Set("ALPACA_HTTP_BIND", "127.0.0.1")
	form.Set("ALPACA_HTTP_PORT", "11111")
	form.Set("ALPACA_DISCOVERY_PORT", "32227")
	form.Set("POLL_INTERVAL_SECONDS", "5")
	form.Set("STALE_AFTER_SECONDS", "30")
	form.Set("FAIL_CLOSED", "true")
	form.Set("CONNECTED_ON_STARTUP", "true")
	form.Set("CLOUD_COVER_UNSAFE_PERCENT", "80")
	form.Set("CLOUD_COVER_CAUTION_PERCENT", "50")
	form.Set("MANUAL_OVERRIDE", "auto")
	form.Set("LOG_LEVEL", "info")
	form.Set("SQM_MIN_SAFE", "not-a-number")

	w := serve(t, h, http.MethodPost, "/setup", form.Encode())
	if w.Code != http.StatusSeeOther {
		t.Fatalf("POST /setup: want 303, got %d", w.Code)
	}

	// Invalid value should be ignored, original value retained
	cfg = cfgHolder.Get()
	if cfg.SQMMinSafe == nil || *cfg.SQMMinSafe != 18.5 {
		t.Errorf("SQMMinSafe should remain 18.5 after invalid input, got %v", cfg.SQMMinSafe)
	}
}

// ---------- POST /api/service/restart & /api/service/stop -------------------

func TestServiceRestart_WithCallback_Returns200(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	called := make(chan struct{}, 1)
	h.WithServiceControl(func() { called <- struct{}{} }, nil)

	w := serve(t, h, http.MethodPost, "/api/service/restart", "")
	if w.Code != http.StatusOK {
		t.Fatalf("POST /api/service/restart: want 200, got %d — body: %s", w.Code, w.Body.String())
	}
	var resp struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("POST /api/service/restart: invalid JSON: %v", err)
	}
	if !resp.OK {
		t.Errorf("POST /api/service/restart: want ok=true, got false; message: %s", resp.Message)
	}
	if resp.Message == "" {
		t.Error("POST /api/service/restart: want non-empty message")
	}
}

func TestServiceStop_WithCallback_Returns200(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	called := make(chan struct{}, 1)
	h.WithServiceControl(nil, func() { called <- struct{}{} })

	w := serve(t, h, http.MethodPost, "/api/service/stop", "")
	if w.Code != http.StatusOK {
		t.Fatalf("POST /api/service/stop: want 200, got %d — body: %s", w.Code, w.Body.String())
	}
	var resp struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("POST /api/service/stop: invalid JSON: %v", err)
	}
	if !resp.OK {
		t.Errorf("POST /api/service/stop: want ok=true, got false; message: %s", resp.Message)
	}
	if resp.Message == "" {
		t.Error("POST /api/service/stop: want non-empty message")
	}
}

func TestServiceRestart_WithoutCallback_Returns501(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	// no WithServiceControl call

	w := serve(t, h, http.MethodPost, "/api/service/restart", "")
	if w.Code != http.StatusNotImplemented {
		t.Errorf("POST /api/service/restart without callback: want 501, got %d", w.Code)
	}
	var resp struct {
		OK bool `json:"ok"`
	}
	json.NewDecoder(w.Body).Decode(&resp) //nolint:errcheck
	if resp.OK {
		t.Error("POST /api/service/restart without callback: want ok=false")
	}
}

func TestServiceStop_WithoutCallback_Returns501(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	// no WithServiceControl call

	w := serve(t, h, http.MethodPost, "/api/service/stop", "")
	if w.Code != http.StatusNotImplemented {
		t.Errorf("POST /api/service/stop without callback: want 501, got %d", w.Code)
	}
	var resp struct {
		OK bool `json:"ok"`
	}
	json.NewDecoder(w.Body).Decode(&resp) //nolint:errcheck
	if resp.OK {
		t.Error("POST /api/service/stop without callback: want ok=false")
	}
}

func TestServiceRestart_GET_DoesNotTriggerAction(t *testing.T) {
	// GET /api/service/restart is not a registered route (only POST is).
	// It falls through to the /api/ catch-all and returns 404.
	// This is the intended safety behaviour: service actions require an explicit POST.
	h, _, _ := newTestWebHandler(t, true, safeEv())
	triggered := false
	h.WithServiceControl(func() { triggered = true }, nil)

	w := serve(t, h, http.MethodGet, "/api/service/restart", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("GET /api/service/restart: want 404, got %d", w.Code)
	}
	if triggered {
		t.Error("GET /api/service/restart: must not trigger restart callback")
	}
}

func TestServiceStop_GET_DoesNotTriggerAction(t *testing.T) {
	// GET /api/service/stop is not a registered route (only POST is).
	// It falls through to the /api/ catch-all and returns 404.
	h, _, _ := newTestWebHandler(t, true, safeEv())
	triggered := false
	h.WithServiceControl(nil, func() { triggered = true })

	w := serve(t, h, http.MethodGet, "/api/service/stop", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("GET /api/service/stop: want 404, got %d", w.Code)
	}
	if triggered {
		t.Error("GET /api/service/stop: must not trigger stop callback")
	}
}

func TestDashboard_ServiceControlsVisible_WhenWired(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	h.WithServiceControl(func() {}, func() {})

	w := serve(t, h, http.MethodGet, "/", "")
	if w.Code != http.StatusOK {
		t.Fatalf("dashboard: want 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "service-controls") {
		t.Error("dashboard: expected service-controls section when callbacks wired")
	}
	if !strings.Contains(body, "Restart service") {
		t.Error("dashboard: expected 'Restart service' button")
	}
	if !strings.Contains(body, "Stop service") {
		t.Error("dashboard: expected 'Stop service' button")
	}
}

func TestDashboard_ServiceControlsHidden_WhenNotWired(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	// no WithServiceControl

	w := serve(t, h, http.MethodGet, "/", "")
	if w.Code != http.StatusOK {
		t.Fatalf("dashboard: want 200, got %d", w.Code)
	}
	body := w.Body.String()
	if strings.Contains(body, "service-controls") {
		t.Error("dashboard: service-controls section should be absent when not wired")
	}
}

func TestGetSetup_DisplaysOptionalFloats(t *testing.T) {
	h, cfgHolder, _ := newTestWebHandler(t, true, safeEv())

	// Set optional float values
	cfg := cfgHolder.Get()
	sqmVal := 18.75
	humidVal := 85.5
	dewVal := 2.3
	cfg.SQMMinSafe = &sqmVal
	cfg.HumidityMaxSafe = &humidVal
	cfg.DewpointMarginMinC = &dewVal
	cfgHolder.Update(cfg)

	w := serve(t, h, http.MethodGet, "/setup", "")
	if w.Code != http.StatusOK {
		t.Fatalf("GET /setup: want 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "18.75") {
		t.Error("GET /setup: expected SQM_MIN_SAFE value 18.75 in response")
	}
	if !strings.Contains(body, "85.5") {
		t.Error("GET /setup: expected HUMIDITY_MAX_SAFE value 85.5 in response")
	}
	if !strings.Contains(body, "2.3") {
		t.Error("GET /setup: expected DEWPOINT_MARGIN_MIN_C value 2.3 in response")
	}
}

// ---------- route conflict prevention tests ---------------------------------

func TestDashboard_GET_RootPath(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	w := serve(t, h, http.MethodGet, "/", "")
	if w.Code != http.StatusOK {
		t.Errorf("GET /: want 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "SQMeter") {
		t.Error("GET /: expected dashboard HTML")
	}
}

func TestDashboard_HEAD_RootPath(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	w := serve(t, h, http.MethodHead, "/", "")
	if w.Code != http.StatusOK {
		t.Errorf("HEAD /: want 200, got %d", w.Code)
	}
}

func TestDashboard_POST_RootPath_MethodNotAllowed(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	w := serve(t, h, http.MethodPost, "/", "")
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /: want 405, got %d", w.Code)
	}
	allow := w.Header().Get("Allow")
	if allow != "GET, HEAD" {
		t.Errorf("POST /: want Allow: GET, HEAD, got %q", allow)
	}
}

func TestDashboard_UnknownPath_NotFound(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	w := serve(t, h, http.MethodGet, "/nonexistent", "")
	if w.Code != http.StatusNotFound {
		t.Errorf("GET /nonexistent: want 404, got %d", w.Code)
	}
}

// ---------- redesigned dashboard branding and structure ----------------------

func TestDashboard_IncludesASCOMSQMeterBridgeBranding(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	w := serve(t, h, http.MethodGet, "/", "")
	if w.Code != http.StatusOK {
		t.Fatalf("dashboard: want 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "ASCOM SQMeter Bridge") {
		t.Error("dashboard: expected 'ASCOM SQMeter Bridge' branding")
	}
}

func TestDashboard_IncludesSafetyMonitorSection(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	w := serve(t, h, http.MethodGet, "/", "")
	body := w.Body.String()
	if !strings.Contains(body, "Safety Monitor") {
		t.Error("dashboard: expected 'Safety Monitor' section")
	}
	if !strings.Contains(body, "SafetyMonitor/0") {
		t.Error("dashboard: expected 'SafetyMonitor/0' device listed")
	}
}

func TestDashboard_IncludesObservingConditionsSection(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	w := serve(t, h, http.MethodGet, "/", "")
	body := w.Body.String()
	if !strings.Contains(body, "Observing Conditions") {
		t.Error("dashboard: expected 'Observing Conditions' section")
	}
	if !strings.Contains(body, "ObservingConditions/0") {
		t.Error("dashboard: expected 'ObservingConditions/0' device listed")
	}
}

func TestDashboard_NetworkExposureWarning_WhenWideOpen(t *testing.T) {
	h, cfgHolder, _ := newTestWebHandler(t, true, safeEv())
	cfg := *cfgHolder.Get()
	cfg.AlpacaHTTPBind = "0.0.0.0"
	cfgHolder.Update(&cfg) //nolint:errcheck

	w := serve(t, h, http.MethodGet, "/", "")
	body := w.Body.String()
	if !strings.Contains(body, "0.0.0.0") {
		t.Error("dashboard: expected bind address 0.0.0.0 in network warning")
	}
	if !strings.Contains(body, "reachable from the network") {
		t.Error("dashboard: expected network exposure warning text")
	}
}

func TestDashboard_NoNetworkWarning_WhenLoopback(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	// default bind is 127.0.0.1
	w := serve(t, h, http.MethodGet, "/", "")
	body := w.Body.String()
	if strings.Contains(body, "reachable from the network") {
		t.Error("dashboard: must not show network warning for 127.0.0.1 bind")
	}
}
