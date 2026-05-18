package pool

import (
	"context"
	"time"

	"fission-sandbox/internal/metrics"
)

type Pool struct {
	leases  chan struct{}
	metrics *metrics.Metrics
}

func New(capacity int, runtimeMetrics *metrics.Metrics) *Pool {
	if capacity <= 0 {
		capacity = 1
	}
	if runtimeMetrics == nil {
		runtimeMetrics = metrics.Default
	}
	runtimeMetrics.SetPoolCapacity(capacity)

	return &Pool{
		leases:  make(chan struct{}, capacity),
		metrics: runtimeMetrics,
	}
}

func (p *Pool) Acquire(ctx context.Context) (*Lease, error) {
	queued := false
	if len(p.leases) == cap(p.leases) {
		queued = true
		p.metrics.AddQueued(1)
	}

	start := time.Now()
	select {
	case p.leases <- struct{}{}:
		if queued {
			p.metrics.AddQueued(-1)
		}
		p.metrics.RecordQueueWait(time.Since(start))
		p.metrics.AddActive(1)
		return &Lease{pool: p}, nil
	case <-ctx.Done():
		if queued {
			p.metrics.AddQueued(-1)
		}
		p.metrics.RecordQueueWait(time.Since(start))
		return nil, ctx.Err()
	}
}

func (p *Pool) release() {
	select {
	case <-p.leases:
		p.metrics.AddActive(-1)
	default:
	}
}
