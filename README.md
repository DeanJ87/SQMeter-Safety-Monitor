# sqmeter-alpaca-safetymonitor

A standalone **ASCOM Alpaca SafetyMonitor** bridge for the [SQMeter ESP32](https://deanj87.github.io/SQMeter/) sky-quality sensor.

Runs as a single `.exe` on your N.I.N.A. Windows machine.  No ASCOM COM drivers, no registration, no Visual Studio templates — pure Alpaca over HTTP/UDP.

[Privacy policy](PRIVACY.md)

---

## What it does

- Polls `GET /api/sensors` on your SQMeter every few seconds
- Evaluates configurable safety rules (cloud cover, SQM, humidity, dew-point margin, sensor health)
- Exposes a standards-compliant **ASCOM Alpaca SafetyMonitor** device at `http://localhost:11111`
- Responds to **ASCOM Alpaca UDP discovery** on port 32227 so N.I.N.A. finds it automatically
- Serves a live **web dashboard** at `http://localhost:11111/`
- Provides a `/status.json` debug endpoint

It answers one question: **"Is it safe for the observatory to operate right now?"**

> **Scope note:** This project is *only* the SafetyMonitor bridge.  An ObservingConditions driver (temperature, humidity, sky brightness, etc. as Alpaca properties) is a separate future project.

---

## Quick start (Windows)

1. Download `sqmeter-alpaca-safetymonitor-windows-amd64.exe` from [Releases](../../releases)
2. Create a `config.json` next to the exe (see [Configuration](#configuration)), or set environment variables
3. Double-click the exe — a console window will open
4. Browse to `http://localhost:11111` to see the dashboard
5. In N.I.N.A. → Equipment → Safety Monitor → select **ASCOM Alpaca** and connect

---

## Configuration

Copy `.env.example` and rename it to set environment variables, **or** create a `config.json` with the same keys:

```json
{
  "SQMETER_BASE_URL": "http://192.168.1.100",
  "ALPACA_HTTP_PORT": 11111,
  "CLOUD_COVER_UNSAFE_PERCENT": 80,
  "FAIL_CLOSED": true
}
```

| Key | Default | Description |
|-----|---------|-------------|
| `SQMETER_BASE_URL` | *required* | Base URL of your SQMeter device |
| `ALPACA_HTTP_BIND` | `0.0.0.0` | HTTP bind address |
| `ALPACA_HTTP_PORT` | `11111` | HTTP port for Alpaca API + web UI |
| `ALPACA_DISCOVERY_PORT` | `32227` | UDP port for Alpaca discovery |
| `POLL_INTERVAL_SECONDS` | `5` | How often to poll the SQMeter |
| `STALE_AFTER_SECONDS` | `30` | Report unsafe if data is older than this |
| `FAIL_CLOSED` | `true` | Report unsafe if SQMeter is unreachable |
| `CONNECTED_ON_STARTUP` | `true` | Start in connected state |
| `CLOUD_COVER_UNSAFE_PERCENT` | `80` | Cloud cover % that triggers UNSAFE |
| `CLOUD_COVER_CAUTION_PERCENT` | `50` | Cloud cover % that triggers a warning |
| `REQUIRE_LIGHT_SENSOR_STATUS_OK` | `true` | Unsafe if light sensor reports error |
| `REQUIRE_ENVIRONMENT_STATUS_OK` | `true` | Unsafe if env sensor reports error |
| `REQUIRE_IR_TEMPERATURE_STATUS_OK` | `true` | Unsafe if IR sensor reports error |
| `SQM_MIN_SAFE` | *(off)* | Unsafe if SQM drops below this |
| `HUMIDITY_MAX_SAFE` | *(off)* | Unsafe if humidity exceeds this |
| `DEWPOINT_MARGIN_MIN_C` | *(off)* | Unsafe if temp−dewpoint < this (°C) |
| `MANUAL_OVERRIDE` | `auto` | `auto` \| `force_safe` \| `force_unsafe` |
| `LOG_LEVEL` | `info` | `debug` \| `info` \| `warn` \| `error` |

Environment variables always override the config file.

### Sensor status codes

The SQMeter reports a `status` field for each sensor:

| Value | Meaning |
|-------|---------|
| 0 | OK |
| 1 | Sensor not found |
| 2 | Read error |
| 3 | Stale data |

---

## Safety rules

The bridge declares UNSAFE if **any** of the following are true:

- `Connected = false`
- `MANUAL_OVERRIDE = force_unsafe`
- SQMeter unreachable and `FAIL_CLOSED = true`
- No successful data yet and `FAIL_CLOSED = true`
- Most recent successful data is older than `STALE_AFTER_SECONDS`
- A required sensor reports status ≠ 0
- Cloud cover ≥ `CLOUD_COVER_UNSAFE_PERCENT`
- `SQM_MIN_SAFE` set and SQM < minimum
- `HUMIDITY_MAX_SAFE` set and humidity > maximum
- `DEWPOINT_MARGIN_MIN_C` set and (temperature − dew point) < margin

The web UI and `/status.json` always show the reason(s) for any UNSAFE state.

---

## curl test examples

```bash
# Management API
curl http://localhost:11111/management/apiversions
curl http://localhost:11111/management/v1/description
curl http://localhost:11111/management/v1/configureddevices

# SafetyMonitor
curl "http://localhost:11111/api/v1/safetymonitor/0/issafe?ClientID=1&ClientTransactionID=1"
curl "http://localhost:11111/api/v1/safetymonitor/0/connected?ClientID=1&ClientTransactionID=2"
curl "http://localhost:11111/api/v1/safetymonitor/0/name?ClientID=1&ClientTransactionID=3"

# Status / health
curl http://localhost:11111/status.json
curl http://localhost:11111/health

# Force refresh via Alpaca action
curl -X PUT http://localhost:11111/api/v1/safetymonitor/0/action \
     -d "Action=refresh&ClientID=1&ClientTransactionID=10"

# Disconnect
curl -X PUT http://localhost:11111/api/v1/safetymonitor/0/connected \
     -d "Connected=false&ClientID=1&ClientTransactionID=11"
```

---

## N.I.N.A. setup

1. Start `sqmeter-alpaca-safetymonitor.exe`
2. Open N.I.N.A.
3. Go to **Equipment → Safety Monitor**
4. In the device selector, choose **ASCOM Alpaca**
5. Click the **Refresh** / discovery button — **SQMeter SafetyMonitor** should appear
6. Select it and click **Connect**
7. Watch the IsSafe indicator; verify it matches `http://localhost:11111/status.json`

If discovery does not work, manually add the device:

- Host: `127.0.0.1`
- Port: `11111`
- Device type: `SafetyMonitor`
- Device number: `0`

---

## Running as a Windows service

### Option A: NSSM (recommended)

```cmd
nssm install SQMeterAlpaca "C:\path\to\sqmeter-alpaca-safetymonitor.exe"
nssm set SQMeterAlpaca AppDirectory "C:\path\to\"
nssm set SQMeterAlpaca AppStdout "C:\path\to\logs\out.log"
nssm set SQMeterAlpaca AppStderr "C:\path\to\logs\err.log"
nssm start SQMeterAlpaca
```

### Option B: Task Scheduler

1. Open Task Scheduler → Create Task
2. Trigger: At system startup
3. Action: Start a program → path to exe
4. Set "Run whether user is logged on or not"

---

## Firewall (Windows)

Allow the HTTP and UDP ports through Windows Firewall (run as Administrator):

```cmd
netsh advfirewall firewall add rule name="SQMeter Alpaca HTTP" dir=in action=allow protocol=TCP localport=11111
netsh advfirewall firewall add rule name="SQMeter Alpaca Discovery" dir=in action=allow protocol=UDP localport=32227
```

---

## Building from source

```bash
git clone https://github.com/your-org/sqmeter-alpaca-safetymonitor
cd sqmeter-alpaca-safetymonitor
make build          # ./bin/sqmeter-alpaca-safetymonitor (current platform)
make build-windows  # ./dist/sqmeter-alpaca-safetymonitor-windows-amd64.exe
make test           # run all tests with race detector
make lint           # gofmt check + go vet
```

Build artifacts go into `./bin/` and `./dist/` — both are git-ignored.

### Version injection

```bash
VERSION=v0.1.0 make build
```

Or GoReleaser handles this automatically on tagged releases.

### Module path

If you fork this repository, update the module path in `go.mod` and all
`import` statements from `github.com/sqmeter-alpaca/sqmeter-alpaca-safetymonitor`
to your own path.

---

## GitHub Actions

| Workflow | Trigger | Jobs |
|----------|---------|------|
| `ci.yml` | push/PR to `main` | lint, test, build Windows + Linux |
| `release.yml` | `v*.*.*` tag pushed | tests + GoReleaser → GitHub Release |

To publish a release:

```bash
git tag v0.1.0
git push origin v0.1.0
```

---

## Alpaca Conform testing

For ASCOM Conform Universal testing:

1. Download [ASCOM Conform Universal](https://github.com/ASCOMInitiative/ConformU/releases)
2. Configure it to connect to your Alpaca device at `http://127.0.0.1:11111` device `0`
3. Run the SafetyMonitor conformance check

Target: `v1.0.0` once all Conform tests pass.

---

## Versioning

- `v0.1.0` — initial usable release
- `v0.2.x` — feature additions
- `v1.0.0` — after Alpaca Conform testing passes

---

## Branch naming

| Pattern | Use |
|---------|-----|
| `main` | stable, CI-protected |
| `feature/alpaca-safetymonitor` | new features |
| `fix/discovery-response` | bug fixes |
| `chore/ci-release` | CI/build changes |
