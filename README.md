# SQMeter ASCOM Alpaca

[![CI](https://github.com/DeanJ87/SQMeter-ASCOM-Alpaca/actions/workflows/ci.yml/badge.svg)](https://github.com/DeanJ87/SQMeter-ASCOM-Alpaca/actions/workflows/ci.yml)
[![codecov](https://codecov.io/github/DeanJ87/SQMeter-ASCOM-Alpaca/graph/badge.svg?token=I7DHSX92BN)](https://codecov.io/github/DeanJ87/SQMeter-ASCOM-Alpaca)

A native **ASCOM Alpaca bridge** for the [SQMeter ESP32](https://deanj87.github.io/SQMeter/) sky-quality sensor.

Runs as a single `.exe` (or Windows service) on your observatory PC. No ASCOM COM drivers, no ASCOM Remote Server, no Visual Studio templates — pure Alpaca over HTTP/UDP.

[Privacy policy](PRIVACY.md)

---

## Project scope and naming

| Name | Meaning |
|------|---------|
| **SQMeter ASCOM Alpaca** | This project — the bridge/service that reads from an SQMeter ESP32 and speaks ASCOM Alpaca. |
| **SQMeter SafetyMonitor** | The Alpaca `SafetyMonitor` device exposed by this bridge. Used by N.I.N.A. for safety decisions. |
| **SQMeter ObservingConditions** | The Alpaca `ObservingConditions` device exposed by this bridge. Used by N.I.N.A. for capture metadata (FITS/XISF headers). |

Both devices are served from a single service on the same HTTP port. The binary and Go module are named `sqmeter-ascom-alpaca`.

---

## What it does

- Polls `GET /api/sensors` on your SQMeter every few seconds
- Evaluates configurable safety rules (cloud cover, SQM, humidity, dew-point margin, sensor health)
- Exposes a standards-compliant **ASCOM Alpaca SafetyMonitor** device at `http://localhost:11111`
- Exposes a standards-compliant **ASCOM Alpaca ObservingConditions** device at the same port
- Responds to **ASCOM Alpaca UDP discovery** on port 32227 so N.I.N.A. finds both devices automatically
- Serves a live **web dashboard** at `http://localhost:11111/`
- Provides a `/status.json` debug endpoint and a `--diagnostics` CLI command

It answers two questions: **"Is it safe for the observatory to operate right now?"** and **"What are the current sky conditions?"**

---

## Quick start (Windows)

1. Download `sqmeter-ascom-alpaca-setup-vX.Y.Z.exe` from [Releases](https://github.com/DeanJ87/SQMeter-ASCOM-Alpaca/releases)
2. Run the installer as Administrator — it installs the binary, registers a Windows service, and starts it
3. On first run the setup page opens automatically at `http://localhost:11111/setup`
4. Complete setup to point the bridge at your SQMeter
5. Browse to `http://localhost:11111` to see the dashboard
6. In N.I.N.A. → Equipment → Safety Monitor → select **ASCOM Alpaca** → click **Refresh** → select **SQMeter SafetyMonitor** → **Connect**

For full N.I.N.A. setup (SafetyMonitor + ObservingConditions), see [docs/nina.md](docs/nina.md).

---

## Documentation

- [docs/configuration.md](docs/configuration.md) — config file, all settings, CLI flags, sensor status codes
- [docs/windows-service.md](docs/windows-service.md) — service install/start/stop/uninstall, NSSM, firewall
- [docs/upgrading.md](docs/upgrading.md) — upgrade steps, config preservation, schema migration, rollback
- [docs/nina.md](docs/nina.md) — N.I.N.A. SafetyMonitor and ObservingConditions setup
- [docs/nina-alpaca-discovery.md](docs/nina-alpaca-discovery.md) — Alpaca discovery deep-dive, port numbers, PowerShell checks, ASCOM Simulators coexistence
- [docs/troubleshooting.md](docs/troubleshooting.md) — discovery issues, diagnostics CLI, common problems

---

## Safety rules

The bridge declares UNSAFE if **any** of the following are true:

- `Connected = false`
- `MANUAL_OVERRIDE = force_unsafe`
- SQMeter unreachable and `FAIL_CLOSED = true`
- No successful data yet and `FAIL_CLOSED = true`
- Most recent successful data is older than `STALE_AFTER_SECONDS` seconds
- A required sensor reports status ≠ 0
- Cloud cover ≥ `CLOUD_COVER_UNSAFE_PERCENT`
- `SQM_MIN_SAFE` is set and SQM < minimum
- `HUMIDITY_MAX_SAFE` is set and humidity > maximum
- `DEWPOINT_MARGIN_MIN_C` is set and (temperature − dew point) < margin

The web UI and `/status.json` always show the reason(s) for any UNSAFE state.

> **Important:** This is a safety integration. Test thoroughly before using it
> for automated roof or dome control. Verify IsSafe behaviour against known
> sensor conditions before relying on it for automation.

---

## ObservingConditions properties

| Property | Source | Notes |
|---|---|---|
| `cloudcover` | IR temperature differential | Requires IR sensor OK |
| `dewpoint` | BME280 | Requires env sensor OK |
| `humidity` | BME280 | Requires env sensor OK |
| `pressure` | BME280 | Requires env sensor OK |
| `skybrightness` | TSL2591 lux | Requires light sensor OK |
| `skyquality` | TSL2591 SQM | Requires light sensor OK |
| `skytemperature` | MLX90614 object temp | Requires IR sensor OK |
| `temperature` | BME280 | Requires env sensor OK |
| `rainrate` | — | Not implemented (no rain sensor) |
| `starfwhm` | — | Not implemented |
| `winddirection` | — | Not implemented (no anemometer) |
| `windgust` | — | Not implemented |
| `windspeed` | — | Not implemented |
| `averageperiod` | — | Always 0; averaging not supported |

When a sensor is temporarily unavailable (hardware error, stale data), the property returns an Alpaca error `0x04FF` with a descriptive message rather than a silently wrong value.

---

## curl quick-test

```bash
# List all Alpaca devices served by this bridge
curl http://localhost:11111/management/v1/configureddevices

# SafetyMonitor — is it safe?
curl "http://localhost:11111/api/v1/safetymonitor/0/issafe?ClientID=1&ClientTransactionID=1"

# ObservingConditions — sky quality
curl "http://localhost:11111/api/v1/observingconditions/0/skyquality?ClientID=1&ClientTransactionID=1"

# Health and full status
curl http://localhost:11111/health
curl http://localhost:11111/status.json
```

See [docs/nina-alpaca-discovery.md](docs/nina-alpaca-discovery.md) for a complete curl/PowerShell reference.

---

## Building from source

```bash
git clone https://github.com/DeanJ87/SQMeter-ASCOM-Alpaca
cd SQMeter-ASCOM-Alpaca
npm ci              # install local Tailwind CLI and bundled IBM Plex font package
npm run build:assets # regenerate embedded CSS and local font assets
make build          # ./bin/sqmeter-ascom-alpaca (current platform)
make build-windows  # ./dist/sqmeter-ascom-alpaca-windows-amd64.exe
make test           # run all tests with race detector
make lint           # gofmt check + go vet
```

The web dashboard uses Tailwind CSS v4 at build time. Generated CSS is written
to `internal/web/static/app.css`, and IBM Plex WOFF2 files plus the OFL license
are bundled locally so the running service never loads fonts or CSS from a CDN.
Build artifacts go into `./bin/` and `./dist/` — both are git-ignored.

### Version injection

```bash
VERSION=v0.1.0 make build
```

Or GoReleaser handles this automatically on tagged releases.

---

## GitHub Actions

| Workflow | Trigger | Jobs |
|----------|---------|------|
| `ci.yml` | push/PR to `main` | lint, test, build Windows + Linux, ASCOM Conform |
| `release.yml` | `v*.*.*` tag pushed | tests + GoReleaser → GitHub Release + Windows installer |

To publish a release:

```bash
git tag v0.1.0
git push origin v0.1.0
```

---

## Alpaca Conform testing

1. Download [ASCOM Conform Universal](https://github.com/ASCOMInitiative/ConformU/releases)
2. Connect to `http://127.0.0.1:11111`, device `0`
3. Run the SafetyMonitor conformance check; repeat for ObservingConditions

Target: `v1.0.0` once all Conform tests pass.

---

## Versioning

- `v0.1.0` — initial usable release
- `v0.2.x` — feature additions
- `v1.0.0` — after Alpaca Conform testing passes
