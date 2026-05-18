# Fission Sandbox

Fission Sandbox is a Go API service for executing untrusted commands inside isolated Firecracker microVMs. It is built for AI agents, automation systems, and backend services that need to run arbitrary shell or Python workloads without giving those workloads access to the host machine, network, secrets, or persistent storage.

The current implementation provides a real single-host Firecracker execution path:

```text
POST /run
-> create per-run workspace
-> boot Firecracker microVM
-> mount workspace at /work
-> run command inside the guest
-> capture stdout/stderr/exitCode from serial output
-> destroy VM
-> return JSON response
```

The runtime has also gained the first efficiency foundations for Sprint 2 and Sprint 2.5: bounded execution concurrency, metrics, cleanup, workspace template support, memory logging, benchmark tooling, and a warm-pool state machine. Live VM reuse is intentionally not enabled yet because the current guest init is one-shot; safe reuse requires a persistent guest agent and reset validation before the same VM can execute a second untrusted workload.

## Current Capabilities

- Authenticated HTTP API with `POST /run`.
- `mock` executor mode for local development without Firecracker.
- `firecracker` executor mode for real Linux/KVM hosts.
- Per-request Firecracker microVM boot and teardown.
- Per-request writable ext4 workspace mounted at `/work`.
- Request file injection into `/work/files`.
- Command execution through `/work/command.sh`.
- Result capture through a `FISSION_RESULT <base64-json>` serial marker.
- Bounded concurrent execution using an internal lease pool.
- Optional workspace template image path to avoid full ext4 creation per request.
- Failed-run artifact preservation for debugging.
- Artifact cleanup worker for old preserved VM work directories.
- Prometheus-style `/metrics` endpoint.
- Periodic Go runtime memory logging.
- Runtime benchmark script for sequential and concurrent runs.
- Warm-pool lifecycle primitives for future true VM reuse.

## Security Model

Fission Sandbox is designed around isolation-first execution. The platform should not trade isolation guarantees for latency improvements.

Current runtime guarantees:

- No network device is attached to the guest VM.
- The root filesystem is mounted read-only.
- Each request receives a separate writable workspace image.
- Request files are extracted into `/work/files` inside the guest.
- The VM process is killed after execution or timeout.
- Successful run artifacts are deleted automatically.
- Failed and non-zero runs are preserved for inspection and later cleanup.
- Secrets are not injected into the guest runtime.
- Local secrets must live in `.env`, which is ignored by Git.

Important warm-pool boundary:

- The repository contains a warm-pool state machine and reset lifecycle interfaces.
- `/run` still uses the safe cold-VM path.
- True live VM reuse is not enabled until a guest agent can prove reset cleanliness.
- Reusing a live VM without a reset protocol would risk file, process, or memory state leakage.

## Architecture

```text
cmd/api
  HTTP entrypoint
  registers /run and /metrics

internal/api
  request validation
  auth enforcement
  response serialization

internal/executor
  runtime selection
  execution logging
  run-level metrics

internal/runtime
  mock runtime
  Firecracker runtime
  workspace image creation
  serial result parsing

internal/pool
  bounded lease pool
  warm-pool state machine
  reset/recycle/eviction primitives

internal/metrics
  in-memory counters and latency samples
  Prometheus text rendering
  periodic memory logging

internal/cleanup
  old artifact cleanup worker

scripts
  host verification
  rootfs build
  guest fission-init
  runtime benchmark
```

## API

### `POST /run`

Executes a command inside the selected runtime.

Headers:

```http
Content-Type: application/json
x-sandbox-auth: <SANDBOX_AUTH_TOKEN>
```

Request:

```json
{
  "runId": "example-1",
  "command": "python3 script.py",
  "files": [
    {
      "name": "script.py",
      "content": "print('hello from vm')\n",
      "mode": 755
    }
  ],
  "timeoutMs": 30000,
  "memoryMb": 128,
  "cpuCount": 1
}
```

Response:

```json
{
  "stdout": "hello from vm\n",
  "stderr": "",
  "exitCode": 0,
  "durationMs": 1135,
  "timedOut": false,
  "vmId": "fc-example-1-20260518173643"
}
```

Validation rules:

- `runId` is required.
- `command` is required.
- `timeoutMs`, `memoryMb`, and `cpuCount` may be omitted or set to `0` to use defaults.
- File names must be relative paths.
- Absolute paths, `..`, and Windows-style path separators are rejected.

### `GET /metrics`

Returns Prometheus-style text metrics.

Example:

```text
# TYPE fission_runs_total gauge
fission_runs_total 1
# TYPE fission_workspace_builds_total gauge
fission_workspace_builds_total 1
# TYPE fission_vm_pool_capacity gauge
fission_vm_pool_capacity 1
# TYPE fission_execution_duration_ms_p50 gauge
fission_execution_duration_ms_p50 1092
# TYPE fission_vm_boot_duration_ms_p50 gauge
fission_vm_boot_duration_ms_p50 106
# TYPE fission_go_heap_alloc_bytes gauge
fission_go_heap_alloc_bytes 1724144
```

Tracked metric categories:

- Total runs, failures, timeouts, and VM failures.
- Active and queued executions.
- Queue wait duration.
- VM boot duration.
- Execution duration.
- Workspace builds and workspace template copies.
- Warm-pool lease, reset, recycle, eviction, and readiness metrics.
- Go heap, GC pause, and goroutine stats.

## Configuration

Create a local `.env` from `.env.example`:

```sh
cp .env.example .env
```

Do not commit `.env`. It is ignored by Git and is the required place for local secrets such as `SANDBOX_AUTH_TOKEN`.

Core settings:

| Variable | Required | Description |
| --- | --- | --- |
| `PORT` | yes | HTTP server port. |
| `SANDBOX_AUTH_TOKEN` | yes | Token required in `x-sandbox-auth`. Store only in `.env` or the process environment. |
| `SANDBOX_DEFAULT_TIMEOUT_MS` | yes | Default execution timeout. |
| `SANDBOX_DEFAULT_MEMORY_MB` | yes | Default VM memory. |
| `SANDBOX_DEFAULT_CPU_COUNT` | yes | Default requested CPU count. |
| `EXECUTOR_MODE` | yes | `mock` or `firecracker`. |

Firecracker settings:

| Variable | Required | Description |
| --- | --- | --- |
| `FIRECRACKER_BIN` | firecracker mode | Path to Firecracker binary. |
| `FIRECRACKER_KERNEL_IMAGE` | firecracker mode | Guest kernel image path. |
| `FIRECRACKER_KERNEL_ARGS` | optional | Kernel arguments. The runtime appends `init=/sbin/fission-init`. |
| `FIRECRACKER_ROOTFS_IMAGE` | firecracker mode | Root filesystem image path. |
| `FIRECRACKER_WORKDIR` | firecracker mode | Host directory for per-run VM artifacts. |
| `FIRECRACKER_WORKSPACE_IMAGE_MB` | firecracker mode | Workspace image size when using `mkfs.ext4`. |
| `FIRECRACKER_WORKSPACE_TEMPLATE_IMAGE` | optional | Empty ext4 template copied per request before injecting workspace files. |

Runtime efficiency settings:

| Variable | Default | Description |
| --- | --- | --- |
| `MAX_CONCURRENT_VMS` | `1` | Maximum concurrent VM executions. |
| `WARM_POOL_SIZE` | `0` | Reserved for true warm VM pool target size. Live reuse is not wired yet. |
| `ARTIFACT_RETENTION_HOURS` | `24` | How long failed VM artifacts are retained. |
| `CLEANUP_INTERVAL_SECONDS` | `300` | Artifact cleanup interval. |
| `MEMORY_LOG_INTERVAL_SECONDS` | `60` | Interval for Go memory stats logging. |

Example Firecracker `.env`:

```env
PORT=8080
SANDBOX_AUTH_TOKEN=replace-with-local-value
SANDBOX_DEFAULT_TIMEOUT_MS=30000
SANDBOX_DEFAULT_MEMORY_MB=128
SANDBOX_DEFAULT_CPU_COUNT=1
EXECUTOR_MODE=firecracker
FIRECRACKER_BIN=/usr/local/bin/firecracker
FIRECRACKER_KERNEL_IMAGE=/opt/fission/vmlinux
FIRECRACKER_KERNEL_ARGS=console=ttyS0 reboot=k panic=1 pci=off root=/dev/vda ro
FIRECRACKER_ROOTFS_IMAGE=/opt/fission/rootfs.ext4
FIRECRACKER_WORKDIR=/var/lib/fission-sandbox
FIRECRACKER_WORKSPACE_IMAGE_MB=64
FIRECRACKER_WORKSPACE_TEMPLATE_IMAGE=
MAX_CONCURRENT_VMS=1
WARM_POOL_SIZE=0
ARTIFACT_RETENTION_HOURS=24
CLEANUP_INTERVAL_SECONDS=300
MEMORY_LOG_INTERVAL_SECONDS=60
```

## Host Requirements

Firecracker mode requires:

- Linux host.
- KVM support.
- `/dev/kvm` available to the service user.
- Firecracker binary.
- `mkfs.ext4`.
- `debugfs` when using `FIRECRACKER_WORKSPACE_TEMPLATE_IMAGE`.
- Compatible guest kernel image.
- Rootfs containing `/sbin/fission-init`, `bash`, `python3`, `tar`, and mount utilities.

Recommended target:

- Amazon Linux 2023 on EC2 metal.
- Firecracker installed at `/usr/local/bin/firecracker`.
- Kernel at `/opt/fission/vmlinux`.
- Rootfs at `/opt/fission/rootfs.ext4`.
- Workdir at `/var/lib/fission-sandbox`.

Verify host readiness:

```sh
./scripts/verify-firecracker-host.sh
```

## Root Filesystem

Build a rootfs with:

```sh
sudo ./scripts/build-rootfs.sh /opt/fission/rootfs.ext4
```

The build script installs [scripts/fission-init](scripts/fission-init) into the guest as `/sbin/fission-init`.

Guest init flow:

```text
mount proc/sys/dev
mount /dev/vdb at /work
create /work/files
extract /work/files.tar into /work/files
run /work/command.sh
write stdout/stderr/exitCode into JSON
print FISSION_RESULT <base64-json> to serial
sync and reboot
```

The rootfs must contain `/work` as a mountpoint because the root filesystem is read-only at runtime.

## Running Locally

Mock mode:

```sh
cp .env.example .env
# set EXECUTOR_MODE=mock
make run
```

Firecracker mode:

```sh
./scripts/verify-firecracker-host.sh
make run
```

Test request:

```sh
curl -X POST http://localhost:8080/run \
  -H "Content-Type: application/json" \
  -H "x-sandbox-auth: ${SANDBOX_AUTH_TOKEN}" \
  -d '{
    "runId": "vm-hello",
    "command": "python3 -c \"print(123)\"",
    "timeoutMs": 30000,
    "memoryMb": 128,
    "cpuCount": 1
  }'
```

Expected Firecracker response:

```json
{
  "stdout": "123\n",
  "stderr": "",
  "exitCode": 0,
  "durationMs": 1000,
  "timedOut": false,
  "vmId": "fc-vm-hello-..."
}
```

File injection example:

```sh
curl -X POST http://localhost:8080/run \
  -H "Content-Type: application/json" \
  -H "x-sandbox-auth: ${SANDBOX_AUTH_TOKEN}" \
  -d '{
    "runId": "vm-file",
    "command": "python3 script.py",
    "files": [
      {
        "name": "script.py",
        "content": "print(456)\n",
        "mode": 755
      }
    ],
    "timeoutMs": 30000,
    "memoryMb": 128,
    "cpuCount": 1
  }'
```

Expected response:

```json
{
  "stdout": "456\n",
  "stderr": "",
  "exitCode": 0,
  "durationMs": 1100,
  "timedOut": false,
  "vmId": "fc-vm-file-..."
}
```

## Benchmarking

Run sequential and concurrent API benchmarks against a running server:

```sh
SANDBOX_AUTH_TOKEN="${SANDBOX_AUTH_TOKEN}" ./scripts/benchmark-runtime.sh
```

Defaults:

- `SEQUENTIAL_RUNS=100`
- `CONCURRENT_RUNS=20`
- `API_URL=http://localhost:8080/run`

Override example:

```sh
SEQUENTIAL_RUNS=10 CONCURRENT_RUNS=4 ./scripts/benchmark-runtime.sh
```

The script loads `.env` if present and requires `SANDBOX_AUTH_TOKEN` from `.env` or the environment.

## Debugging Firecracker Runs

Successful runs remove their work directory.

Failed runs and non-zero guest exits preserve artifacts under:

```text
/var/lib/fission-sandbox/fc-<run-id>-<timestamp>
```

Useful files:

| File | Purpose |
| --- | --- |
| `serial.log` | Guest boot logs and `FISSION_RESULT` marker. |
| `firecracker.log` | Firecracker API and VMM logs. |
| `firecracker.stderr.log` | Early Firecracker stderr output. |
| `workspace.ext4` | Per-run workspace disk. |
| `workspace/command.sh` | Generated guest command script. |
| `workspace/files.tar` | Uploaded request files. |

Common failure patterns:

- Missing `/dev/kvm`: host is not exposing KVM to the service.
- Missing `firecracker`: install Firecracker and set `FIRECRACKER_BIN`.
- Guest returns empty stdout/stderr with exit code `1`: inspect `serial.log` for mount or init errors.
- Timeout: confirm kernel/rootfs compatibility and guest init behavior.

## Development

Run tests:

```sh
make test
```

Format code:

```sh
make fmt
```

Build binary:

```sh
make build
```

The `Makefile` uses a workspace-local `.gocache` so tests and builds work in sandboxed environments where `/root/.cache` may be read-only.

## Project Status

Completed:

- Sprint 0 control plane skeleton.
- Authenticated `/run` endpoint.
- Mock runtime.
- Firecracker cold execution runtime.
- Real VM stdout/stderr/exitCode validation.
- File injection into `/work/files`.
- Bounded concurrency lease pool.
- Metrics endpoint.
- Memory logging.
- Artifact cleanup.
- Optional workspace template injection path.
- Warm-pool state machine and reset lifecycle primitives.

Not yet complete:

- Live Firecracker VM reuse through `/run`.
- Persistent guest agent.
- Reset validation inside the guest.
- Snapshot restore path.
- Production multi-node scheduling.
- Tenant-aware auth and quota policy.

## Warm Pool Roadmap

The repository now has the control-plane primitives for true warm VM reuse, but the runtime intentionally remains on the cold path until the guest can safely participate in reset.

Required next steps:

1. Replace one-shot `fission-init` with a long-lived guest agent.
2. Add a host-to-guest command channel.
3. Have the guest agent accept exactly one workload per lease.
4. Add reset commands that clear `/work`, kill stray processes, and validate clean state.
5. Return the VM to `IDLE` only after reset validation passes.
6. Evict and replace any VM that crashes, times out, or fails reset.
7. Compare cold boot latency against warm lease and reset latency.

Hard rule:

```text
No VM reuse without reset validation.
```

## License

See [LICENSE](LICENSE).
