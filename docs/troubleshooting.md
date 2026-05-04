# Troubleshooting

---

## Service is not starting

1. Check the Windows Event Log (Event Viewer → Windows Logs → Application) for
   errors from `SQMeterASCOMAlpaca`.
2. Run from a Command Prompt to see console output directly:
   ```cmd
   sqmeter-ascom-alpaca.exe --check-config
   ```
   This validates `config.json` and exits with a summary. Fix any reported
   errors before trying to start the service.
3. If `config.json` is missing, run:
   ```cmd
   sqmeter-ascom-alpaca.exe --write-default-config
   ```
   This writes a default config to the platform-appropriate path and exits.
4. If `config_version` in the config file is newer than the binary supports,
   the service refuses to start. Upgrade the binary or restore a backup config
   from `%ProgramData%\SQMeter ASCOM Alpaca\`.

---

## Running the diagnostics command

While the service is running, open a Command Prompt and run:

```cmd
sqmeter-ascom-alpaca.exe --diagnostics
```

This queries `GET /api/diagnostics` on the running service and prints a report
covering version, config path, HTTP bind and port, discovery health, poller
state, and current safety status with reasons.

The same data is available as JSON:

```powershell
curl.exe http://127.0.0.1:11111/api/diagnostics
```

---

## Alpaca HTTP API not responding

```powershell
curl.exe http://127.0.0.1:11111/health
curl.exe http://127.0.0.1:11111/management/apiversions
```

If these fail:

1. Confirm the service is running:
   ```cmd
   sqmeter-ascom-alpaca.exe --service status
   ```
2. Confirm the port is bound:
   ```powershell
   netstat -ano | findstr ":11111"
   ```
3. Check that `ALPACA_HTTP_PORT` in `config.json` matches the port you are
   querying.
4. If the service is bound to `127.0.0.1` only, queries from another machine
   will fail — change `ALPACA_HTTP_BIND` to `0.0.0.0` in `config.json` and
   restart.

---

## N.I.N.A. cannot find the devices

See [docs/nina-alpaca-discovery.md](nina-alpaca-discovery.md) for full
discovery troubleshooting, including:

- UDP discovery port checks (PowerShell snippet)
- Listener/PID checks with `netstat`
- ASCOM Alpaca Simulators coexistence
- Manual device entry as a fallback

---

## SafetyMonitor reports UNSAFE unexpectedly

1. Check `http://localhost:11111/status.json` — the `reasons` array lists every
   active UNSAFE condition.
2. Check the dashboard at `http://localhost:11111/` — UNSAFE reasons are shown
   prominently.
3. Common causes:
   - SQMeter unreachable and `FAIL_CLOSED=true` (default) — verify the SQMeter
     URL in `/setup`.
   - A required sensor (`REQUIRE_*_STATUS_OK`) is reporting a non-zero status
     code (not found, read error, stale).
   - Cloud cover above `CLOUD_COVER_UNSAFE_PERCENT`.
   - `MANUAL_OVERRIDE=force_unsafe`.
4. Run `--diagnostics` for a structured safety report.

---

## SQMeter connection failing

1. In the web setup at `/setup`, click **Test connection** next to
   `SQMETER_BASE_URL` to confirm the bridge can reach the sensor.
2. Verify the SQMeter URL is reachable from the machine running the bridge:
   ```powershell
   curl.exe http://<sqmeter-ip>/api/sensors
   ```
3. Confirm the SQMeter is on the same LAN and the IP or hostname is correct.

---

## Dashboard looks stale

The web dashboard auto-refreshes every 10 seconds. If it appears stale after
a config change, do a hard refresh (`Ctrl+Shift+R` in most browsers).

For the live current state:

```powershell
curl.exe http://127.0.0.1:11111/status.json
```

---

## Port conflicts

| Port | Protocol | Used for |
|------|----------|---------|
| `11111` | TCP | Alpaca HTTP API and web UI |
| `32227` | UDP | Alpaca discovery |

If another process already holds these ports:

```powershell
netstat -ano | findstr ":11111"
netstat -ano | findstr ":32227"
Get-Process -Id <PID>
```

Change `ALPACA_HTTP_PORT` or `ALPACA_DISCOVERY_PORT` in `config.json` if you
cannot free the ports, then restart the service.

---

## Config version mismatch

If the service logs `config_version X is newer than the binary supports`, the
running binary is older than the config file. Upgrade the binary or restore an
earlier backup config from `%ProgramData%\SQMeter ASCOM Alpaca\`.

---

## Firewall blocking ports

Run as Administrator:

```cmd
netsh advfirewall firewall add rule name="SQMeter Alpaca HTTP" dir=in action=allow protocol=TCP localport=11111
netsh advfirewall firewall add rule name="SQMeter Alpaca Discovery" dir=in action=allow protocol=UDP localport=32227
```

The installer does not add these rules automatically.
