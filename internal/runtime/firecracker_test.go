package runtime

import (
	"archive/tar"
	"encoding/base64"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"fission-sandbox/internal/models"
)

func TestParseSerialResultExtractsMarkerPayload(t *testing.T) {
	payload := map[string]any{
		"stdout":   "hello\n",
		"stderr":   "",
		"exitCode": 7,
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}

	serial := strings.Join([]string{
		"boot log line",
		markerPrefix + base64.StdEncoding.EncodeToString(raw),
		"",
	}, "\n")

	result, ok := parseSerialResult(serial, "fc-test")
	if !ok {
		t.Fatal("expected marker to be parsed")
	}
	if result.Stdout != "hello\n" {
		t.Fatalf("unexpected stdout: %q", result.Stdout)
	}
	if result.Stderr != "" {
		t.Fatalf("unexpected stderr: %q", result.Stderr)
	}
	if result.ExitCode != 7 {
		t.Fatalf("unexpected exit code: %d", result.ExitCode)
	}
	if result.VMID != "fc-test" {
		t.Fatalf("unexpected vm id: %q", result.VMID)
	}
}

func TestParseSerialResultRejectsInvalidPayload(t *testing.T) {
	serial := markerPrefix + "not-base64"
	if _, ok := parseSerialResult(serial, "fc-test"); ok {
		t.Fatal("expected invalid payload to be rejected")
	}
}

func TestReadSerialChunkReadsOnlyNewData(t *testing.T) {
	serialPath := filepath.Join(t.TempDir(), "serial.log")
	if err := os.WriteFile(serialPath, []byte("first\n"), 0o600); err != nil {
		t.Fatalf("write serial: %v", err)
	}

	data, offset, err := readSerialChunk(serialPath, 0)
	if err != nil {
		t.Fatalf("read first chunk: %v", err)
	}
	if string(data) != "first\n" {
		t.Fatalf("unexpected first chunk: %q", string(data))
	}

	file, err := os.OpenFile(serialPath, os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("open serial append: %v", err)
	}
	if _, err := file.WriteString("second\n"); err != nil {
		file.Close()
		t.Fatalf("append serial: %v", err)
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close serial: %v", err)
	}

	data, _, err = readSerialChunk(serialPath, offset)
	if err != nil {
		t.Fatalf("read second chunk: %v", err)
	}
	if string(data) != "second\n" {
		t.Fatalf("unexpected second chunk: %q", string(data))
	}
}

func TestWriteWorkspaceCreatesExpectedFiles(t *testing.T) {
	dir := t.TempDir()
	req := models.RunRequest{
		Command: "echo hello",
		Files: []models.File{
			{Name: "main.py", Content: "print('ok')\n", Mode: 0o755},
			{Name: "nested/data.txt", Content: "payload", Mode: 0},
		},
	}

	if err := writeWorkspace(dir, req); err != nil {
		t.Fatalf("writeWorkspace: %v", err)
	}

	commandBytes, err := os.ReadFile(filepath.Join(dir, commandName))
	if err != nil {
		t.Fatalf("read command script: %v", err)
	}
	command := string(commandBytes)
	if !strings.Contains(command, "cd /work/files") || !strings.Contains(command, "echo hello") {
		t.Fatalf("unexpected command script: %q", command)
	}

	filesTar, err := os.Open(filepath.Join(dir, filesTarName))
	if err != nil {
		t.Fatalf("open files tar: %v", err)
	}
	defer filesTar.Close()

	tr := tar.NewReader(filesTar)
	headers := map[string]int64{}
	for {
		header, err := tr.Next()
		if err != nil {
			break
		}
		headers[header.Name] = header.Mode
	}

	if headers["main.py"] != 0o755 {
		t.Fatalf("unexpected mode for main.py: %o", headers["main.py"])
	}
	if headers["nested/data.txt"] != 0o644 {
		t.Fatalf("unexpected default mode for nested/data.txt: %o", headers["nested/data.txt"])
	}
}

func TestWriteWorkspaceRejectsTraversalFileNames(t *testing.T) {
	dir := t.TempDir()
	req := models.RunRequest{
		Command: "echo hello",
		Files: []models.File{
			{Name: "../escape.txt", Content: "bad"},
		},
	}

	err := writeWorkspace(dir, req)
	if err == nil {
		t.Fatal("expected traversal file name to be rejected")
	}
	if !strings.Contains(err.Error(), "relative path without traversal") {
		t.Fatalf("unexpected error: %v", err)
	}
}
