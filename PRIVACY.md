# Privacy Policy

SQMeter ASCOM Alpaca is a local bridge between an SQMeter ESP32 device and ASCOM Alpaca clients on your network.

## Data Collection

The application does not collect analytics, telemetry, personal data, or usage data.

It reads sensor values from the SQMeter device URL that you configure and exposes SafetyMonitor and ObservingConditions data through the local Alpaca HTTP API and web interface.

## Network Communication

The application communicates with:

- The configured SQMeter device on your local network.
- ASCOM Alpaca clients that connect to the HTTP API.
- Local network clients using Alpaca discovery, when discovery is enabled.

The application does not send data to a project-operated server.

## Local Files

The application may store local configuration and a generated device UUID file. These files remain on the machine where the application runs unless you copy or back them up yourself.

## Updates

The application does not automatically upload data when checking for or installing updates. Release artifacts are distributed through the project repository.

## Contact

Use the project issue tracker for privacy questions or concerns.
