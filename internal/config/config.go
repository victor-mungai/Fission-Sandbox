package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Server      ServerConfig
	Auth        AuthConfig
	Limits      LimitsConfig
	Executor    ExecutorConfig
	Firecracker FirecrackerConfig
	Runtime     RuntimeConfig
}

type ServerConfig struct {
	Port string
}

type AuthConfig struct {
	Token string
}

type LimitsConfig struct {
	TimeoutMs int
	MemoryMb  int
	CPUCount  float64
}

type ExecutorConfig struct {
	Mode string
}

type FirecrackerConfig struct {
	BinaryPath             string
	KernelImage            string
	KernelArgs             string
	RootfsImage            string
	WorkDir                string
	WorkspaceImageMb       int
	WorkspaceTemplateImage string
	DefaultTimeoutMs       int
}

type RuntimeConfig struct {
	MaxConcurrentVMs         int
	WarmPoolSize             int
	ArtifactRetentionHours   int
	CleanupIntervalSeconds   int
	MemoryLogIntervalSeconds int
}

func Load() Config {
	_ = LoadEnvFile(".env")

	return Config{
		Server: ServerConfig{
			Port: os.Getenv("PORT"),
		},
		Auth: AuthConfig{
			Token: os.Getenv("SANDBOX_AUTH_TOKEN"),
		},
		Limits: LimitsConfig{
			TimeoutMs: getEnvInt("SANDBOX_DEFAULT_TIMEOUT_MS"),
			MemoryMb:  getEnvInt("SANDBOX_DEFAULT_MEMORY_MB"),
			CPUCount:  getEnvFloat("SANDBOX_DEFAULT_CPU_COUNT"),
		},
		Executor: ExecutorConfig{
			Mode: os.Getenv("EXECUTOR_MODE"),
		},
		Firecracker: FirecrackerConfig{
			BinaryPath:             os.Getenv("FIRECRACKER_BIN"),
			KernelImage:            os.Getenv("FIRECRACKER_KERNEL_IMAGE"),
			KernelArgs:             os.Getenv("FIRECRACKER_KERNEL_ARGS"),
			RootfsImage:            os.Getenv("FIRECRACKER_ROOTFS_IMAGE"),
			WorkDir:                os.Getenv("FIRECRACKER_WORKDIR"),
			WorkspaceImageMb:       getEnvInt("FIRECRACKER_WORKSPACE_IMAGE_MB"),
			WorkspaceTemplateImage: os.Getenv("FIRECRACKER_WORKSPACE_TEMPLATE_IMAGE"),
			DefaultTimeoutMs:       getEnvInt("SANDBOX_DEFAULT_TIMEOUT_MS"),
		},
		Runtime: RuntimeConfig{
			MaxConcurrentVMs:         getEnvIntDefault("MAX_CONCURRENT_VMS", 1),
			WarmPoolSize:             getEnvIntDefault("WARM_POOL_SIZE", 0),
			ArtifactRetentionHours:   getEnvIntDefault("ARTIFACT_RETENTION_HOURS", 24),
			CleanupIntervalSeconds:   getEnvIntDefault("CLEANUP_INTERVAL_SECONDS", 300),
			MemoryLogIntervalSeconds: getEnvIntDefault("MEMORY_LOG_INTERVAL_SECONDS", 60),
		},
	}
}

func (c Config) Validate() error {
	var missing []string
	if strings.TrimSpace(c.Server.Port) == "" {
		missing = append(missing, "PORT")
	}
	if strings.TrimSpace(c.Auth.Token) == "" {
		missing = append(missing, "SANDBOX_AUTH_TOKEN")
	}
	if c.Limits.TimeoutMs <= 0 {
		missing = append(missing, "SANDBOX_DEFAULT_TIMEOUT_MS")
	}
	if c.Limits.MemoryMb <= 0 {
		missing = append(missing, "SANDBOX_DEFAULT_MEMORY_MB")
	}
	if c.Limits.CPUCount <= 0 {
		missing = append(missing, "SANDBOX_DEFAULT_CPU_COUNT")
	}
	if c.Executor.Mode == "" {
		missing = append(missing, "EXECUTOR_MODE")
	}
	if c.Executor.Mode != "" && c.Executor.Mode != "mock" && c.Executor.Mode != "firecracker" {
		missing = append(missing, "EXECUTOR_MODE must be mock or firecracker")
	}
	if c.Executor.Mode == "firecracker" {
		if strings.TrimSpace(c.Firecracker.BinaryPath) == "" {
			missing = append(missing, "FIRECRACKER_BIN")
		}
		if strings.TrimSpace(c.Firecracker.KernelImage) == "" {
			missing = append(missing, "FIRECRACKER_KERNEL_IMAGE")
		}
		if strings.TrimSpace(c.Firecracker.RootfsImage) == "" {
			missing = append(missing, "FIRECRACKER_ROOTFS_IMAGE")
		}
		if strings.TrimSpace(c.Firecracker.WorkDir) == "" {
			missing = append(missing, "FIRECRACKER_WORKDIR")
		}
		if c.Firecracker.WorkspaceImageMb <= 0 {
			missing = append(missing, "FIRECRACKER_WORKSPACE_IMAGE_MB")
		}
	}
	if c.Runtime.MaxConcurrentVMs <= 0 {
		missing = append(missing, "MAX_CONCURRENT_VMS must be greater than 0")
	}
	if c.Runtime.WarmPoolSize < 0 {
		missing = append(missing, "WARM_POOL_SIZE must be greater than or equal to 0")
	}
	if c.Runtime.WarmPoolSize > c.Runtime.MaxConcurrentVMs {
		missing = append(missing, "WARM_POOL_SIZE must be less than or equal to MAX_CONCURRENT_VMS")
	}
	if c.Runtime.ArtifactRetentionHours < 0 {
		missing = append(missing, "ARTIFACT_RETENTION_HOURS must be greater than or equal to 0")
	}
	if c.Runtime.CleanupIntervalSeconds <= 0 {
		missing = append(missing, "CLEANUP_INTERVAL_SECONDS must be greater than 0")
	}
	if c.Runtime.MemoryLogIntervalSeconds <= 0 {
		missing = append(missing, "MEMORY_LOG_INTERVAL_SECONDS must be greater than 0")
	}

	if len(missing) > 0 {
		return fmt.Errorf("missing or invalid environment values: %s", strings.Join(missing, ", "))
	}
	return nil
}

func getEnvInt(key string) int {
	value := os.Getenv(key)
	if value == "" {
		return 0
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0
	}
	return parsed
}

func getEnvIntDefault(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvFloat(key string) float64 {
	value := os.Getenv(key)
	if value == "" {
		return 0
	}

	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return 0
	}
	return parsed
}
