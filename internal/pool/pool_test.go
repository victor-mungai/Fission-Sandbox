package pool

import (
	"context"
	"testing"
	"time"

	"fission-sandbox/internal/metrics"
)

func TestPoolLimitsConcurrentLeases(t *testing.T) {
	p := New(1, metrics.New())

	lease, err := p.Acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire first lease: %v", err)
	}
	defer lease.Release()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()

	if _, err := p.Acquire(ctx); err == nil {
		t.Fatal("expected second lease to time out")
	}

	lease.Release()
	if _, err := p.Acquire(context.Background()); err != nil {
		t.Fatalf("acquire after release: %v", err)
	}
}
