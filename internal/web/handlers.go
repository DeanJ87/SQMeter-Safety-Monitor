package web

import (
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"sqmeter-alpaca-safetymonitor/internal/alpaca"
	"sqmeter-alpaca-safetymonitor/internal/config"
	"sqmeter-alpaca-safetymonitor/internal/discovery"
	"sqmeter-alpaca-safetymonitor/internal/state"
)

// Handler serves the local web dashboard, setup page, and utility endpoints.
type Handler struct {
	cfgHolder       *config.Holder
	holder          *state.Holder
	dash            *template.Template
	setup           *template.Template
	startT          time.Time
	discoveryStatus func() discovery.Status
	version         string
	onRestart       func()
	onStop          func()
}

// WithServiceControl registers callbacks for web-triggered restart and stop.
// restart is called to request a graceful exit that the service manager should
// restart (exit code 1); stop is called for a clean shutdown (exit code 0).
// Either or both may be nil; missing callbacks return HTTP 501.
func (h *Handler) WithServiceControl(restart, stop func()) {
	h.onRestart = restart
	h.onStop = stop
}

// WithDiscovery registers a discovery status getter so the handler can expose
// listener health via the dashboard and /status.json.
func (h *Handler) WithDiscovery(fn func() discovery.Status) {
	h.discoveryStatus = fn
}

// WithVersion sets the version string included in diagnostics output.
func (h *Handler) WithVersion(v string) {
	h.version = v
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
	mux.HandleFunc("GET /api/diagnostics", h.Diagnostics)
	mux.HandleFunc("POST /api/service/restart", h.ServiceRestart)
	mux.HandleFunc("POST /api/service/stop", h.ServiceStop)
}

// ---------- dashboard --------------------------------------------------------

type DashboardData struct {
	SQMeterURL            string
	HTTPPort              int
	DiscoveryPort         int
	Uptime                string
	State                 state.EvaluatedState
	Connected             bool
	Override              string
	LastPoll              string
	LastSuccess           string
	HasData               bool
	DewMargin             float64
	WideOpen              bool
	DiscoveryRunning      bool
	DiscoveryHealthy      bool
	DiscoveryError        string
	DiscoveryHasStats     bool // true when we have a status getter
	ServiceControlEnabled bool // true when restart/stop callbacks are wired
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
	if h.discoveryStatus != nil {
		ds := h.discoveryStatus()
		data.DiscoveryHasStats = true
		data.DiscoveryRunning = ds.Running
		data.DiscoveryHealthy = ds.Healthy
		data.DiscoveryError = ds.LastError
	}
	data.ServiceControlEnabled = h.onRestart != nil || h.onStop != nil
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

	type fullStatus struct {
		alpaca.StatusJSON
		Discovery *discovery.Status `json:"discovery,omitempty"`
	}
	full := fullStatus{StatusJSON: j}
	if h.discoveryStatus != nil {
		ds := h.discoveryStatus()
		full.Discovery = &ds
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(full)
}

// ---------- setup page -------------------------------------------------------

type SetupData struct {
	Config                *config.Config
	ConfigPath            string
	WideOpen              bool
	SavedOK               bool
	NeedsRestart          bool
	RestartRequiredFields []string
	ErrorMsg              string
	// Pre-formatted optional fields (empty string = disabled)
	SQMMinSafe         string
	HumidityMaxSafe    string
	DewpointMarginMinC string
}

func newSetupData(cfgHolder *config.Holder, q url.Values) SetupData {
	cfg := cfgHolder.Get()
	restartFields := q["restart_required_fields"]
	d := SetupData{
		Config:                cfg,
		ConfigPath:            cfgHolder.Path(),
		WideOpen:              config.IsWideOpen(cfg.AlpacaHTTPBind),
		SavedOK:               q.Get("saved") == "1",
		NeedsRestart:          q.Get("restart") == "1" || len(restartFields) > 0,
		RestartRequiredFields: restartFields,
		ErrorMsg:              q.Get("error"),
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

	q := url.Values{"saved": {"1"}}
	for _, field := range config.RestartRequiredFields(old, &updated) {
		q.Add("restart_required_fields", field)
	}
	if len(q["restart_required_fields"]) > 0 {
		q.Set("restart", "1")
	}
	http.Redirect(w, r, "/setup?"+q.Encode(), http.StatusSeeOther)
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

	restartFields := config.RestartRequiredFields(old, &updated)
	if err := h.cfgHolder.Update(&updated); err != nil {
		http.Error(w, `{"error":"`+err.Error()+`"}`, http.StatusUnprocessableEntity)
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	type result struct {
		Config                *config.Config `json:"config"`
		RestartRequired       bool           `json:"restart_required"`
		RestartRequiredFields []string       `json:"restart_required_fields"`
		NeedsRestart          bool           `json:"needsRestart"`
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(result{
		Config:                h.cfgHolder.Get(),
		RestartRequired:       len(restartFields) > 0,
		RestartRequiredFields: restartFields,
		NeedsRestart:          len(restartFields) > 0,
	})
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

// ---------- diagnostics ------------------------------------------------------

// DiagnosticsReport is the payload returned by GET /api/diagnostics.
// It aggregates service health, configuration, and discovery status into a
// single snapshot that is safe to paste into a GitHub issue.
type DiagnosticsReport struct {
	Version   string `json:"version"`
	Timestamp string `json:"timestamp"`
	Uptime    string `json:"uptime"`

	Config DiagnosticsConfig `json:"config"`

	// Discovery is nil when no discovery getter has been registered.
	Discovery *discovery.Status `json:"discovery,omitempty"`

	Poller DiagnosticsPoller `json:"poller"`
	Safety DiagnosticsSafety `json:"safety"`
}

type DiagnosticsConfig struct {
	Path          string `json:"path"`
	SQMeterURL    string `json:"sqmeterUrl"`
	HTTPBind      string `json:"httpBind"`
	HTTPPort      int    `json:"httpPort"`
	DiscoveryPort int    `json:"discoveryPort"`
	WideOpen      bool   `json:"wideOpen"`
}

type DiagnosticsPoller struct {
	LastPollUTC    string  `json:"lastPollUtc"`
	LastSuccessUTC string  `json:"lastSuccessUtc"`
	LastError      *string `json:"lastError"`
}

type DiagnosticsSafety struct {
	Connected      bool     `json:"connected"`
	IsSafe         bool     `json:"isSafe"`
	ManualOverride string   `json:"manualOverride"`
	Reasons        []string `json:"reasons"`
	Warnings       []string `json:"warnings"`
}

const diagTimeFmt = "2006-01-02 15:04:05 UTC"

// Diagnostics handles GET /api/diagnostics.
func (h *Handler) Diagnostics(w http.ResponseWriter, r *http.Request) {
	s := h.holder.Get()
	cfg := h.cfgHolder.Get()

	var lastPoll, lastSuccess string
	if !s.LastPollUTC.IsZero() {
		lastPoll = s.LastPollUTC.UTC().Format(diagTimeFmt)
	}
	if !s.LastSuccessfulPollUTC.IsZero() {
		lastSuccess = s.LastSuccessfulPollUTC.UTC().Format(diagTimeFmt)
	}
	var lastErr *string
	if s.LastError != "" {
		e := s.LastError
		lastErr = &e
	}

	reasons := s.Reasons
	if reasons == nil {
		reasons = []string{}
	}
	warnings := s.Warnings
	if warnings == nil {
		warnings = []string{}
	}

	report := DiagnosticsReport{
		Version:   h.version,
		Timestamp: time.Now().UTC().Format(diagTimeFmt),
		Uptime:    time.Since(h.startT).Round(time.Second).String(),
		Config: DiagnosticsConfig{
			Path:          h.cfgHolder.Path(),
			SQMeterURL:    cfg.SQMeterBaseURL,
			HTTPBind:      cfg.AlpacaHTTPBind,
			HTTPPort:      cfg.AlpacaHTTPPort,
			DiscoveryPort: cfg.AlpacaDiscoveryPort,
			WideOpen:      config.IsWideOpen(cfg.AlpacaHTTPBind),
		},
		Poller: DiagnosticsPoller{
			LastPollUTC:    lastPoll,
			LastSuccessUTC: lastSuccess,
			LastError:      lastErr,
		},
		Safety: DiagnosticsSafety{
			Connected:      h.holder.IsConnected(),
			IsSafe:         s.IsSafe,
			ManualOverride: cfg.ManualOverride,
			Reasons:        reasons,
			Warnings:       warnings,
		},
	}
	if h.discoveryStatus != nil {
		ds := h.discoveryStatus()
		report.Discovery = &ds
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(report)
}

// ---------- service controls -------------------------------------------------

type serviceControlResponse struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// ServiceRestart handles POST /api/service/restart.
// It responds with JSON and then, after flushing the response, calls the
// registered restart callback. The process exits with code 1 so that a
// service manager configured for "restart on failure" (e.g. Windows Service
// recovery options or NSSM) will restart it automatically. If no service
// manager restart policy is configured the process simply exits.
func (h *Handler) ServiceRestart(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	if h.onRestart == nil {
		w.WriteHeader(http.StatusNotImplemented)
		enc.Encode(serviceControlResponse{OK: false, Message: "restart not available"}) //nolint:errcheck
		return
	}
	enc.Encode(serviceControlResponse{ //nolint:errcheck
		OK:      true,
		Message: "restart initiated; service will exit — restart depends on service manager configuration",
	})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	go func() {
		time.Sleep(200 * time.Millisecond)
		h.onRestart()
	}()
}

// ServiceStop handles POST /api/service/stop.
// It responds with JSON and then calls the registered stop callback after
// flushing the response. The process exits cleanly (code 0); the service will
// remain stopped until manually restarted. N.I.N.A. and other Alpaca clients
// will lose safety integration until the service is running again.
func (h *Handler) ServiceStop(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	if h.onStop == nil {
		w.WriteHeader(http.StatusNotImplemented)
		enc.Encode(serviceControlResponse{OK: false, Message: "stop not available"}) //nolint:errcheck
		return
	}
	enc.Encode(serviceControlResponse{ //nolint:errcheck
		OK:      true,
		Message: "stop initiated; N.I.N.A./Alpaca safety integration will be unavailable until the service is restarted",
	})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	go func() {
		time.Sleep(200 * time.Millisecond)
		h.onStop()
	}()
}

// ---------- SQMeter connection test -----------------------------------------

// TestSQMeter handles POST /api/test-sqmeter.
// It accepts {"url":"http://..."} and probes that host with a 2-second TCP
// dial, returning {"ok":true/false,"message":"..."}. A missing or empty url
// field falls back to the currently configured SQMeterBaseURL. The URL must
// use http or https; host reachability is tested at the TCP layer only.
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
		_ = json.NewEncoder(w).Encode(response{OK: ok, Message: msg})
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, 4096))
	if err != nil {
		writeResult(false, "could not read request body")
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
		writeResult(false, "no SQMeter URL configured")
		return
	}

	parsed, err := url.Parse(target)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		writeResult(false, "URL must use http or https scheme")
		return
	}

	port := parsed.Port()
	if port == "" {
		if parsed.Scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	addr := net.JoinHostPort(parsed.Hostname(), port)
	conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
	if err != nil {
		writeResult(false, "connection failed: "+err.Error())
		return
	}
	conn.Close() /* #nosec G104 -- TCP probe; connection immediately discarded; close error is irrelevant */ //nolint:errcheck

	writeResult(true, "connected to "+addr)
}
