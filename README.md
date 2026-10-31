# key-value-server-go

A simple, concurrent key-value HTTP server with persistent JSON file storage.

## Features

- **Concurrent-safe operations** using `github.com/orcaman/concurrent-map/v2`
- **Automatic persistence** to disk at configurable intervals
- **Base64 encoding** for safe storage of arbitrary binary data in JSON format
- **Atomic writes** using temp file + rename pattern
- **Configurable** via environment variables
- **RESTful API** with standard HTTP methods

## Pre-requisites

You'll need to install:

- [Go](https://go.dev/doc/install) (1.20+ recommended)
- [K6](https://k6.io/docs/getting-started/installation/) (optional, for load testing)

## Dependencies

The project uses the following Go modules:

| Package | Version | Purpose |
|---------|---------|---------|
| [github.com/go-chi/chi/v5](https://github.com/go-chi/chi) | v5.5.0 | Lightweight HTTP router |
| [github.com/orcaman/concurrent-map/v2](https://github.com/orcaman/concurrent-map) | v2.0.1 | Thread-safe concurrent map |

To install dependencies:
```bash
go mod tidy
```

## Quick Start

### Using `go run`

```bash
go run .
```

### Using compiled binary

```bash
go build -o key-value-server
./key-value-server
```

The server will start on `http://0.0.0.0:8080` by default.

## Configuration

The server is configured via environment variables:

| Variable | Description | Default | Required |
|----------|-------------|---------|----------|
| `PORT` | HTTP server port | `8080` | No |
| `ADDRESS` | Server bind address/hostname | `""` (all interfaces) | No |
| `STORAGE_DIR` | Directory for persistent data storage | `/var/tmp/` | No |
| `SAVE_INTERVAL` | Interval in seconds between automatic saves | `60` | No |

### Examples

Start server on port 3000:
```bash
PORT=3000 go run .
```

Start server on specific address:
```bash
ADDRESS=127.0.0.1 PORT=8080 go run .
```

Use custom storage directory:
```bash
STORAGE_DIR=/my/data/dir go run .
```

Save every 30 seconds:
```bash
SAVE_INTERVAL=30 go run .
```

## API Reference

### Endpoints

All endpoints are under the `/key/{key}` path.

#### GET `/key/{key}`

Retrieve the value for a given key.

**Request:**
```bash
curl -s http://localhost:8080/key/mykey
```

**Response - Success (200 OK):**
```json
"my value"
```

**Response - Not Found (404):**
```
(no body, HTTP 404)
```

#### POST `/key/{key}`

Set or update a value for a key. The request body is used as the value.

**Request:**
```bash
curl -s -X POST http://localhost:8080/key/mykey -d "my value"
```

**Response - Success (200 OK):**
```json
{"status": "success"}
```

**Response - Error (500 Internal Server Error):**
```json
{"error": "error message"}
```

#### DELETE `/key/{key}`

Remove a key-value pair.

**Request:**
```bash
curl -X DELETE -s http://localhost:8080/key/mykey
```

**Response - Success (200 OK):**
```json
{"status": "success"}
```

**Note:** DELETE is idempotent - it returns success even if the key doesn't exist.

### Response Formats

| Method | Success Status | Success Body | Error Status | Error Body |
|--------|---------------|--------------|--------------|------------|
| GET | 200 OK | `"value"` (string) | 404 Not Found | (empty) |
| POST | 200 OK | `{"status": "success"}` | 500 Internal Server Error | `{"error": "message"}` |
| DELETE | 200 OK | `{"status": "success"}` | - | - |

## Persistence

Data is automatically:
- **Loaded** from `STORAGE_DIR/data.json` on startup
- **Saved** to disk every `SAVE_INTERVAL` seconds

### Data Format

The persistence file contains base64-encoded key-value pairs in JSON format:
```json
{
  "base64EncodedKey1": "base64EncodedValue1",
  "base64EncodedKey2": "base64EncodedValue2"
}
```

Base64 URL encoding is used (with `-` and `_` instead of `+` and `/`), which is safe for use in URLs and JSON.

### Atomic Writes

File writes use an atomic pattern:
1. Write to a temporary file (`data.json.*.tmp`)
2. Sync to disk (fsync)
3. Rename temp file to `data.json`

This ensures readers never see a partially written file.

### File Permissions

- New directories are created with `0755` permissions
- New files default to `0644` permissions
- Existing file permissions are preserved on save

## Examples

Send a key-value pair:
```bash
curl -s -X POST http://localhost:8080/key/1 -d "1"
```

Retrieve the value for key `1`:
```bash
curl -s http://localhost:8080/key/1
```

Delete the value for key `1`:
```bash
curl -X DELETE -s http://localhost:8080/key/1
```

Store JSON data:
```bash
curl -s -X POST http://localhost:8080/key/user -d '{"name":"John","age":30}'
```

Store binary data:
```bash
echo "binary data" | curl -s -X POST http://localhost:8080/key/bin --data-binary @-
```

## Building

### Build the binary

```bash
go build -o key-value-server .
```

### Build with release optimizations

```bash
go build -ldflags="-s -w" -o key-value-server .
```

## Running Tests

### Run all tests

```bash
go test ./...
```

### Run tests with verbose output

```bash
go test -v ./...
```

### Run tests with race detector

```bash
go test -race ./...
```

### Run benchmarks

```bash
go test -bench=. -benchmem ./...
```

## Load Testing

Load tests are provided using [K6](https://k6.io/). Test scripts are located in the `testdata/` directory.

### Run load tests

```bash
# GET requests (read-heavy)
k6 run --vus 200 --duration 60s ./testdata/k6_get_loadtest.js

# POST requests (write-heavy)
k6 run --vus 200 --duration 60s ./testdata/k6_post_loadtest.js

# DELETE requests
k6 run --vus 200 --duration 60s ./testdata/k6_delete_loadtest.js

# Mixed workload
k6 run --vus 200 --duration 60s ./testdata/k6_mixed_loadtest.js
```

### K6 Options

| Option | Description | Default |
|--------|-------------|---------|
| `--vus N` | Virtual users (concurrent connections) | 10 |
| `--duration D` | Test duration | 10s |
| `--rps N` | Requests per second (alternative to VUs) | - |
| `--out O` | Output format (json, influxdb, etc.) | - |

Example with custom configuration:
```bash
k6 run --vus 500 --rps 1000 --duration 5m ./testdata/k6_get_loadtest.js
```

## Architecture

### Components

- **HTTP Server**: Built with [chi router](https://github.com/go-chi/chi) for flexible routing
- **Data Store**: In-memory concurrent map (`github.com/orcaman/concurrent-map/v2`)
- **Persistence Layer**: JSON file storage with base64 encoding for binary safety
- **Scheduler**: Background goroutine for periodic data saves

### Concurrency Model

- All operations are thread-safe using concurrent data structures
- Data is stored in two synchronized maps:
  - `data`: Primary key-value store (raw strings)
  - `base64Data`: Persistence copy (base64-encoded strings)
- Load uses concurrent goroutines for decoding
- Save runs in a background goroutine

### Performance Characteristics

- **Memory**: Data stored in memory for fast access
- **Disk I/O**: Minimal - only on scheduled saves
- **Concurrency**: High - designed for concurrent read/write operations
- **Latency**: Low - in-memory operations with async persistence

## License

This project is open source. Feel free to use, modify, and distribute as needed.

## Contributing

Contributions are welcome! Please feel free to submit issues or pull requests.
