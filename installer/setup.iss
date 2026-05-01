; SQMeter Alpaca SafetyMonitor - Inno Setup installer script
; Build with: ISCC.exe /DAppVersion=1.2.3 setup.iss
;
; Upgrade behaviour (install over an existing version):
;   1. BeforeInstall hook stops and uninstalls the running service.
;   2. Installer replaces the binary.
;   3. [Run] re-installs and restarts the service.
;   4. config.json lives in %ProgramData%\SQMeter SafetyMonitor\ and is never
;      touched by the installer, so user configuration is preserved on upgrade.
;   5. device-uuid.txt is in the same ProgramData directory and is similarly
;      left untouched.
;
; ProgramData directory:
;   The binary defaults to %ProgramData%\SQMeter SafetyMonitor\ for config
;   and the device UUID. The installer creates this directory on fresh install
;   and writes a default config.json only when one does not already exist.
;   On upgrade the existing config is always preserved.
;
; Uninstall behaviour:
;   The service is stopped and unregistered. The binary and install-directory
;   files are removed. The ProgramData directory (config, UUID, logs) is NOT
;   removed, so user settings survive uninstall. Delete
;   %ProgramData%\SQMeter SafetyMonitor\ manually if a clean removal is wanted.
;
; Automatic update checking is not implemented. Users upgrade by downloading
; the new installer from GitHub Releases and running it over the existing
; installation.

#ifndef AppVersion
  #define AppVersion "dev"
#endif

#define AppName      "SQMeter Alpaca SafetyMonitor"
#define AppPublisher "DeanJ87"
#define AppURL       "https://github.com/DeanJ87/SQMeter-Safety-Monitor"
#define ServiceName  "SQMeterAlpacaSafetyMonitor"
#define ExeName      "sqmeter-alpaca-safetymonitor.exe"
#define SetupBase    "sqmeter-alpaca-safetymonitor-setup"
; AppDataDir must match config.AppDataDirName in internal/config/paths.go.
#define AppDataDir   "SQMeter SafetyMonitor"

[Setup]
AppId={{E3A7C2B1-4F8D-4E2A-9C3B-1D5F7A8E0B2C}
AppName={#AppName}
AppVersion={#AppVersion}
AppPublisher={#AppPublisher}
AppPublisherURL={#AppURL}
AppSupportURL={#AppURL}/issues
AppUpdatesURL={#AppURL}/releases
DefaultDirName={autopf}\{#AppName}
DefaultGroupName={#AppName}
DisableProgramGroupPage=yes
PrivilegesRequired=admin
OutputDir=Output
OutputBaseFilename={#SetupBase}-{#AppVersion}
Compression=lzma
SolidCompression=yes
WizardStyle=modern
; Require Windows 10 or later (needed for modern TLS and IPv6 UDP)
MinVersion=10.0

[Languages]
Name: "english"; MessagesFile: "compiler:Default.isl"

[Dirs]
; Create the ProgramData directory that the binary uses for config and the
; device UUID. The directory is not removed on uninstall (preserves user data).
Name: "{commonappdata}\{#AppDataDir}"; Flags: uninsneveruninstall

[Files]
; Main binary - placed in the install directory.
; BeforeInstall stops and uninstalls any existing service so the binary is
; not locked when the installer tries to replace it (Windows locks running
; executables). On a fresh install the old binary does not exist and the
; stop/uninstall calls fail silently, which is harmless.
Source: "bin\{#ExeName}"; DestDir: "{app}"; Flags: ignoreversion; \
  BeforeInstall: StopExistingService

[Run]
; Write a default config.json only when no config exists yet (fresh install).
; On upgrade the existing config in ProgramData is left untouched.
Filename: "{app}\{#ExeName}"; \
  Parameters: "--write-default-config"; \
  Flags: runhidden waituntilterminated; \
  StatusMsg: "Writing default configuration..."; \
  Check: not FileExists(ExpandConstant('{commonappdata}\{#AppDataDir}\config.json'))

; Re-install the Windows service against the new binary path.
; On a fresh install this registers the service for the first time.
; On an upgrade the old service was uninstalled by BeforeInstall, so this
; is always a clean registration.
Filename: "{app}\{#ExeName}"; Parameters: "--service install"; \
  Flags: runhidden waituntilterminated; \
  StatusMsg: "Installing service..."

; Start the service so it is immediately available after install or upgrade.
Filename: "{app}\{#ExeName}"; Parameters: "--service start"; \
  Flags: runhidden waituntilterminated; \
  StatusMsg: "Starting service..."

; Open the configuration page in the default browser (skipped in silent installs)
Filename: "http://localhost:11111/setup"; \
  Flags: shellexec nowait; \
  Description: "Open configuration page in browser"; \
  Check: not WizardSilent

[UninstallRun]
; Stop then uninstall the service before removing files
Filename: "{app}\{#ExeName}"; Parameters: "--service stop"; \
  Flags: runhidden waituntilterminated
Filename: "{app}\{#ExeName}"; Parameters: "--service uninstall"; \
  Flags: runhidden waituntilterminated

[Icons]
Name: "{group}\Configuration"; \
  Filename: "{app}\{#ExeName}"; \
  Parameters: ""; \
  Comment: "Open SQMeter SafetyMonitor configuration (opens browser)"
Name: "{group}\Uninstall {#AppName}"; Filename: "{uninstallexe}"

[Code]
// StopExistingService is called by BeforeInstall on the main binary file entry.
// It stops and uninstalls the running service so the installer can replace the
// locked executable. Both calls are best-effort: a non-zero exit code (e.g.
// service not found on a fresh install) is intentionally ignored.
procedure StopExistingService;
var ResultCode: Integer;
begin
  Exec(ExpandConstant('{app}\{#ExeName}'), '--service stop', '', SW_HIDE,
       ewWaitUntilTerminated, ResultCode);
  Exec(ExpandConstant('{app}\{#ExeName}'), '--service uninstall', '', SW_HIDE,
       ewWaitUntilTerminated, ResultCode);
end;
