# Sprint 1 Firecracker Notes

Sprint 1 replaces the mock executor with a real Firecracker runtime on a single Linux host.

## Host Requirements

- Linux host with KVM enabled
- `/dev/kvm` available to the service user
- `firecracker`
- `mkfs.ext4`
- Root filesystem image containing `/sbin/fission-init`
- Kernel image compatible with the root filesystem

Verify the host:

```sh
scripts/verify-firecracker-host.sh
```

## Root Filesystem

Build a minimal rootfs with:

- `bash`
- `python3`
- `coreutils`
- `tar`
- mount utilities
- `/sbin/fission-init`

```sh
sudo scripts/build-rootfs.sh /opt/fission/rootfs.ext4
```

The guest init script mounts the per-request workspace drive at `/work`, extracts uploaded files into `/work/files`, runs `/work/command.sh`, prints a base64 JSON result marker to the serial console, and reboots the VM.

## Firecracker Mode

Set `.env`:

```env
EXECUTOR_MODE=firecracker
FIRECRACKER_BIN=/usr/local/bin/firecracker
FIRECRACKER_KERNEL_IMAGE=/opt/fission/vmlinux
FIRECRACKER_KERNEL_ARGS=console=ttyS0 reboot=k panic=1 pci=off root=/dev/vda rw
FIRECRACKER_ROOTFS_IMAGE=/opt/fission/rootfs.ext4
FIRECRACKER_WORKDIR=/var/lib/fission-sandbox
FIRECRACKER_WORKSPACE_IMAGE_MB=64
```

Run:

```sh
make run
```

Then call:

```sh
curl -X POST http://localhost:8080/run \
  -H "Content-Type: application/json" \
  -H "x-sandbox-auth: ${SANDBOX_AUTH_TOKEN}" \
  -d '{"runId":"vm-hello","command":"python3 -c \"print(123)\"","timeoutMs":30000,"memoryMb":128,"cpuCount":1}'
```

## Current Sprint 1 Constraints

- One VM runs at a time through a process-level lock.
- No network device is attached.
- Rootfs is read-only.
- Each request gets a temporary ext4 workspace drive.
- Timeout kills the Firecracker process.
- Performance is intentionally not optimized yet.
