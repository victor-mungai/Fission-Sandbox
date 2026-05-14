package runtime

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"sync"
	"time"

	"fission-sandbox/internal/config"
	"fission-sandbox/internal/models"
)

const (
	markerPrefix = "FISSION_RESULT "
	stdoutFile   = "stdout.txt"
	stderrFile   = "stderr.txt"
	exitCodeFile = "exitcode"
	filesTarName = "files.tar"
	commandName  = "command.sh"
)

var firecrackerVMLock sync.Mutex

type FirecrackerRuntime struct {
	config config.FirecrackerConfig
	logger *slog.Logger
}

type firecrackerDrive struct {
	DriveID      string `json:"drive_id"`
	PathOnHost   string `json:"path_on_host"`
	IsRootDevice bool   `json:"is_root_device"`
	IsReadOnly   bool   `json:"is_read_only"`
}

func NewFirecrackerRuntime(cfg config.FirecrackerConfig, logger *slog.Logger) *FirecrackerRuntime {
	return &FirecrackerRuntime{
		config: cfg,
		logger: logger,
	}
}

func (r *FirecrackerRuntime) Execute(ctx context.Context, req models.RunRequest) (Result, error) {
	firecrackerVMLock.Lock()
	defer firecrackerVMLock.Unlock()

	if goruntime.GOOS != "linux" {
		return Result{ExitCode: 1, VMID: "unsupported-host"}, errors.New("firecracker runtime requires a Linux host with KVM")
	}

	timeout := time.Duration(req.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = time.Duration(r.config.DefaultTimeoutMs) * time.Millisecond
	}

	vmCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	vmID := "fc-" + sanitizeRunID(req.RunID) + "-" + time.Now().UTC().Format("20060102150405")
	workDir := filepath.Join(r.config.WorkDir, vmID)
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		return Result{ExitCode: 1, VMID: vmID}, err
	}
	defer os.RemoveAll(workDir)

	workspaceDir := filepath.Join(workDir, "workspace")
	if err := os.MkdirAll(workspaceDir, 0o700); err != nil {
		return Result{ExitCode: 1, VMID: vmID}, err
	}

	if err := writeWorkspace(workspaceDir, req); err != nil {
		return Result{ExitCode: 1, VMID: vmID}, err
	}

	workspaceImage := filepath.Join(workDir, "workspace.ext4")
	if err := buildWorkspaceImage(vmCtx, workspaceDir, workspaceImage, r.config.WorkspaceImageMb); err != nil {
		return Result{ExitCode: 1, VMID: vmID}, err
	}

	socketPath := filepath.Join(workDir, "firecracker.sock")
	serialPath := filepath.Join(workDir, "serial.log")
	logPath := filepath.Join(workDir, "firecracker.log")

	serialFile, err := os.Create(serialPath)
	if err != nil {
		return Result{ExitCode: 1, VMID: vmID}, err
	}
	defer serialFile.Close()

	firecrackerCmd := exec.CommandContext(vmCtx, r.config.BinaryPath, "--api-sock", socketPath)
	firecrackerCmd.Stdout = serialFile
	firecrackerCmd.Stderr = io.Discard

	if err := firecrackerCmd.Start(); err != nil {
		return Result{ExitCode: 1, VMID: vmID}, fmt.Errorf("start firecracker: %w", err)
	}
	defer stopProcess(firecrackerCmd.Process)

	if err := waitForSocket(vmCtx, socketPath); err != nil {
		return Result{ExitCode: 1, VMID: vmID}, err
	}

	client := newFirecrackerClient(socketPath)
	if err := r.configureVM(vmCtx, client, req, logPath, workspaceImage); err != nil {
		return Result{ExitCode: 1, VMID: vmID}, err
	}

	if err := putJSON(vmCtx, client, "/actions", map[string]string{"action_type": "InstanceStart"}); err != nil {
		return Result{ExitCode: 1, VMID: vmID}, err
	}

	result, err := waitForResult(vmCtx, serialPath, vmID)
	if err != nil {
		timedOut := errors.Is(vmCtx.Err(), context.DeadlineExceeded)
		return Result{
			Stderr:   err.Error(),
			ExitCode: 124,
			TimedOut: timedOut,
			VMID:     vmID,
		}, err
	}

	return result, nil
}

func (r *FirecrackerRuntime) configureVM(ctx context.Context, client *http.Client, req models.RunRequest, logPath string, workspaceImage string) error {
	vcpuCount := int(math.Ceil(req.CPUCount))
	if vcpuCount < 1 {
		vcpuCount = 1
	}

	if err := putJSON(ctx, client, "/logger", map[string]any{
		"log_path":        logPath,
		"level":           "Info",
		"show_level":      true,
		"show_log_origin": false,
	}); err != nil {
		return err
	}

	if err := putJSON(ctx, client, "/machine-config", map[string]any{
		"vcpu_count":   vcpuCount,
		"mem_size_mib": req.MemoryMb,
		"smt":          false,
	}); err != nil {
		return err
	}

	if err := putJSON(ctx, client, "/boot-source", map[string]string{
		"kernel_image_path": r.config.KernelImage,
		"boot_args":         strings.TrimSpace(r.config.KernelArgs + " init=/sbin/fission-init"),
	}); err != nil {
		return err
	}

	if err := putJSON(ctx, client, "/drives/rootfs", firecrackerDrive{
		DriveID:      "rootfs",
		PathOnHost:   r.config.RootfsImage,
		IsRootDevice: true,
		IsReadOnly:   true,
	}); err != nil {
		return err
	}

	return putJSON(ctx, client, "/drives/workspace", firecrackerDrive{
		DriveID:      "workspace",
		PathOnHost:   workspaceImage,
		IsRootDevice: false,
		IsReadOnly:   false,
	})
}

func writeWorkspace(dir string, req models.RunRequest) error {
	filesTarPath := filepath.Join(dir, filesTarName)
	filesTar, err := os.Create(filesTarPath)
	if err != nil {
		return err
	}

	tw := tar.NewWriter(filesTar)
	for _, file := range req.Files {
		mode := int64(file.Mode)
		if mode == 0 {
			mode = 0o644
		}
		content := []byte(file.Content)
		if err := tw.WriteHeader(&tar.Header{
			Name: filepath.ToSlash(file.Name),
			Mode: mode,
			Size: int64(len(content)),
		}); err != nil {
			filesTar.Close()
			return err
		}
		if _, err := tw.Write(content); err != nil {
			filesTar.Close()
			return err
		}
	}
	if err := tw.Close(); err != nil {
		filesTar.Close()
		return err
	}
	if err := filesTar.Close(); err != nil {
		return err
	}

	command := "#!/bin/bash\nset +e\ncd /work/files\n" + req.Command + "\n"
	return os.WriteFile(filepath.Join(dir, commandName), []byte(command), 0o755)
}

func buildWorkspaceImage(ctx context.Context, srcDir string, imagePath string, sizeMb int) error {
	if sizeMb <= 0 {
		sizeMb = 64
	}

	imageFile, err := os.Create(imagePath)
	if err != nil {
		return err
	}
	if err := imageFile.Truncate(int64(sizeMb) * 1024 * 1024); err != nil {
		imageFile.Close()
		return err
	}
	if err := imageFile.Close(); err != nil {
		return err
	}

	cmd := exec.CommandContext(ctx, "mkfs.ext4", "-q", "-F", "-d", srcDir, imagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("mkfs.ext4 workspace image: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func waitForResult(ctx context.Context, serialPath string, vmID string) (Result, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return Result{}, ctx.Err()
		case <-ticker.C:
			data, err := os.ReadFile(serialPath)
			if err != nil && !errors.Is(err, os.ErrNotExist) {
				return Result{}, err
			}

			if result, ok := parseSerialResult(string(data), vmID); ok {
				return result, nil
			}
		}
	}
}

func parseSerialResult(data string, vmID string) (Result, bool) {
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, markerPrefix) {
			continue
		}

		payload := strings.TrimPrefix(line, markerPrefix)
		var encoded struct {
			Stdout   string `json:"stdout"`
			Stderr   string `json:"stderr"`
			ExitCode int    `json:"exitCode"`
		}
		raw, err := base64.StdEncoding.DecodeString(payload)
		if err != nil {
			return Result{}, false
		}
		if err := json.Unmarshal(raw, &encoded); err != nil {
			return Result{}, false
		}
		return Result{
			Stdout:   encoded.Stdout,
			Stderr:   encoded.Stderr,
			ExitCode: encoded.ExitCode,
			TimedOut: false,
			VMID:     vmID,
		}, true
	}
	return Result{}, false
}

func putJSON(ctx context.Context, client *http.Client, path string, body any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, "http://firecracker"+path, bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		responseBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("firecracker api %s failed: status=%d body=%s", path, resp.StatusCode, strings.TrimSpace(string(responseBody)))
	}
	return nil
}

func newFirecrackerClient(socketPath string) *http.Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network string, address string) (net.Conn, error) {
			dialer := net.Dialer{}
			return dialer.DialContext(ctx, "unix", socketPath)
		},
	}

	return &http.Client{
		Transport: transport,
		Timeout:   10 * time.Second,
	}
}

func waitForSocket(ctx context.Context, socketPath string) error {
	ticker := time.NewTicker(25 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			if _, err := os.Stat(socketPath); err == nil {
				return nil
			}
		}
	}
}

func sanitizeRunID(runID string) string {
	var builder strings.Builder
	for _, r := range runID {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' {
			builder.WriteRune(r)
		}
	}
	if builder.Len() == 0 {
		return "run"
	}
	return builder.String()
}

func stopProcess(process *os.Process) {
	if process == nil {
		return
	}
	_ = process.Kill()
	_, _ = process.Wait()
}
