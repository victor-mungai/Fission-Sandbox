package main

import (
	"log/slog"
	"net/http"
	"os"

	"fission-sandbox/internal/api"
	"fission-sandbox/internal/config"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		logger.Error("invalid configuration", "error", err)
		os.Exit(1)
	}

	server := api.NewServer(cfg, logger)

	mux := http.NewServeMux()
	mux.HandleFunc("/run", server.HandleRun)

	addr := ":" + cfg.Server.Port
	logger.Info("fission sandbox api starting", "addr", addr)

	if err := http.ListenAndServe(addr, mux); err != nil {
		logger.Error("api server stopped", "error", err)
		os.Exit(1)
	}
}
