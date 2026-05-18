package pool

import (
	"context"
	"log/slog"
	"time"
)

type HealthMonitor struct {
	pool     *WarmPool
	interval time.Duration
	logger   *slog.Logger
}

func NewHealthMonitor(pool *WarmPool, interval time.Duration, logger *slog.Logger) *HealthMonitor {
	if interval <= 0 {
		interval = 5 * time.Second
	}
	return &HealthMonitor{pool: pool, interval: interval, logger: logger}
}

func (m *HealthMonitor) Start(ctx context.Context) {
	if m == nil || m.pool == nil {
		return
	}

	go func() {
		ticker := time.NewTicker(m.interval)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				m.repair(ctx)
			}
		}
	}()
}

func (m *HealthMonitor) repair(ctx context.Context) {
	for m.pool.ReadyCount() < cap(m.pool.ready) {
		if m.logger != nil {
			m.logger.Warn("warm pool below target; replacing vm", "ready", m.pool.ReadyCount(), "target", cap(m.pool.ready))
		}
		m.pool.Replace(ctx)
		if m.pool.ReadyCount() == cap(m.pool.ready) {
			return
		}
	}
}
