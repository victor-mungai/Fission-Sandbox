package models

type File struct {
	Name    string `json:"name"`
	Content string `json:"content"`
	Mode    int    `json:"mode"`
}

type RunRequest struct {
	RunID     string  `json:"runId"`
	Command   string  `json:"command"`
	Files     []File  `json:"files,omitempty"`
	TimeoutMs int     `json:"timeoutMs"`
	MemoryMb  int     `json:"memoryMb"`
	CPUCount  float64 `json:"cpuCount"`
}

type RunResponse struct {
	Stdout     string `json:"stdout"`
	Stderr     string `json:"stderr"`
	ExitCode   int    `json:"exitCode"`
	DurationMs int    `json:"durationMs"`
	TimedOut   bool   `json:"timedOut"`
	VMId       string `json:"vmId"`
}

type ErrorResponse struct {
	Error string `json:"error"`
}
