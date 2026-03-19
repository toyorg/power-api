# 3D Printer Power Control API

A Go-based REST API for controlling a 3D printer's power state through MQTT and SSH. This API provides endpoints to check and control the power state of a 3D printer, including safe shutdown procedures that consider extruder temperature.

## Features

- Get current printer power state
- Control printer power (ON/OFF)
- Safe shutdown procedure with temperature monitoring
- MQTT integration with Zigbee2MQTT
- SSH-based shutdown commands
- Automatic MQTT reconnection handling

## Prerequisites

- Go 1.x
- MQTT broker (e.g., Mosquitto)
- SSH access to the printer host
- Zigbee2MQTT setup
- Moonraker instance running

## Environment Variables

The following environment variables need to be set:

```env
mqtt_host=<MQTT broker hostname>
mqtt_user=<MQTT username>
mqtt_pass=<MQTT password>
ssh_host=<SSH host address>
ssh_user=<SSH username>
ssh_pass=<SSH password>
moonraker_url=<Moonraker API URL>
```

## Installation

1. Clone the repository
2. Install dependencies:
```bash
go mod download
```
3. Build the application:
```bash
go build -o power-api
```

## Usage

### Running the Server

```bash
./power-api
```

The server will start on port 8000.

### API Endpoints

#### GET /api/3d-printer
Returns the current power state of the 3D printer.

Response:
```json
{
    "state": "ON"
}
```

#### POST /api/3d-printer
Controls the power state of the 3D printer.

Request body:
```json
{
    "state": "ON"
}
```

Supported states:
- `ON`: Powers on the printer
- `OFF`: Initiates safe shutdown procedure

## Safe Shutdown Procedure

When powering off the printer, the API:
1. Checks if the extruder temperature is below 50°C
2. Executes shutdown command via SSH
3. Monitors host availability
4. Turns off the power relay

## Dependencies

- github.com/eclipse/paho.mqtt.golang
- github.com/gin-gonic/gin
- golang.org/x/crypto/ssh

## Error Handling

- MQTT connection failures with automatic reconnection
- SSH command execution errors
- API request timeouts
- Invalid state requests
