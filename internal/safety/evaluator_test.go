package safety_test

import (
	"testing"
	"time"

	"sqmeter-alpaca-safetymonitor/internal/config"
	"sqmeter-alpaca-safetymonitor/internal/safety"
	"sqmeter-alpaca-safetymonitor/internal/sqmclient"
)

func defaultCfg() *config.Config {
	return config.Defaults()
}

func clearSensors() *sqmclient.SensorsResponse {
	return &sqmclient.SensorsResponse{
		SkyQuality: sqmclient.SkyQuality{SQM: 21.5, Bortle: 2.0},
		Environment: sqmclient.Environment{
			Temperature: 12.4, Humidity: 72.1, Dewpoint: 7.8, Status: 0,
		},
		IRTemperature:   sqmclient.IRTemperature{Status: 0},
		LightSensor:     sqmclient.LightSensor{Status: 0},
		CloudConditions: sqmclient.CloudConditions{CloudCoverPercent: 5.0, Description: "Clear"},
	}
}

func safeInput() safety.Input {
	now := time.Now().UTC()
	return safety.Input{
		Connected:             true,
		HasData:               true,
		Sensors:               clearSensors(),
		LastSuccessfulPollUTC: now,
		LastPollUTC:           now,
	}
}

func TestIsSafe_AllClear(t *testing.T) {
	s := safety.Evaluate(defaultCfg(), safeInput())
	if !s.IsSafe {
		t.Errorf("expected safe, got reasons: %v", s.Reasons)
	}
	if len(s.Reasons) != 0 {
		t.Errorf("unexpected reasons: %v", s.Reasons)
	}
}

func TestIsSafe_NotConnected(t *testing.T) {
	in := safeInput()
	in.Connected = false
	s := safety.Evaluate(defaultCfg(), in)
	if s.IsSafe {
		t.Error("expected unsafe when not connected")
	}
	if len(s.Reasons) == 0 {
		t.Error("expected at least one reason")
	}
}

func TestIsSafe_ForceUnsafe(t *testing.T) {
	cfg := defaultCfg()
	cfg.ManualOverride = "force_unsafe"
	s := safety.Evaluate(cfg, safeInput())
	if s.IsSafe {
		t.Error("expected unsafe with force_unsafe override")
	}
}

func TestIsSafe_ForceSafeWithData(t *testing.T) {
	cfg := defaultCfg()
	cfg.ManualOverride = "force_safe"
	s := safety.Evaluate(cfg, safeInput())
	if !s.IsSafe {
		t.Errorf("expected safe with force_safe and valid data, got reasons: %v", s.Reasons)
	}
	if len(s.Warnings) == 0 {
		t.Error("expected a force_safe warning")
	}
}

func TestIsSafe_ForceSafeNoDataFailClosed(t *testing.T) {
	cfg := defaultCfg()
	cfg.ManualOverride = "force_safe"
	cfg.FailClosed = true
	in := safeInput()
	in.HasData = false
	in.Sensors = nil
	s := safety.Evaluate(cfg, in)
	if s.IsSafe {
		t.Error("expected unsafe: force_safe with no data and FAIL_CLOSED=true")
	}
}

func TestIsSafe_CloudCoverUnsafe(t *testing.T) {
	cfg := defaultCfg()
	cfg.CloudCoverUnsafePct = 80
	in := safeInput()
	in.Sensors.CloudConditions.CloudCoverPercent = 85.0
	s := safety.Evaluate(cfg, in)
	if s.IsSafe {
		t.Error("expected unsafe: cloud cover 85% >= 80% threshold")
	}
}

func TestIsSafe_CloudCoverCaution(t *testing.T) {
	cfg := defaultCfg()
	cfg.CloudCoverCautionPct = 50
	cfg.CloudCoverUnsafePct = 80
	in := safeInput()
	in.Sensors.CloudConditions.CloudCoverPercent = 60.0
	s := safety.Evaluate(cfg, in)
	if !s.IsSafe {
		t.Errorf("expected safe in caution zone, got reasons: %v", s.Reasons)
	}
	if len(s.Warnings) == 0 {
		t.Error("expected caution warning")
	}
}

func TestIsSafe_StaleData(t *testing.T) {
	cfg := defaultCfg()
	cfg.StaleAfterSeconds = 30
	in := safeInput()
	in.LastSuccessfulPollUTC = time.Now().Add(-60 * time.Second)
	s := safety.Evaluate(cfg, in)
	if s.IsSafe {
		t.Error("expected unsafe: data is stale")
	}
}

func TestIsSafe_NoDataFailClosed(t *testing.T) {
	cfg := defaultCfg()
	cfg.FailClosed = true
	in := safeInput()
	in.HasData = false
	in.Sensors = nil
	s := safety.Evaluate(cfg, in)
	if s.IsSafe {
		t.Error("expected unsafe: no data with FAIL_CLOSED=true")
	}
}

func TestIsSafe_LightSensorError(t *testing.T) {
	cfg := defaultCfg()
	cfg.RequireLightStatus = true
	in := safeInput()
	in.Sensors.LightSensor.Status = 2 // read error
	s := safety.Evaluate(cfg, in)
	if s.IsSafe {
		t.Error("expected unsafe: light sensor status != 0")
	}
}

func TestIsSafe_EnvSensorError(t *testing.T) {
	cfg := defaultCfg()
	cfg.RequireEnvStatus = true
	in := safeInput()
	in.Sensors.Environment.Status = 1 // not found
	s := safety.Evaluate(cfg, in)
	if s.IsSafe {
		t.Error("expected unsafe: environment sensor status != 0")
	}
}

func TestIsSafe_IRSensorError(t *testing.T) {
	cfg := defaultCfg()
	cfg.RequireIRStatus = true
	in := safeInput()
	in.Sensors.IRTemperature.Status = 3 // stale
	s := safety.Evaluate(cfg, in)
	if s.IsSafe {
		t.Error("expected unsafe: IR temperature sensor status != 0")
	}
}

func TestIsSafe_SQMBelowMin(t *testing.T) {
	cfg := defaultCfg()
	minSQM := 19.0
	cfg.SQMMinSafe = &minSQM
	in := safeInput()
	in.Sensors.SkyQuality.SQM = 17.5
	s := safety.Evaluate(cfg, in)
	if s.IsSafe {
		t.Error("expected unsafe: SQM below minimum")
	}
}

func TestIsSafe_HumidityTooHigh(t *testing.T) {
	cfg := defaultCfg()
	maxHum := 85.0
	cfg.HumidityMaxSafe = &maxHum
	in := safeInput()
	in.Sensors.Environment.Humidity = 90.0
	s := safety.Evaluate(cfg, in)
	if s.IsSafe {
		t.Error("expected unsafe: humidity above maximum")
	}
}

func TestIsSafe_DewpointMarginTooSmall(t *testing.T) {
	cfg := defaultCfg()
	minMargin := 3.0
	cfg.DewpointMarginMinC = &minMargin
	in := safeInput()
	in.Sensors.Environment.Temperature = 10.0
	in.Sensors.Environment.Dewpoint = 9.0 // margin = 1°C < 3°C
	s := safety.Evaluate(cfg, in)
	if s.IsSafe {
		t.Error("expected unsafe: dew point margin too small")
	}
}

func TestEvaluatedState_ValuesPopulated(t *testing.T) {
	s := safety.Evaluate(defaultCfg(), safeInput())
	if s.Values.SQM != 21.5 {
		t.Errorf("Values.SQM: want 21.5, got %v", s.Values.SQM)
	}
	if s.Values.CloudCondition != "Clear" {
		t.Errorf("Values.CloudCondition: want Clear, got %q", s.Values.CloudCondition)
	}
}
