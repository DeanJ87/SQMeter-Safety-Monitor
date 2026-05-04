package web_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"sqmeter-ascom-alpaca/internal/alpaca"
	"sqmeter-ascom-alpaca/internal/config"
	"sqmeter-ascom-alpaca/internal/state"
	"sqmeter-ascom-alpaca/internal/web"
)

// TestRouteRegistration_NoConflict verifies that web and alpaca handlers can
// be registered together without ServeMux pattern conflicts (Go 1.23+).
func TestRouteRegistration_NoConflict(t *testing.T) {
	cfgHolder := config.NewHolder(config.Defaults(), "")
	holder := state.NewHolder(true)

	webHandler, err := web.New(cfgHolder, holder)
	if err != nil {
		t.Fatalf("web.New: %v", err)
	}

	alpacaHandler := alpaca.New(cfgHolder, holder, "test-uuid", "v0.0.0-test", nil)

	// Register both handlers (this would panic on Go 1.23 if routes conflict)
	mux := http.NewServeMux()
	alpacaHandler.Register(mux)
	webHandler.Register(mux)

	// Test GET / returns dashboard HTML with correct branding
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("GET /: want 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "ASCOM SQMeter Bridge") {
		t.Error("GET /: expected ASCOM SQMeter Bridge branding in dashboard HTML")
	}

	// Test unknown /api/ path returns JSON 404, not HTML
	req = httptest.NewRequest(http.MethodGet, "/api/v1/badpath", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("GET /api/v1/badpath: want 404, got %d", w.Code)
	}
	body = w.Body.String()
	if strings.Contains(body, "<html") || strings.Contains(body, "ASCOM SQMeter Bridge") {
		t.Error("GET /api/v1/badpath: must not return HTML dashboard")
	}
	if !strings.Contains(body, "not found") {
		t.Error("GET /api/v1/badpath: expected 'not found' in JSON response")
	}

	// Test valid Alpaca endpoint still works
	req = httptest.NewRequest(http.MethodGet, "/api/v1/safetymonitor/0/description", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("GET /api/v1/safetymonitor/0/description: want 200, got %d", w.Code)
	}

	// Test unknown web path returns 404
	req = httptest.NewRequest(http.MethodGet, "/nonexistent", nil)
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("GET /nonexistent: want 404, got %d", w.Code)
	}
}
