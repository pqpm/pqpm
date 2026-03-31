package types

// ServiceConfig represents a single service definition from the user's .pqpm.toml
type ServiceConfig struct {
	Command    string            `toml:"command"`
	Restart    string            `toml:"restart"`     // "always", "on-failure", "never"
	MaxMemory  string            `toml:"max_memory"`  // e.g. "512MB"
	CPULimit   string            `toml:"cpu_limit"`   // e.g. "20%"
	WorkingDir string            `toml:"working_dir"` // optional working directory
	LogFile    string            `toml:"log_file"`    // optional log file path
	Env        map[string]string `toml:"env"`         // optional environment variables
}

// UserConfig represents the full .pqpm.toml file
type UserConfig struct {
	Service map[string]ServiceConfig `toml:"service"`
}

// ProcessInfo holds runtime information about a managed process
type ProcessInfo struct {
	Name        string        `json:"name"`
	PID         int           `json:"pid"`
	Status      string        `json:"status"` // "running", "stopped", "crashed"
	UID         uint32        `json:"uid"`
	GID         uint32        `json:"gid"`
	Command     string        `json:"command"`
	Restarts    int           `json:"restarts"`
	MemoryUsage string        `json:"memory_usage"` // e.g. "12.5MB"
	CPUUsage    string        `json:"cpu_usage"`    // e.g. "1.2%"
	Uptime      string        `json:"uptime"`       // e.g. "2h 30m"
	Config      ServiceConfig `json:"config"`
}

// DaemonRequest is a message sent from the CLI to the daemon over the Unix socket
type DaemonRequest struct {
	Action  string `json:"action"`  // "start", "stop", "restart", "status", "log"
	Service string `json:"service"` // service name
}

// DaemonResponse is the reply from the daemon back to the CLI
type DaemonResponse struct {
	Success  bool          `json:"success"`
	Message  string        `json:"message"`
	Services []ProcessInfo `json:"services,omitempty"`
}

// PersistedService holds the information needed to restart a service.
type PersistedService struct {
	Name   string        `json:"name"`
	UID    uint32        `json:"uid"`
	GID    uint32        `json:"gid"`
	Config ServiceConfig `json:"config"`
}

// DaemonState is the structure of the file used to persist services across restarts.
type DaemonState struct {
	Services []PersistedService `json:"services"`
}
