package state

import (
	"sync"
	"time"

	"sqmeter-ascom-alpaca/internal/sqmclient"
)

// Values are the sensor readings that informed the safety decision.
type Values struct {
	SQM               float64
	Bortle            float64
	Temperature       float64
	Humidity          float64
	Dewpoint          float64
	CloudCoverPercent float64
	CloudCondition    string
	TemperatureDelta  float64
	CorrectedDelta    float64
}

// EvaluatedState is the output of a safety evaluation cycle.
type EvaluatedState struct {
	IsSafe                bool
	Reasons               []string
	Warnings              []string
	LastPollUTC           time.Time
	LastSuccessfulPollUTC time.Time
	LastError             string
	RawSensors            *sqmclient.SensorsResponse
	Values                Values
}

// Holder is a thread-safe container for the current evaluated state and
// the logical connected flag.
type Holder struct {
	mu        sync.RWMutex
	connected bool
	ev        EvaluatedState
}

// NewHolder initialises a Holder with the given connected state.
func NewHolder(connectedOnStartup bool) *Holder {
	return &Holder{
		connected: connectedOnStartup,
		ev: EvaluatedState{
			Reasons:  []string{},
			Warnings: []string{},
		},
	}
}

// SetConnected sets the logical connected flag.
func (h *Holder) SetConnected(v bool) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.connected = v
}

// IsConnected returns the logical connected flag.
func (h *Holder) IsConnected() bool {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.connected
}

// Update atomically replaces the evaluated state.
func (h *Holder) Update(s EvaluatedState) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.ev = s
}

// Get returns a snapshot of the current evaluated state.
func (h *Holder) Get() EvaluatedState {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.ev
}
