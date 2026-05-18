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
	"time"

	"fission-sandbox/internal/config"
	"fission-sandbox/internal/metrics"
	"fission-sandbox/internal/models"
	"fission-sandbox/internal/pool"
)

const (
	markerPrefix         = "FISSION_RESULT "
	stdoutFile           = "stdout.txt"
	stderrFile           = "stderr.txt"
	filesTarName         = "files.tar"
	commandName          = "command.sh"
	maxSerialBufferBytes = 1024 * 1024
)

type FirecrackerRuntime struct {
	config  config.FirecrackerConfig
	runtime config.RuntimeConfig
	metrics *metrics.Metrics
	pool    *pool.Pool
	logger  *slog.Logger
}

type firecrackerDrive struct {
	DriveID      string `json:"drive_id"`
	PathOnHost   string `json:"path_on_host"`
	IsRootDevice bool   `json:"is_root_device"`
	IsReadOnly   bool   `json:"is_read_only"`
}

func NewFirecrackerRuntime(cfg config.FirecrackerConfig, runtimeCfg config.RuntimeConfig, runtimeMetrics *metrics.Metrics, logger *slog.Logger) *FirecrackerRuntime {
	maxConcurrent := runtimeCfg.MaxConcurrentVMs
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}
	if runtimeMetrics == nil {
		runtimeMetrics = metrics.Default
	}

	return &FirecrackerRuntime{
		config:  cfg,
		runtime: runtimeCfg,
		metrics: runtimeMetrics,
		pool:    pool.New(maxConcurrent, runtimeMetrics),
		logger:  logger,
	}
}

func (r *FirecrackerRuntime) Execute(ctx context.Context, req models.RunRequest) (Result, error) {
	if goruntime.GOOS != "linux" {
		return Result{ExitCode: 1, VMID: "unsupported-host"}, errors.New("firecracker runtime requires a Linux host with KVM")
	}

	timeout := time.Duration(req.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = time.Duration(r.config.DefaultTimeoutMs) * time.Millisecond
	}

	vmCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	lease, err := r.pool.Acquire(vmCtx)
	if err != nil {
		return Result{ExitCode: 124, TimedOut: errors.Is(vmCtx.Err(), context.DeadlineExceeded), VMID: "queue-timeout"}, err
	}
	defer lease.Release()

	vmID := "fc-" + sanitizeRunID(req.RunID) + "-" + time.Now().UTC().Format("20060102150405")
	workDir := filepath.Join(r.config.WorkDir, vmID)
	if err := os.MkdirAll(workDir, 0o700); err != nil {
		return Result{ExitCode: 1, VMID: vmID}, err
	}
	removeWorkDir := true
	preserveReason := "failed execution"
	defer func() {
		if removeWorkDir {
			_ = os.RemoveAll(workDir)
			return
		}
		if r.logger != nil {
			r.logger.Warn("preserving firecracker workdir", "runId", req.RunID, "vmId", vmID, "reason", preserveReason, "workDir", workDir)
		}
	}()
	fail := func(result Result, err error) (Result, error) {
		removeWorkDir = false
		return result, err
	}

	workspaceDir := filepath.Join(workDir, "workspace")
	if err := os.MkdirAll(workspaceDir, 0o700); err != nil {
		return fail(Result{ExitCode: 1, VMID: vmID}, err)
	}

	if err := writeWorkspace(workspaceDir, req); err != nil {
		return fail(Result{ExitCode: 1, VMID: vmID}, err)
	}

	workspaceImage := filepath.Join(workDir, "workspace.ext4")
	copiedTemplate, err := buildWorkspaceImage(vmCtx, workspaceDir, workspaceImage, r.config.WorkspaceImageMb, r.config.WorkspaceTemplateImage)
	if err != nil {
		return fail(Result{ExitCode: 1, VMID: vmID}, err)
	}
	r.metrics.RecordWorkspaceBuild(copiedTemplate)

	socketPath := filepath.Join(workDir, "firecracker.sock")
	serialPath := filepath.Join(workDir, "serial.log")
	logPath := filepath.Join(workDir, "firecracker.log")
	stderrPath := filepath.Join(workDir, "firecracker.stderr.log")

	serialFile, err := os.Create(serialPath)
	if err != nil {
		return fail(Result{ExitCode: 1, VMID: vmID}, err)
	}
	defer serialFile.Close()

	stderrFile, err := os.Create(stderrPath)
	if err != nil {
		return fail(Result{ExitCode: 1, VMID: vmID}, err)
	}
	defer stderrFile.Close()

	firecrackerCmd := exec.CommandContext(vmCtx, r.config.BinaryPath, "--api-sock", socketPath)
	firecrackerCmd.Stdout = serialFile
	firecrackerCmd.Stderr = stderrFile

	bootStart := time.Now()
	if err := firecrackerCmd.Start(); err != nil {
		return fail(Result{ExitCode: 1, VMID: vmID}, fmt.Errorf("start firecracker: %w", err))
	}
	defer stopProcess(firecrackerCmd.Process)

	if err := waitForSocket(vmCtx, socketPath); err != nil {
		return fail(Result{ExitCode: 1, VMID: vmID}, err)
	}

	client := newFirecrackerClient(socketPath)
	if err := r.configureVM(vmCtx, client, req, logPath, workspaceImage); err != nil {
		return fail(Result{ExitCode: 1, VMID: vmID}, err)
	}

	if err := putJSON(vmCtx, client, "/actions", map[string]string{"action_type": "InstanceStart"}); err != nil {
		return fail(Result{ExitCode: 1, VMID: vmID}, err)
	}
	r.metrics.RecordVMBoot(time.Since(bootStart))

	result, err := waitForResult(vmCtx, serialPath, vmID)
	if err != nil {
		timedOut := errors.Is(vmCtx.Err(), context.DeadlineExceeded)
		return fail(Result{
			Stderr:   err.Error(),
			ExitCode: 124,
			TimedOut: timedOut,
			VMID:     vmID,
		}, err)
	}
	if result.ExitCode != 0 {
		removeWorkDir = false
		preserveReason = fmt.Sprintf("non-zero guest exit: %d", result.ExitCode)
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
		name, err := sanitizeWorkspaceFileName(file.Name)
		if err != nil {
			filesTar.Close()
			return err
		}
		mode := int64(file.Mode)
		if mode == 0 {
			mode = 0o644
		}
		content := []byte(file.Content)
		if err := tw.WriteHeader(&tar.Header{
			Name: name,
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

func sanitizeWorkspaceFileName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("file name is required")
	}
	if strings.HasPrefix(name, "/") || strings.Contains(name, "\\") {
		return "", errors.New("file name must be a relative path without traversal")
	}

	cleaned := filepath.Clean(name)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(os.PathSeparator)) {
		return "", errors.New("file name must be a relative path without traversal")
	}

	return filepath.ToSlash(cleaned), nil
}

func buildWorkspaceImage(ctx context.Context, srcDir string, imagePath string, sizeMb int, templatePath string) (bool, error) {
	if strings.TrimSpace(templatePath) != "" {
		if err := copyFile(templatePath, imagePath); err != nil {
			return false, fmt.Errorf("copy workspace template: %w", err)
		}
		if err := injectWorkspaceFiles(ctx, srcDir, imagePath); err != nil {
			return false, err
		}
		return true, nil
	}

	if sizeMb <= 0 {
		sizeMb = 64
	}

	imageFile, err := os.Create(imagePath)
	if err != nil {
		return false, err
	}
	if err := imageFile.Truncate(int64(sizeMb) * 1024 * 1024); err != nil {
		imageFile.Close()
		return false, err
	}
	if err := imageFile.Close(); err != nil {
		return false, err
	}

	cmd := exec.CommandContext(ctx, "mkfs.ext4", "-q", "-F", "-d", srcDir, imagePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return false, fmt.Errorf("mkfs.ext4 workspace image: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return false, nil
}

func copyFile(srcPath string, dstPath string) error {
	src, err := os.Open(srcPath)
	if err != nil {
		return err
	}
	defer src.Close()

	dst, err := os.OpenFile(dstPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer dst.Close()

	if _, err := io.Copy(dst, src); err != nil {
		return err
	}
	return dst.Sync()
}

func injectWorkspaceFiles(ctx context.Context, srcDir string, imagePath string) error {
	for _, name := range []string{filesTarName, commandName} {
		hostPath := filepath.Join(srcDir, name)
		cmd := exec.CommandContext(ctx, "debugfs", "-w", "-R", "write "+hostPath+" /"+name, imagePath)
		output, err := cmd.CombinedOutput()
		if err != nil {
			return fmt.Errorf("debugfs inject %s: %w: %s", name, err, strings.TrimSpace(string(output)))
		}
	}
	return nil
}

func waitForResult(ctx context.Context, serialPath string, vmID string) (Result, error) {
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()

	var offset int64
	var buffer string

	for {
		select {
		case <-ctx.Done():
			return Result{}, ctx.Err()
		case <-ticker.C:
			data, nextOffset, err := readSerialChunk(serialPath, offset)
			if err != nil {
				return Result{}, err
			}
			offset = nextOffset
			buffer += string(data)
			if len(buffer) > maxSerialBufferBytes {
				buffer = buffer[len(buffer)-maxSerialBufferBytes:]
			}

			if result, ok := parseSerialResult(buffer, vmID); ok {
				return result, nil
			}
		}
	}
}

func readSerialChunk(serialPath string, offset int64) ([]byte, int64, error) {
	file, err := os.Open(serialPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, offset, nil
		}
		return nil, offset, err
	}
	defer file.Close()

	info, err := file.Stat()
	if err != nil {
		return nil, offset, err
	}
	if info.Size() < offset {
		offset = 0
	}
	if _, err := file.Seek(offset, io.SeekStart); err != nil {
		return nil, offset, err
	}

	data, err := io.ReadAll(file)
	if err != nil {
		return nil, offset, err
	}
	return data, offset + int64(len(data)), nil
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
