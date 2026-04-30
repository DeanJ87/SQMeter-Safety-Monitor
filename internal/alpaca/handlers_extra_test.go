package alpaca_test

import (
	"context"
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

func doPUT(t *testing.T, h *alpaca.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	h.Register(mux)
	req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

// ---------- connected / connecting -------------------------------------------

func TestGetConnected_True(t *testing.T) {
	h := newTestHandler(true, safeState())
	w := doGET(t, h, "/api/v1/safetymonitor/0/connected")
	var resp alpaca.Response[bool]
	json.NewDecoder(w.Body).Decode(&resp)
	if !resp.Value {
		t.Error("expected Connected=true")
	}
}

func TestGetConnected_False(t *testing.T) {
	h := newTestHandler(false, safeState())
	w := doGET(t, h, "/api/v1/safetymonitor/0/connected")
	var resp alpaca.Response[bool]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Value {
		t.Error("expected Connected=false")
	}
}

func TestGetConnecting_AlwaysFalse(t *testing.T) {
	h := newTestHandler(true, safeState())
	w := doGET(t, h, "/api/v1/safetymonitor/0/connecting")
	var resp alpaca.Response[bool]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != 0 {
		t.Fatalf("unexpected error: %v", resp.ErrorMessage)
	}
	if resp.Value {
		t.Error("Connecting should always be false")
	}
}

// ---------- metadata endpoints -----------------------------------------------

func TestGetDeviceDescription_NonEmpty(t *testing.T) {
	h := newTestHandler(true, safeState())
	w := doGET(t, h, "/api/v1/safetymonitor/0/description")
	var resp alpaca.Response[string]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Value == "" {
		t.Error("Description should not be empty")
	}
}

func TestGetDriverInfo_NonEmpty(t *testing.T) {
	h := newTestHandler(true, safeState())
	w := doGET(t, h, "/api/v1/safetymonitor/0/driverinfo")
	var resp alpaca.Response[string]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Value == "" {
		t.Error("DriverInfo should not be empty")
	}
}

func TestGetDriverVersion_MatchesConstructorArg(t *testing.T) {
	h := newTestHandler(true, safeState())
	w := doGET(t, h, "/api/v1/safetymonitor/0/driverversion")
	var resp alpaca.Response[string]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Value != "0.1.0-test" {
		t.Errorf("DriverVersion: want 0.1.0-test, got %q", resp.Value)
	}
}

func TestGetInterfaceVersion_IsOne(t *testing.T) {
	h := newTestHandler(true, safeState())
	w := doGET(t, h, "/api/v1/safetymonitor/0/interfaceversion")
	var resp alpaca.Response[int]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Value != 1 {
		t.Errorf("InterfaceVersion: want 1, got %d", resp.Value)
	}
}

func TestGetName_NonEmpty(t *testing.T) {
	h := newTestHandler(true, safeState())
	w := doGET(t, h, "/api/v1/safetymonitor/0/name")
	var resp alpaca.Response[string]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Value == "" {
		t.Error("Name should not be empty")
	}
}

// ---------- DeviceState ------------------------------------------------------

func TestGetDeviceState_ContainsRequiredItems(t *testing.T) {
	h := newTestHandler(true, safeState())
	w := doGET(t, h, "/api/v1/safetymonitor/0/devicestate")
	var resp alpaca.Response[[]alpaca.DeviceStateItem]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != 0 {
		t.Fatalf("unexpected error: %v", resp.ErrorMessage)
	}
	names := map[string]bool{}
	for _, item := range resp.Value {
		names[item.Name] = true
	}
	for _, required := range []string{"IsSafe", "TimeStamp"} {
		if !names[required] {
			t.Errorf("DeviceState missing item %q, got: %v", required, resp.Value)
		}
	}
}

// ---------- PUT connect / disconnect -----------------------------------------

func TestPutConnect_SetsConnected(t *testing.T) {
	cfgHolder := config.NewHolder(config.Defaults(), "")
	holder := state.NewHolder(false)
	h := alpaca.New(cfgHolder, holder, "uuid", "0.1.0", nil)

	w := doPUT(t, h, "/api/v1/safetymonitor/0/connect", "ClientID=1&ClientTransactionID=1")
	var resp alpaca.VoidResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != 0 {
		t.Errorf("unexpected error: %v", resp.ErrorMessage)
	}
	if !holder.IsConnected() {
		t.Error("expected connected after PUT /connect")
	}
}

func TestPutDisconnect_SetsDisconnected(t *testing.T) {
	cfgHolder := config.NewHolder(config.Defaults(), "")
	holder := state.NewHolder(true)
	h := alpaca.New(cfgHolder, holder, "uuid", "0.1.0", nil)

	w := doPUT(t, h, "/api/v1/safetymonitor/0/disconnect", "ClientID=1&ClientTransactionID=1")
	var resp alpaca.VoidResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != 0 {
		t.Errorf("unexpected error: %v", resp.ErrorMessage)
	}
	if holder.IsConnected() {
		t.Error("expected disconnected after PUT /disconnect")
	}
}

func TestPutConnected_SetFalse(t *testing.T) {
	cfgHolder := config.NewHolder(config.Defaults(), "")
	holder := state.NewHolder(true)
	h := alpaca.New(cfgHolder, holder, "uuid", "0.1.0", nil)

	w := doPUT(t, h, "/api/v1/safetymonitor/0/connected",
		"Connected=false&ClientID=1&ClientTransactionID=1")
	var resp alpaca.VoidResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != 0 {
		t.Errorf("unexpected error: %v", resp.ErrorMessage)
	}
	if holder.IsConnected() {
		t.Error("expected disconnected after PUT Connected=false")
	}
}

func TestPutConnected_InvalidValue_ReturnsError(t *testing.T) {
	h := newTestHandler(true, safeState())
	w := doPUT(t, h, "/api/v1/safetymonitor/0/connected",
		"Connected=maybe&ClientID=1&ClientTransactionID=1")
	var resp alpaca.VoidResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber == 0 {
		t.Error("expected non-zero error for invalid Connected value")
	}
}

// ---------- PUT action (refresh) ---------------------------------------------

func TestPutAction_Refresh_CallsRefreshFn(t *testing.T) {
	called := false
	cfgHolder := config.NewHolder(config.Defaults(), "")
	holder := state.NewHolder(true)
	holder.Update(state.EvaluatedState{
		IsSafe:                true,
		Reasons:               []string{},
		Warnings:              []string{},
		LastSuccessfulPollUTC: time.Now(),
		LastPollUTC:           time.Now(),
	})
	h := alpaca.New(cfgHolder, holder, "uuid", "0.1.0", func(_ context.Context) {
		called = true
	})

	mux := http.NewServeMux()
	h.Register(mux)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/safetymonitor/0/action",
		strings.NewReader("Action=refresh&ClientID=1&ClientTransactionID=1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var resp alpaca.Response[string]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != 0 {
		t.Fatalf("unexpected error: %v", resp.ErrorMessage)
	}
	if !called {
		t.Error("expected refresh function to be called")
	}
}

// ---------- not-implemented commands -----------------------------------------

func TestPutCommandBool_NotImplemented(t *testing.T) {
	h := newTestHandler(true, safeState())
	w := doPUT(t, h, "/api/v1/safetymonitor/0/commandbool", "Command=foo")
	var resp alpaca.VoidResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != alpaca.ErrNotImplemented {
		t.Errorf("want ErrNotImplemented (%d), got %d", alpaca.ErrNotImplemented, resp.ErrorNumber)
	}
}

func TestPutCommandString_NotImplemented(t *testing.T) {
	h := newTestHandler(true, safeState())
	w := doPUT(t, h, "/api/v1/safetymonitor/0/commandstring", "Command=foo")
	var resp alpaca.VoidResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != alpaca.ErrNotImplemented {
		t.Errorf("want ErrNotImplemented (%d), got %d", alpaca.ErrNotImplemented, resp.ErrorNumber)
	}
}
