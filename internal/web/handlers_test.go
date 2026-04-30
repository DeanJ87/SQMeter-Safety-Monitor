package web_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"sqmeter-alpaca-safetymonitor/internal/config"
	"sqmeter-alpaca-safetymonitor/internal/discovery"
	"sqmeter-alpaca-safetymonitor/internal/state"
	"sqmeter-alpaca-safetymonitor/internal/web"
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
	return state.EvaluatedState{
		IsSafe:                true,
		Reasons:               []string{},
		Warnings:              []string{},
		LastPollUTC:           now,
		LastSuccessfulPollUTC: now,
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

func TestDashboard_AliasedFromStatus(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	w := serve(t, h, http.MethodGet, "/status", "")
	if w.Code != http.StatusOK {
		t.Errorf("/status: want 200, got %d", w.Code)
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
		NeedsRestart bool `json:"needsRestart"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if !resp.NeedsRestart {
		t.Error("expected needsRestart=true when HTTP port changes")
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
