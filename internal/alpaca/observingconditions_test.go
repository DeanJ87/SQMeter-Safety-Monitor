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
	"sqmeter-alpaca-safetymonitor/internal/sqmclient"
	"sqmeter-alpaca-safetymonitor/internal/state"
)

// ---------- test helpers -----------------------------------------------------

func newOCHandler(connected bool, ev state.EvaluatedState) *alpaca.OCHandler {
	cfgHolder := config.NewHolder(config.Defaults(), "")
	holder := state.NewHolder(connected)
	holder.Update(ev)
	return alpaca.NewOC(cfgHolder, holder, "oc-uuid-1234-5678-90ab", "0.1.0-test", nil)
}

// newOCMux registers both a SafetyMonitor (for the /api/ catch-all) and the
// given OCHandler on a fresh mux.
func newOCMux(h *alpaca.OCHandler) *http.ServeMux {
	smHolder := state.NewHolder(true)
	smCfg := config.NewHolder(config.Defaults(), "")
	sm := alpaca.New(smCfg, smHolder, "sm-uuid", "0.1.0-test", nil)
	mux := http.NewServeMux()
	sm.Register(mux)
	h.Register(mux)
	return mux
}

func ocGET(t *testing.T, h *alpaca.OCHandler, path string) *httptest.ResponseRecorder {
	t.Helper()
	mux := newOCMux(h)
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	req := httptest.NewRequest(http.MethodGet, path+sep+"ClientID=1&ClientTransactionID=42", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func ocPUT(t *testing.T, h *alpaca.OCHandler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	mux := newOCMux(h)
	req := httptest.NewRequest(http.MethodPut, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	return w
}

func goodSensors() *sqmclient.SensorsResponse {
	return &sqmclient.SensorsResponse{
		LightSensor: sqmclient.LightSensor{Lux: 0.05, Status: 0},
		SkyQuality:  sqmclient.SkyQuality{SQM: 21.5, Bortle: 3},
		Environment: sqmclient.Environment{
			Temperature: 12.3,
			Humidity:    55.0,
			Dewpoint:    3.4,
			Pressure:    1013.2,
			Status:      0,
		},
		IRTemperature: sqmclient.IRTemperature{
			ObjectTemp:  -30.0,
			AmbientTemp: 12.3,
			Status:      0,
		},
		CloudConditions: sqmclient.CloudConditions{
			CloudCoverPercent: 10.0,
		},
	}
}

func ocStateWithSensors() state.EvaluatedState {
	now := time.Now().UTC()
	sensors := goodSensors()
	return state.EvaluatedState{
		IsSafe:                true,
		Reasons:               []string{},
		Warnings:              []string{},
		LastSuccessfulPollUTC: now,
		LastPollUTC:           now,
		RawSensors:            sensors,
		Values: state.Values{
			SQM:               sensors.SkyQuality.SQM,
			Temperature:       sensors.Environment.Temperature,
			Humidity:          sensors.Environment.Humidity,
			Dewpoint:          sensors.Environment.Dewpoint,
			CloudCoverPercent: sensors.CloudConditions.CloudCoverPercent,
		},
	}
}

// ---------- management: configureddevices lists both devices -----------------

func TestOC_ConfiguredDevices_TwoDevices(t *testing.T) {
	cfgHolder := config.NewHolder(config.Defaults(), "")
	holder := state.NewHolder(true)
	smHandler := alpaca.New(cfgHolder, holder, "sm-uuid-1234", "0.1.0-test", nil)
	ocHandler := alpaca.NewOC(cfgHolder, holder, "oc-uuid-5678", "0.1.0-test", nil)
	smHandler.AddConfiguredDevice(ocHandler.ConfiguredDevice())

	mux := http.NewServeMux()
	smHandler.Register(mux)
	ocHandler.Register(mux)

	req := httptest.NewRequest(http.MethodGet, "/management/v1/configureddevices?ClientID=1&ClientTransactionID=1", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var resp alpaca.Response[[]alpaca.ConfiguredDevice]
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.ErrorNumber != 0 {
		t.Fatalf("error: %v", resp.ErrorMessage)
	}
	if len(resp.Value) != 2 {
		t.Fatalf("expected 2 configured devices, got %d", len(resp.Value))
	}
	types := map[string]bool{}
	for _, d := range resp.Value {
		types[d.DeviceType] = true
	}
	if !types["SafetyMonitor"] {
		t.Error("configureddevices missing SafetyMonitor")
	}
	if !types["ObservingConditions"] {
		t.Error("configureddevices missing ObservingConditions")
	}
}

// ---------- standard device endpoints ----------------------------------------

func TestOC_GetConnected_True(t *testing.T) {
	h := newOCHandler(true, ocStateWithSensors())
	w := ocGET(t, h, "/api/v1/observingconditions/0/connected")
	var resp alpaca.Response[bool]
	json.NewDecoder(w.Body).Decode(&resp)
	if !resp.Value {
		t.Error("expected Connected=true")
	}
}

func TestOC_GetConnected_False(t *testing.T) {
	h := newOCHandler(false, ocStateWithSensors())
	w := ocGET(t, h, "/api/v1/observingconditions/0/connected")
	var resp alpaca.Response[bool]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Value {
		t.Error("expected Connected=false")
	}
}

func TestOC_GetConnecting_AlwaysFalse(t *testing.T) {
	h := newOCHandler(true, ocStateWithSensors())
	w := ocGET(t, h, "/api/v1/observingconditions/0/connecting")
	var resp alpaca.Response[bool]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Value {
		t.Error("Connecting should always be false")
	}
}

func TestOC_GetDescription_NonEmpty(t *testing.T) {
	h := newOCHandler(true, ocStateWithSensors())
	w := ocGET(t, h, "/api/v1/observingconditions/0/description")
	var resp alpaca.Response[string]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Value == "" {
		t.Error("Description should not be empty")
	}
}

func TestOC_GetInterfaceVersion_IsOne(t *testing.T) {
	h := newOCHandler(true, ocStateWithSensors())
	w := ocGET(t, h, "/api/v1/observingconditions/0/interfaceversion")
	var resp alpaca.Response[int]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Value != 1 {
		t.Errorf("InterfaceVersion: want 1, got %d", resp.Value)
	}
}

func TestOC_GetName_NonEmpty(t *testing.T) {
	h := newOCHandler(true, ocStateWithSensors())
	w := ocGET(t, h, "/api/v1/observingconditions/0/name")
	var resp alpaca.Response[string]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Value == "" {
		t.Error("Name should not be empty")
	}
}

func TestOC_GetSupportedActions_Empty(t *testing.T) {
	h := newOCHandler(true, ocStateWithSensors())
	w := ocGET(t, h, "/api/v1/observingconditions/0/supportedactions")
	var resp alpaca.Response[[]string]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != 0 {
		t.Fatalf("error: %v", resp.ErrorMessage)
	}
	if len(resp.Value) != 0 {
		t.Errorf("expected empty supportedactions, got %v", resp.Value)
	}
}

func TestOC_GetDeviceState_ContainsExpectedItems(t *testing.T) {
	h := newOCHandler(true, ocStateWithSensors())
	w := ocGET(t, h, "/api/v1/observingconditions/0/devicestate")
	var resp alpaca.Response[[]alpaca.DeviceStateItem]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != 0 {
		t.Fatalf("error: %v", resp.ErrorMessage)
	}
	names := map[string]bool{}
	for _, item := range resp.Value {
		names[item.Name] = true
	}
	for _, want := range []string{"Temperature", "Humidity", "CloudCover", "TimeStamp"} {
		if !names[want] {
			t.Errorf("DeviceState missing %q, got %v", want, resp.Value)
		}
	}
}

// ---------- connect / disconnect ---------------------------------------------

func TestOC_PutConnect_SetsConnected(t *testing.T) {
	cfgHolder := config.NewHolder(config.Defaults(), "")
	holder := state.NewHolder(false)
	h := alpaca.NewOC(cfgHolder, holder, "uuid", "0.1.0", nil)
	w := ocPUT(t, h, "/api/v1/observingconditions/0/connect", "ClientID=1&ClientTransactionID=1")
	var resp alpaca.VoidResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != 0 {
		t.Errorf("unexpected error: %v", resp.ErrorMessage)
	}
	if !holder.IsConnected() {
		t.Error("expected connected after PUT /connect")
	}
}

func TestOC_PutDisconnect_SetsDisconnected(t *testing.T) {
	cfgHolder := config.NewHolder(config.Defaults(), "")
	holder := state.NewHolder(true)
	h := alpaca.NewOC(cfgHolder, holder, "uuid", "0.1.0", nil)
	w := ocPUT(t, h, "/api/v1/observingconditions/0/disconnect", "ClientID=1&ClientTransactionID=1")
	var resp alpaca.VoidResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != 0 {
		t.Errorf("unexpected error: %v", resp.ErrorMessage)
	}
	if holder.IsConnected() {
		t.Error("expected disconnected after PUT /disconnect")
	}
}

func TestOC_PutConnected_InvalidValue(t *testing.T) {
	h := newOCHandler(true, ocStateWithSensors())
	w := ocPUT(t, h, "/api/v1/observingconditions/0/connected", "Connected=maybe")
	var resp alpaca.VoidResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber == 0 {
		t.Error("expected error for invalid Connected value")
	}
}

func TestOC_PutAction_ReturnsError(t *testing.T) {
	h := newOCHandler(true, ocStateWithSensors())
	w := ocPUT(t, h, "/api/v1/observingconditions/0/action", "Action=anything&ClientID=1&ClientTransactionID=1")
	var resp alpaca.Response[string]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber == 0 {
		t.Error("expected error: OC has no custom actions")
	}
}

func TestOC_PutCommandBlind_NotImplemented(t *testing.T) {
	h := newOCHandler(true, ocStateWithSensors())
	w := ocPUT(t, h, "/api/v1/observingconditions/0/commandblind", "Command=foo")
	var resp alpaca.VoidResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != alpaca.ErrNotImplemented {
		t.Errorf("want ErrNotImplemented (%d), got %d", alpaca.ErrNotImplemented, resp.ErrorNumber)
	}
}

// ---------- averageperiod ----------------------------------------------------

func TestOC_GetAveragePeriod_ReturnsZero(t *testing.T) {
	h := newOCHandler(true, ocStateWithSensors())
	w := ocGET(t, h, "/api/v1/observingconditions/0/averageperiod")
	var resp alpaca.Response[float64]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != 0 {
		t.Fatalf("error: %v", resp.ErrorMessage)
	}
	if resp.Value != 0.0 {
		t.Errorf("AveragePeriod: want 0.0, got %f", resp.Value)
	}
}

func TestOC_PutAveragePeriod_ZeroAccepted(t *testing.T) {
	h := newOCHandler(true, ocStateWithSensors())
	w := ocPUT(t, h, "/api/v1/observingconditions/0/averageperiod", "AveragePeriod=0&ClientID=1")
	var resp alpaca.VoidResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != 0 {
		t.Errorf("unexpected error for AveragePeriod=0: %v", resp.ErrorMessage)
	}
}

func TestOC_PutAveragePeriod_NonZeroRejected(t *testing.T) {
	h := newOCHandler(true, ocStateWithSensors())
	w := ocPUT(t, h, "/api/v1/observingconditions/0/averageperiod", "AveragePeriod=1.5&ClientID=1")
	var resp alpaca.VoidResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != alpaca.ErrInvalidValue {
		t.Errorf("want ErrInvalidValue (%d), got %d", alpaca.ErrInvalidValue, resp.ErrorNumber)
	}
}

// ---------- sensor happy paths -----------------------------------------------

func TestOC_GetTemperature_Happy(t *testing.T) {
	h := newOCHandler(true, ocStateWithSensors())
	w := ocGET(t, h, "/api/v1/observingconditions/0/temperature")
	var resp alpaca.Response[float64]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != 0 {
		t.Fatalf("unexpected error: %v", resp.ErrorMessage)
	}
	if resp.Value != 12.3 {
		t.Errorf("Temperature: want 12.3, got %f", resp.Value)
	}
}

func TestOC_GetHumidity_Happy(t *testing.T) {
	h := newOCHandler(true, ocStateWithSensors())
	w := ocGET(t, h, "/api/v1/observingconditions/0/humidity")
	var resp alpaca.Response[float64]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != 0 {
		t.Fatalf("unexpected error: %v", resp.ErrorMessage)
	}
	if resp.Value != 55.0 {
		t.Errorf("Humidity: want 55.0, got %f", resp.Value)
	}
}

func TestOC_GetDewPoint_Happy(t *testing.T) {
	h := newOCHandler(true, ocStateWithSensors())
	w := ocGET(t, h, "/api/v1/observingconditions/0/dewpoint")
	var resp alpaca.Response[float64]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != 0 {
		t.Fatalf("unexpected error: %v", resp.ErrorMessage)
	}
	if resp.Value != 3.4 {
		t.Errorf("DewPoint: want 3.4, got %f", resp.Value)
	}
}

func TestOC_GetPressure_Happy(t *testing.T) {
	h := newOCHandler(true, ocStateWithSensors())
	w := ocGET(t, h, "/api/v1/observingconditions/0/pressure")
	var resp alpaca.Response[float64]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != 0 {
		t.Fatalf("unexpected error: %v", resp.ErrorMessage)
	}
	if resp.Value != 1013.2 {
		t.Errorf("Pressure: want 1013.2, got %f", resp.Value)
	}
}

func TestOC_GetSkyQuality_Happy(t *testing.T) {
	h := newOCHandler(true, ocStateWithSensors())
	w := ocGET(t, h, "/api/v1/observingconditions/0/skyquality")
	var resp alpaca.Response[float64]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != 0 {
		t.Fatalf("unexpected error: %v", resp.ErrorMessage)
	}
	if resp.Value != 21.5 {
		t.Errorf("SkyQuality: want 21.5, got %f", resp.Value)
	}
}

func TestOC_GetSkyBrightness_Happy(t *testing.T) {
	h := newOCHandler(true, ocStateWithSensors())
	w := ocGET(t, h, "/api/v1/observingconditions/0/skybrightness")
	var resp alpaca.Response[float64]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != 0 {
		t.Fatalf("unexpected error: %v", resp.ErrorMessage)
	}
	if resp.Value != 0.05 {
		t.Errorf("SkyBrightness: want 0.05, got %f", resp.Value)
	}
}

func TestOC_GetSkyTemperature_Happy(t *testing.T) {
	h := newOCHandler(true, ocStateWithSensors())
	w := ocGET(t, h, "/api/v1/observingconditions/0/skytemperature")
	var resp alpaca.Response[float64]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != 0 {
		t.Fatalf("unexpected error: %v", resp.ErrorMessage)
	}
	if resp.Value != -30.0 {
		t.Errorf("SkyTemperature: want -30.0, got %f", resp.Value)
	}
}

func TestOC_GetCloudCover_Happy(t *testing.T) {
	h := newOCHandler(true, ocStateWithSensors())
	w := ocGET(t, h, "/api/v1/observingconditions/0/cloudcover")
	var resp alpaca.Response[float64]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != 0 {
		t.Fatalf("unexpected error: %v", resp.ErrorMessage)
	}
	if resp.Value != 10.0 {
		t.Errorf("CloudCover: want 10.0, got %f", resp.Value)
	}
}

// ---------- not connected -----------------------------------------------------

func TestOC_SensorEndpoints_NotConnected_ReturnNotConnected(t *testing.T) {
	paths := []string{
		"/api/v1/observingconditions/0/temperature",
		"/api/v1/observingconditions/0/humidity",
		"/api/v1/observingconditions/0/dewpoint",
		"/api/v1/observingconditions/0/pressure",
		"/api/v1/observingconditions/0/skyquality",
		"/api/v1/observingconditions/0/skybrightness",
		"/api/v1/observingconditions/0/skytemperature",
		"/api/v1/observingconditions/0/cloudcover",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			h := newOCHandler(false, ocStateWithSensors())
			w := ocGET(t, h, path)
			var resp alpaca.Response[float64]
			json.NewDecoder(w.Body).Decode(&resp)
			if resp.ErrorNumber != alpaca.ErrNotConnected {
				t.Errorf("want ErrNotConnected (%d), got %d (%s)",
					alpaca.ErrNotConnected, resp.ErrorNumber, resp.ErrorMessage)
			}
		})
	}
}

// ---------- no data yet -------------------------------------------------------

func TestOC_SensorEndpoint_NoData_ReturnsError(t *testing.T) {
	emptyState := state.EvaluatedState{
		Reasons:  []string{},
		Warnings: []string{},
		// RawSensors nil — no poll has succeeded
	}
	h := newOCHandler(true, emptyState)
	w := ocGET(t, h, "/api/v1/observingconditions/0/temperature")
	var resp alpaca.Response[float64]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber == 0 {
		t.Error("expected error when no sensor data is available")
	}
}

// ---------- sensor error ------------------------------------------------------

func TestOC_Temperature_SensorError_ReturnsUnspecified(t *testing.T) {
	ev := ocStateWithSensors()
	ev.RawSensors.Environment.Status = 2 // read error
	h := newOCHandler(true, ev)
	w := ocGET(t, h, "/api/v1/observingconditions/0/temperature")
	var resp alpaca.Response[float64]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != alpaca.ErrUnspecified {
		t.Errorf("want ErrUnspecified (%d), got %d", alpaca.ErrUnspecified, resp.ErrorNumber)
	}
}

func TestOC_SkyQuality_SensorError_ReturnsUnspecified(t *testing.T) {
	ev := ocStateWithSensors()
	ev.RawSensors.LightSensor.Status = 1 // not found
	h := newOCHandler(true, ev)
	w := ocGET(t, h, "/api/v1/observingconditions/0/skyquality")
	var resp alpaca.Response[float64]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != alpaca.ErrUnspecified {
		t.Errorf("want ErrUnspecified (%d), got %d", alpaca.ErrUnspecified, resp.ErrorNumber)
	}
}

// ---------- unsupported sensors -----------------------------------------------

func TestOC_UnsupportedSensors_NotImplemented(t *testing.T) {
	paths := []string{
		"/api/v1/observingconditions/0/rainrate",
		"/api/v1/observingconditions/0/starfwhm",
		"/api/v1/observingconditions/0/winddirection",
		"/api/v1/observingconditions/0/windgust",
		"/api/v1/observingconditions/0/windspeed",
	}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			h := newOCHandler(true, ocStateWithSensors())
			w := ocGET(t, h, path)
			var resp alpaca.Response[float64]
			json.NewDecoder(w.Body).Decode(&resp)
			if resp.ErrorNumber != alpaca.ErrNotImplemented {
				t.Errorf("want ErrNotImplemented (%d), got %d", alpaca.ErrNotImplemented, resp.ErrorNumber)
			}
		})
	}
}

// ---------- timesincemeasurement ---------------------------------------------

func TestOC_TimeSinceMeasurement_Supported(t *testing.T) {
	h := newOCHandler(true, ocStateWithSensors())
	w := ocGET(t, h, "/api/v1/observingconditions/0/timesincemeasurement?SensorName=temperature")
	var resp alpaca.Response[float64]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != 0 {
		t.Fatalf("unexpected error: %v", resp.ErrorMessage)
	}
	if resp.Value < 0 {
		t.Errorf("TimeSinceMeasurement should be >= 0, got %f", resp.Value)
	}
}

func TestOC_TimeSinceMeasurement_Unsupported(t *testing.T) {
	h := newOCHandler(true, ocStateWithSensors())
	w := ocGET(t, h, "/api/v1/observingconditions/0/timesincemeasurement?SensorName=rainrate")
	var resp alpaca.Response[float64]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != alpaca.ErrNotImplemented {
		t.Errorf("want ErrNotImplemented (%d), got %d", alpaca.ErrNotImplemented, resp.ErrorNumber)
	}
}

func TestOC_TimeSinceMeasurement_UnknownSensor(t *testing.T) {
	h := newOCHandler(true, ocStateWithSensors())
	w := ocGET(t, h, "/api/v1/observingconditions/0/timesincemeasurement?SensorName=bogus")
	var resp alpaca.Response[float64]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != alpaca.ErrInvalidValue {
		t.Errorf("want ErrInvalidValue (%d), got %d", alpaca.ErrInvalidValue, resp.ErrorNumber)
	}
}

func TestOC_TimeSinceMeasurement_NoData(t *testing.T) {
	emptyState := state.EvaluatedState{Reasons: []string{}, Warnings: []string{}}
	h := newOCHandler(true, emptyState)
	w := ocGET(t, h, "/api/v1/observingconditions/0/timesincemeasurement?SensorName=temperature")
	var resp alpaca.Response[float64]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber == 0 {
		t.Error("expected error when no measurement has been taken")
	}
}

// ---------- sensordescription ------------------------------------------------

func TestOC_SensorDescription_Supported(t *testing.T) {
	h := newOCHandler(true, ocStateWithSensors())
	w := ocGET(t, h, "/api/v1/observingconditions/0/sensordescription?SensorName=temperature")
	var resp alpaca.Response[string]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != 0 {
		t.Fatalf("unexpected error: %v", resp.ErrorMessage)
	}
	if resp.Value == "" {
		t.Error("SensorDescription should not be empty")
	}
}

func TestOC_SensorDescription_Unsupported(t *testing.T) {
	h := newOCHandler(true, ocStateWithSensors())
	w := ocGET(t, h, "/api/v1/observingconditions/0/sensordescription?SensorName=rainrate")
	var resp alpaca.Response[string]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != alpaca.ErrNotImplemented {
		t.Errorf("want ErrNotImplemented (%d), got %d", alpaca.ErrNotImplemented, resp.ErrorNumber)
	}
}

func TestOC_SensorDescription_Unknown(t *testing.T) {
	h := newOCHandler(true, ocStateWithSensors())
	w := ocGET(t, h, "/api/v1/observingconditions/0/sensordescription?SensorName=bogus")
	var resp alpaca.Response[string]
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != alpaca.ErrInvalidValue {
		t.Errorf("want ErrInvalidValue (%d), got %d", alpaca.ErrInvalidValue, resp.ErrorNumber)
	}
}

// ---------- refresh ----------------------------------------------------------

func TestOC_PutRefresh_CallsRefreshFn(t *testing.T) {
	refreshCalled := false
	cfgHolder := config.NewHolder(config.Defaults(), "")
	holder := state.NewHolder(true)
	holder.Update(ocStateWithSensors())
	h := alpaca.NewOC(cfgHolder, holder, "uuid", "0.1.0-test", func(_ context.Context) {
		refreshCalled = true
	})

	mux := newOCMux(h)
	req := httptest.NewRequest(http.MethodPut, "/api/v1/observingconditions/0/refresh",
		strings.NewReader("ClientID=1&ClientTransactionID=1"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	var resp alpaca.VoidResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.ErrorNumber != 0 {
		t.Fatalf("unexpected error: %v", resp.ErrorMessage)
	}
	if !refreshCalled {
		t.Error("expected refresh function to be called")
	}
}

// ---------- bad paths --------------------------------------------------------

func TestOC_BadPath_WrongDeviceType(t *testing.T) {
	h := newOCHandler(true, ocStateWithSensors())
	mux := newOCMux(h)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/OBSERVINGCONDITIONS/0/temperature", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code < 400 {
		t.Errorf("expected 4xx for capitalised device type, got %d", w.Code)
	}
}

func TestOC_BadPath_WrongDeviceNumber(t *testing.T) {
	h := newOCHandler(true, ocStateWithSensors())
	mux := newOCMux(h)
	req := httptest.NewRequest(http.MethodGet, "/api/v1/observingconditions/1/temperature", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code < 400 {
		t.Errorf("expected 4xx for device number 1, got %d", w.Code)
	}
}

func TestOC_ClientTransactionID_Echoed(t *testing.T) {
	h := newOCHandler(true, ocStateWithSensors())
	mux := newOCMux(h)
	req := httptest.NewRequest(http.MethodGet,
		"/api/v1/observingconditions/0/connected?ClientID=1&ClientTransactionID=77", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	var env struct {
		ClientTransactionID uint32 `json:"ClientTransactionID"`
	}
	json.NewDecoder(w.Body).Decode(&env)
	if env.ClientTransactionID != 77 {
		t.Errorf("ClientTransactionID: want 77, got %d", env.ClientTransactionID)
	}
}
