package pool

import (
	"context"
	"errors"
	"sync"
	"testing"

	"fission-sandbox/internal/metrics"
)

type fakeWarmVM struct {
	mu          sync.Mutex
	id          string
	state       VMState
	resetCalls  int
	stopCalls   int
	resetErr    error
	healthyErr  error
	transitions []VMState
}

func (v *fakeWarmVM) ID() string {
	return v.id
}

func (v *fakeWarmVM) State() VMState {
	v.mu.Lock()
	defer v.mu.Unlock()
	return v.state
}

func (v *fakeWarmVM) SetState(state VMState) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.state != "" {
		if err := validateVMStateTransition(v.state, state); err != nil {
			return err
		}
	}
	v.state = state
	v.transitions = append(v.transitions, state)
	return nil
}

func (v *fakeWarmVM) Execute(context.Context, any) (any, error) {
	return nil, nil
}

func (v *fakeWarmVM) Reset(context.Context) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.resetCalls++
	return v.resetErr
}

func (v *fakeWarmVM) Healthy(context.Context) error {
	return v.healthyErr
}

func (v *fakeWarmVM) Stop(context.Context) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.stopCalls++
	return nil
}

func TestVMStateTransitions(t *testing.T) {
	valid := []struct {
		from VMState
		to   VMState
	}{
		{VMStateIdle, VMStateBusy},
		{VMStateBusy, VMStateResetting},
		{VMStateResetting, VMStateIdle},
		{VMStateBusy, VMStateEvicted},
	}
	for _, transition := range valid {
		if err := validateVMStateTransition(transition.from, transition.to); err != nil {
			t.Fatalf("expected valid transition %s -> %s: %v", transition.from, transition.to, err)
		}
	}

	if err := validateVMStateTransition(VMStateIdle, VMStateResetting); err == nil {
		t.Fatal("expected invalid transition to be rejected")
	}
}

func TestWarmPoolLeaseResetAndReturn(t *testing.T) {
	ctx := context.Background()
	vm := &fakeWarmVM{id: "warm-1"}
	p, err := NewWarmPool(ctx, 1, func(context.Context) (WarmVM, error) {
		return vm, nil
	}, metrics.New(), nil)
	if err != nil {
		t.Fatalf("new warm pool: %v", err)
	}

	lease, err := p.Lease(ctx)
	if err != nil {
		t.Fatalf("lease warm vm: %v", err)
	}
	if lease.VM().State() != VMStateBusy {
		t.Fatalf("expected busy vm, got %s", lease.VM().State())
	}

	lease.Release(ctx)
	if vm.State() != VMStateIdle {
		t.Fatalf("expected idle vm after release, got %s", vm.State())
	}
	if vm.resetCalls != 1 {
		t.Fatalf("expected one reset call, got %d", vm.resetCalls)
	}
	if p.ReadyCount() != 1 {
		t.Fatalf("expected ready count 1, got %d", p.ReadyCount())
	}
}

func TestWarmPoolEvictsOnResetFailure(t *testing.T) {
	ctx := context.Background()
	broken := &fakeWarmVM{id: "broken", resetErr: errors.New("reset failed")}
	replacement := &fakeWarmVM{id: "replacement"}
	calls := 0

	p, err := NewWarmPool(ctx, 1, func(context.Context) (WarmVM, error) {
		calls++
		if calls == 1 {
			return broken, nil
		}
		return replacement, nil
	}, metrics.New(), nil)
	if err != nil {
		t.Fatalf("new warm pool: %v", err)
	}

	lease, err := p.Lease(ctx)
	if err != nil {
		t.Fatalf("lease warm vm: %v", err)
	}
	lease.Release(ctx)

	if broken.State() != VMStateEvicted {
		t.Fatalf("expected broken vm to be evicted, got %s", broken.State())
	}
	if broken.stopCalls != 1 {
		t.Fatalf("expected broken vm to be stopped, got %d", broken.stopCalls)
	}
	if p.EvictedCount() != 1 {
		t.Fatalf("expected one eviction, got %d", p.EvictedCount())
	}
}
