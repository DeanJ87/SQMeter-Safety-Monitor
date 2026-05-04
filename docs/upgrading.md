# Upgrading

The supported upgrade path is **install the new installer over the existing
version** — you do not need to uninstall first.

---

## Steps

1. Download the new installer (`sqmeter-ascom-alpaca-setup-vX.Y.Z.exe`)
   from [GitHub Releases](https://github.com/DeanJ87/SQMeter-ASCOM-Alpaca/releases).
2. Run the installer as Administrator.
3. The installer stops and unregisters the existing service, replaces the
   binary, re-registers the service, and starts it.
4. `config.json` and `device-uuid.txt` in
   `%ProgramData%\SQMeter ASCOM Alpaca\` are **never touched** by the
   installer — your settings are always preserved.
5. If you are upgrading from a beta build that stored config in
   `%ProgramData%\SQMeter SafetyMonitor\`, the new binary automatically
   copies your config to the new path on first startup (see
   [App data path migration](#app-data-path-migration) below).

---

## What the installer does during an upgrade

| Step | Details |
|------|---------|
| Stop service | The existing service is stopped and unregistered before the binary is replaced, so the running executable is not locked. |
| Replace binary | The new `sqmeter-ascom-alpaca.exe` is written to the install directory. |
| Re-register service | The service is registered against the new binary and started. |
| Config preserved | `config.json`, `device-uuid.txt`, and `device-oc-uuid.txt` in `%ProgramData%\SQMeter ASCOM Alpaca\` are not modified. |

---

## App data path migration

Starting with this release the default Windows data directory changed from
`%ProgramData%\SQMeter SafetyMonitor\` (used in beta builds) to
`%ProgramData%\SQMeter ASCOM Alpaca\`.

On the first startup after upgrading, the binary checks for the legacy
directory and automatically copies `config.json` and any `.bak` backup files
to the new location. The legacy directory is **never deleted**, so your
original files remain available for manual rollback.

### What happens when both paths exist

If `%ProgramData%\SQMeter ASCOM Alpaca\config.json` already exists (e.g. you
ran the new binary once before), migration is skipped and the legacy path is
left untouched. The new path always wins — legacy config is never overwritten.

### Manual rollback (path migration)

If you need to revert to the legacy path:

1. Stop the service:
   ```cmd
   sqmeter-ascom-alpaca.exe --service stop
   ```
2. Copy `config.json` from `%ProgramData%\SQMeter SafetyMonitor\` back to
   `%ProgramData%\SQMeter ASCOM Alpaca\` (or use `--config` to point at the
   legacy path directly).
3. Start the service:
   ```cmd
   sqmeter-ascom-alpaca.exe --service start
   ```

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
   from `%ProgramData%\SQMeter ASCOM Alpaca\` in that case.

---

## Migrating from an older installation (config path change)

Versions prior to the `%ProgramData%` config path change stored `config.json`
beside the executable (e.g.
`C:\Program Files\SQMeter ASCOM Alpaca\config.json`).

To migrate manually:

1. Stop the service:
   ```cmd
   sqmeter-ascom-alpaca.exe --service stop
   ```
2. Copy `config.json` from the install directory to
   `%ProgramData%\SQMeter ASCOM Alpaca\`.
3. Start the service:
   ```cmd
   sqmeter-ascom-alpaca.exe --service start
   ```

If no config exists in `%ProgramData%`, the service starts with built-in
defaults and opens the setup page automatically on the first interactive run.

---

## Automatic updates

Automatic update checking is **not implemented**. New releases are published on
[GitHub Releases](https://github.com/DeanJ87/SQMeter-ASCOM-Alpaca/releases).

Run the following to see the currently installed version and the releases URL:

```cmd
sqmeter-ascom-alpaca.exe --version
```
