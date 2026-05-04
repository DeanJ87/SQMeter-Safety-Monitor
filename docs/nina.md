# N.I.N.A. setup

SQMeter ASCOM Alpaca exposes two Alpaca devices from a single service:

| Device | N.I.N.A. equipment slot | Purpose |
|--------|------------------------|---------|
| `SQMeter SafetyMonitor` | Equipment → Safety Monitor | Controls whether N.I.N.A. considers it safe to operate (IsSafe) |
| `SQMeter ObservingConditions` | Equipment → Weather | Provides sky-condition data for capture metadata (FITS/XISF headers) |

Both devices are served from the same HTTP port and share the same SQMeter
polling loop. No extra configuration is required beyond the initial setup.

---

## SafetyMonitor

1. Start the service (or confirm it is running at `http://localhost:11111`).
2. Open N.I.N.A.
3. Go to **Equipment → Safety Monitor**.
4. In the driver selector, choose **ASCOM Alpaca**.
5. Click the **Refresh** / discovery button.
   **SQMeter SafetyMonitor** should appear in the list.
6. Select it and click **Connect**.
7. Verify the IsSafe indicator; cross-check against
   `http://localhost:11111/status.json`.

N.I.N.A. uses the SafetyMonitor IsSafe value for safety-critical decisions such
as roof/dome automation. Test thoroughly before relying on it for automation.

---

## ObservingConditions

1. Go to **Equipment → Weather** (labelled "Weather" in the N.I.N.A. equipment
   panel).
2. In the driver selector, choose **ASCOM Alpaca**.
3. Click **Refresh** — **SQMeter ObservingConditions** should appear.
4. Select it and click **Connect**.

N.I.N.A. uses the ObservingConditions values to populate FITS/XISF metadata
headers on captured images (temperature, humidity, dew point, sky quality, etc.)
where supported. The exact headers written depend on the N.I.N.A. version and
the imaging sequence settings.

---

## Manual device entry (if discovery does not work)

If the Refresh button does not show the devices, add them manually:

| Field | SafetyMonitor | ObservingConditions |
|-------|--------------|-------------------|
| Host | `127.0.0.1` | `127.0.0.1` |
| Port | `11111` | `11111` |
| Device type | `SafetyMonitor` | `ObservingConditions` |
| Device number | `0` | `0` |

Use the machine's LAN IP address instead of `127.0.0.1` if N.I.N.A. is on a
different machine (and ensure `ALPACA_HTTP_BIND` is `0.0.0.0` in `config.json`).

---

## Discovery troubleshooting

For a full explanation of Alpaca discovery, port numbers, PowerShell checks,
and ASCOM Alpaca Simulators coexistence, see
[docs/nina-alpaca-discovery.md](nina-alpaca-discovery.md).

---

## Notes

- N.I.N.A. (and other Alpaca clients) find both devices via the same UDP
  discovery broadcast. The bridge replies once; N.I.N.A. then queries
  `/management/v1/configureddevices` and lists all devices from that server.
- ASCOM Remote Server is **not required**. This bridge implements the Alpaca
  protocol natively — there is no COM driver or ASCOM Platform dependency.
- Both devices share live sensor data. If the SQMeter is unreachable, both the
  SafetyMonitor (UNSAFE) and ObservingConditions (error responses) reflect this.
