# N.I.N.A. Alpaca discovery setup and troubleshooting

This document explains how ASCOM Alpaca discovery works, how to connect SQMeter
SafetyMonitor in N.I.N.A., and how to diagnose problems when the device does not
appear or when another Alpaca server (such as ASCOM Alpaca Simulators) is
interfering.

---

## How this device appears in N.I.N.A.

SQMeter SafetyMonitor is a **native ASCOM Alpaca SafetyMonitor** device.

- It runs an HTTP Alpaca API on port `11111` by default.
- It listens on UDP port `32227` for Alpaca discovery broadcasts.
- It does **not** need ASCOM Remote Server. See [ASCOM Remote Server clarification](#ascom-remote-server-clarification).

In N.I.N.A., go to **Equipment → Safety Monitor**, select **ASCOM Alpaca** in
the driver selector, and click the refresh/discovery button. N.I.N.A. will send
a UDP discovery broadcast, receive a reply from each Alpaca server on the
network, and then query `/management/v1/configureddevices` on each one.
SQMeter SafetyMonitor will appear as:

```
DeviceName: SQMeter SafetyMonitor
DeviceType: SafetyMonitor
DeviceNumber: 0
```

Select it and click **Connect**.

---

## Alpaca port numbers explained

There are two distinct port numbers involved. Both are part of the Alpaca
standard.

| Port | Protocol | Purpose |
|------|----------|---------|
| `11111` (default, configurable) | TCP/HTTP | Alpaca HTTP API — all device endpoints, management endpoints, web UI |
| `32227` (default, configurable) | UDP | Alpaca discovery — receives broadcast probes, replies with the HTTP port |

**`11111` is still Alpaca.** It is a common misconception that port `11111` is
reserved for ASCOM Remote Server. Port `11111` is simply the default Alpaca HTTP
port used by many Alpaca servers. SQMeter SafetyMonitor uses it as its default,
and you can change it in `config.json`.

**`32227` is the discovery port.** N.I.N.A. (and other Alpaca clients) broadcast
the string `alpacadiscovery1` to UDP port `32227` on the local network. Every
Alpaca server that is listening replies with a JSON packet:

```json
{"AlpacaPort": 11111}
```

The client then makes an HTTP request to that port to enumerate devices.

Multiple Alpaca servers can share UDP port `32227` simultaneously. On Windows,
SQMeter SafetyMonitor uses `SO_REUSEADDR` so it can coexist with other Alpaca
servers such as ASCOM Alpaca Simulators. Each server replies independently with
its own HTTP port.

Example when both servers are running:

| Server | UDP reply |
|--------|-----------|
| SQMeter SafetyMonitor | `{"AlpacaPort": 11111}` |
| ASCOM Alpaca Simulators | `{"AlpacaPort": 32323}` |

N.I.N.A. queries both and lists all devices it finds.

---

## ASCOM Remote Server clarification

**ASCOM Remote Server is not required** for this device.

ASCOM Remote Server is a bridge that exposes classic ASCOM COM drivers (Windows
in-process DLLs) over the Alpaca HTTP protocol. Use it when you have an existing
COM driver and want to reach it from a remote machine or from software that
prefers Alpaca.

SQMeter SafetyMonitor is already a native Alpaca server. It speaks the Alpaca
HTTP protocol directly, with no COM layer. Installing or running ASCOM Remote
Server alongside it is harmless but unnecessary.

---

## Manual HTTP checks

Use these to confirm the Alpaca API is responding before opening N.I.N.A.

```powershell
# List supported Alpaca API versions
curl.exe http://127.0.0.1:11111/management/apiversions

# List all devices this server exposes
curl.exe http://127.0.0.1:11111/management/v1/configureddevices

# Query the IsSafe value directly
curl.exe "http://127.0.0.1:11111/api/v1/safetymonitor/0/issafe?ClientID=1&ClientTransactionID=1"
```

Expected output for `configureddevices`:

```json
{
  "Value": [
    {
      "DeviceName": "SQMeter SafetyMonitor",
      "DeviceType": "SafetyMonitor",
      "DeviceNumber": 0,
      "UniqueID": "..."
    }
  ],
  "ErrorNumber": 0,
  "ErrorMessage": ""
}
```

You can also check the service status and discovery health:

```powershell
curl.exe http://127.0.0.1:11111/status.json
curl.exe http://127.0.0.1:11111/health
```

`/health` returns a short JSON object:

```json
{"status":"ok","isSafe":true}
```

or, if the device is disconnected or reporting unsafe:

```json
{"status":"unsafe","isSafe":false}
```

`/status.json` returns the full internal state including the discovery listener
health under a `discovery` key.

---

## Manual listener checks

Confirm that the service is actually bound to the expected ports.

```powershell
# Check the HTTP port
netstat -ano | findstr ":11111"

# Check the UDP discovery port
netstat -ano | findstr ":32227"
```

The output shows one line per bound socket, including the PID in the last column.
To find which process owns a PID:

```powershell
Get-Process -Id <PID>
```

If port `11111` is bound to `127.0.0.1:11111` (loopback only), N.I.N.A. running
on the same machine can reach it. If you need to reach it from another machine,
the service must be bound to `0.0.0.0:11111` — change `ALPACA_HTTP_BIND` in
`config.json`.

---

## UDP discovery check

This PowerShell snippet broadcasts the Alpaca discovery message and collects all
responses for three seconds:

```powershell
$udp = New-Object System.Net.Sockets.UdpClient
$udp.EnableBroadcast = $true
$udp.Client.ReceiveTimeout = 3000
$bytes = [System.Text.Encoding]::ASCII.GetBytes("alpacadiscovery1")
$ep = New-Object System.Net.IPEndPoint([System.Net.IPAddress]::Broadcast, 32227)
$udp.Send($bytes, $bytes.Length, $ep) | Out-Null

$remote = New-Object System.Net.IPEndPoint([System.Net.IPAddress]::Any, 0)
$deadline = [DateTime]::UtcNow.AddSeconds(3)
while ([DateTime]::UtcNow -lt $deadline) {
    try {
        $data = $udp.Receive([ref]$remote)
        $reply = [System.Text.Encoding]::ASCII.GetString($data)
        Write-Host "Reply from $($remote.Address):$($remote.Port) => $reply"
    } catch { break }
}
$udp.Close()
```

**Expected output** when SQMeter SafetyMonitor is running:

```
Reply from 127.0.0.1:32227 => {"AlpacaPort":11111}
```

If ASCOM Alpaca Simulators is also running, you will see a second line:

```
Reply from 127.0.0.1:32227 => {"AlpacaPort":32323}
```

If you see only the Simulators line and not the SQMeter line, the SQMeter
SafetyMonitor service is not running or not listening on UDP — see
[Troubleshooting](#troubleshooting) below.

---

## Troubleshooting

### N.I.N.A. does not show the device, but the HTTP API works

The HTTP API responding means the Alpaca server itself is running. The problem is
in UDP discovery.

1. Run the [UDP discovery check](#udp-discovery-check) above. If you get no
   reply from port `11111`, the discovery listener is not running.
2. Check `curl.exe http://127.0.0.1:11111/status.json` and look at the
   `discovery` key. If `"healthy": false` or `"running": false`, the UDP listener
   failed to start — usually because another process already holds port `32227`
   without `SO_REUSEADDR`.
3. Check `netstat -ano | findstr ":32227"` to see which process holds the port.
4. As a workaround, add the device manually in N.I.N.A.:
   - Host: `127.0.0.1` (or the machine's LAN IP if connecting remotely)
   - Port: `11111`
   - Device type: `SafetyMonitor`
   - Device number: `0`

### Only ASCOM Alpaca Simulators appears in N.I.N.A.

The Simulators responded to discovery but SQMeter SafetyMonitor did not. This
usually means one of:

- **SQMeter SafetyMonitor is not running.** Start `sqmeter-alpaca-safetymonitor.exe`.
- **Discovery listener failed to bind.** Check `/status.json` → `discovery.healthy`.
- **Firewall is blocking UDP 32227 inbound.** See [Firewall](#firewall).
- **You are connecting from a different machine** and the service is bound to
  `127.0.0.1` only — discovery broadcasts from another machine will not reach a
  loopback-only listener.

The Simulators appearing proves that discovery itself works. The issue is specific
to the SQMeter SafetyMonitor listener.

### Service is bound to `127.0.0.1` but you need LAN access

The default `ALPACA_HTTP_BIND` is `127.0.0.1`, which accepts connections only
from the same machine. To expose the service on the local network:

1. Edit `config.json` and set `"ALPACA_HTTP_BIND": "0.0.0.0"`.
2. Restart the service.
3. Add a firewall rule for TCP `11111` and UDP `32227` (see below).

Only do this on a trusted local network.

### Firewall blocking TCP 11111 or UDP 32227

Run these commands as Administrator to add inbound allow rules:

```cmd
netsh advfirewall firewall add rule name="SQMeter Alpaca HTTP" dir=in action=allow protocol=TCP localport=11111
netsh advfirewall firewall add rule name="SQMeter Alpaca Discovery" dir=in action=allow protocol=UDP localport=32227
```

After adding the rules, re-run the UDP discovery check to confirm.

### Discovery shows as unhealthy in `/status.json`

`"healthy": false` in the `discovery` object means the UDP listener encountered
an error. The `last_error` field contains the specific message.

Common cause: another process bound port `32227` without `SO_REUSEADDR` before
SQMeter SafetyMonitor started. Find the owner with:

```powershell
netstat -ano | findstr ":32227"
Get-Process -Id <PID>
```

If the conflict is with ASCOM Alpaca Simulators, try stopping the Simulators
first, restarting SQMeter SafetyMonitor, then starting the Simulators again.
Both should coexist once SQMeter SafetyMonitor has successfully bound with
`SO_REUSEADDR`.

### The dashboard looks stale but you changed something

The web dashboard at `http://localhost:11111/` is a server-rendered page. If it
looks stale, do a hard refresh in your browser (`Ctrl+Shift+R`) rather than a
normal reload. The page does not auto-refresh.

For the current live state, use:

```powershell
curl.exe http://127.0.0.1:11111/status.json
```

### Testing from the wrong machine or session

If you are running a remote desktop or SSH session, be aware:

- UDP broadcast discovery is limited to the local subnet. A broadcast from a
  remote desktop session's machine will target that machine's subnet, not
  yours.
- `127.0.0.1` always refers to the machine where the command runs, not the
  machine where the service runs.

For cross-machine testing, use the LAN IP address of the machine running
SQMeter SafetyMonitor (and ensure `ALPACA_HTTP_BIND` is `0.0.0.0`).
