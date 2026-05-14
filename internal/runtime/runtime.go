package runtime

import (
	"context"

	"fission-sandbox/internal/models"
)

type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
	TimedOut bool
	VMID     string
}

type Runtime interface {
	Execute(ctx context.Context, req models.RunRequest) (Result, error)
}
