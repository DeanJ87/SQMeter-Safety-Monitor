# Configuration

All runtime settings for SQMeter ASCOM Alpaca live in a single JSON file,
`config.json`. The web setup UI at `http://localhost:11111/setup` reads and
writes this file directly.

---

## Config file location

| Platform | Default path |
|----------|-------------|
| Windows | `%ProgramData%\SQMeter ASCOM Alpaca\config.json` |
| Linux / macOS | `<directory containing the executable>/config.json` |

On Windows the config file lives in `%ProgramData%`
(typically `C:\ProgramData\SQMeter ASCOM Alpaca\config.json`), not beside the
`.exe`. This keeps the install directory under `Program Files` free of mutable
user data.

Override the path at runtime with `--config <path>`.

---

## Source of truth

`config.json` is the authoritative source for all application settings.

- No `.env` file is auto-loaded.
- Broad application-setting environment variables are not supported.
- The only environment variable that influences runtime behaviour is `LOG_LEVEL`
  (a process/logging concern). All other settings must be in `config.json`.
- The setup UI reads and writes `config.json` directly. Changes take effect
  immediately for most settings. Network settings require a service restart
  (the UI warns you which fields are affected).

---

## Config keys

You can edit `config.json` directly or use the web setup UI. Example minimal
config:

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
| `SQMETER_BASE_URL` | *required* | Base URL of your SQMeter device (e.g. `http://sqmeter.local`) |
| `ALPACA_HTTP_BIND` | `127.0.0.1` | HTTP bind address. Use `0.0.0.0` for LAN access. **Restart required.** |
| `ALPACA_HTTP_PORT` | `11111` | HTTP port for Alpaca API and web UI. **Restart required.** |
| `ALPACA_DISCOVERY_PORT` | `32227` | UDP port for Alpaca discovery. **Restart required.** |
| `POLL_INTERVAL_SECONDS` | `5` | How often to poll the SQMeter (seconds). Applied immediately. |
| `STALE_AFTER_SECONDS` | `30` | Report unsafe if data is older than this. Must be ≥ `POLL_INTERVAL_SECONDS`. Applied immediately. |
| `FAIL_CLOSED` | `true` | Report unsafe when SQMeter is unreachable or data is stale. Applied immediately. |
| `CONNECTED_ON_STARTUP` | `true` | Start in connected state on next launch. |
| `CLOUD_COVER_UNSAFE_PERCENT` | `80` | Cloud cover % that triggers UNSAFE. Applied immediately. |
| `CLOUD_COVER_CAUTION_PERCENT` | `50` | Cloud cover % that triggers a caution warning. Applied immediately. |
| `REQUIRE_LIGHT_SENSOR_STATUS_OK` | `true` | Report unsafe if light sensor (TSL2591) reports status ≠ 0. Applied immediately. |
| `REQUIRE_ENVIRONMENT_STATUS_OK` | `true` | Report unsafe if environment sensor (BME280) reports status ≠ 0. Applied immediately. |
| `REQUIRE_IR_TEMPERATURE_STATUS_OK` | `true` | Report unsafe if IR sensor (MLX90614) reports status ≠ 0. Applied immediately. |
| `SQM_MIN_SAFE` | *(off)* | Report unsafe if SQM drops below this (mag/arcsec²). Omit to disable. Applied immediately. |
| `HUMIDITY_MAX_SAFE` | *(off)* | Report unsafe if humidity exceeds this (%). Omit to disable. Applied immediately. |
| `DEWPOINT_MARGIN_MIN_C` | *(off)* | Report unsafe if (temperature − dew point) < this margin (°C). Omit to disable. Applied immediately. |
| `MANUAL_OVERRIDE` | `auto` | `auto` — use safety rules; `force_safe` — bypass all rules; `force_unsafe` — always report UNSAFE. Applied immediately. |
| `LOG_LEVEL` | `info` | `debug` \| `info` \| `warn` \| `error`. Can also be set by the `LOG_LEVEL` environment variable. |
| `config_version` | *(managed)* | Schema version. Managed automatically — do not edit by hand. |

### Restart-required fields

Changing `ALPACA_HTTP_BIND`, `ALPACA_HTTP_PORT`, or `ALPACA_DISCOVERY_PORT`
requires a service restart before the new values take effect. The setup UI
displays a warning banner listing which fields need a restart after you save.

### Network security

Binding `ALPACA_HTTP_BIND` to `0.0.0.0` exposes the service on all network
interfaces. Only do this on a trusted LAN. Do not expose these ports to the
internet. The dashboard shows a warning banner when the service is reachable
from the network.

---

## CLI flags

| Flag | Description |
|------|-------------|
| `--config <path>` | Path to the JSON config file (overrides the platform default) |
| `--write-default-config` | Write default settings to the `--config` path and exit |
| `--check-config` | Validate the config file and exit with a summary |
| `--diagnostics` | Print a live diagnostics report (requires the service to be running) |
| `--version` | Print version, commit, build date, and the releases URL |
| `--service <cmd>` | Manage the Windows service: `install` \| `uninstall` \| `start` \| `stop` \| `status` |

For CI or scripted testing, generate a temporary config file and pass it with
`--config` rather than relying on environment variables.

---

## Sensor status codes

The SQMeter reports a `status` field for each sensor:

| Value | Meaning |
|-------|---------|
| `0` | OK |
| `1` | Sensor not found |
| `2` | Read error |
| `3` | Stale data |

When `REQUIRE_*_STATUS_OK` is set to `true` and a sensor reports status ≠ 0,
the bridge reports UNSAFE. The reason is displayed in the web UI and in
`/status.json`.

---

## Diagnostics

Run the diagnostics command while the service is running:

```cmd
sqmeter-ascom-alpaca.exe --diagnostics
```

This queries `GET /api/diagnostics` on the running service and prints a
structured report covering version, config, discovery health, poller state,
and current safety status.

The same data is available in JSON format at:

```
http://localhost:11111/api/diagnostics
```
