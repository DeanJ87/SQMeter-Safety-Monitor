# Windows service

SQMeter ASCOM Alpaca ships with a built-in Windows service wrapper. The Windows
installer registers and starts the service automatically. You can also manage it
manually with the CLI flags below.

---

## Service details

| Property | Value |
|----------|-------|
| Service name | `SQMeterAlpacaSafetyMonitor` |
| Display name | `SQMeter Alpaca SafetyMonitor` |
| Startup type | Automatic (set by installer) |
| Config / data directory | `%ProgramData%\SQMeter SafetyMonitor\` |
| Install directory | `%ProgramFiles%\SQMeter Alpaca SafetyMonitor\` (default) |

---

## Service commands

Run these as Administrator from a Command Prompt or PowerShell:

```cmd
sqmeter-alpaca-safetymonitor.exe --service install
sqmeter-alpaca-safetymonitor.exe --service start
sqmeter-alpaca-safetymonitor.exe --service stop
sqmeter-alpaca-safetymonitor.exe --service status
sqmeter-alpaca-safetymonitor.exe --service uninstall
```

The Windows installer handles `install`, `start`, `stop`, and `uninstall`
automatically during fresh installs and upgrades. You only need to run these
commands manually for troubleshooting or if you installed the binary without
using the installer.

---

## Web-based service controls

The dashboard at `http://localhost:11111/` includes **Restart service** and
**Stop service** buttons when the service is running.

- **Restart** — the process exits with code 1. The Windows Service manager
  restarts it automatically if the recovery action is set to "Restart the
  service" (the installer configures this). Without a recovery action the
  process exits and stays stopped.
- **Stop** — the process exits cleanly (code 0) and stays stopped until
  manually restarted with `--service start`.

Both controls use `POST` requests; plain navigation links (`GET`) cannot
trigger them. If the service is bound to a non-loopback address
(`ALPACA_HTTP_BIND=0.0.0.0`) the dashboard shows a network-reachability
warning next to these controls.

> **Note:** Only expose the web UI on a trusted LAN. The Restart and Stop
> controls stop the safety bridge, causing N.I.N.A. and all Alpaca clients to
> lose safety integration.

---

## Running without the installer (NSSM)

If you prefer NSSM over the built-in service wrapper:

```cmd
nssm install SQMeterAlpaca "C:\path\to\sqmeter-alpaca-safetymonitor.exe"
nssm set SQMeterAlpaca AppDirectory "C:\path\to\"
nssm set SQMeterAlpaca AppStdout "C:\path\to\logs\out.log"
nssm set SQMeterAlpaca AppStderr "C:\path\to\logs\err.log"
nssm set SQMeterAlpaca AppExit Default Restart
nssm start SQMeterAlpaca
```

Setting `AppExit Default Restart` enables NSSM to restart the process when a
web-triggered restart exits with code 1.

---

## Running as a scheduled task (alternative)

1. Open Task Scheduler → Create Task
2. Trigger: At system startup
3. Action: Start a program → path to the `.exe`
4. Set "Run whether user is logged on or not"
5. Set "Run with highest privileges" if the service needs to bind ports < 1024

Note: A scheduled task does not get the automatic restart behaviour of a
service manager. Use the built-in service or NSSM if you need automatic
restart after a web-triggered restart.

---

## Firewall rules

Allow the HTTP and UDP ports through Windows Firewall (run as Administrator):

```cmd
netsh advfirewall firewall add rule name="SQMeter Alpaca HTTP" dir=in action=allow protocol=TCP localport=11111
netsh advfirewall firewall add rule name="SQMeter Alpaca Discovery" dir=in action=allow protocol=UDP localport=32227
```

The installer does not add firewall rules automatically. Add them if N.I.N.A.
is on a different machine or if discovery does not work from the local machine.

---

## Security notes

- The default bind address is `127.0.0.1` (loopback only). N.I.N.A. running on
  the same machine can always reach the service.
- Only change `ALPACA_HTTP_BIND` to `0.0.0.0` if you need to reach the service
  from another machine on the LAN.
- Do not expose port `11111` or `32227` to the internet. These ports provide
  no authentication.
- The web UI service controls (Restart, Stop) are available to anyone who can
  reach the web UI. On a LAN-exposed bind address this means anyone on your
  network can stop the safety bridge.
