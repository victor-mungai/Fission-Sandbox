#!/usr/bin/env bash
set -euo pipefail

ROOTFS_PATH="${1:-/opt/fission/rootfs.ext4}"
ROOTFS_SIZE_MB="${ROOTFS_SIZE_MB:-512}"
DEBIAN_RELEASE="${DEBIAN_RELEASE:-bookworm}"
MIRROR="${DEBIAN_MIRROR:-http://deb.debian.org/debian}"
WORKDIR="$(mktemp -d)"
MOUNTDIR="${WORKDIR}/mnt"
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

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

mkdir -p "${MOUNTDIR}/work"
install -m 0755 "${SCRIPT_DIR}/fission-init" "${MOUNTDIR}/sbin/fission-init"
echo "rootfs created at ${ROOTFS_PATH}"
