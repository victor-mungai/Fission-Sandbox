package metrics

import (
	"log/slog"
	"runtime"
	"time"
)

func StartMemoryLogger(intervalSeconds int, logger *slog.Logger) {
	if intervalSeconds <= 0 || logger == nil {
		return
	}

	interval := time.Duration(intervalSeconds) * time.Second
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for range ticker.C {
			var mem runtime.MemStats
			runtime.ReadMemStats(&mem)
			logger.Info("runtime memory stats",
				"heapAllocBytes", mem.HeapAlloc,
				"heapSysBytes", mem.HeapSys,
				"numGC", mem.NumGC,
				"pauseTotalNs", mem.PauseTotalNs,
				"goroutines", runtime.NumGoroutine(),
			)
		}
	}()
}
