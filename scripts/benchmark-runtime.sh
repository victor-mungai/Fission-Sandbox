#!/usr/bin/env bash
set -euo pipefail

API_URL="${API_URL:-http://localhost:8080/run}"
SEQUENTIAL_RUNS="${SEQUENTIAL_RUNS:-100}"
CONCURRENT_RUNS="${CONCURRENT_RUNS:-20}"

if [[ -f .env ]]; then
  set -a
  # shellcheck disable=SC1091
  source .env
  set +a
fi

if [[ -z "${SANDBOX_AUTH_TOKEN:-}" ]]; then
  echo "error: SANDBOX_AUTH_TOKEN must be set in .env or the environment"
  exit 1
fi

TMPDIR="$(mktemp -d)"
cleanup() {
  rm -rf "${TMPDIR}"
}
trap cleanup EXIT

run_once() {
  local run_id="$1"
  local output_file="$2"
  local start_ms end_ms latency_ms

  start_ms="$(date +%s%3N)"
  curl -sS -X POST "${API_URL}" \
    -H "Content-Type: application/json" \
    -H "x-sandbox-auth: ${SANDBOX_AUTH_TOKEN}" \
    -d "{\"runId\":\"${run_id}\",\"command\":\"python3 -c \\\"print(123)\\\"\",\"timeoutMs\":30000,\"memoryMb\":128,\"cpuCount\":1}" \
    >"${output_file}.json"
  end_ms="$(date +%s%3N)"
  latency_ms=$((end_ms - start_ms))

  printf "%s\n" "${latency_ms}" >"${output_file}.latency"
}

summarize() {
  local label="$1"
  local pattern="$2"
  local sorted="${TMPDIR}/${label}.sorted"
  local count p50_index p95_index p50 p95

  find "${TMPDIR}" -name "${pattern}" -type f -print0 | xargs -0 cat | sort -n >"${sorted}"
  count="$(wc -l <"${sorted}")"
  if [[ "${count}" -eq 0 ]]; then
    echo "${label}: no samples"
    return
  fi

  p50_index=$(( (count + 1) / 2 ))
  p95_index=$(( (count * 95 + 99) / 100 ))
  p50="$(sed -n "${p50_index}p" "${sorted}")"
  p95="$(sed -n "${p95_index}p" "${sorted}")"

  echo "${label}: count=${count} p50_ms=${p50} p95_ms=${p95}"
}

echo "sequential benchmark: ${SEQUENTIAL_RUNS} runs"
for i in $(seq 1 "${SEQUENTIAL_RUNS}"); do
  run_once "bench-seq-${i}" "${TMPDIR}/seq-${i}"
done
summarize "sequential" "seq-*.latency"

echo "concurrent benchmark: ${CONCURRENT_RUNS} runs"
for i in $(seq 1 "${CONCURRENT_RUNS}"); do
  run_once "bench-concurrent-${i}" "${TMPDIR}/concurrent-${i}" &
done
wait
summarize "concurrent" "concurrent-*.latency"
