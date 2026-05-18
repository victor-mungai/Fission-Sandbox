package executor

import (
	"context"
	"log/slog"
	"time"

	"fission-sandbox/internal/config"
	"fission-sandbox/internal/metrics"
	"fission-sandbox/internal/models"
	sandboxruntime "fission-sandbox/internal/runtime"
)

type Executor struct {
	logger  *slog.Logger
	runtime sandboxruntime.Runtime
}

func New(cfg config.Config, logger *slog.Logger) *Executor {
	var runtime sandboxruntime.Runtime
	if cfg.Executor.Mode == "firecracker" {
		runtime = sandboxruntime.NewFirecrackerRuntime(cfg.Firecracker, cfg.Runtime, metrics.Default, logger)
	} else {
		runtime = sandboxruntime.NewMockRuntime()
	}

	return &Executor{
		logger:  logger,
		runtime: runtime,
	}
}

func (e *Executor) Run(ctx context.Context, req models.RunRequest) models.RunResponse {
	start := time.Now()

	e.logger.Info("execution started", "runId", req.RunID, "command", req.Command)

	durationMs := int(time.Since(start).Milliseconds())
	result, err := e.runtime.Execute(ctx, req)
	if err != nil {
		durationMs = int(time.Since(start).Milliseconds())
		e.logger.Warn("execution failed", "runId", req.RunID, "durationMs", durationMs, "error", err)
		metrics.Default.RecordRun(time.Since(start), result.ExitCode, result.TimedOut, true)
		return models.RunResponse{
			Stdout:     result.Stdout,
			Stderr:     result.Stderr,
			ExitCode:   result.ExitCode,
			DurationMs: durationMs,
			TimedOut:   result.TimedOut,
			VMId:       result.VMID,
		}
	}

	durationMs = int(time.Since(start).Milliseconds())
	e.logger.Info("execution finished", "runId", req.RunID, "durationMs", durationMs, "exitCode", result.ExitCode, "vmId", result.VMID)
	metrics.Default.RecordRun(time.Since(start), result.ExitCode, result.TimedOut, false)

	return models.RunResponse{
		Stdout:     result.Stdout,
		Stderr:     result.Stderr,
		ExitCode:   result.ExitCode,
		DurationMs: durationMs,
		TimedOut:   result.TimedOut,
		VMId:       result.VMID,
	}
}
