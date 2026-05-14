package runtime

import (
	"context"
	"time"

	"fission-sandbox/internal/models"
)

type MockRuntime struct{}

func NewMockRuntime() *MockRuntime {
	return &MockRuntime{}
}

func (r *MockRuntime) Execute(ctx context.Context, req models.RunRequest) (Result, error) {
	select {
	case <-ctx.Done():
		return Result{
			Stderr:   ctx.Err().Error(),
			ExitCode: 1,
			TimedOut: false,
			VMID:     "mock-vm-001",
		}, ctx.Err()
	case <-time.After(500 * time.Millisecond):
	}

	return Result{
		Stdout:   "mock output from: " + req.Command,
		Stderr:   "",
		ExitCode: 0,
		TimedOut: false,
		VMID:     "mock-vm-001",
	}, nil
}
