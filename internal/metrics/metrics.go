package metrics

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"time"
)

type Metrics struct {
	mu sync.Mutex

	RunTotal       uint64
	RunFailures    uint64
	RunTimeouts    uint64
	VMFailures     uint64
	WorkspaceBuild uint64
	WorkspaceCopy  uint64
	WarmRecycle    uint64
	WarmEviction   uint64
	WarmResetFail  uint64

	ActiveExecutions int
	QueuedRequests   int
	PoolCapacity     int
	WarmPoolTarget   int
	WarmPoolReady    int

	ExecutionDurations []time.Duration
	QueueWaits         []time.Duration
	VMBootDurations    []time.Duration
	WarmLeaseDurations []time.Duration
	WarmResetDurations []time.Duration
}

type Snapshot struct {
	RunTotal       uint64
	RunFailures    uint64
	RunTimeouts    uint64
	VMFailures     uint64
	WorkspaceBuild uint64
	WorkspaceCopy  uint64
	WarmRecycle    uint64
	WarmEviction   uint64
	WarmResetFail  uint64

	ActiveExecutions int
	QueuedRequests   int
	PoolCapacity     int
	WarmPoolTarget   int
	WarmPoolReady    int

	ExecutionDurations []time.Duration
	QueueWaits         []time.Duration
	VMBootDurations    []time.Duration
	WarmLeaseDurations []time.Duration
	WarmResetDurations []time.Duration

	Memory runtime.MemStats
}

var Default = New()

func New() *Metrics {
	return &Metrics{}
}

func (m *Metrics) SetPoolCapacity(capacity int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.PoolCapacity = capacity
}

func (m *Metrics) SetWarmPoolTarget(target int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.WarmPoolTarget = target
}

func (m *Metrics) SetWarmPoolReady(ready int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.WarmPoolReady = ready
}

func (m *Metrics) AddQueued(delta int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.QueuedRequests += delta
}

func (m *Metrics) AddActive(delta int) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.ActiveExecutions += delta
}

func (m *Metrics) RecordQueueWait(duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.QueueWaits = appendSample(m.QueueWaits, duration)
}

func (m *Metrics) RecordVMBoot(duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.VMBootDurations = appendSample(m.VMBootDurations, duration)
}

func (m *Metrics) RecordWorkspaceBuild(copiedTemplate bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if copiedTemplate {
		m.WorkspaceCopy++
		return
	}
	m.WorkspaceBuild++
}

func (m *Metrics) RecordWarmLease(duration time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.WarmLeaseDurations = appendSample(m.WarmLeaseDurations, duration)
}

func (m *Metrics) RecordWarmReset(duration time.Duration, success bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !success {
		m.WarmResetFail++
	}
	m.WarmResetDurations = appendSample(m.WarmResetDurations, duration)
}

func (m *Metrics) AddWarmRecycle() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.WarmRecycle++
}

func (m *Metrics) AddWarmEviction() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.WarmEviction++
}

func (m *Metrics) RecordRun(duration time.Duration, exitCode int, timedOut bool, vmFailure bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.RunTotal++
	if exitCode != 0 {
		m.RunFailures++
	}
	if timedOut {
		m.RunTimeouts++
	}
	if vmFailure {
		m.VMFailures++
	}
	m.ExecutionDurations = appendSample(m.ExecutionDurations, duration)
}

func (m *Metrics) Snapshot() Snapshot {
	m.mu.Lock()
	defer m.mu.Unlock()

	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)

	return Snapshot{
		RunTotal:           m.RunTotal,
		RunFailures:        m.RunFailures,
		RunTimeouts:        m.RunTimeouts,
		VMFailures:         m.VMFailures,
		WorkspaceBuild:     m.WorkspaceBuild,
		WorkspaceCopy:      m.WorkspaceCopy,
		WarmRecycle:        m.WarmRecycle,
		WarmEviction:       m.WarmEviction,
		WarmResetFail:      m.WarmResetFail,
		ActiveExecutions:   m.ActiveExecutions,
		QueuedRequests:     m.QueuedRequests,
		PoolCapacity:       m.PoolCapacity,
		WarmPoolTarget:     m.WarmPoolTarget,
		WarmPoolReady:      m.WarmPoolReady,
		ExecutionDurations: cloneDurations(m.ExecutionDurations),
		QueueWaits:         cloneDurations(m.QueueWaits),
		VMBootDurations:    cloneDurations(m.VMBootDurations),
		WarmLeaseDurations: cloneDurations(m.WarmLeaseDurations),
		WarmResetDurations: cloneDurations(m.WarmResetDurations),
		Memory:             mem,
	}
}

func (s Snapshot) Prometheus() string {
	var builder strings.Builder
	writeGauge(&builder, "fission_runs_total", float64(s.RunTotal))
	writeGauge(&builder, "fission_run_failures_total", float64(s.RunFailures))
	writeGauge(&builder, "fission_run_timeouts_total", float64(s.RunTimeouts))
	writeGauge(&builder, "fission_vm_failures_total", float64(s.VMFailures))
	writeGauge(&builder, "fission_workspace_builds_total", float64(s.WorkspaceBuild))
	writeGauge(&builder, "fission_workspace_template_copies_total", float64(s.WorkspaceCopy))
	writeGauge(&builder, "fission_vm_recycle_total", float64(s.WarmRecycle))
	writeGauge(&builder, "fission_vm_evictions_total", float64(s.WarmEviction))
	writeGauge(&builder, "fission_vm_reset_failures_total", float64(s.WarmResetFail))
	writeGauge(&builder, "fission_active_executions", float64(s.ActiveExecutions))
	writeGauge(&builder, "fission_queued_requests", float64(s.QueuedRequests))
	writeGauge(&builder, "fission_vm_pool_capacity", float64(s.PoolCapacity))
	writeGauge(&builder, "fission_warm_pool_target", float64(s.WarmPoolTarget))
	writeGauge(&builder, "fission_warm_pool_ready", float64(s.WarmPoolReady))
	writeGauge(&builder, "fission_execution_duration_ms_p50", percentileMs(s.ExecutionDurations, 0.50))
	writeGauge(&builder, "fission_execution_duration_ms_p95", percentileMs(s.ExecutionDurations, 0.95))
	writeGauge(&builder, "fission_queue_wait_ms_p50", percentileMs(s.QueueWaits, 0.50))
	writeGauge(&builder, "fission_queue_wait_ms_p95", percentileMs(s.QueueWaits, 0.95))
	writeGauge(&builder, "fission_vm_boot_duration_ms_p50", percentileMs(s.VMBootDurations, 0.50))
	writeGauge(&builder, "fission_vm_boot_duration_ms_p95", percentileMs(s.VMBootDurations, 0.95))
	writeGauge(&builder, "fission_vm_lease_time_warm_ms_p50", percentileMs(s.WarmLeaseDurations, 0.50))
	writeGauge(&builder, "fission_vm_lease_time_warm_ms_p95", percentileMs(s.WarmLeaseDurations, 0.95))
	writeGauge(&builder, "fission_vm_reset_time_ms_p50", percentileMs(s.WarmResetDurations, 0.50))
	writeGauge(&builder, "fission_vm_reset_time_ms_p95", percentileMs(s.WarmResetDurations, 0.95))
	writeGauge(&builder, "fission_go_heap_alloc_bytes", float64(s.Memory.HeapAlloc))
	writeGauge(&builder, "fission_go_heap_sys_bytes", float64(s.Memory.HeapSys))
	writeGauge(&builder, "fission_go_goroutines", float64(runtime.NumGoroutine()))
	writeGauge(&builder, "fission_go_gc_pause_total_ns", float64(s.Memory.PauseTotalNs))
	return builder.String()
}

func appendSample(samples []time.Duration, sample time.Duration) []time.Duration {
	const maxSamples = 1024
	samples = append(samples, sample)
	if len(samples) <= maxSamples {
		return samples
	}
	copy(samples, samples[len(samples)-maxSamples:])
	return samples[:maxSamples]
}

func cloneDurations(samples []time.Duration) []time.Duration {
	cloned := make([]time.Duration, len(samples))
	copy(cloned, samples)
	return cloned
}

func percentileMs(samples []time.Duration, percentile float64) float64 {
	if len(samples) == 0 {
		return 0
	}

	sorted := cloneDurations(samples)
	for i := 1; i < len(sorted); i++ {
		value := sorted[i]
		j := i - 1
		for j >= 0 && sorted[j] > value {
			sorted[j+1] = sorted[j]
			j--
		}
		sorted[j+1] = value
	}

	index := int(float64(len(sorted)-1) * percentile)
	return float64(sorted[index].Milliseconds())
}

func writeGauge(builder *strings.Builder, name string, value float64) {
	fmt.Fprintf(builder, "# TYPE %s gauge\n%s %.0f\n", name, name, value)
}
