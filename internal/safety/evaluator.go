package safety

import (
	"fmt"
	"time"

	"sqmeter-alpaca-safetymonitor/internal/config"
	"sqmeter-alpaca-safetymonitor/internal/sqmclient"
	"sqmeter-alpaca-safetymonitor/internal/state"
)

// Input is everything the evaluator needs to produce a safety decision.
type Input struct {
	Connected             bool
	HasData               bool
	Sensors               *sqmclient.SensorsResponse
	LastSuccessfulPollUTC time.Time
	LastPollUTC           time.Time
	LastError             string
}

// Evaluate applies the configured safety rules to the current sensor reading
// and returns a fully-populated EvaluatedState.  It never panics on bad data.
func Evaluate(cfg *config.Config, in Input) state.EvaluatedState {
	s := state.EvaluatedState{
		IsSafe:                true,
		Reasons:               []string{},
		Warnings:              []string{},
		LastPollUTC:           in.LastPollUTC,
		LastSuccessfulPollUTC: in.LastSuccessfulPollUTC,
		LastError:             in.LastError,
		RawSensors:            in.Sensors,
	}

	if !in.Connected {
		s.IsSafe = false
		s.Reasons = append(s.Reasons, "not connected")
		return s
	}

	if cfg.ManualOverride == "force_unsafe" {
		s.IsSafe = false
		s.Reasons = append(s.Reasons, "manual override: force_unsafe")
		return s
	}

	if cfg.ManualOverride == "force_safe" {
		if !in.HasData && cfg.FailClosed {
			s.IsSafe = false
			s.Reasons = append(s.Reasons, "force_safe active but no valid data yet and FAIL_CLOSED=true")
		} else {
			s.IsSafe = true
			s.Warnings = append(s.Warnings, "WARNING: force_safe override is active — all safety rules are bypassed")
		}
		return s
	}

	if !in.HasData {
		if cfg.FailClosed {
			s.IsSafe = false
			s.Reasons = append(s.Reasons, "no successful sensor data yet (FAIL_CLOSED=true)")
		}
		return s
	}

	if !in.LastSuccessfulPollUTC.IsZero() {
		age := time.Since(in.LastSuccessfulPollUTC)
		threshold := time.Duration(cfg.StaleAfterSeconds) * time.Second
		if age > threshold {
			s.IsSafe = false
			s.Reasons = append(s.Reasons, fmt.Sprintf(
				"sensor data is stale: last success %s ago (threshold %s)",
				age.Round(time.Second), threshold,
			))
		}
	}

	if in.Sensors == nil {
		if cfg.FailClosed {
			s.IsSafe = false
			s.Reasons = append(s.Reasons, "sensor data is nil (FAIL_CLOSED=true)")
		}
		return s
	}

	sensors := in.Sensors
	s.Values = state.Values{
		SQM:               sensors.SkyQuality.SQM,
		Bortle:            sensors.SkyQuality.Bortle,
		Temperature:       sensors.Environment.Temperature,
		Humidity:          sensors.Environment.Humidity,
		Dewpoint:          sensors.Environment.Dewpoint,
		CloudCoverPercent: sensors.CloudConditions.CloudCoverPercent,
		CloudCondition:    sensors.CloudConditions.Description,
		TemperatureDelta:  sensors.CloudConditions.TemperatureDelta,
		CorrectedDelta:    sensors.CloudConditions.CorrectedDelta,
	}

	if cfg.RequireLightStatus && sensors.LightSensor.Status != 0 {
		s.IsSafe = false
		s.Reasons = append(s.Reasons, fmt.Sprintf(
			"light sensor error (status=%d)", sensors.LightSensor.Status,
		))
	}
	if cfg.RequireEnvStatus && sensors.Environment.Status != 0 {
		s.IsSafe = false
		s.Reasons = append(s.Reasons, fmt.Sprintf(
			"environment sensor error (status=%d)", sensors.Environment.Status,
		))
	}
	if cfg.RequireIRStatus && sensors.IRTemperature.Status != 0 {
		s.IsSafe = false
		s.Reasons = append(s.Reasons, fmt.Sprintf(
			"IR temperature sensor error (status=%d)", sensors.IRTemperature.Status,
		))
	}

	if sensors.CloudConditions.CloudCoverPercent >= cfg.CloudCoverUnsafePct {
		s.IsSafe = false
		s.Reasons = append(s.Reasons, fmt.Sprintf(
			"cloud cover %.1f%% >= unsafe threshold %.1f%%",
			sensors.CloudConditions.CloudCoverPercent, cfg.CloudCoverUnsafePct,
		))
	} else if sensors.CloudConditions.CloudCoverPercent >= cfg.CloudCoverCautionPct {
		s.Warnings = append(s.Warnings, fmt.Sprintf(
			"cloud cover %.1f%% >= caution threshold %.1f%%",
			sensors.CloudConditions.CloudCoverPercent, cfg.CloudCoverCautionPct,
		))
	}

	if cfg.SQMMinSafe != nil && sensors.SkyQuality.SQM < *cfg.SQMMinSafe {
		s.IsSafe = false
		s.Reasons = append(s.Reasons, fmt.Sprintf(
			"SQM %.2f mag/arcsec² < minimum %.2f",
			sensors.SkyQuality.SQM, *cfg.SQMMinSafe,
		))
	}

	if cfg.HumidityMaxSafe != nil && sensors.Environment.Humidity > *cfg.HumidityMaxSafe {
		s.IsSafe = false
		s.Reasons = append(s.Reasons, fmt.Sprintf(
			"humidity %.1f%% > maximum %.1f%%",
			sensors.Environment.Humidity, *cfg.HumidityMaxSafe,
		))
	}

	if cfg.DewpointMarginMinC != nil {
		margin := sensors.Environment.Temperature - sensors.Environment.Dewpoint
		if margin < *cfg.DewpointMarginMinC {
			s.IsSafe = false
			s.Reasons = append(s.Reasons, fmt.Sprintf(
				"dew point margin %.1f°C < minimum %.1f°C (temp=%.1f°C, dew=%.1f°C)",
				margin, *cfg.DewpointMarginMinC,
				sensors.Environment.Temperature, sensors.Environment.Dewpoint,
			))
		}
	}

	return s
}
