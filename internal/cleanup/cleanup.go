package cleanup

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func StartArtifactCleaner(workDir string, retentionHours int, intervalSeconds int, logger *slog.Logger) {
	if strings.TrimSpace(workDir) == "" || retentionHours < 0 || intervalSeconds <= 0 {
		return
	}

	retention := time.Duration(retentionHours) * time.Hour
	interval := time.Duration(intervalSeconds) * time.Second

	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			cleanArtifacts(workDir, retention, logger)
			<-ticker.C
		}
	}()
}

func cleanArtifacts(workDir string, retention time.Duration, logger *slog.Logger) {
	entries, err := os.ReadDir(workDir)
	if err != nil {
		if !os.IsNotExist(err) && logger != nil {
			logger.Warn("artifact cleanup failed to read workdir", "workDir", workDir, "error", err)
		}
		return
	}

	now := time.Now()
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "fc-") {
			continue
		}

		path := filepath.Join(workDir, entry.Name())
		info, err := entry.Info()
		if err != nil {
			continue
		}
		if now.Sub(info.ModTime()) < retention {
			continue
		}

		if err := os.RemoveAll(path); err != nil {
			if logger != nil {
				logger.Warn("artifact cleanup failed", "path", path, "error", err)
			}
			continue
		}
		if logger != nil {
			logger.Info("removed expired firecracker artifact", "path", path)
		}
	}
}
