package api

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"fission-sandbox/internal/auth"
	"fission-sandbox/internal/config"
	"fission-sandbox/internal/executor"
	"fission-sandbox/internal/models"
)

type Server struct {
	exec   *executor.Executor
	auth   *auth.Middleware
	config config.Config
	logger *slog.Logger
}

func NewServer(cfg config.Config, logger *slog.Logger) *Server {
	return &Server{
		exec:   executor.New(cfg, logger),
		auth:   auth.New(cfg.Auth.Token),
		config: cfg,
		logger: logger,
	}
}

func (s *Server) HandleRun(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeError(w, http.StatusMethodNotAllowed, "method not allowed")
		return
	}

	if !s.auth.Validate(r) {
		s.logger.Warn("run request rejected", "reason", "unauthorized")
		writeError(w, http.StatusUnauthorized, "unauthorized")
		return
	}

	var req models.RunRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(&req); err != nil {
		s.logger.Warn("run request rejected", "reason", "invalid_json", "error", err)
		writeError(w, http.StatusBadRequest, "bad request")
		return
	}

	if err := validateRunRequest(req); err != nil {
		s.logger.Warn("run request rejected", "reason", "validation_failed", "error", err, "runId", req.RunID)
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.applyDefaults(&req)

	res := s.exec.Run(r.Context(), req)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(res); err != nil {
		s.logger.Error("failed to write run response", "error", err, "runId", req.RunID)
	}
}

func (s *Server) applyDefaults(req *models.RunRequest) {
	if req.TimeoutMs == 0 {
		req.TimeoutMs = s.config.Limits.TimeoutMs
	}
	if req.MemoryMb == 0 {
		req.MemoryMb = s.config.Limits.MemoryMb
	}
	if req.CPUCount == 0 {
		req.CPUCount = s.config.Limits.CPUCount
	}
}

func validateRunRequest(req models.RunRequest) error {
	if strings.TrimSpace(req.RunID) == "" {
		return errors.New("runId is required")
	}
	if strings.TrimSpace(req.Command) == "" {
		return errors.New("command is required")
	}
	if req.TimeoutMs < 0 {
		return errors.New("timeoutMs must be greater than or equal to 0")
	}
	if req.MemoryMb < 0 {
		return errors.New("memoryMb must be greater than or equal to 0")
	}
	if req.CPUCount < 0 {
		return errors.New("cpuCount must be greater than or equal to 0")
	}
	for _, file := range req.Files {
		name := strings.TrimSpace(file.Name)
		if name == "" {
			return errors.New("file name is required")
		}
		if strings.HasPrefix(name, "/") || strings.Contains(name, "..") || strings.Contains(name, "\\") {
			return errors.New("file name must be a relative path without traversal")
		}
	}
	return nil
}

func writeError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(models.ErrorResponse{
		Error: message,
	})
}
