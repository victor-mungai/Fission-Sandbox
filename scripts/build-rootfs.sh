#!/usr/bin/env bash
set -euo pipefail

ROOTFS_PATH="${1:-/opt/fission/rootfs.ext4}"
ROOTFS_SIZE_MB="${ROOTFS_SIZE_MB:-512}"
DEBIAN_RELEASE="${DEBIAN_RELEASE:-bookworm}"
MIRROR="${DEBIAN_MIRROR:-http://deb.debian.org/debian}"
WORKDIR="$(mktemp -d)"
MOUNTDIR="${WORKDIR}/mnt"

cleanup() {
  if mountpoint -q "${MOUNTDIR}"; then
    umount "${MOUNTDIR}"
  fi
  rm -rf "${WORKDIR}"
}
trap cleanup EXIT

if [[ "$(id -u)" -ne 0 ]]; then
  echo "error: run as root because rootfs creation requires mount/chroot"
  exit 1
fi

for binary in debootstrap mkfs.ext4 mount chroot; do
  if ! command -v "${binary}" >/dev/null 2>&1; then
    echo "error: missing required binary: ${binary}"
    exit 1
  fi
done

mkdir -p "$(dirname "${ROOTFS_PATH}")" "${MOUNTDIR}"
truncate -s "${ROOTFS_SIZE_MB}M" "${ROOTFS_PATH}"
mkfs.ext4 -F "${ROOTFS_PATH}"
mount -o loop "${ROOTFS_PATH}" "${MOUNTDIR}"

debootstrap --variant=minbase --include=bash,python3,coreutils,tar,mount,util-linux "${DEBIAN_RELEASE}" "${MOUNTDIR}" "${MIRROR}"

cat > "${MOUNTDIR}/sbin/fission-init" <<'INIT'
#!/bin/bash
set +e

mount -t proc proc /proc
mount -t sysfs sysfs /sys
mount -t devtmpfs devtmpfs /dev

mkdir -p /work /work/files
mount /dev/vdb /work

if [[ -f /work/files.tar ]]; then
  tar -xf /work/files.tar -C /work/files
fi

chmod +x /work/command.sh
/bin/bash /work/command.sh >/work/stdout.txt 2>/work/stderr.txt
exit_code=$?

python3 - "${exit_code}" <<'PY'
import base64
import json
import sys

def read_text(path):
    try:
        with open(path, "r", encoding="utf-8", errors="replace") as handle:
            return handle.read()
    except FileNotFoundError:
        return ""

payload = {
    "stdout": read_text("/work/stdout.txt"),
    "stderr": read_text("/work/stderr.txt"),
    "exitCode": int(sys.argv[1]),
}
encoded = base64.b64encode(json.dumps(payload).encode("utf-8")).decode("ascii")
print("FISSION_RESULT " + encoded)
PY

sync
reboot -f
INIT

chmod 0755 "${MOUNTDIR}/sbin/fission-init"
echo "rootfs created at ${ROOTFS_PATH}"
