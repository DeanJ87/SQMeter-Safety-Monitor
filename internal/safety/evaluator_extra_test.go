package safety_test

import (
	"testing"
	"time"

	"sqmeter-alpaca-safetymonitor/internal/safety"
)

// ---------- fail-open (no data) ----------------------------------------------

func TestIsSafe_NoDataFailOpen(t *testing.T) {
	cfg := defaultCfg()
	cfg.FailClosed = false
	in := safeInput()
	in.HasData = false
	in.Sensors = nil
	s := safety.Evaluate(cfg, in)
	if !s.IsSafe {
		t.Errorf("expected safe: no data with FAIL_CLOSED=false, got reasons: %v", s.Reasons)
	}
}

func TestIsSafe_ForceSafeNoDataFailOpen(t *testing.T) {
	cfg := defaultCfg()
	cfg.ManualOverride = "force_safe"
	cfg.FailClosed = false
	in := safeInput()
	in.HasData = false
	in.Sensors = nil
	s := safety.Evaluate(cfg, in)
	if !s.IsSafe {
		t.Errorf("expected safe: force_safe, no data, FAIL_CLOSED=false, got: %v", s.Reasons)
	}
	if len(s.Warnings) == 0 {
		t.Error("expected force_safe warning")
	}
}

// ---------- cloud cover boundary ---------------------------------------------

func TestIsSafe_CloudCoverAtExactUnsafeThreshold(t *testing.T) {
	cfg := defaultCfg()
	cfg.CloudCoverUnsafePct = 80
	in := safeInput()
	in.Sensors.CloudConditions.CloudCoverPercent = 80.0 // exactly at threshold (>=)
	s := safety.Evaluate(cfg, in)
	if s.IsSafe {
		t.Error("expected unsafe: cloud cover at exactly the unsafe threshold (>=)")
	}
}

func TestIsSafe_CloudCoverJustBelowUnsafeThreshold(t *testing.T) {
	cfg := defaultCfg()
	cfg.CloudCoverUnsafePct = 80
	in := safeInput()
	in.Sensors.CloudConditions.CloudCoverPercent = 79.9
	s := safety.Evaluate(cfg, in)
	if !s.IsSafe {
		t.Errorf("expected safe: cloud cover just below unsafe threshold, got: %v", s.Reasons)
	}
}

func TestIsSafe_CloudCoverAtExactCautionThreshold(t *testing.T) {
	cfg := defaultCfg()
	cfg.CloudCoverCautionPct = 50
	cfg.CloudCoverUnsafePct = 80
	in := safeInput()
	in.Sensors.CloudConditions.CloudCoverPercent = 50.0 // at caution (>=)
	s := safety.Evaluate(cfg, in)
	if !s.IsSafe {
		t.Errorf("expected safe at caution threshold, got: %v", s.Reasons)
	}
	if len(s.Warnings) == 0 {
		t.Error("expected caution warning at exact caution threshold")
	}
}

// ---------- SQM boundary -----------------------------------------------------

func TestIsSafe_SQMAtExactMinimum(t *testing.T) {
	cfg := defaultCfg()
	minSQM := 19.0
	cfg.SQMMinSafe = &minSQM
	in := safeInput()
	in.Sensors.SkyQuality.SQM = 19.0 // exactly at min — rule is <, so this is safe
	s := safety.Evaluate(cfg, in)
	if !s.IsSafe {
		t.Errorf("expected safe: SQM exactly at minimum (< check), got: %v", s.Reasons)
	}
}

func TestIsSafe_SQMJustBelowMinimum(t *testing.T) {
	cfg := defaultCfg()
	minSQM := 19.0
	cfg.SQMMinSafe = &minSQM
	in := safeInput()
	in.Sensors.SkyQuality.SQM = 18.99
	s := safety.Evaluate(cfg, in)
	if s.IsSafe {
		t.Error("expected unsafe: SQM just below minimum")
	}
}

// ---------- humidity boundary ------------------------------------------------

func TestIsSafe_HumidityAtExactMaximum(t *testing.T) {
	cfg := defaultCfg()
	maxHum := 85.0
	cfg.HumidityMaxSafe = &maxHum
	in := safeInput()
	in.Sensors.Environment.Humidity = 85.0 // exactly at max — rule is >, so this is safe
	s := safety.Evaluate(cfg, in)
	if !s.IsSafe {
		t.Errorf("expected safe: humidity exactly at maximum (> check), got: %v", s.Reasons)
	}
}

func TestIsSafe_HumidityJustAboveMaximum(t *testing.T) {
	cfg := defaultCfg()
	maxHum := 85.0
	cfg.HumidityMaxSafe = &maxHum
	in := safeInput()
	in.Sensors.Environment.Humidity = 85.01
	s := safety.Evaluate(cfg, in)
	if s.IsSafe {
		t.Error("expected unsafe: humidity just above maximum")
	}
}

// ---------- dew point margin boundary ----------------------------------------

func TestIsSafe_DewpointMarginAtExactMinimum(t *testing.T) {
	cfg := defaultCfg()
	minMargin := 3.0
	cfg.DewpointMarginMinC = &minMargin
	in := safeInput()
	in.Sensors.Environment.Temperature = 13.0
	in.Sensors.Environment.Dewpoint = 10.0 // margin = 3.0°C exactly — rule is <, so this is safe
	s := safety.Evaluate(cfg, in)
	if !s.IsSafe {
		t.Errorf("expected safe: dew margin exactly at minimum (< check), got: %v", s.Reasons)
	}
}

// ---------- sensor disabled --------------------------------------------------

func TestIsSafe_LightSensorError_WhenDisabled(t *testing.T) {
	cfg := defaultCfg()
	cfg.RequireLightStatus = false
	in := safeInput()
	in.Sensors.LightSensor.Status = 2
	s := safety.Evaluate(cfg, in)
	if !s.IsSafe {
		t.Errorf("expected safe: light sensor error ignored when RequireLightStatus=false, got: %v", s.Reasons)
	}
}

func TestIsSafe_EnvSensorError_WhenDisabled(t *testing.T) {
	cfg := defaultCfg()
	cfg.RequireEnvStatus = false
	in := safeInput()
	in.Sensors.Environment.Status = 1
	s := safety.Evaluate(cfg, in)
	if !s.IsSafe {
		t.Errorf("expected safe: env sensor error ignored when RequireEnvStatus=false, got: %v", s.Reasons)
	}
}

func TestIsSafe_IRSensorError_WhenDisabled(t *testing.T) {
	cfg := defaultCfg()
	cfg.RequireIRStatus = false
	in := safeInput()
	in.Sensors.IRTemperature.Status = 3
	s := safety.Evaluate(cfg, in)
	if !s.IsSafe {
		t.Errorf("expected safe: IR sensor error ignored when RequireIRStatus=false, got: %v", s.Reasons)
	}
}

// ---------- multiple simultaneous failures -----------------------------------

func TestIsSafe_MultipleFailures_MultipleReasons(t *testing.T) {
	cfg := defaultCfg()
	cfg.CloudCoverUnsafePct = 80
	cfg.RequireLightStatus = true
	minSQM := 20.0
	cfg.SQMMinSafe = &minSQM

	in := safeInput()
	in.Sensors.CloudConditions.CloudCoverPercent = 90.0 // fail
	in.Sensors.LightSensor.Status = 1                   // fail
	in.Sensors.SkyQuality.SQM = 15.0                    // fail

	s := safety.Evaluate(cfg, in)
	if s.IsSafe {
		t.Error("expected unsafe with multiple failures")
	}
	if len(s.Reasons) < 3 {
		t.Errorf("expected at least 3 reasons, got %d: %v", len(s.Reasons), s.Reasons)
	}
}

// ---------- config change affects outcome ------------------------------------

func TestIsSafe_ConfigChange_BecomesUnsafe(t *testing.T) {
	in := safeInput()
	in.Sensors.CloudConditions.CloudCoverPercent = 60.0

	cfgA := defaultCfg()
	cfgA.CloudCoverUnsafePct = 80 // 60% is safe under this config
	sA := safety.Evaluate(cfgA, in)
	if !sA.IsSafe {
		t.Errorf("expected safe with threshold 80%%, got: %v", sA.Reasons)
	}

	cfgB := defaultCfg()
	cfgB.CloudCoverUnsafePct = 50 // 60% is unsafe under this config
	sB := safety.Evaluate(cfgB, in)
	if sB.IsSafe {
		t.Error("expected unsafe after config threshold lowered to 50%")
	}
}

// ---------- error propagation ------------------------------------------------

func TestIsSafe_LastErrorPreserved(t *testing.T) {
	in := safeInput()
	in.LastError = "connection timeout"
	s := safety.Evaluate(defaultCfg(), in)
	if s.LastError != "connection timeout" {
		t.Errorf("LastError not preserved: got %q", s.LastError)
	}
}

func TestIsSafe_PollTimesPreserved(t *testing.T) {
	now := time.Now().UTC()
	in := safeInput()
	in.LastPollUTC = now
	in.LastSuccessfulPollUTC = now.Add(-5 * time.Second)
	s := safety.Evaluate(defaultCfg(), in)
	if !s.LastPollUTC.Equal(now) {
		t.Error("LastPollUTC not preserved")
	}
}

// ---------- stale data with non-zero last success ----------------------------

func TestIsSafe_StaleData_JustBelowThreshold(t *testing.T) {
	cfg := defaultCfg()
	cfg.StaleAfterSeconds = 30
	in := safeInput()
	// 29 seconds ago — just within the threshold
	in.LastSuccessfulPollUTC = time.Now().Add(-29 * time.Second)
	s := safety.Evaluate(cfg, in)
	if !s.IsSafe {
		t.Errorf("expected safe: data not yet stale, got: %v", s.Reasons)
	}
}

// ---------- SQM not configured (nil pointer) ---------------------------------

func TestIsSafe_SQMNotConfigured_AnyValueSafe(t *testing.T) {
	cfg := defaultCfg()
	cfg.SQMMinSafe = nil // not configured
	in := safeInput()
	in.Sensors.SkyQuality.SQM = 1.0 // terrible sky quality but no rule set
	s := safety.Evaluate(cfg, in)
	if !s.IsSafe {
		t.Errorf("expected safe when SQM rule not configured, got: %v", s.Reasons)
	}
}

// ---------- values are populated from sensors --------------------------------

func TestEvaluatedState_AllValuesPopulated(t *testing.T) {
	in := safeInput()
	in.Sensors.Environment.Temperature = 15.5
	in.Sensors.Environment.Humidity = 60.0
	in.Sensors.Environment.Dewpoint = 8.0
	in.Sensors.CloudConditions.CloudCoverPercent = 10.0
	in.Sensors.CloudConditions.TemperatureDelta = -20.0
	in.Sensors.CloudConditions.CorrectedDelta = -18.0

	s := safety.Evaluate(defaultCfg(), in)

	checks := []struct {
		name string
		got  float64
		want float64
	}{
		{"Temperature", s.Values.Temperature, 15.5},
		{"Humidity", s.Values.Humidity, 60.0},
		{"Dewpoint", s.Values.Dewpoint, 8.0},
		{"CloudCoverPercent", s.Values.CloudCoverPercent, 10.0},
		{"TemperatureDelta", s.Values.TemperatureDelta, -20.0},
		{"CorrectedDelta", s.Values.CorrectedDelta, -18.0},
	}
	for _, c := range checks {
		if c.got != c.want {
			t.Errorf("Values.%s: want %v, got %v", c.name, c.want, c.got)
		}
	}
}
