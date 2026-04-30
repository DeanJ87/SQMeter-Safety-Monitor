package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"sqmeter-alpaca-safetymonitor/internal/alpaca"
	"sqmeter-alpaca-safetymonitor/internal/config"
	"sqmeter-alpaca-safetymonitor/internal/state"
)

// Handler serves the local web dashboard, setup page, and utility endpoints.
type Handler struct {
	cfgHolder *config.Holder
	holder    *state.Holder
	dash      *template.Template
	setup     *template.Template
	startT    time.Time
}

// New creates a Handler with embedded templates.
func New(cfgHolder *config.Holder, holder *state.Holder) (*Handler, error) {
	dash, err := template.ParseFS(templateFS, "templates/dashboard.html")
	if err != nil {
		return nil, fmt.Errorf("parsing dashboard template: %w", err)
	}
	setup, err := template.ParseFS(templateFS, "templates/setup.html")
	if err != nil {
		return nil, fmt.Errorf("parsing setup template: %w", err)
	}
	return &Handler{
		cfgHolder: cfgHolder,
		holder:    holder,
		dash:      dash,
		setup:     setup,
		startT:    time.Now(),
	}, nil
}

// Register wires web routes onto mux.
func (h *Handler) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /", h.Dashboard)
	mux.HandleFunc("GET /status", h.Dashboard)
	mux.HandleFunc("GET /health", h.Health)
	mux.HandleFunc("GET /status.json", h.StatusJSON)
	mux.HandleFunc("GET /setup", h.GetSetup)
	mux.HandleFunc("POST /setup", h.PostSetup)
	mux.HandleFunc("GET /config.json", h.GetConfigJSON)
	mux.HandleFunc("PUT /config.json", h.PutConfigJSON)
	mux.HandleFunc("POST /api/test-sqmeter", h.TestSQMeter)
}

// ---------- dashboard --------------------------------------------------------

type DashboardData struct {
	SQMeterURL    string
	HTTPPort      int
	DiscoveryPort int
	Uptime        string
	State         state.EvaluatedState
	Connected     bool
	Override      string
	LastPoll      string
	LastSuccess   string
	HasData       bool
	DewMargin     float64
	WideOpen      bool
}

func (h *Handler) Dashboard(w http.ResponseWriter, r *http.Request) {
	s := h.holder.Get()
	cfg := h.cfgHolder.Get()

	lastPoll := "—"
	if !s.LastPollUTC.IsZero() {
		lastPoll = s.LastPollUTC.UTC().Format("2006-01-02 15:04:05 UTC")
	}
	lastSuccess := "—"
	if !s.LastSuccessfulPollUTC.IsZero() {
		lastSuccess = s.LastSuccessfulPollUTC.UTC().Format("2006-01-02 15:04:05 UTC")
	}

	data := DashboardData{
		SQMeterURL:    cfg.SQMeterBaseURL,
		HTTPPort:      cfg.AlpacaHTTPPort,
		DiscoveryPort: cfg.AlpacaDiscoveryPort,
		Uptime:        time.Since(h.startT).Round(time.Second).String(),
		State:         s,
		Connected:     h.holder.IsConnected(),
		Override:      cfg.ManualOverride,
		LastPoll:      lastPoll,
		LastSuccess:   lastSuccess,
		HasData:       !s.LastSuccessfulPollUTC.IsZero(),
		DewMargin:     s.Values.Temperature - s.Values.Dewpoint,
		WideOpen:      config.IsWideOpen(cfg.AlpacaHTTPBind),
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.dash.Execute(w, data); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
	}
}

// ---------- health / status.json ---------------------------------------------

func (h *Handler) Health(w http.ResponseWriter, r *http.Request) {
	s := h.holder.Get()
	code := http.StatusOK
	status := "ok"
	if !h.holder.IsConnected() {
		status = "disconnected"
		code = http.StatusServiceUnavailable
	} else if !s.IsSafe {
		status = "unsafe"
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	fmt.Fprintf(w, `{"status":%q,"isSafe":%v}`, status, s.IsSafe)
}

func (h *Handler) StatusJSON(w http.ResponseWriter, r *http.Request) {
	s := h.holder.Get()
	cfg := h.cfgHolder.Get()
	j := alpaca.BuildStatusJSON(s, h.holder.IsConnected(), cfg.ManualOverride)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(j)
}

// ---------- setup page -------------------------------------------------------

type SetupData struct {
	Config       *config.Config
	ConfigPath   string
	WideOpen     bool
	SavedOK      bool
	NeedsRestart bool
	ErrorMsg     string
	// Pre-formatted optional fields (empty string = disabled)
	SQMMinSafe         string
	HumidityMaxSafe    string
	DewpointMarginMinC string
}

func newSetupData(cfgHolder *config.Holder, q url.Values) SetupData {
	cfg := cfgHolder.Get()
	d := SetupData{
		Config:       cfg,
		ConfigPath:   cfgHolder.Path(),
		WideOpen:     config.IsWideOpen(cfg.AlpacaHTTPBind),
		SavedOK:      q.Get("saved") == "1",
		NeedsRestart: q.Get("restart") == "1",
		ErrorMsg:     q.Get("error"),
	}
	if cfg.SQMMinSafe != nil {
		d.SQMMinSafe = fmt.Sprintf("%.2f", *cfg.SQMMinSafe)
	}
	if cfg.HumidityMaxSafe != nil {
		d.HumidityMaxSafe = fmt.Sprintf("%.1f", *cfg.HumidityMaxSafe)
	}
	if cfg.DewpointMarginMinC != nil {
		d.DewpointMarginMinC = fmt.Sprintf("%.1f", *cfg.DewpointMarginMinC)
	}
	return d
}

func (h *Handler) GetSetup(w http.ResponseWriter, r *http.Request) {
	data := newSetupData(h.cfgHolder, r.URL.Query())
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := h.setup.Execute(w, data); err != nil {
		http.Error(w, "template error: "+err.Error(), http.StatusInternalServerError)
	}
}

func (h *Handler) PostSetup(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Redirect(w, r, "/setup?error="+url.QueryEscape("form parse error: "+err.Error()), http.StatusSeeOther)
		return
	}

	old := h.cfgHolder.Get()
	updated := *old // copy

	formStr(r, "SQMETER_BASE_URL", &updated.SQMeterBaseURL)
	formStr(r, "ALPACA_HTTP_BIND", &updated.AlpacaHTTPBind)
	formInt(r, "ALPACA_HTTP_PORT", &updated.AlpacaHTTPPort)
	formInt(r, "ALPACA_DISCOVERY_PORT", &updated.AlpacaDiscoveryPort)
	formInt(r, "POLL_INTERVAL_SECONDS", &updated.PollIntervalSeconds)
	formInt(r, "STALE_AFTER_SECONDS", &updated.StaleAfterSeconds)
	updated.FailClosed = r.FormValue("FAIL_CLOSED") == "true"
	updated.ConnectedOnStartup = r.FormValue("CONNECTED_ON_STARTUP") == "true"
	formFloat(r, "CLOUD_COVER_UNSAFE_PERCENT", &updated.CloudCoverUnsafePct)
	formFloat(r, "CLOUD_COVER_CAUTION_PERCENT", &updated.CloudCoverCautionPct)
	updated.RequireLightStatus = r.FormValue("REQUIRE_LIGHT_SENSOR_STATUS_OK") == "true"
	updated.RequireEnvStatus = r.FormValue("REQUIRE_ENVIRONMENT_STATUS_OK") == "true"
	updated.RequireIRStatus = r.FormValue("REQUIRE_IR_TEMPERATURE_STATUS_OK") == "true"
	formOptFloat(r, "SQM_MIN_SAFE", &updated.SQMMinSafe)
	formOptFloat(r, "HUMIDITY_MAX_SAFE", &updated.HumidityMaxSafe)
	formOptFloat(r, "DEWPOINT_MARGIN_MIN_C", &updated.DewpointMarginMinC)
	formStr(r, "MANUAL_OVERRIDE", &updated.ManualOverride)
	formStr(r, "LOG_LEVEL", &updated.LogLevel)

	if err := h.cfgHolder.Update(&updated); err != nil {
		http.Redirect(w, r, "/setup?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}

	q := "/setup?saved=1"
	if config.NeedsRestart(old, &updated) {
		q += "&restart=1"
	}
	http.Redirect(w, r, q, http.StatusSeeOther)
}

// ---------- config JSON API --------------------------------------------------

func (h *Handler) GetConfigJSON(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(h.cfgHolder.Get())
}

func (h *Handler) PutConfigJSON(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<16))
	if err != nil {
		http.Error(w, `{"error":"read error"}`, http.StatusBadRequest)
		return
	}

	old := h.cfgHolder.Get()
	updated := *old // copy; unmarshal only touches fields present in JSON

	if err := json.Unmarshal(body, &updated); err != nil {
		http.Error(w, `{"error":"invalid JSON: `+err.Error()+`"}`, http.StatusBadRequest)
		return
	}

	restartNeeded := config.NeedsRestart(old, &updated)
	if err := h.cfgHolder.Update(&updated); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusUnprocessableEntity)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	type result struct {
		Config       *config.Config `json:"config"`
		NeedsRestart bool           `json:"needsRestart"`
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(result{Config: h.cfgHolder.Get(), NeedsRestart: restartNeeded})
}

// ---------- form helpers -----------------------------------------------------

func formStr(r *http.Request, key string, dst *string) {
	if v := r.FormValue(key); v != "" {
		*dst = v
	}
}

func formInt(r *http.Request, key string, dst *int) {
	if v := r.FormValue(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			*dst = n
		}
	}
}

func formFloat(r *http.Request, key string, dst *float64) {
	if v := r.FormValue(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			*dst = f
		}
	}
}

func formOptFloat(r *http.Request, key string, dst **float64) {
	v := r.FormValue(key)
	if v == "" {
		*dst = nil
		return
	}
	if f, err := strconv.ParseFloat(v, 64); err == nil {
		*dst = &f
	}
}

// ---------- SQMeter connection test -----------------------------------------

// TestSQMeter handles POST /api/test-sqmeter.
// It accepts {"url":"http://..."} and probes the given URL with a short
// timeout, returning {"ok":true/false,"message":"..."}.
// The URL must use http or https. A missing or empty url field falls back to
// the currently configured SQMeterBaseURL.
func (h *Handler) TestSQMeter(w http.ResponseWriter, r *http.Request) {
	type request struct {
		URL string `json:"url"`
	}
	type response struct {
		OK      bool   `json:"ok"`
		Message string `json:"message"`
	}
	writeResult := func(ok bool, msg string) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		enc := json.NewEncoder(w)
		_ = enc.Encode(response{OK: ok, Message: msg})
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
	if err != nil {
		writeResult(false, "failed to read request body")
		return
	}

	var req request
	if len(body) > 0 {
		if err := json.Unmarshal(body, &req); err != nil {
			writeResult(false, "invalid JSON: "+err.Error())
			return
		}
	}

	target := req.URL
	if target == "" {
		target = h.cfgHolder.Get().SQMeterBaseURL
	}
	if target == "" {
		writeResult(false, "no URL provided and no SQMeter URL is configured")
		return
	}

	parsed, err := url.Parse(target)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		writeResult(false, "URL must use http or https")
		return
	}

	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(target)
	if err != nil {
		writeResult(false, "connection failed: "+err.Error())
		return
	}
	resp.Body.Close()

	writeResult(true, fmt.Sprintf("connected (HTTP %d)", resp.StatusCode))
}
