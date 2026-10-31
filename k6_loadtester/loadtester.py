#!/usr/bin/env python3
"""
K6 Load Tester for Key-Value Server

Runs k6 load tests with configurable VU ranges, collects metrics via k6 REST API,
and generates a CLI chart using matplotlib to find the maximum throughput.

Usage:
    python loadtester.py [--min-vus MIN] [--max-vus MAX] [--step STEP] 
                        [--server-addr ADDRESS] [--linger] 
                        [--k6-address K6_ADDR] [--duration DURATION]

Example:
    python loadtester.py --min-vus 10 --max-vus 100 --step 10 --server-addr localhost:8080
"""

import argparse
import json
import os
import subprocess
import sys
import time
import threading
from pathlib import Path
from typing import List, Dict, Optional


# Project structure
PROJECT_ROOT = Path(__file__).parent.parent.resolve()
TESTDATA_DIR = PROJECT_ROOT / "testdata"
K6_SCRIPTS = {
    "get": TESTDATA_DIR / "k6_get_loadtest.js",
    "post": TESTDATA_DIR / "k6_post_loadtest.js",
    "delete": TESTDATA_DIR / "k6_delete_loadtest.js",
}


def parse_args():
    """Parse command line arguments."""
    parser = argparse.ArgumentParser(
        description="K6 Load Tester for Key-Value Server - Runs simultaneous GET/POST/DELETE tests"
    )
    
    parser.add_argument(
        "--min-vus", 
        type=int, 
        default=10,
        help="Minimum number of virtual users (default: 10)"
    )
    
    parser.add_argument(
        "--max-vus", 
        type=int, 
        default=100,
        help="Maximum number of virtual users (default: 100)"
    )
    
    parser.add_argument(
        "--step", 
        type=int, 
        default=10,
        help="Step size for VU increment (default: 10)"
    )
    
    parser.add_argument(
        "--server-addr", 
        type=str, 
        default="localhost:8080",
        help="Address of the key-value server (default: localhost:8080)"
    )
    
    parser.add_argument(
        "--linger", 
        action="store_true",
        default=True,
        help="Keep k6 API server alive past test end (default: True)"
    )
    
    parser.add_argument(
        "--k6-address", 
        type=str, 
        default="localhost:6565",
        help="Base address for k6 REST API servers. Each k6 instance uses consecutive ports: base+0, base+1, base+2 (default: localhost:6565)"
    )
    
    parser.add_argument(
        "--duration", 
        type=str, 
        default="30s",
        help="Duration of each test (default: 30s)"
    )
    
    parser.add_argument(
        "--output", 
        type=str, 
        default="loadtest_results.png",
        help="Output filename for the chart (default: loadtest_results.png)"
    )

    parser.add_argument(
        "--data-file",
        type=str,
        default="data.json",
        help="Persistent key value data storage in json. It will be removed to avoid adding startup data load workload to measurement (default: \"data.json\")"
    )

    return parser.parse_args()


def check_k6_scripts():
    """Verify that all k6 test scripts exist."""
    missing = []
    for name, path in K6_SCRIPTS.items():
        if not path.exists():
            missing.append(name)
    
    if missing:
        print(f"Error: Missing k6 test scripts: {', '.join(missing)}")
        print(f"Expected to find them in: {TESTDATA_DIR}")
        sys.exit(1)
    
    print(f"Found k6 scripts: {', '.join(K6_SCRIPTS.keys())}")


def start_go_server(server_addr: str, data_file: str) -> subprocess.Popen:
    """Start the Go key-value server."""
    print(f"Starting Go server on {server_addr}...")
    
    # Set PORT environment variable
    port = server_addr.split(":")[-1]
    env = dict(os.environ)
    env["PORT"] = port
    env["STORAGE_DIR"] = Path(data_file).parent.resolve()
    
    cmd = ["go", "run", "."]
    
    proc = subprocess.Popen(
        cmd,
        cwd=PROJECT_ROOT,
        env=env,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True
    )
    
    # Wait for server to start
    time.sleep(2)
    
    # Check if server is running
    try:
        import urllib.request
        urllib.request.urlopen(f"http://{server_addr}/key/test", timeout=5)
        print(f"Go server is running at http://{server_addr}")
    except Exception as e:
        print(f"Warning: Could not verify server startup: {e}")
    
    return proc


def stop_go_server(go_server_proc: subprocess.Popen) -> None:
    """Stop the Go server process gracefully."""
    print("  Stopping Go server...")
    try:
        go_server_proc.terminate()
        go_server_proc.wait(timeout=5)
        print("  Go server stopped")
    except Exception as e:
        print(f"  Warning: Could not stop Go server gracefully: {e}")
        try:
            go_server_proc.kill()
            go_server_proc.wait()
            print("  Go server killed")
        except:
            pass


def cleanup_data_file(data_file: str) -> None:
    """Remove the data.json file to avoid contamination between iterations."""
    data_file = Path(data_file).resolve()
    if data_file.exists():
        try:
            data_file.unlink()
            print(f"  Removed {data_file}")
        except Exception as e:
            print(f"  Warning: Could not remove {data_file}: {e}")


def get_k6_api_url(k6_address: str) -> str:
    """Get the full k6 API URL."""
    return f"http://{k6_address}/v1/metrics"


def fetch_k6_metrics(k6_address: str) -> Optional[Dict]:
    """Fetch metrics from k6 REST API."""
    import urllib.request
    import urllib.error
    
    api_url = get_k6_api_url(k6_address)
    
    try:
        with urllib.request.urlopen(api_url, timeout=5) as response:
            data = json.loads(response.read().decode('utf-8'))
            return data
    except (urllib.error.URLError, json.JSONDecodeError) as e:
        print(f"Warning: Could not fetch k6 metrics: {e}")
        return None


def run_k6_test(
    script_path: Path,
    vus: int,
    duration: str,
    k6_address: str,
    linger: bool
) -> subprocess.Popen:
    """Run a single k6 test with the given parameters."""
    cmd = [
        "k6", "run",
        "--vus", str(vus),
        "--duration", duration,
        "--summary-mode",
        "disabled"
    ]
    
    if linger:
        cmd.extend(["--linger"])
    
    if k6_address:
        cmd.extend(["--address", k6_address])
    
    cmd.append(str(script_path))
    
    print(f"  Running k6 test: {' '.join(cmd)}")
    
    proc = subprocess.Popen(
        cmd,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True
    )
    
    return proc


def run_simultaneous_tests(
    vus: int,
    duration: str,
    k6_base_address: str,
    linger: bool
) -> Dict[str, subprocess.Popen]:
    """Run all three k6 tests simultaneously with consecutive ports."""
    processes = {}
    
    # Parse base address to extract host and port
    # k6_base_address format: hostname:port
    parts = k6_base_address.rsplit(":", 1)
    if len(parts) == 2:
        host = parts[0]
        base_port = int(parts[1])
    else:
        host = k6_base_address
        base_port = 6565  # Default k6 port
    
    # Assign consecutive ports to each k6 instance
    for idx, (name, script_path) in enumerate(K6_SCRIPTS.items()):
        port = base_port + idx
        k6_address = f"{host}:{port}"
        proc = run_k6_test(script_path, vus, duration, k6_address, linger)
        processes[name] = proc
    
    return processes


def wait_for_duration(duration_seconds: int) -> None:
    """Wait for the specified duration in seconds."""
    # print(f"  Waiting for {duration_seconds} seconds for tests to complete...")
    time.sleep(duration_seconds)


def terminate_k6_processes(processes: Dict[str, subprocess.Popen]) -> None:
    """Send SIGTERM to all k6 processes to gracefully terminate them."""
    # print("  Sending SIGTERM to k6 processes...")
    for name, proc in processes.items():
        try:
            proc.kill()
            # print(f"    SIGTERM sent to {name}")
        except Exception as e:
            print(f"    Warning: Could not terminate {name}: {e}")
    
    # Wait for processes to exit
    print("  Waiting for k6 processes to exit...")
    for name, proc in processes.items():
        try:
            proc.wait(timeout=3)
            # print(f"    {name} exited with code {proc.returncode}")
        except subprocess.TimeoutExpired:
            print(f"    Warning: {name} did not exit, sending SIGKILL...")
            proc.kill()
            proc.wait()
            # print(f"    {name} killed")


def get_throughput_from_metrics(metrics: Optional[Dict]) -> Optional[float]:
    """Extract throughput (requests per second) from k6 metrics.
    
    Uses the 'rate' field from the 'http_reqs' metric which is the actual
    requests per second that the server handled.
    
    Metrics format from /v1/metrics:
    {"data": [{"type":"metrics","id":"http_reqs","attributes":{"sample":{"rate": X}}}]}
    """
    if not metrics:
        return None
    
    try:
        # Parse the /v1/metrics response format: {"data": [...]}
        if "data" in metrics:
            for metric in metrics["data"]:
                if metric.get("id") == "http_reqs":
                    print(f'{metric}')
                    attributes = metric.get("attributes", {})
                    sample = attributes.get("sample", {})
                    rate = float(sample.get("rate", 0.0))
                    return rate
        
        # Fallback: try old format {"metrics": {"http_reqs": {"values": {"rate": X}}}}
        if "metrics" in metrics:
            http_reqs = metrics["metrics"].get("http_reqs", {})
            values = http_reqs.get("values", {})
            rate = values.get("rate", 0)
            return float(rate)
            
    except (KeyError, TypeError, ValueError):
        return None
    
    return None


def collect_metrics_during_test(
    k6_address: str,
    duration_seconds: int,
    stop_event: threading.Event
) -> List[Optional[Dict]]:
    """Collect metrics from k6 API during test execution."""
    metrics_list = []
    
    start_time = time.time()
    
    while not stop_event.is_set():
        elapsed = time.time() - start_time
        
        if elapsed > duration_seconds:
            break
        
        metrics = fetch_k6_metrics(k6_address)
        metrics_list.append(metrics)
        
        time.sleep(1)
    
    return metrics_list


def run_single_test_iteration(
    vus: int,
    duration: str,
    k6_base_address: str,
    linger: bool,
    run_number: int = 1
) -> Dict[str, Optional[float]]:
    """Run a single test iteration and collect metrics from each k6 instance.
    
    Flow:
    1. Start all k6 tests simultaneously
    2. Wait for the specified duration
    3. Pull metrics from each k6 instance's REST API
    4. Send SIGTERM to terminate all k6 processes
    """
    print(f"  Run {run_number}: Testing with {vus} VUs")
    
    # Parse base address to extract host and port
    parts = k6_base_address.rsplit(":", 1)
    if len(parts) == 2:
        host = parts[0]
        base_port = int(parts[1])
    else:
        host = k6_base_address
        base_port = 6565
    
    # Parse duration to seconds
    duration_seconds = parse_duration(duration)
    
    # Start all tests with consecutive ports
    print("    Starting k6 test processes...")
    processes = run_simultaneous_tests(vus, duration, k6_base_address, linger)
    
    # Build list of all k6 API addresses
    k6_api_addresses = {}
    for idx, name in enumerate(K6_SCRIPTS.keys()):
        port = base_port + idx
        k6_api_addresses[name] = f"{host}:{port}"
    
    # Wait for the full duration
    wait_for_duration(duration_seconds)
    
    # After duration, pull metrics from each k6 instance
    print("    Pulling metrics from k6 instances...")
    results = {}
    for name, api_addr in k6_api_addresses.items():
        final_metrics = fetch_k6_metrics(api_addr)
        if final_metrics:
            # Get throughput from http_reqs rate
            throughput = get_throughput_from_metrics(final_metrics)
            results[name] = throughput
            
            if throughput is not None:
                print(f"      {name}: {throughput:.2f} req/s")
            else:
                print(f"      {name}: metrics not available")
        else:
            results[name] = None
            print(f"      {name}: could not fetch metrics")
    
    # Terminate all k6 processes
    terminate_k6_processes(processes)
    
    # Also calculate combined throughput
    combined = sum([r for r in results.values() if r is not None])
    results["combined"] = combined if combined > 0 else None
    
    if combined is not None:
        print(f"    Combined throughput: {combined:.2f} req/s")
    
    return results


def run_test_iteration(
    vus: int,
    duration: str,
    k6_base_address: str,
    linger: bool,
    server_addr: str = "localhost:8080",
    data_file: str = "data.json"
) -> Dict[str, Optional[float]]:
    """Run multiple test iterations (4 runs) and return average of top 2 results.
    
    For each VU count:
    1. Run 4 separate test iterations
    2. Collect all results
    3. For each test type (get, post, delete, combined), sort results and average top 2
    4. Print detailed results for each run
    """
    print(f"\n=== ITERATION: {vus} VUs ===")
    
    NUM_RUNS = 4
    all_results = {name: [] for name in K6_SCRIPTS.keys()}
    all_results["combined"] = []
    
    for run_num in range(1, NUM_RUNS + 1):
        # Clean up data file before each run
        cleanup_data_file(data_file)
        
        # Get a fresh Go server for each run
        go_server = start_go_server(server_addr, data_file)
        
        try:
            single_results = run_single_test_iteration(
                vus, duration, k6_base_address, linger, run_num
            )
            
            # Store results for each test type
            for name in all_results.keys():
                all_results[name].append(single_results.get(name))
        finally:
            stop_go_server(go_server)
            time.sleep(1)
    
    # Calculate average of top 2 results for each test type
    averaged_results = {}
    print(f"\n  Results for {vus} VUs (4 runs, averaging top 2):")
    
    for name in all_results.keys():
        # Filter out None values
        valid_results = [r for r in all_results[name] if r is not None]
        
        if len(valid_results) >= 2:
            # Sort descending and take top 2
            sorted_results = sorted(valid_results, reverse=True)
            top2 = sorted_results[:2]
            average = sum(top2) / len(top2)
            averaged_results[name] = average
            
            # Print all runs and the averaged result
            print(f"    {name.upper()}: runs=[{', '.join(f'{r:.2f}' for r in all_results[name] if r is not None)}], top2=[{', '.join(f'{r:.2f}' for r in top2)}], avg_top2={average:.2f} req/s")
        elif len(valid_results) == 1:
            averaged_results[name] = valid_results[0]
            print(f"    {name.upper()}: runs=[{valid_results[0]:.2f}], only 1 valid result")
        else:
            averaged_results[name] = None
            print(f"    {name.upper()}: no valid results")
    
    return averaged_results


def parse_duration(duration_str: str) -> int:
    """Parse duration string (e.g., '30s', '1m', '2h') to seconds."""
    import re
    
    match = re.match(r'^(\d+)([smh]?)$', duration_str.lower())
    if not match:
        print(f"Warning: Could not parse duration '{duration_str}', defaulting to 30 seconds")
        return 30
    
    value = int(match.group(1))
    unit = match.group(2)
    
    multipliers = {
        's': 1,
        'm': 60,
        'h': 3600,
        '': 1
    }
    
    return value * multipliers.get(unit, 1)


def run_load_test_series(
    min_vus: int,
    max_vus: int,
    step: int,
    server_addr: str,
    k6_address: str,
    duration: str,
    linger: bool,
    data_file: str
) -> Dict[int, Dict[str, Optional[float]]]:
    """Run a series of load tests with increasing VU counts.
    
    For each VU iteration:
    1. Run 4 separate test iterations (handled inside run_test_iteration)
    2. Average the top 2 results for each test type
    3. Each run within an iteration has its own clean Go server and data file
    """
    results = {}
    
    iteration_count = 0
    for vus in range(min_vus, max_vus + 1, step):
        iteration_count += 1
        print(f"\n{'='*60}")
        print(f"  ITERATION {iteration_count}: {vus} VUs")
        print(f"{'='*60}")
        
        iteration_results = run_test_iteration(
            vus, duration, k6_address, linger, server_addr, data_file
        )
        results[vus] = iteration_results
        
        # Small delay between VU iterations
        print("\n  Waiting before next VU iteration...")
        time.sleep(1)
    
    return results


def generate_chart(
    results: Dict[int, Dict[str, Optional[float]]],
    output_file: str
):
    """Generate a CLI chart using matplotlib."""
    # Import matplotlib here to allow --help to work without it installed
    import matplotlib
    matplotlib.use('Agg')  # Use non-interactive backend for CLI
    import matplotlib.pyplot as plt
    
    print("\nGenerating chart...")
    
    # Prepare data
    vus_list = sorted(results.keys())
    
    # Extract throughput data for each test type
    data = {}
    for test_type in K6_SCRIPTS.keys():
        data[test_type] = []
        for vus in vus_list:
            throughput = results[vus].get(test_type)
            data[test_type].append(throughput if throughput is not None else 0)
    
    # Also include combined
    data["combined"] = []
    for vus in vus_list:
        throughput = results[vus].get("combined")
        data["combined"].append(throughput if throughput is not None else 0)
    
    # Find maximum throughput and corresponding VUs
    max_throughput = 0
    max_vus = 0
    max_test_type = ""
    
    for vus in vus_list:
        for test_type, throughput_list in data.items():
            idx = vus_list.index(vus)
            throughput = throughput_list[idx]
            if throughput and throughput > max_throughput:
                max_throughput = throughput
                max_vus = vus
                max_test_type = test_type
    
    # Create figure
    plt.figure(figsize=(12, 8))
    
    # Plot each test type
    colors = {'get': 'blue', 'post': 'green', 'delete': 'red', 'combined': 'purple'}
    markers = {'get': 'o', 'post': 's', 'delete': '^', 'combined': '*'}
    
    for test_type, throughput_list in data.items():
        plt.plot(
            vus_list,
            throughput_list,
            label=test_type.upper(),
            color=colors.get(test_type, 'gray'),
            marker=markers.get(test_type, 'o'),
            linewidth=2,
            markersize=8
        )
    
    # Add title and labels
    plt.title(
        f"K6 Load Test Results\n"
        f"Max Throughput: {max_throughput:.2f} req/s at {max_vus} VUs ({max_test_type.upper()})",
        fontsize=14,
        pad=20
    )
    plt.xlabel("Virtual Users (VUs)", fontsize=12)
    plt.ylabel("Requests per Second (req/s)", fontsize=12)
    plt.grid(True, alpha=0.3)
    plt.legend(fontsize=10)
    
    # Annotate the maximum point
    if max_throughput > 0:
        idx = vus_list.index(max_vus)
        plt.annotate(
            f"Max: {max_throughput:.2f} req/s",
            xy=(max_vus, max_throughput),
            xytext=(max_vus + 5, max_throughput + 5),
            arrowprops=dict(facecolor='black', shrink=0.05),
            fontsize=10,
            bbox=dict(boxstyle="round,pad=0.3", fc="yellow", alpha=0.7)
        )
    
    # Save the chart
    plt.savefig(output_file, dpi=300, bbox_inches='tight')
    print(f"Chart saved to: {output_file}")
    
    # Also print text summary
    print("\n=== Summary ===")
    print(f"Max throughput: {max_throughput:.2f} req/s")
    print(f"At VUs: {max_vus}")
    print(f"Test type: {max_test_type.upper()}")
    
    # Print full results table
    print("\n=== Full Results ===")
    print(f"{'VUs':<8} {'GET':<12} {'POST':<12} {'DELETE':<12} {'COMBINED':<12}")
    print("-" * 60)
    for vus in vus_list:
        get_t = results[vus].get('get', 0) or 0
        post_t = results[vus].get('post', 0) or 0
        delete_t = results[vus].get('delete', 0) or 0
        combined_t = results[vus].get('combined', 0) or 0
        print(f"{vus:<8} {get_t:<12.2f} {post_t:<12.2f} {delete_t:<12.2f} {combined_t:<12.2f}")
    
    return max_vus, max_throughput, max_test_type


def check_dependencies():
    """Check that required dependencies are available."""
    import importlib
    
    # Check Python packages
    required_packages = ['matplotlib']
    missing = []
    
    for package in required_packages:
        try:
            importlib.import_module(package)
        except ImportError:
            missing.append(package)
    
    if missing:
        print(f"Error: Missing Python packages: {', '.join(missing)}")
        print("Please run: pip install " + " ".join(missing))
        sys.exit(1)
    
    # Check external commands
    try:
        subprocess.run(["k6", "--version"], capture_output=True, check=True)
    except (subprocess.CalledProcessError, FileNotFoundError):
        print("Error: k6 is not installed or not in PATH")
        print("Please install k6 from: https://k6.io/docs/getting-started/installation/")
        sys.exit(1)
    
    try:
        subprocess.run(["go", "version"], capture_output=True, check=True)
    except (subprocess.CalledProcessError, FileNotFoundError):
        print("Error: Go is not installed or not in PATH")
        print("Please install Go from: https://go.dev/doc/install")
        sys.exit(1)


def main():
    import os
    
    # Parse arguments
    args = parse_args()
    
    # Check dependencies
    print("Checking dependencies...")
    check_dependencies()
    
    # Check k6 scripts
    check_k6_scripts()
    
    # Run load test series
    print(f"\nRunning load tests from {args.min_vus} to {args.max_vus} VUs with step {args.step}")
    print(f"Server address: {args.server_addr}")
    print(f"k6 API address: {args.k6_address}")
    print(f"Duration: {args.duration}")
    print(f"Linger: {args.linger}")
    print(f"Data File: {args.data_file}")

    results = run_load_test_series(
        min_vus=args.min_vus,
        max_vus=args.max_vus,
        step=args.step,
        server_addr=args.server_addr,
        k6_address=args.k6_address,
        duration=args.duration,
        linger=args.linger,
        data_file=args.data_file
    )
    
    # Generate chart
    max_vus, max_throughput, max_test_type = generate_chart(
        results,
        args.output
    )
    
    print(f"\n=== Conclusion ===")
    print(f"The maximum number of requests was achieved with {max_vus} VUs")
    print(f"Maximum throughput: {max_throughput:.2f} req/s ({max_test_type.upper()})")


if __name__ == "__main__":
    main()
