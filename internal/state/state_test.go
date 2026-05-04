package state_test

import (
	"testing"
	"time"

	"sqmeter-ascom-alpaca/internal/state"
)

func TestNewHolder_ConnectedOnStartup(t *testing.T) {
	h := state.NewHolder(true)
	if !h.IsConnected() {
		t.Error("expected connected=true after NewHolder(true)")
	}
}

func TestNewHolder_DisconnectedOnStartup(t *testing.T) {
	h := state.NewHolder(false)
	if h.IsConnected() {
		t.Error("expected connected=false after NewHolder(false)")
	}
}

func TestSetConnected_Toggle(t *testing.T) {
	h := state.NewHolder(false)
	h.SetConnected(true)
	if !h.IsConnected() {
		t.Error("expected connected after SetConnected(true)")
	}
	h.SetConnected(false)
	if h.IsConnected() {
		t.Error("expected disconnected after SetConnected(false)")
	}
}

func TestUpdate_Get_RoundTrip(t *testing.T) {
	h := state.NewHolder(true)
	now := time.Now().UTC().Truncate(time.Millisecond)
	ev := state.EvaluatedState{
		IsSafe:                true,
		Reasons:               []string{"r1"},
		Warnings:              []string{"w1"},
		LastPollUTC:           now,
		LastSuccessfulPollUTC: now,
		LastError:             "some error",
	}
	h.Update(ev)

	got := h.Get()
	if !got.IsSafe {
		t.Error("IsSafe should be true")
	}
	if len(got.Reasons) != 1 || got.Reasons[0] != "r1" {
		t.Errorf("Reasons: want [r1], got %v", got.Reasons)
	}
	if len(got.Warnings) != 1 || got.Warnings[0] != "w1" {
		t.Errorf("Warnings: want [w1], got %v", got.Warnings)
	}
	if got.LastError != "some error" {
		t.Errorf("LastError: want 'some error', got %q", got.LastError)
	}
}

func TestGet_InitialState_IsSafeFalse(t *testing.T) {
	h := state.NewHolder(true)
	ev := h.Get()
	if ev.IsSafe {
		t.Error("initial IsSafe should be false (zero value)")
	}
}

func TestUpdate_ReplacesState(t *testing.T) {
	h := state.NewHolder(true)

	h.Update(state.EvaluatedState{IsSafe: true})
	if !h.Get().IsSafe {
		t.Error("expected IsSafe=true after first update")
	}

	h.Update(state.EvaluatedState{IsSafe: false, Reasons: []string{"cloud"}})
	got := h.Get()
	if got.IsSafe {
		t.Error("expected IsSafe=false after second update")
	}
	if len(got.Reasons) != 1 {
		t.Errorf("expected 1 reason, got %d", len(got.Reasons))
	}
}
