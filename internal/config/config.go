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
	BinaryPath       string
	KernelImage      string
	KernelArgs       string
	RootfsImage      string
	WorkDir          string
	WorkspaceImageMb int
	DefaultTimeoutMs int
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
			BinaryPath:       os.Getenv("FIRECRACKER_BIN"),
			KernelImage:      os.Getenv("FIRECRACKER_KERNEL_IMAGE"),
			KernelArgs:       os.Getenv("FIRECRACKER_KERNEL_ARGS"),
			RootfsImage:      os.Getenv("FIRECRACKER_ROOTFS_IMAGE"),
			WorkDir:          os.Getenv("FIRECRACKER_WORKDIR"),
			WorkspaceImageMb: getEnvInt("FIRECRACKER_WORKSPACE_IMAGE_MB"),
			DefaultTimeoutMs: getEnvInt("SANDBOX_DEFAULT_TIMEOUT_MS"),
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
