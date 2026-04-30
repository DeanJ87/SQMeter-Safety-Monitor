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

// ---------- POST /api/test-sqmeter ------------------------------------------

func TestTestSQMeter_ReachableURL(t *testing.T) {
	// Spin up a small HTTP server to act as the SQMeter.
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

func TestTestSQMeter_UnreachableURL(t *testing.T) {
	h, _, _ := newTestWebHandler(t, true, safeEv())
	// Use an address that is not listening — connect should fail quickly.
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

	// Send empty JSON body — handler should fall back to configured URL.
	w := serve(t, h, http.MethodPost, "/api/test-sqmeter", `{}`)
	var resp struct {
		OK bool `json:"ok"`
	}
	json.NewDecoder(w.Body).Decode(&resp)
	if !resp.OK {
		t.Error("test-sqmeter: want ok=true when falling back to configured URL")
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
