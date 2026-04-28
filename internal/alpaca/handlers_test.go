package alpaca_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"sqmeter-alpaca-safetymonitor/internal/alpaca"
	"sqmeter-alpaca-safetymonitor/internal/config"
	"sqmeter-alpaca-safetymonitor/internal/state"
)

func newTestHandler(connected bool, ev state.EvaluatedState) *alpaca.Handler {
	cfgHolder := config.NewHolder(config.Defaults(), "")
	holder := state.NewHolder(connected)
	holder.Update(ev)
	return alpaca.New(cfgHolder, holder, "test-uuid-1234-5678-90ab", "0.1.0-test", nil)
}

func safeState() state.EvaluatedState {
	now := time.Now().UTC()
	return state.EvaluatedState{
		IsSafe:                true,
		Reasons:               []string{},
		Warnings:              []string{},
		LastSuccessfulPollUTC: now,
		LastPollUTC:           now,
	}
}

func doGET(t *testing.T, h *alpaca.Handler, path string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	h.Register(mux)
	req := httptest.NewRequest(http.MethodGet, path+"?ClientID=1&ClientTransactionID=42", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

// ---------- envelope shape ---------------------------------------------------

func TestEnvelopeShape(t *testing.T) {
	h := newTestHandler(true, safeState())
	w := doGET(t, h, "/management/apiversions")

	var env map[string]any
	if err := json.NewDecoder(w.Body).Decode(&env); err != nil {
		t.Fatalf("decode: %v", err)
	}
	for _, field := range []string{"Value", "ClientTransactionID", "ServerTransactionID", "ErrorNumber", "ErrorMessage"} {
		if _, ok := env[field]; !ok {
			t.Errorf("envelope missing field %q", field)
		}
	}
}

func TestClientTransactionIDEchoed(t *testing.T) {
	h := newTestHandler(true, safeState())
	w := doGET(t, h, "/management/apiversions")

	var env struct {
		ClientTransactionID uint32 `json:"ClientTransactionID"`
	}
	json.NewDecoder(w.Body).Decode(&env)
	if env.ClientTransactionID != 42 {
		t.Errorf("ClientTransactionID: want 42, got %d", env.ClientTransactionID)
	}
}

func TestServerTransactionIDIncrements(t *testing.T) {
	h := newTestHandler(true, safeState())
	mux := http.NewServeMux()
	h.Register(mux)

	var ids []uint32
	for range 3 {
		req := httptest.NewRequest(http.MethodGet, "/management/apiversions", nil)
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, req)
		var env struct{ ServerTransactionID uint32 }
		json.NewDecoder(w.Body).Decode(&env)
		ids = append(ids, env.ServerTransactionID)
	}
	for i := 1; i < len(ids); i++ {
		if ids[i] <= ids[i-1] {
			t.Errorf("ServerTransactionID not monotonically increasing: %v", ids)
		}
	}
}

// ---------- management endpoints ---------------------------------------------

func TestGetAPIVersions(t *testing.T) {
	h := newTestHandler(true, safeState())
	w := doGET(t, h, "/management/apiversions")

	var resp alpaca.Response[[]int]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != 0 {
		t.Errorf("unexpected error: %v", resp.ErrorMessage)
	}
	if len(resp.Value) == 0 || resp.Value[0] != 1 {
		t.Errorf("expected [1], got %v", resp.Value)
	}
}

func TestGetDescription(t *testing.T) {
	h := newTestHandler(true, safeState())
	w := doGET(t, h, "/management/v1/description")

	var resp alpaca.Response[alpaca.ServerDescription]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != 0 {
		t.Fatalf("error: %v", resp.ErrorMessage)
	}
	if resp.Value.ServerName == "" {
		t.Error("ServerName should not be empty")
	}
}

func TestGetConfiguredDevices(t *testing.T) {
	h := newTestHandler(true, safeState())
	w := doGET(t, h, "/management/v1/configureddevices")

	var resp alpaca.Response[[]alpaca.ConfiguredDevice]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != 0 {
		t.Fatalf("error: %v", resp.ErrorMessage)
	}
	if len(resp.Value) != 1 {
		t.Fatalf("expected 1 configured device, got %d", len(resp.Value))
	}
	dev := resp.Value[0]
	if dev.DeviceType != "SafetyMonitor" {
		t.Errorf("DeviceType: want SafetyMonitor, got %q", dev.DeviceType)
	}
	if dev.DeviceNumber != 0 {
		t.Errorf("DeviceNumber: want 0, got %d", dev.DeviceNumber)
	}
	if dev.UniqueID == "" {
		t.Error("UniqueID should not be empty")
	}
}

// ---------- IsSafe -----------------------------------------------------------

func TestGetIsSafe_Safe(t *testing.T) {
	h := newTestHandler(true, safeState())
	w := doGET(t, h, "/api/v1/safetymonitor/0/issafe")

	var resp alpaca.Response[bool]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != 0 {
		t.Fatalf("unexpected error %d: %s", resp.ErrorNumber, resp.ErrorMessage)
	}
	if !resp.Value {
		t.Error("expected IsSafe=true")
	}
}

func TestGetIsSafe_Unsafe(t *testing.T) {
	ev := safeState()
	ev.IsSafe = false
	ev.Reasons = []string{"cloud cover 85.0% >= unsafe threshold 80.0%"}
	h := newTestHandler(true, ev)
	w := doGET(t, h, "/api/v1/safetymonitor/0/issafe")

	var resp alpaca.Response[bool]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != 0 {
		t.Fatalf("unexpected error: %v", resp.ErrorMessage)
	}
	if resp.Value {
		t.Error("expected IsSafe=false")
	}
}

func TestGetIsSafe_NotConnected(t *testing.T) {
	h := newTestHandler(false, safeState())
	w := doGET(t, h, "/api/v1/safetymonitor/0/issafe")

	var resp alpaca.Response[bool]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != alpaca.ErrNotConnected {
		t.Errorf("want ErrorNumber=%d (NotConnected), got %d", alpaca.ErrNotConnected, resp.ErrorNumber)
	}
}

// ---------- PUT connected ----------------------------------------------------

func TestPutConnected_SetTrue(t *testing.T) {
	cfgHolder := config.NewHolder(config.Defaults(), "")
	holder := state.NewHolder(false)
	h := alpaca.New(cfgHolder, holder, "uuid", "0.1.0", nil)
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/safetymonitor/0/connected",
		strings.NewReader("Connected=true&ClientID=1&ClientTransactionID=1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var resp alpaca.VoidResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != 0 {
		t.Errorf("unexpected error: %v", resp.ErrorMessage)
	}
	if !holder.IsConnected() {
		t.Error("expected holder to be connected after PUT Connected=true")
	}
}

// ---------- SupportedActions -------------------------------------------------

func TestGetSupportedActions(t *testing.T) {
	h := newTestHandler(true, safeState())
	w := doGET(t, h, "/api/v1/safetymonitor/0/supportedactions")

	var resp alpaca.Response[[]string]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != 0 {
		t.Fatalf("error: %v", resp.ErrorMessage)
	}

	found := map[string]bool{}
	for _, a := range resp.Value {
		found[a] = true
	}
	for _, want := range []string{"status", "refresh"} {
		if !found[want] {
			t.Errorf("supported actions missing %q, got %v", want, resp.Value)
		}
	}
}

// ---------- Action -----------------------------------------------------------

func TestPutAction_Status(t *testing.T) {
	h := newTestHandler(true, safeState())
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/safetymonitor/0/action",
		strings.NewReader("Action=status&ClientID=1&ClientTransactionID=1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var resp alpaca.Response[string]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != 0 {
		t.Fatalf("error: %v", resp.ErrorMessage)
	}
	if resp.Value == "" {
		t.Error("expected non-empty status JSON string")
	}
}

func TestPutAction_UnknownAction(t *testing.T) {
	h := newTestHandler(true, safeState())
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/safetymonitor/0/action",
		strings.NewReader("Action=bogus&ClientID=1&ClientTransactionID=1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var resp alpaca.Response[string]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber == 0 {
		t.Error("expected non-zero error for unknown action")
	}
}

// ---------- commandXxx not implemented ---------------------------------------

func TestPutCommandBlind_NotImplemented(t *testing.T) {
	h := newTestHandler(true, safeState())
	mux := http.NewServeMux()
	h.Register(mux)

	req := httptest.NewRequest(http.MethodPut, "/api/v1/safetymonitor/0/commandblind",
		strings.NewReader("Command=foo"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var resp alpaca.VoidResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != alpaca.ErrNotImplemented {
		t.Errorf("want ErrorNumber=%d (NotImplemented), got %d", alpaca.ErrNotImplemented, resp.ErrorNumber)
	}
}
