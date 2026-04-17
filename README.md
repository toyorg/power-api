# 3D Printer Power Control API

REST API in Go for controlling a 3D printer power relay through MQTT, with a safe power-off flow coordinated with Moonraker and SSH.

## Current behavior (from code)

- HTTP server listens on `:8000`
- Endpoints:
    - `GET /api/3d-printer`
    - `POST /api/3d-printer`
- MQTT topics are currently hardcoded:
    - read state from `zigbee2mqtt/R`
    - publish commands to `zigbee2mqtt/R/set`
- `POST OFF` is a **blocking** flow (waits until print is done and hotend cools down)

## Prerequisites

- Go `1.25+` (module is set to `go 1.25.0`)
- Reachable MQTT broker on port `1883`
- SSH access to printer host (password auth)
- Moonraker API reachable by this service

## Environment variables

The app loads variables from:

1. `.env` in project root
2. `/root/power-api/.env` (useful for deployments)
3. process environment

Use lowercase keys exactly as below:

```env
mqtt_host=
mqtt_user=
mqtt_pass=
ssh_host=
ssh_user=
ssh_pass=
moonraker_url=
threshold_temp=
```

Notes:

- `threshold_temp` defaults to `49` when missing/invalid
- no strict startup validation is done for empty values, so wrong/missing config will usually fail at runtime (MQTT/HTTP/SSH operations)

## Local run

1. Prepare env file:

```bash
cp template.env .env
```

2. Fill `.env` with your values.

3. Run:

```bash
go run .
```

The service starts on `http://localhost:8000`.

## Build

```bash
go build -o power-api .
```

## Tests

Tests are organized in the dedicated `tests/` folder and split by source responsibility:

- `tests/config_test.go`
- `tests/moonraker_test.go`
- `tests/mqtt_test.go`
- `tests/handlers_test.go`
- `tests/shutdown_test.go`
- `tests/helpers_test.go`

Run all tests:

```bash
go test ./...
```

## Code coverage

Generate and print coverage summary:

```bash
make coverage
```

This runs tests from `./tests` and measures coverage for `./src/...`.

Generate HTML coverage report:

```bash
make coverage-html
```

This creates:

- `coverage.out` (profile)
- `coverage.html` (visual report)

## API

### `GET /api/3d-printer`

Returns current relay state from MQTT.

Example response:

```json
{
    "state": "ON"
}
```

### `POST /api/3d-printer`

Request body:

```json
{
    "state": "ON"
}
```

Allowed values:

- `ON` – immediately publishes MQTT `ON`
- `OFF` – starts safe shutdown flow:
    1. poll Moonraker print state (`standby` or `complete` required)
    2. poll Moonraker temperature store and compute average of recent extruder readings
    3. wait until average temp is below `threshold_temp`
    4. execute SSH command `/sbin/shutdown 0`
    5. wait until host no longer responds to ping
    6. publish MQTT `OFF`

## Moonraker endpoints used

- `{moonraker_url}/printer/objects/query?print_stats`
- `{moonraker_url}/server/temperature_store`

## Docker image (current `Dockerfile`)

- multi-stage build
- final image is `scratch`
- binary entrypoint is `/power-api`

If you run in a container, pass env vars explicitly (or mount a file at `/root/power-api/.env` if your runtime allows it).

## Operational notes

- MQTT client uses auto-reconnect
- API request for `POST OFF` can be long-running due to cooldown and host shutdown waiting
- app runs Gin in `release` mode
