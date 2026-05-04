# Upgrading

The supported upgrade path is **install the new installer over the existing
version** — you do not need to uninstall first.

---

## Steps

1. Download the new installer (`sqmeter-alpaca-safetymonitor-setup-vX.Y.Z.exe`)
   from [GitHub Releases](https://github.com/DeanJ87/SQMeter-ASCOM-Alpaca/releases).
2. Run the installer as Administrator.
3. The installer stops and unregisters the existing service, replaces the
   binary, re-registers the service, and starts it.
4. `config.json` and `device-uuid.txt` in
   `%ProgramData%\SQMeter SafetyMonitor\` are **never touched** by the
   installer — your settings are always preserved.

---

## What the installer does during an upgrade

| Step | Details |
|------|---------|
| Stop service | The existing service is stopped and unregistered before the binary is replaced, so the running executable is not locked. |
| Replace binary | The new `sqmeter-alpaca-safetymonitor.exe` is written to the install directory. |
| Re-register service | The service is registered against the new binary and started. |
| Config preserved | `config.json`, `device-uuid.txt`, and `device-oc-uuid.txt` in `%ProgramData%\SQMeter SafetyMonitor\` are not modified. |

---

## Config schema migration

When a new version introduces a config schema change, the binary migrates
the config automatically on startup:

1. The binary reads the on-disk `config.json` and detects a `config_version`
   older than the current schema version.
2. It creates a timestamped backup beside `config.json`
   (e.g. `config.20240115T120000Z.bak`).
3. It rewrites `config.json` with the migrated settings and the updated
   `config_version`.

If the on-disk `config_version` is **newer** than the binary supports, the
service will not start. This means you have downgraded the binary below the
version that wrote the config. Either upgrade the binary back to match, or
restore a backup config.

---

## Rollback

To roll back to a previous version:

1. Download the older installer from
   [GitHub Releases](https://github.com/DeanJ87/SQMeter-ASCOM-Alpaca/releases).
2. Run it over the current installation using the same install-over-existing
   process.
3. `config.json` is unaffected. If the older binary does not understand the
   current `config_version`, it will refuse to start — restore a backup config
   from `%ProgramData%\SQMeter SafetyMonitor\` in that case.

---

## Migrating from an older installation (config path change)

Versions prior to the `%ProgramData%` config path change stored `config.json`
beside the executable (e.g.
`C:\Program Files\SQMeter Alpaca SafetyMonitor\config.json`).

To migrate manually:

1. Stop the service:
   ```cmd
   sqmeter-alpaca-safetymonitor.exe --service stop
   ```
2. Copy `config.json` from the install directory to
   `%ProgramData%\SQMeter SafetyMonitor\`.
3. Start the service:
   ```cmd
   sqmeter-alpaca-safetymonitor.exe --service start
   ```

If no config exists in `%ProgramData%`, the service starts with built-in
defaults and opens the setup page automatically on the first interactive run.

---

## Automatic updates

Automatic update checking is **not implemented**. New releases are published on
[GitHub Releases](https://github.com/DeanJ87/SQMeter-ASCOM-Alpaca/releases).

Run the following to see the currently installed version and the releases URL:

```cmd
sqmeter-alpaca-safetymonitor.exe --version
```
