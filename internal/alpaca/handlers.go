package alpaca

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"sqmeter-alpaca-safetymonitor/internal/config"
	"sqmeter-alpaca-safetymonitor/internal/state"
)

const (
	interfaceVersion = 1
	deviceName       = "SQMeter SafetyMonitor"
	deviceType       = "SafetyMonitor"
	deviceNumber     = 0
	description      = "ASCOM Alpaca SafetyMonitor bridge for SQMeter ESP32"
	driverInfo       = "SQMeter Alpaca SafetyMonitor"
)

// Handler serves all ASCOM Alpaca endpoints for the SafetyMonitor device.
type Handler struct {
	cfgHolder  *config.Holder
	holder     *state.Holder
	serverTxID atomic.Uint32
	deviceUUID string
	version    string
	refreshFn  func(ctx context.Context)
}

// New creates a Handler.  refreshFn is called synchronously by the "refresh"
// custom action to perform an immediate SQMeter poll.
func New(cfgHolder *config.Holder, holder *state.Holder, deviceUUID, version string, refreshFn func(ctx context.Context)) *Handler {
	return &Handler{
		cfgHolder:  cfgHolder,
		holder:     holder,
		deviceUUID: deviceUUID,
		version:    version,
		refreshFn:  refreshFn,
	}
}

// Register wires all Alpaca routes onto mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /management/apiversions", h.GetAPIVersions)
	mux.HandleFunc("GET /management/v1/description", h.GetServerDescription)
	mux.HandleFunc("GET /management/v1/configureddevices", h.GetConfiguredDevices)

	mux.HandleFunc("GET /api/v1/safetymonitor/0/connected", h.GetConnected)
	mux.HandleFunc("GET /api/v1/safetymonitor/0/connecting", h.GetConnecting)
	mux.HandleFunc("GET /api/v1/safetymonitor/0/description", h.GetDescription)
	mux.HandleFunc("GET /api/v1/safetymonitor/0/driverinfo", h.GetDriverInfo)
	mux.HandleFunc("GET /api/v1/safetymonitor/0/driverversion", h.GetDriverVersion)
	mux.HandleFunc("GET /api/v1/safetymonitor/0/interfaceversion", h.GetInterfaceVersion)
	mux.HandleFunc("GET /api/v1/safetymonitor/0/name", h.GetName)
	mux.HandleFunc("GET /api/v1/safetymonitor/0/supportedactions", h.GetSupportedActions)
	mux.HandleFunc("GET /api/v1/safetymonitor/0/devicestate", h.GetDeviceState)
	mux.HandleFunc("GET /api/v1/safetymonitor/0/issafe", h.GetIsSafe)

	mux.HandleFunc("PUT /api/v1/safetymonitor/0/connected", h.PutConnected)
	mux.HandleFunc("PUT /api/v1/safetymonitor/0/connect", h.PutConnect)
	mux.HandleFunc("PUT /api/v1/safetymonitor/0/disconnect", h.PutDisconnect)
	mux.HandleFunc("PUT /api/v1/safetymonitor/0/action", h.PutAction)
	mux.HandleFunc("PUT /api/v1/safetymonitor/0/commandblind", h.PutCommandNotImpl)
	mux.HandleFunc("PUT /api/v1/safetymonitor/0/commandbool", h.PutCommandNotImpl)
	mux.HandleFunc("PUT /api/v1/safetymonitor/0/commandstring", h.PutCommandNotImpl)
}

// ---------- helpers ----------------------------------------------------------

func (h *Handler) nextTxID() uint32 { return h.serverTxID.Add(1) }

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if err := json.NewEncoder(w).Encode(v); err != nil {
		http.Error(w, "JSON encode error", http.StatusInternalServerError)
	}
}

func parseGetParams(r *http.Request) (clientID, clientTxID uint32) {
	q := r.URL.Query()
	if v, err := strconv.ParseUint(q.Get("ClientID"), 10, 32); err == nil {
		clientID = uint32(v)
	}
	if v, err := strconv.ParseUint(q.Get("ClientTransactionID"), 10, 32); err == nil {
		clientTxID = uint32(v)
	}
	return
}

func parsePutParams(r *http.Request) (clientID, clientTxID uint32) {
	_ = r.ParseForm()
	if v, err := strconv.ParseUint(r.FormValue("ClientID"), 10, 32); err == nil {
		clientID = uint32(v)
	}
	if v, err := strconv.ParseUint(r.FormValue("ClientTransactionID"), 10, 32); err == nil {
		clientTxID = uint32(v)
	}
	return
}

func okResp[T any](clientTxID, serverTxID uint32, value T) Response[T] {
	return Response[T]{Value: value, ClientTransactionID: clientTxID, ServerTransactionID: serverTxID}
}

func errResp[T any](clientTxID, serverTxID uint32, errNum int, msg string) Response[T] {
	var zero T
	return Response[T]{
		Value:               zero,
		ClientTransactionID: clientTxID,
		ServerTransactionID: serverTxID,
		ErrorNumber:         errNum,
		ErrorMessage:        msg,
	}
}

func voidOK(clientTxID, serverTxID uint32) VoidResponse {
	return VoidResponse{ClientTransactionID: clientTxID, ServerTransactionID: serverTxID}
}

func voidErr(clientTxID, serverTxID uint32, errNum int, msg string) VoidResponse {
	return VoidResponse{
		ClientTransactionID: clientTxID,
		ServerTransactionID: serverTxID,
		ErrorNumber:         errNum,
		ErrorMessage:        msg,
	}
}

// ---------- Management API ---------------------------------------------------

func (h *Handler) GetAPIVersions(w http.ResponseWriter, r *http.Request) {
	_, clientTxID := parseGetParams(r)
	writeJSON(w, okResp(clientTxID, h.nextTxID(), []int{1}))
}

func (h *Handler) GetServerDescription(w http.ResponseWriter, r *http.Request) {
	_, clientTxID := parseGetParams(r)
	writeJSON(w, okResp(clientTxID, h.nextTxID(), ServerDescription{
		ServerName:          "SQMeter Alpaca SafetyMonitor",
		Manufacturer:        "sqmeter-alpaca",
		ManufacturerVersion: h.version,
		Location:            "local",
	}))
}

func (h *Handler) GetConfiguredDevices(w http.ResponseWriter, r *http.Request) {
	_, clientTxID := parseGetParams(r)
	writeJSON(w, okResp(clientTxID, h.nextTxID(), []ConfiguredDevice{
		{DeviceName: deviceName, DeviceType: deviceType, DeviceNumber: deviceNumber, UniqueID: h.deviceUUID},
	}))
}

// ---------- Common device GET endpoints --------------------------------------

func (h *Handler) GetConnected(w http.ResponseWriter, r *http.Request) {
	_, clientTxID := parseGetParams(r)
	writeJSON(w, okResp(clientTxID, h.nextTxID(), h.holder.IsConnected()))
}

func (h *Handler) GetConnecting(w http.ResponseWriter, r *http.Request) {
	_, clientTxID := parseGetParams(r)
	writeJSON(w, okResp(clientTxID, h.nextTxID(), false))
}

func (h *Handler) GetDescription(w http.ResponseWriter, r *http.Request) {
	_, clientTxID := parseGetParams(r)
	writeJSON(w, okResp(clientTxID, h.nextTxID(), description))
}

func (h *Handler) GetDriverInfo(w http.ResponseWriter, r *http.Request) {
	_, clientTxID := parseGetParams(r)
	writeJSON(w, okResp(clientTxID, h.nextTxID(), driverInfo))
}

func (h *Handler) GetDriverVersion(w http.ResponseWriter, r *http.Request) {
	_, clientTxID := parseGetParams(r)
	writeJSON(w, okResp(clientTxID, h.nextTxID(), h.version))
}

func (h *Handler) GetInterfaceVersion(w http.ResponseWriter, r *http.Request) {
	_, clientTxID := parseGetParams(r)
	writeJSON(w, okResp(clientTxID, h.nextTxID(), interfaceVersion))
}

func (h *Handler) GetName(w http.ResponseWriter, r *http.Request) {
	_, clientTxID := parseGetParams(r)
	writeJSON(w, okResp(clientTxID, h.nextTxID(), deviceName))
}

func (h *Handler) GetSupportedActions(w http.ResponseWriter, r *http.Request) {
	_, clientTxID := parseGetParams(r)
	writeJSON(w, okResp(clientTxID, h.nextTxID(), []string{"status", "refresh"}))
}

func (h *Handler) GetDeviceState(w http.ResponseWriter, r *http.Request) {
	_, clientTxID := parseGetParams(r)
	s := h.holder.Get()
	items := []DeviceStateItem{
		{Name: "IsSafe", Value: s.IsSafe},
		{Name: "TimeStamp", Value: time.Now().UTC().Format(time.RFC3339Nano)},
	}
	writeJSON(w, okResp(clientTxID, h.nextTxID(), items))
}

// ---------- SafetyMonitor-specific -------------------------------------------

func (h *Handler) GetIsSafe(w http.ResponseWriter, r *http.Request) {
	_, clientTxID := parseGetParams(r)
	serverTxID := h.nextTxID()
	if !h.holder.IsConnected() {
		writeJSON(w, errResp[bool](clientTxID, serverTxID, ErrNotConnected, "device is not connected"))
		return
	}
	writeJSON(w, okResp(clientTxID, serverTxID, h.holder.Get().IsSafe))
}

// ---------- Common device PUT endpoints --------------------------------------

func (h *Handler) PutConnected(w http.ResponseWriter, r *http.Request) {
	_, clientTxID := parsePutParams(r)
	serverTxID := h.nextTxID()
	switch r.FormValue("Connected") {
	case "true", "True", "TRUE":
		h.holder.SetConnected(true)
		writeJSON(w, voidOK(clientTxID, serverTxID))
	case "false", "False", "FALSE":
		h.holder.SetConnected(false)
		writeJSON(w, voidOK(clientTxID, serverTxID))
	default:
		writeJSON(w, voidErr(clientTxID, serverTxID, ErrInvalidValue, "Connected must be true or false"))
	}
}

func (h *Handler) PutConnect(w http.ResponseWriter, r *http.Request) {
	_, clientTxID := parsePutParams(r)
	h.holder.SetConnected(true)
	writeJSON(w, voidOK(clientTxID, h.nextTxID()))
}

func (h *Handler) PutDisconnect(w http.ResponseWriter, r *http.Request) {
	_, clientTxID := parsePutParams(r)
	h.holder.SetConnected(false)
	writeJSON(w, voidOK(clientTxID, h.nextTxID()))
}

func (h *Handler) PutAction(w http.ResponseWriter, r *http.Request) {
	_, clientTxID := parsePutParams(r)
	serverTxID := h.nextTxID()
	cfg := h.cfgHolder.Get()

	switch r.FormValue("Action") {
	case "status":
		s := h.holder.Get()
		b, _ := json.Marshal(BuildStatusJSON(s, h.holder.IsConnected(), cfg.ManualOverride))
		writeJSON(w, okResp(clientTxID, serverTxID, string(b)))
	case "refresh":
		if h.refreshFn != nil {
			h.refreshFn(r.Context())
		}
		s := h.holder.Get()
		b, _ := json.Marshal(BuildStatusJSON(s, h.holder.IsConnected(), cfg.ManualOverride))
		writeJSON(w, okResp(clientTxID, serverTxID, string(b)))
	default:
		writeJSON(w, errResp[string](clientTxID, serverTxID, ErrInvalidOp,
			"unknown action: "+r.FormValue("Action")+"; supported: status, refresh"))
	}
}

func (h *Handler) PutCommandNotImpl(w http.ResponseWriter, r *http.Request) {
	_, clientTxID := parsePutParams(r)
	writeJSON(w, voidErr(clientTxID, h.nextTxID(), ErrNotImplemented, "not implemented"))
}

// ---------- StatusJSON -------------------------------------------------------

// StatusJSON is the shape returned by /status.json and the "status" action.
type StatusJSON struct {
	IsSafe                bool         `json:"isSafe"`
	Reasons               []string     `json:"reasons"`
	Warnings              []string     `json:"warnings"`
	LastPollUTC           string       `json:"lastPollUtc"`
	LastSuccessfulPollUTC string       `json:"lastSuccessfulPollUtc"`
	LastError             *string      `json:"lastError"`
	Connected             bool         `json:"connected"`
	ManualOverride        string       `json:"manualOverride"`
	Values                StatusValues `json:"values"`
}

// StatusValues contains the sensor readings included in StatusJSON.
type StatusValues struct {
	SQM               float64 `json:"sqm"`
	Bortle            float64 `json:"bortle"`
	Temperature       float64 `json:"temperature"`
	Humidity          float64 `json:"humidity"`
	Dewpoint          float64 `json:"dewpoint"`
	CloudCoverPercent float64 `json:"cloudCoverPercent"`
	CloudCondition    string  `json:"cloudCondition"`
	TemperatureDelta  float64 `json:"temperatureDelta"`
	CorrectedDelta    float64 `json:"correctedDelta"`
}

// BuildStatusJSON builds the status payload used by /status.json and Alpaca actions.
func BuildStatusJSON(s state.EvaluatedState, connected bool, override string) StatusJSON {
	j := StatusJSON{
		IsSafe:         s.IsSafe,
		Reasons:        s.Reasons,
		Warnings:       s.Warnings,
		Connected:      connected,
		ManualOverride: override,
		Values: StatusValues{
			SQM:               s.Values.SQM,
			Bortle:            s.Values.Bortle,
			Temperature:       s.Values.Temperature,
			Humidity:          s.Values.Humidity,
			Dewpoint:          s.Values.Dewpoint,
			CloudCoverPercent: s.Values.CloudCoverPercent,
			CloudCondition:    s.Values.CloudCondition,
			TemperatureDelta:  s.Values.TemperatureDelta,
			CorrectedDelta:    s.Values.CorrectedDelta,
		},
	}
	if !s.LastPollUTC.IsZero() {
		j.LastPollUTC = s.LastPollUTC.UTC().Format(time.RFC3339)
	}
	if !s.LastSuccessfulPollUTC.IsZero() {
		j.LastSuccessfulPollUTC = s.LastSuccessfulPollUTC.UTC().Format(time.RFC3339)
	}
	if s.LastError != "" {
		j.LastError = &s.LastError
	}
	return j
}
