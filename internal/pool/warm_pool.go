package pool

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"fission-sandbox/internal/metrics"
)

type WarmVM interface {
	ID() string
	State() VMState
	SetState(VMState) error
	Execute(context.Context, any) (any, error)
	Reset(context.Context) error
	Healthy(context.Context) error
	Stop(context.Context) error
}

type WarmVMFactory func(context.Context) (WarmVM, error)

type WarmPool struct {
	ready   chan WarmVM
	factory WarmVMFactory
	metrics *metrics.Metrics
	logger  *slog.Logger

	mu      sync.Mutex
	evicted uint64
	closed  bool
}

func NewWarmPool(ctx context.Context, size int, factory WarmVMFactory, runtimeMetrics *metrics.Metrics, logger *slog.Logger) (*WarmPool, error) {
	if size <= 0 {
		return nil, errors.New("warm pool size must be greater than 0")
	}
	if factory == nil {
		return nil, errors.New("warm pool factory is required")
	}
	if runtimeMetrics == nil {
		runtimeMetrics = metrics.Default
	}

	p := &WarmPool{
		ready:   make(chan WarmVM, size),
		factory: factory,
		metrics: runtimeMetrics,
		logger:  logger,
	}
	runtimeMetrics.SetWarmPoolTarget(size)

	for i := 0; i < size; i++ {
		vm, err := factory(ctx)
		if err != nil {
			p.Close(ctx)
			return nil, fmt.Errorf("create warm vm %d: %w", i, err)
		}
		if err := vm.SetState(VMStateIdle); err != nil {
			p.Close(ctx)
			return nil, err
		}
		p.ready <- vm
	}
	runtimeMetrics.SetWarmPoolReady(len(p.ready))

	return p, nil
}

func (p *WarmPool) Lease(ctx context.Context) (*WarmLease, error) {
	start := time.Now()
	select {
	case vm := <-p.ready:
		p.metrics.RecordWarmLease(time.Since(start))
		p.metrics.SetWarmPoolReady(len(p.ready))
		if err := vm.SetState(VMStateBusy); err != nil {
			p.evict(ctx, vm, "state transition failed")
			return nil, err
		}
		return &WarmLease{pool: p, vm: vm}, nil
	case <-ctx.Done():
		p.metrics.RecordWarmLease(time.Since(start))
		return nil, ctx.Err()
	}
}

func (p *WarmPool) Return(ctx context.Context, vm WarmVM) {
	if vm == nil {
		return
	}
	if err := vm.SetState(VMStateResetting); err != nil {
		p.evict(ctx, vm, "failed to enter reset state")
		return
	}

	start := time.Now()
	if err := vm.Reset(ctx); err != nil {
		p.metrics.RecordWarmReset(time.Since(start), false)
		p.evict(ctx, vm, "reset failed")
		return
	}
	p.metrics.RecordWarmReset(time.Since(start), true)
	p.metrics.AddWarmRecycle()

	if err := vm.Healthy(ctx); err != nil {
		p.evict(ctx, vm, "health check failed after reset")
		return
	}
	if err := vm.SetState(VMStateIdle); err != nil {
		p.evict(ctx, vm, "failed to return to idle")
		return
	}

	select {
	case p.ready <- vm:
		p.metrics.SetWarmPoolReady(len(p.ready))
	default:
		p.evict(ctx, vm, "ready queue full")
	}
}

func (p *WarmPool) Replace(ctx context.Context) {
	p.mu.Lock()
	closed := p.closed
	p.mu.Unlock()
	if closed {
		return
	}

	vm, err := p.factory(ctx)
	if err != nil {
		if p.logger != nil {
			p.logger.Warn("failed to replace warm vm", "error", err)
		}
		return
	}
	if err := vm.SetState(VMStateIdle); err != nil {
		_ = vm.Stop(ctx)
		return
	}

	select {
	case p.ready <- vm:
		p.metrics.SetWarmPoolReady(len(p.ready))
	default:
		_ = vm.Stop(ctx)
	}
}

func (p *WarmPool) Close(ctx context.Context) {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	close(p.ready)
	p.mu.Unlock()

	for vm := range p.ready {
		_ = vm.SetState(VMStateEvicted)
		_ = vm.Stop(ctx)
	}
	p.metrics.SetWarmPoolReady(0)
}

func (p *WarmPool) evict(ctx context.Context, vm WarmVM, reason string) {
	p.mu.Lock()
	p.evicted++
	p.mu.Unlock()
	p.metrics.AddWarmEviction()

	_ = vm.SetState(VMStateEvicted)
	_ = vm.Stop(ctx)
	if p.logger != nil {
		p.logger.Warn("evicted warm vm", "vmId", vm.ID(), "reason", reason)
	}
	go p.Replace(context.Background())
}

func (p *WarmPool) ReadyCount() int {
	return len(p.ready)
}

func (p *WarmPool) EvictedCount() uint64 {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.evicted
}

type WarmLease struct {
	pool *WarmPool
	vm   WarmVM
	once sync.Once
}

func (l *WarmLease) VM() WarmVM {
	if l == nil {
		return nil
	}
	return l.vm
}

func (l *WarmLease) Release(ctx context.Context) {
	if l == nil || l.pool == nil || l.vm == nil {
		return
	}
	l.once.Do(func() {
		l.pool.Return(ctx, l.vm)
	})
}

func (l *WarmLease) Evict(ctx context.Context, reason string) {
	if l == nil || l.pool == nil || l.vm == nil {
		return
	}
	l.once.Do(func() {
		l.pool.evict(ctx, l.vm, reason)
	})
}
