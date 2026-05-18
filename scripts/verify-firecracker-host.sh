#!/usr/bin/env bash
set -euo pipefail

echo "== Fission Sandbox Firecracker host check =="

if [[ "$(uname -s)" != "Linux" ]]; then
  echo "error: Firecracker requires Linux"
  exit 1
fi

if [[ ! -e /dev/kvm ]]; then
  echo "error: /dev/kvm not found. Enable virtualization and run on a KVM-capable host."
  exit 1
fi

if [[ ! -r /dev/kvm || ! -w /dev/kvm ]]; then
  echo "warning: current user cannot read/write /dev/kvm"
  echo "         add the user to the kvm group or run the service with appropriate permissions"
fi

for binary in firecracker mkfs.ext4; do
  if ! command -v "${binary}" >/dev/null 2>&1; then
    echo "error: missing required binary: ${binary}"
    exit 1
  fi
done

firecracker --version
echo "host check passed"
