# K6 Load Tester for Key-Value Server

A Python project that runs k6 load tests against the Go key-value server simultaneously with GET, POST, and DELETE operations, collects metrics via k6 REST API, and generates a CLI chart using matplotlib to find the maximum throughput.

## Prerequisites

- Python 3.7+
- [Go](https://go.dev/doc/install) (to run the key-value server)
- [k6](https://k6.io/docs/getting-started/installation/) (for load testing)
- pip (Python package manager)

## Quick Start

### 1. Set up the virtual environment

Run the setup script:

```bash
python3 setup_venv.py
```

This creates:
- A virtual environment in `venv/`
- Installs required Python packages
- Creates a convenience `activate` script

### 2. Activate and run

Using the activate script:

```bash
./activate --min-vus 10 --max-vus 100 --step 10
```

Or manually:

```bash
# On Linux/Mac
source venv/bin/activate
python3 loadtester.py --min-vus 10 --max-vus 100 --step 10

# On Windows
venv\Scripts\activate
python loadtester.py --min-vus 10 --max-vus 100 --step 10
```

## Usage

```
python loadtester.py [options]
```

### Options

| Option | Description | Default |
|--------|-------------|---------|
| `--min-vus` | Minimum number of virtual users | 10 |
| `--max-vus` | Maximum number of virtual users | 100 |
| `--step` | Step size for VU increment | 10 |
| `--server-addr` | Address of the key-value server | localhost:8080 |
| `--linger` | Keep k6 API server alive past test end | True |
| `--k6-address` | Base address for k6 REST API servers (each test gets consecutive ports: +0, +1, +2) | localhost:6565 |
| `--duration` | Duration of each test | 30s |
| `--output` | Output filename for the chart | loadtest_results.png |

### Examples

Run a quick test:
```bash
./activate --min-vus 5 --max-vus 50 --step 5 --duration 15s
```

Run with custom server address:
```bash
./activate --server-addr localhost:8081 --min-vus 20 --max-vus 200
```

Run with custom k6 API address:
```bash
./activate --k6-address localhost:7565
```

Run with shorter tests:
```bash
./activate --duration 10s --step 5
```

## How It Works

1. **Clean data file**: Before each iteration, removes `/var/tmp/data.json` to avoid contamination from previous runs
2. **Starts the Go server**: The script starts a fresh key-value server for each iteration using `go run . -release`
3. **Runs simultaneous tests**: All three k6 test scripts (GET, POST, DELETE) are run simultaneously, each with its own REST API port (consecutive: base_port+0, base_port+1, base_port+2)
4. **Waits for duration**: After starting the tests, the script waits for the full `--duration` time
5. **Pulls metrics**: Uses k6 REST API (`--linger` and `--address` flags) to fetch metrics from each k6 instance after the duration completes
6. **Calculates throughput**: Throughput is extracted from the `http_reqs` metric's `rate` field from the k6 `/v1/metrics` endpoint
7. **Terminates processes**: Sends SIGTERM to all k6 processes and stops the Go server after each iteration
8. **Generates chart**: Uses matplotlib to create a visualization of throughput vs VUs
9. **Finds maximum**: Identifies the VU count that produces maximum requests per second

## Output

The script generates:
- A PNG chart file (default: `loadtest_results.png`)
- Text summary with maximum throughput and corresponding VU count
- Detailed results table for all test iterations

## K6 Test Scripts

The script uses the following test scripts from the `testdata/` directory:
- `k6_get_loadtest.js` - Tests GET operations
- `k6_post_loadtest.js` - Tests POST operations  
- `k6_delete_loadtest.js` - Tests DELETE operations

## Configuration

All configuration is done via command line arguments. The script handles:
- Starting and stopping the Go server
- Running k6 with the correct flags
- Collecting and parsing metrics
- Generating the visualization

## Requirements

Python packages (automatically installed by setup_venv.py):
- matplotlib>=3.5.0 (for chart generation)

System dependencies:
- Go (to build and run the key-value server)
- k6 (for load testing)

## Notes

- The `--linger` flag keeps the k6 API server alive so metrics can be collected after tests complete
- The `--address` flag specifies the base address where the k6 REST API listens (default: localhost:6565)
- Each k6 instance uses consecutive ports (base_port+0, base_port+1, base_port+2) to avoid conflicts
- Each test iteration runs all three operations (GET, POST, DELETE) simultaneously
- The chart shows throughput (requests per second) for each operation type
- The maximum throughput is highlighted on the chart
- Throughput is extracted from the `http_reqs` metric's `rate` field from the k6 `/v1/metrics` endpoint, which is the actual requests per second
- After metrics collection, SIGTERM is sent to all k6 processes to terminate them gracefully

## Troubleshooting

If you get errors about missing dependencies:
- Ensure Go and k6 are installed and in your PATH
- Run `python3 setup_venv.py` to create the virtual environment and install Python packages
- Check that the k6 test scripts exist in `testdata/`

If the server fails to start:
- Check that the Go server code can be built with `go build`
- Verify the PORT environment variable is being set correctly
