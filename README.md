# Fission-Sandbox
Fission Sandbox is a lightweight secure execution platform that runs code and shell commands inside ephemeral Firecracker microVMs. It provides a single authenticated API endpoint that allows backend systems and AI agents to safely execute untrusted workloads in fully isolated environments with strict CPU, memory, and time limits. Each execution runs in a disposable virtual machine with no network access and is destroyed immediately after completion.

---

##  Overview

Fission Sandbox provides a single API endpoint that executes shell or Python commands inside ephemeral Firecracker microVMs and returns the output safely.

Each request runs in a **fully isolated VM** with:

- No internet access
- Strict CPU & memory limits
- Ephemeral filesystem
- Automatic teardown after execution
- Warm pool for low-latency execution

This system is designed for AI agents, automation systems, and backend platforms that need safe code execution.

---

##  Why Fission Sandbox Exists

Modern AI systems and automation platforms require secure execution environments to run untrusted code safely.

Fission Sandbox solves this by providing:

- Secure isolation using microVMs
- Deterministic execution environments
- Strong resource control (CPU, memory, timeout)
- Zero-persistence execution model
- Infrastructure-grade safety boundaries

---

##  API

### POST `/run`

Execute a command inside a sandboxed microVM.

#### Request

```json
{
  "runId": "abc-123",
  "command": "python3 script.py",
  "files": [
    {
      "name": "script.py",
      "content": "print('hello')",
      "mode": 755
    }
  ],
  "timeoutMs": 30000,
  "memoryMb": 128,
  "cpuCount": 0.25
}
```
Response
```
{
  "stdout": "hello",
  "stderr": "",
  "exitCode": 0,
  "durationMs": 1200,
  "timedOut": false,
  "vmId": "fc-vm-9a8b7c"
}
```
# Architecture
Go-based control plane API
Firecracker microVM runtime
Warm VM pool for low-latency execution
Strict sandbox isolation
Internal-only networking (no internet access)
# Requirements
Linux host with KVM support
Firecracker installed
Go 1.21+
Bare-metal EC2 instance (recommended)
# Getting Started
Clone repo
cd fission-sandbox
Build
make build
Run API server
make run
make test
## Security Model

Fission Sandbox is designed with strong isolation guarantees:

No network access inside VM
No host filesystem access
Ephemeral execution environment
Strict CPU, memory, and process limits
VM destroyed after every request
No secrets injected into runtime
# MVP Scope
Included
Single-node execution
Firecracker microVM isolation
HTTP execution API (/run)
Basic warm VM pool
Resource limits enforcement
Not Included (Future Work)
Multi-node orchestration
Kubernetes integration
Job queue system
Persistent storage
Autoscaling infrastructure
# Vision

Fission Sandbox is the foundation for a secure execution layer powering AI agents, developer tools, and automated systems at scale.

---

## Current Status: Sprint 0

This repository currently contains the runnable control plane skeleton:

- Go HTTP API server
- `POST /run` endpoint
- Request and response models
- Constant-time auth token check via `x-sandbox-auth`
- Environment-based configuration loaded from `.env`
- Structured JSON logging with `log/slog`
- Runtime-backed executor with `mock` and `firecracker` modes
- Single-VM Firecracker execution pipeline scaffolding for Linux/KVM hosts

### Local Configuration

Create a local `.env` file from `.env.example`:

```sh
cp .env.example .env
```

Set these values in `.env`:

| Variable | Required |
| --- | --- |
| `PORT` | yes |
| `SANDBOX_AUTH_TOKEN` | yes |
| `SANDBOX_DEFAULT_TIMEOUT_MS` | yes |
| `SANDBOX_DEFAULT_MEMORY_MB` | yes |
| `SANDBOX_DEFAULT_CPU_COUNT` | yes |
| `EXECUTOR_MODE` | yes (`mock` or `firecracker`) |
| `FIRECRACKER_BIN` | yes when `EXECUTOR_MODE=firecracker` |
| `FIRECRACKER_KERNEL_IMAGE` | yes when `EXECUTOR_MODE=firecracker` |
| `FIRECRACKER_KERNEL_ARGS` | optional |
| `FIRECRACKER_ROOTFS_IMAGE` | yes when `EXECUTOR_MODE=firecracker` |
| `FIRECRACKER_WORKDIR` | yes when `EXECUTOR_MODE=firecracker` |
| `FIRECRACKER_WORKSPACE_IMAGE_MB` | yes when `EXECUTOR_MODE=firecracker` |

### Run Locally

```sh
make run
```

### Test Request

```sh
curl -X POST http://localhost:8080/run \
  -H "Content-Type: application/json" \
  -H "x-sandbox-auth: replace-with-local-token" \
  -d '{
    "runId": "test-1",
    "command": "echo hello",
    "timeoutMs": 30000,
    "memoryMb": 128,
    "cpuCount": 0.25
  }'
```

Expected response:

```json
{
  "stdout": "mock output from: echo hello",
  "stderr": "",
  "exitCode": 0,
  "durationMs": 500,
  "timedOut": false,
  "vmId": "mock-vm-001"
}
```

### Firecracker Mode

Sprint 1 Firecracker setup lives in [docs/sprint-1-firecracker.md](docs/sprint-1-firecracker.md).

On a Linux host with KVM:

```sh
scripts/verify-firecracker-host.sh
sudo scripts/build-rootfs.sh /opt/fission/rootfs.ext4
```

Then set `EXECUTOR_MODE=firecracker` in `.env` and provide kernel/rootfs paths.
