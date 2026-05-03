package alpaca

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"sqmeter-alpaca-safetymonitor/internal/config"
	"sqmeter-alpaca-safetymonitor/internal/state"
)

const (
	ocInterfaceVersion = 1
	ocDeviceName       = "SQMeter ObservingConditions"
	ocDeviceType       = "ObservingConditions"
	ocDeviceNumber     = 0
	ocDescription      = "ASCOM Alpaca ObservingConditions bridge for SQMeter ESP32"
	ocDriverInfo       = "SQMeter Alpaca ObservingConditions"
)

// OCHandler serves all ASCOM Alpaca endpoints for the ObservingConditions device.
type OCHandler struct {
	cfgHolder  *config.Holder
	holder     *state.Holder
	serverTxID atomic.Uint32
	deviceUUID string
	version    string
	refreshFn  func(ctx context.Context)
}

// NewOC creates an OCHandler. refreshFn is called synchronously by the refresh
// endpoint to perform an immediate SQMeter poll.
func NewOC(cfgHolder *config.Holder, holder *state.Holder, deviceUUID, version string, refreshFn func(ctx context.Context)) *OCHandler {
	return &OCHandler{
		cfgHolder:  cfgHolder,
		holder:     holder,
		deviceUUID: deviceUUID,
		version:    version,
		refreshFn:  refreshFn,
	}
}

// ConfiguredDevice returns the ConfiguredDevice descriptor for registration in
// the management /configureddevices list.
func (h *OCHandler) ConfiguredDevice() ConfiguredDevice {
	return ConfiguredDevice{
		DeviceName:   ocDeviceName,
		DeviceType:   ocDeviceType,
		DeviceNumber: ocDeviceNumber,
		UniqueID:     h.deviceUUID,
	}
}

// Register wires all ObservingConditions Alpaca routes onto mux.
func (h *OCHandler) Register(mux *http.ServeMux) {
	const base = "/api/v1/observingconditions/0/"

	mux.HandleFunc("GET "+base+"connected", h.GetConnected)
	mux.HandleFunc("GET "+base+"connecting", h.GetConnecting)
	mux.HandleFunc("GET "+base+"description", h.GetDescription)
	mux.HandleFunc("GET "+base+"driverinfo", h.GetDriverInfo)
	mux.HandleFunc("GET "+base+"driverversion", h.GetDriverVersion)
	mux.HandleFunc("GET "+base+"interfaceversion", h.GetInterfaceVersion)
	mux.HandleFunc("GET "+base+"name", h.GetName)
	mux.HandleFunc("GET "+base+"supportedactions", h.GetSupportedActions)
	mux.HandleFunc("GET "+base+"devicestate", h.GetDeviceState)

	mux.HandleFunc("PUT "+base+"connected", h.PutConnected)
	mux.HandleFunc("PUT "+base+"connect", h.PutConnect)
	mux.HandleFunc("PUT "+base+"disconnect", h.PutDisconnect)
	mux.HandleFunc("PUT "+base+"action", h.PutAction)
	mux.HandleFunc("PUT "+base+"commandblind", h.PutCommandNotImpl)
	mux.HandleFunc("PUT "+base+"commandbool", h.PutCommandNotImpl)
	mux.HandleFunc("PUT "+base+"commandstring", h.PutCommandNotImpl)

	mux.HandleFunc("GET "+base+"averageperiod", h.GetAveragePeriod)
	mux.HandleFunc("PUT "+base+"averageperiod", h.PutAveragePeriod)
	mux.HandleFunc("GET "+base+"cloudcover", h.GetCloudCover)
	mux.HandleFunc("GET "+base+"dewpoint", h.GetDewPoint)
	mux.HandleFunc("GET "+base+"humidity", h.GetHumidity)
	mux.HandleFunc("GET "+base+"pressure", h.GetPressure)
	mux.HandleFunc("GET "+base+"rainrate", h.getNotImplementedFloat)
	mux.HandleFunc("GET "+base+"skybrightness", h.GetSkyBrightness)
	mux.HandleFunc("GET "+base+"skyquality", h.GetSkyQuality)
	mux.HandleFunc("GET "+base+"skytemperature", h.GetSkyTemperature)
	mux.HandleFunc("GET "+base+"starfwhm", h.getNotImplementedFloat)
	mux.HandleFunc("GET "+base+"temperature", h.GetTemperature)
	mux.HandleFunc("GET "+base+"winddirection", h.getNotImplementedFloat)
	mux.HandleFunc("GET "+base+"windgust", h.getNotImplementedFloat)
	mux.HandleFunc("GET "+base+"windspeed", h.getNotImplementedFloat)
	mux.HandleFunc("GET "+base+"timesincemeasurement", h.GetTimeSinceMeasurement)
	mux.HandleFunc("GET "+base+"sensordescription", h.GetSensorDescription)
	mux.HandleFunc("PUT "+base+"refresh", h.PutRefresh)
}

func (h *OCHandler) nextTxID() uint32 { return h.serverTxID.Add(1) }

// ---------- Standard device GET endpoints ------------------------------------

func (h *OCHandler) GetConnected(w http.ResponseWriter, r *http.Request) {
	_, clientTxID := parseGetParams(r)
	writeJSON(w, okResp(clientTxID, h.nextTxID(), h.holder.IsConnected()))
}

func (h *OCHandler) GetConnecting(w http.ResponseWriter, r *http.Request) {
	_, clientTxID := parseGetParams(r)
	writeJSON(w, okResp(clientTxID, h.nextTxID(), false))
}

func (h *OCHandler) GetDescription(w http.ResponseWriter, r *http.Request) {
	_, clientTxID := parseGetParams(r)
	writeJSON(w, okResp(clientTxID, h.nextTxID(), ocDescription))
}

func (h *OCHandler) GetDriverInfo(w http.ResponseWriter, r *http.Request) {
	_, clientTxID := parseGetParams(r)
	writeJSON(w, okResp(clientTxID, h.nextTxID(), ocDriverInfo))
}

func (h *OCHandler) GetDriverVersion(w http.ResponseWriter, r *http.Request) {
	_, clientTxID := parseGetParams(r)
	writeJSON(w, okResp(clientTxID, h.nextTxID(), h.version))
}

func (h *OCHandler) GetInterfaceVersion(w http.ResponseWriter, r *http.Request) {
	_, clientTxID := parseGetParams(r)
	writeJSON(w, okResp(clientTxID, h.nextTxID(), ocInterfaceVersion))
}

func (h *OCHandler) GetName(w http.ResponseWriter, r *http.Request) {
	_, clientTxID := parseGetParams(r)
	writeJSON(w, okResp(clientTxID, h.nextTxID(), ocDeviceName))
}

func (h *OCHandler) GetSupportedActions(w http.ResponseWriter, r *http.Request) {
	_, clientTxID := parseGetParams(r)
	writeJSON(w, okResp(clientTxID, h.nextTxID(), []string{}))
}

func (h *OCHandler) GetDeviceState(w http.ResponseWriter, r *http.Request) {
	_, clientTxID := parseGetParams(r)
	ev := h.holder.Get()
	items := []DeviceStateItem{
		{Name: "CloudCover", Value: ev.Values.CloudCoverPercent},
		{Name: "DewPoint", Value: ev.Values.Dewpoint},
		{Name: "Humidity", Value: ev.Values.Humidity},
		{Name: "SkyQuality", Value: ev.Values.SQM},
		{Name: "Temperature", Value: ev.Values.Temperature},
		{Name: "TimeStamp", Value: time.Now().UTC().Format(time.RFC3339Nano)},
	}
	writeJSON(w, okResp(clientTxID, h.nextTxID(), items))
}

// ---------- Standard device PUT endpoints ------------------------------------

func (h *OCHandler) PutConnected(w http.ResponseWriter, r *http.Request) {
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

func (h *OCHandler) PutConnect(w http.ResponseWriter, r *http.Request) {
	_, clientTxID := parsePutParams(r)
	h.holder.SetConnected(true)
	writeJSON(w, voidOK(clientTxID, h.nextTxID()))
}

func (h *OCHandler) PutDisconnect(w http.ResponseWriter, r *http.Request) {
	_, clientTxID := parsePutParams(r)
	h.holder.SetConnected(false)
	writeJSON(w, voidOK(clientTxID, h.nextTxID()))
}

func (h *OCHandler) PutAction(w http.ResponseWriter, r *http.Request) {
	_, clientTxID := parsePutParams(r)
	serverTxID := h.nextTxID()
	action := r.FormValue("Action")
	writeJSON(w, errResp[string](clientTxID, serverTxID, ErrInvalidOp,
		"unknown action: "+action+"; no custom actions are supported"))
}

func (h *OCHandler) PutCommandNotImpl(w http.ResponseWriter, r *http.Request) {
	_, clientTxID := parsePutParams(r)
	writeJSON(w, voidErr(clientTxID, h.nextTxID(), ErrNotImplemented, "not implemented"))
}

// ---------- OC helpers -------------------------------------------------------

// sensorFloat is a shared handler body for OC sensor GET endpoints that require
// a connected device and valid raw sensor data. getValue receives the current
// state and returns (value, errNum, errMsg); a non-zero errNum causes an Alpaca
// error response to be written instead of the value.
func (h *OCHandler) sensorFloat(w http.ResponseWriter, r *http.Request, getValue func(ev state.EvaluatedState) (float64, int, string)) {
	_, clientTxID := parseGetParams(r)
	serverTxID := h.nextTxID()
	if !h.holder.IsConnected() {
		writeJSON(w, errResp[float64](clientTxID, serverTxID, ErrNotConnected, "device is not connected"))
		return
	}
	ev := h.holder.Get()
	if ev.RawSensors == nil {
		writeJSON(w, errResp[float64](clientTxID, serverTxID, ErrUnspecified, "no sensor data available yet"))
		return
	}
	val, errNum, errMsg := getValue(ev)
	if errNum != 0 {
		writeJSON(w, errResp[float64](clientTxID, serverTxID, errNum, errMsg))
		return
	}
	writeJSON(w, okResp(clientTxID, serverTxID, val))
}

func sensorErrMsg(status int, name string) string {
	switch status {
	case 1:
		return name + " sensor not found"
	case 2:
		return name + " sensor read error"
	case 3:
		return name + " sensor data stale"
	default:
		return fmt.Sprintf("%s sensor unavailable (status %d)", name, status)
	}
}

func (h *OCHandler) getNotImplementedFloat(w http.ResponseWriter, r *http.Request) {
	_, clientTxID := parseGetParams(r)
	writeJSON(w, errResp[float64](clientTxID, h.nextTxID(), ErrNotImplemented, "sensor not available on SQMeter ESP32"))
}

// ---------- OC sensor GET endpoints ------------------------------------------

func (h *OCHandler) GetAveragePeriod(w http.ResponseWriter, r *http.Request) {
	_, clientTxID := parseGetParams(r)
	writeJSON(w, okResp(clientTxID, h.nextTxID(), 0.0))
}

func (h *OCHandler) PutAveragePeriod(w http.ResponseWriter, r *http.Request) {
	_, clientTxID := parsePutParams(r)
	serverTxID := h.nextTxID()
	v, err := strconv.ParseFloat(r.FormValue("AveragePeriod"), 64)
	if err != nil || v != 0.0 {
		writeJSON(w, voidErr(clientTxID, serverTxID, ErrInvalidValue, "AveragePeriod must be 0; averaging is not supported"))
		return
	}
	writeJSON(w, voidOK(clientTxID, serverTxID))
}

func (h *OCHandler) GetCloudCover(w http.ResponseWriter, r *http.Request) {
	h.sensorFloat(w, r, func(ev state.EvaluatedState) (float64, int, string) {
		if s := ev.RawSensors.IRTemperature.Status; s != 0 {
			return 0, ErrUnspecified, sensorErrMsg(s, "IR temperature")
		}
		return ev.Values.CloudCoverPercent, 0, ""
	})
}

func (h *OCHandler) GetDewPoint(w http.ResponseWriter, r *http.Request) {
	h.sensorFloat(w, r, func(ev state.EvaluatedState) (float64, int, string) {
		if s := ev.RawSensors.Environment.Status; s != 0 {
			return 0, ErrUnspecified, sensorErrMsg(s, "environment")
		}
		return ev.Values.Dewpoint, 0, ""
	})
}

func (h *OCHandler) GetHumidity(w http.ResponseWriter, r *http.Request) {
	h.sensorFloat(w, r, func(ev state.EvaluatedState) (float64, int, string) {
		if s := ev.RawSensors.Environment.Status; s != 0 {
			return 0, ErrUnspecified, sensorErrMsg(s, "environment")
		}
		return ev.Values.Humidity, 0, ""
	})
}

func (h *OCHandler) GetPressure(w http.ResponseWriter, r *http.Request) {
	h.sensorFloat(w, r, func(ev state.EvaluatedState) (float64, int, string) {
		if s := ev.RawSensors.Environment.Status; s != 0 {
			return 0, ErrUnspecified, sensorErrMsg(s, "environment")
		}
		return ev.RawSensors.Environment.Pressure, 0, ""
	})
}

func (h *OCHandler) GetSkyBrightness(w http.ResponseWriter, r *http.Request) {
	h.sensorFloat(w, r, func(ev state.EvaluatedState) (float64, int, string) {
		if s := ev.RawSensors.LightSensor.Status; s != 0 {
			return 0, ErrUnspecified, sensorErrMsg(s, "light")
		}
		return ev.RawSensors.LightSensor.Lux, 0, ""
	})
}

func (h *OCHandler) GetSkyQuality(w http.ResponseWriter, r *http.Request) {
	h.sensorFloat(w, r, func(ev state.EvaluatedState) (float64, int, string) {
		if s := ev.RawSensors.LightSensor.Status; s != 0 {
			return 0, ErrUnspecified, sensorErrMsg(s, "light")
		}
		return ev.Values.SQM, 0, ""
	})
}

func (h *OCHandler) GetSkyTemperature(w http.ResponseWriter, r *http.Request) {
	h.sensorFloat(w, r, func(ev state.EvaluatedState) (float64, int, string) {
		if s := ev.RawSensors.IRTemperature.Status; s != 0 {
			return 0, ErrUnspecified, sensorErrMsg(s, "IR temperature")
		}
		return ev.RawSensors.IRTemperature.ObjectTemp, 0, ""
	})
}

func (h *OCHandler) GetTemperature(w http.ResponseWriter, r *http.Request) {
	h.sensorFloat(w, r, func(ev state.EvaluatedState) (float64, int, string) {
		if s := ev.RawSensors.Environment.Status; s != 0 {
			return 0, ErrUnspecified, sensorErrMsg(s, "environment")
		}
		return ev.Values.Temperature, 0, ""
	})
}

// ---------- timesincemeasurement / sensordescription -------------------------

// ocSupportedSensors maps lower-case ASCOM sensor names to their descriptions.
var ocSupportedSensors = map[string]string{
	"cloudcover":     "Cloud cover estimated from IR temperature differential (0-100%)",
	"dewpoint":       "Dew point temperature from BME280 environmental sensor (C)",
	"humidity":       "Relative humidity from BME280 environmental sensor (0-100%)",
	"pressure":       "Atmospheric pressure from BME280 environmental sensor (hPa)",
	"skybrightness":  "Sky brightness from TSL2591 light sensor (lux)",
	"skyquality":     "Sky quality (SQM) from TSL2591 light sensor (mag/arcsec2)",
	"skytemperature": "Sky temperature from MLX90614 IR thermometer (C)",
	"temperature":    "Ambient temperature from BME280 environmental sensor (C)",
}

// ocUnsupportedSensors lists sensors that are defined in the OC interface but
// not available on the SQMeter ESP32.
var ocUnsupportedSensors = map[string]bool{
	"rainrate":      true,
	"starfwhm":      true,
	"winddirection": true,
	"windgust":      true,
	"windspeed":     true,
}

func (h *OCHandler) GetTimeSinceMeasurement(w http.ResponseWriter, r *http.Request) {
	_, clientTxID := parseGetParams(r)
	serverTxID := h.nextTxID()
	name := strings.ToLower(queryGetCI(r.URL.Query(), "SensorName"))

	if ocUnsupportedSensors[name] {
		writeJSON(w, errResp[float64](clientTxID, serverTxID, ErrNotImplemented, "sensor not available on SQMeter ESP32: "+name))
		return
	}
	if _, ok := ocSupportedSensors[name]; !ok {
		writeJSON(w, errResp[float64](clientTxID, serverTxID, ErrInvalidValue, "unknown sensor name: "+name))
		return
	}
	ev := h.holder.Get()
	if ev.LastSuccessfulPollUTC.IsZero() {
		writeJSON(w, errResp[float64](clientTxID, serverTxID, ErrUnspecified, "no successful measurement yet"))
		return
	}
	writeJSON(w, okResp(clientTxID, serverTxID, time.Since(ev.LastSuccessfulPollUTC).Seconds()))
}

func (h *OCHandler) GetSensorDescription(w http.ResponseWriter, r *http.Request) {
	_, clientTxID := parseGetParams(r)
	serverTxID := h.nextTxID()
	name := strings.ToLower(queryGetCI(r.URL.Query(), "SensorName"))

	if ocUnsupportedSensors[name] {
		writeJSON(w, errResp[string](clientTxID, serverTxID, ErrNotImplemented, "sensor not available on SQMeter ESP32: "+name))
		return
	}
	desc, ok := ocSupportedSensors[name]
	if !ok {
		writeJSON(w, errResp[string](clientTxID, serverTxID, ErrInvalidValue, "unknown sensor name: "+name))
		return
	}
	writeJSON(w, okResp(clientTxID, serverTxID, desc))
}

// ---------- refresh ----------------------------------------------------------

func (h *OCHandler) PutRefresh(w http.ResponseWriter, r *http.Request) {
	_, clientTxID := parsePutParams(r)
	if h.refreshFn != nil {
		h.refreshFn(r.Context())
	}
	writeJSON(w, voidOK(clientTxID, h.nextTxID()))
}
