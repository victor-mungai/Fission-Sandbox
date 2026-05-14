package api

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"

	"fission-sandbox/internal/config"
	"fission-sandbox/internal/models"
)

func TestHandleRunReturnsMockResponse(t *testing.T) {
	server := NewServer(testConfig(), slog.Default())
	body := []byte(`{
		"runId": "test-1",
		"command": "echo hello",
		"timeoutMs": 30000,
		"memoryMb": 128,
		"cpuCount": 0.25
	}`)

	req := httptest.NewRequest(http.MethodPost, "/run", bytes.NewReader(body))
	req.Header.Set("x-sandbox-auth", "dev-secret-token")
	rec := httptest.NewRecorder()

	server.HandleRun(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rec.Code)
	}

	var response models.RunResponse
	if err := json.NewDecoder(rec.Body).Decode(&response); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	if response.Stdout != "mock output from: echo hello" {
		t.Fatalf("unexpected stdout: %q", response.Stdout)
	}
	if response.ExitCode != 0 {
		t.Fatalf("unexpected exit code: %d", response.ExitCode)
	}
	if response.VMId != "mock-vm-001" {
		t.Fatalf("unexpected vm id: %q", response.VMId)
	}
}

func TestHandleRunRejectsUnauthorizedRequests(t *testing.T) {
	server := NewServer(testConfig(), slog.Default())
	req := httptest.NewRequest(http.MethodPost, "/run", bytes.NewReader([]byte(`{}`)))
	rec := httptest.NewRecorder()

	server.HandleRun(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

func TestHandleRunRejectsInvalidPayload(t *testing.T) {
	server := NewServer(testConfig(), slog.Default())
	req := httptest.NewRequest(http.MethodPost, "/run", bytes.NewReader([]byte(`{"runId":"test-1"}`)))
	req.Header.Set("x-sandbox-auth", "dev-secret-token")
	rec := httptest.NewRecorder()

	server.HandleRun(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected status %d, got %d", http.StatusBadRequest, rec.Code)
	}
}

func testConfig() config.Config {
	return config.Config{
		Server: config.ServerConfig{
			Port: "8080",
		},
		Auth: config.AuthConfig{
			Token: "dev-secret-token",
		},
		Limits: config.LimitsConfig{
			TimeoutMs: 30000,
			MemoryMb:  128,
			CPUCount:  0.25,
		},
		Executor: config.ExecutorConfig{
			Mode: "mock",
		},
	}
}
