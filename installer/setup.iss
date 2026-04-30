; SQMeter Alpaca SafetyMonitor - Inno Setup installer script
; Build with: ISCC.exe /DAppVersion=1.2.3 setup.iss

#ifndef AppVersion
  #define AppVersion "dev"
#endif

#define AppName      "SQMeter Alpaca SafetyMonitor"
#define AppPublisher "DeanJ87"
#define AppURL       "https://github.com/DeanJ87/SQMeter-Safety-Monitor"
#define ServiceName  "SQMeterAlpacaSafetyMonitor"
#define ExeName      "sqmeter-alpaca-safetymonitor.exe"
#define SetupBase    "sqmeter-alpaca-safetymonitor-setup"

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

[Files]
; Main binary - placed in the install directory
Source: "bin\{#ExeName}"; DestDir: "{app}"; Flags: ignoreversion

[Run]
; Install the Windows service (registered against the installed exe path)
Filename: "{app}\{#ExeName}"; Parameters: "--service install"; \
  Flags: runhidden waituntilterminated; \
  StatusMsg: "Installing service..."

; Start the service so it is immediately available
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
