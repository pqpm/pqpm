package process

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/pqpm/pqpm/internal/cgroup"
	"github.com/pqpm/pqpm/internal/logger"
	"github.com/pqpm/pqpm/internal/types"
)

const StateFilePath = "/var/lib/pqpm/state.json"

// Manager tracks and controls all managed processes.
type Manager struct {
	mu        sync.RWMutex
	processes map[string]*ManagedProcess
}

// ManagedProcess represents a single running process with its metadata.
type ManagedProcess struct {
	Info    types.ProcessInfo
	Cmd     *exec.Cmd
	StopCh  chan struct{}
	DoneCh  chan struct{} // closed when the monitor goroutine exits
	Stopped bool
}

// NewManager creates a new process manager.
func NewManager() *Manager {
	return &Manager{
		processes: make(map[string]*ManagedProcess),
	}
}

// Start launches a process for the given user and service configuration.
func (m *Manager) Start(name string, cfg types.ServiceConfig, uid, gid uint32) error {
	key := processKey(name, uid)

	m.mu.Lock()
	if proc, exists := m.processes[key]; exists {
		if !proc.Stopped {
			m.mu.Unlock()
			return fmt.Errorf("service %q is already running (PID %d)", name, proc.Info.PID)
		}
		// Previous instance is stopping — wait for its monitor outside the lock.
		m.mu.Unlock()
		waitMonitorDone(proc, 15*time.Second)
		m.mu.Lock()
		if current, ok := m.processes[key]; ok && current == proc {
			delete(m.processes, key)
		}
	}

	proc, err := m.spawnProcess(name, cfg, uid, gid)
	if err != nil {
		m.mu.Unlock()
		return err
	}

	m.processes[key] = proc

	if err := m.Persist(); err != nil {
		logger.Log.Warn("Failed to persist state", "error", err)
	}
	m.mu.Unlock()

	go m.monitor(key, name, cfg, uid, gid)

	logger.Log.Info("Process started",
		"service", name,
		"pid", proc.Info.PID,
		"uid", uid,
		"command", cfg.Command,
	)

	return nil
}

// Stop terminates a managed process.
func (m *Manager) Stop(name string, uid uint32) error {
	m.mu.Lock()
	key := processKey(name, uid)
	proc, exists := m.processes[key]
	if !exists {
		m.mu.Unlock()
		return fmt.Errorf("service %q not found", name)
	}

	if err := m.stopProcess(key, proc); err != nil {
		m.mu.Unlock()
		return err
	}

	if err := m.Persist(); err != nil {
		logger.Log.Warn("Failed to persist state", "error", err)
	}
	m.mu.Unlock()

	// Wait for the monitor to finish so a subsequent Start can't race.
	waitMonitorDone(proc, 15*time.Second)

	m.mu.Lock()
	// Only delete if this is still the same entry (Start may have replaced it).
	if current, ok := m.processes[key]; ok && current == proc {
		delete(m.processes, key)
	}
	if err := m.Persist(); err != nil {
		logger.Log.Warn("Failed to persist state", "error", err)
	}
	m.mu.Unlock()

	return nil
}

// Restart stops and then starts a process.
func (m *Manager) Restart(name string, cfg types.ServiceConfig, uid, gid uint32) error {
	_ = m.Stop(name, uid)
	return m.Start(name, cfg, uid, gid)
}

// Status returns info about all processes for a given user.
func (m *Manager) Status(uid uint32) []types.ProcessInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []types.ProcessInfo
	for _, proc := range m.processes {
		if proc.Info.UID != uid {
			continue
		}

		info := proc.Info
		alive := processAlive(proc)

		switch {
		case proc.Stopped:
			info.Status = "stopped"
			info.MemoryUsage = "0B"
			info.CPUUsage = "0us"
		case alive:
			info.Status = "running"
			mem, cpu, _ := cgroup.GetMetrics(uid, proc.Info.Name)
			if mem == "0B" || mem == "" {
				mem = rssFromProc(proc.Info.PID)
			}
			if cpu == "" {
				cpu = "0us"
			}
			info.MemoryUsage = mem
			info.CPUUsage = cpu
		default:
			// Exited and waiting for restart backoff (or crashed with restart=never).
			info.Status = "restarting"
			info.MemoryUsage = "0B"
			info.CPUUsage = "0us"
		}
		result = append(result, info)
	}
	return result
}

// StopAll gracefully stops all managed processes (used during daemon shutdown).
// Persisted state is written first so services can be restored on the next daemon start.
// User-stopped services are already omitted from state by Stop(); we must not wipe
// state.json after marking everything stopped here.
func (m *Manager) StopAll() {
	m.mu.Lock()
	if err := m.Persist(); err != nil {
		logger.Log.Warn("Failed to persist state before shutdown", "error", err)
	}

	procs := make([]*ManagedProcess, 0, len(m.processes))
	for key, proc := range m.processes {
		_ = m.stopProcess(key, proc)
		procs = append(procs, proc)
	}
	m.mu.Unlock()

	for _, proc := range procs {
		waitMonitorDone(proc, 15*time.Second)
	}

	m.mu.Lock()
	m.processes = make(map[string]*ManagedProcess)
	m.mu.Unlock()
}

// spawnProcess creates and starts a new OS process with dropped privileges.
func (m *Manager) spawnProcess(name string, cfg types.ServiceConfig, uid, gid uint32) (*ManagedProcess, error) {
	parts := strings.Fields(cfg.Command)
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty command for service %q", name)
	}

	cmd := exec.Command(parts[0], parts[1:]...)

	if len(cfg.Env) > 0 {
		cmd.Env = os.Environ()
		for k, v := range cfg.Env {
			cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", k, v))
		}
	}

	// New process group so stop can signal the whole tree (children included).
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid: uid,
			Gid: gid,
		},
		Setpgid: true,
	}

	if cfg.WorkingDir != "" {
		cmd.Dir = cfg.WorkingDir
	}

	logPath, logFile, err := setupLogFile(name, uid, cfg)
	if err != nil {
		logger.Log.Warn("Failed to set up log file, using /dev/null",
			"service", name,
			"error", err,
		)
		cmd.Stdout = nil
		cmd.Stderr = nil
	} else {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
		logger.Log.Info("Logging process output",
			"service", name,
			"log_file", logPath,
		)
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start service %q: %w", name, err)
	}

	if cfg.MaxMemory != "" || cfg.CPULimit != "" {
		if err := cgroup.ApplyLimits(cmd.Process.Pid, uid, name, cfg.MaxMemory, cfg.CPULimit); err != nil {
			logger.Log.Warn("Failed to apply resource limits (continuing without limits)",
				"service", name,
				"error", err,
			)
		}
	}

	return &ManagedProcess{
		Info: types.ProcessInfo{
			Name:    name,
			PID:     cmd.Process.Pid,
			Status:  "running",
			UID:     uid,
			GID:     gid,
			Command: cfg.Command,
			Config:  cfg,
		},
		Cmd:    cmd,
		StopCh: make(chan struct{}),
		DoneCh: make(chan struct{}),
	}, nil
}

// monitor watches a process and restarts it according to the restart policy.
func (m *Manager) monitor(key, name string, cfg types.ServiceConfig, uid, gid uint32) {
	m.mu.RLock()
	proc, exists := m.processes[key]
	m.mu.RUnlock()
	if !exists {
		return
	}
	defer close(proc.DoneCh)

	for {
		m.mu.RLock()
		current, exists := m.processes[key]
		stopped := !exists || current.Stopped
		if exists {
			proc = current
		}
		m.mu.RUnlock()

		if stopped {
			return
		}

		// Only the monitor waits on the process (stopProcess does not call Wait).
		err := proc.Cmd.Wait()

		m.mu.RLock()
		stopped = proc.Stopped
		m.mu.RUnlock()
		if stopped {
			return
		}

		restart := cfg.Restart
		if restart == "" {
			restart = "always"
		}

		shouldRestart := false
		switch restart {
		case "always":
			shouldRestart = true
		case "on-failure":
			shouldRestart = err != nil
		case "never":
			shouldRestart = false
		}

		if !shouldRestart {
			logger.Log.Info("Process exited, not restarting",
				"service", name,
				"uid", uid,
				"error", err,
			)
			m.mu.Lock()
			if current, ok := m.processes[key]; ok && current == proc {
				proc.Info.Status = "stopped"
				proc.Stopped = true
			}
			m.mu.Unlock()
			return
		}

		logger.Log.Info("Process exited, restarting...",
			"service", name,
			"uid", uid,
			"error", err,
		)

		// Backoff, aborting immediately if stop is requested.
		select {
		case <-proc.StopCh:
			return
		case <-time.After(2 * time.Second):
		}

		m.mu.Lock()
		current, exists = m.processes[key]
		if !exists || current.Stopped || current != proc {
			m.mu.Unlock()
			return
		}

		newProc, spawnErr := m.spawnProcess(name, cfg, uid, gid)
		if spawnErr != nil {
			logger.Log.Error("Failed to restart process",
				"service", name,
				"error", spawnErr,
			)
			proc.Info.Status = "crashed"
			proc.Stopped = true
			m.mu.Unlock()
			return
		}
		newProc.Info.Restarts = proc.Info.Restarts + 1
		// Keep the same DoneCh so Stop can wait on this monitor goroutine.
		newProc.DoneCh = proc.DoneCh
		m.processes[key] = newProc
		proc = newProc
		m.mu.Unlock()
	}
}

// stopProcess marks the process stopped and signals its process group.
// It must be called with m.mu held. It does not call Cmd.Wait — the monitor owns that.
func (m *Manager) stopProcess(key string, proc *ManagedProcess) error {
	if proc.Stopped {
		return nil
	}

	proc.Stopped = true
	close(proc.StopCh)

	if proc.Cmd != nil && proc.Cmd.Process != nil {
		pid := proc.Cmd.Process.Pid
		// Negative PID signals the whole process group.
		if err := syscall.Kill(-pid, syscall.SIGTERM); err != nil {
			// Fallback to signaling the lead process only.
			_ = proc.Cmd.Process.Signal(syscall.SIGTERM)
		}

		deadline := time.Now().Add(10 * time.Second)
		for processAlive(proc) && time.Now().Before(deadline) {
			m.mu.Unlock()
			time.Sleep(100 * time.Millisecond)
			m.mu.Lock()
		}
		if processAlive(proc) {
			_ = syscall.Kill(-pid, syscall.SIGKILL)
			_ = proc.Cmd.Process.Kill()
		}
	}

	cgroup.Cleanup(proc.Info.UID, proc.Info.Name)

	logger.Log.Info("Process stopped",
		"service", proc.Info.Name,
		"pid", proc.Info.PID,
	)

	return nil
}

func processAlive(proc *ManagedProcess) bool {
	if proc == nil || proc.Cmd == nil || proc.Cmd.Process == nil {
		return false
	}
	// Wait has completed → not alive.
	if proc.Cmd.ProcessState != nil {
		return false
	}
	err := proc.Cmd.Process.Signal(syscall.Signal(0))
	return err == nil
}

func waitMonitorDone(proc *ManagedProcess, timeout time.Duration) {
	if proc == nil || proc.DoneCh == nil {
		return
	}
	select {
	case <-proc.DoneCh:
	case <-time.After(timeout):
		logger.Log.Warn("Timed out waiting for monitor to exit",
			"service", proc.Info.Name,
			"pid", proc.Info.PID,
		)
	}
}

func rssFromProc(pid int) string {
	data, err := os.ReadFile(fmt.Sprintf("/proc/%d/status", pid))
	if err != nil {
		return "0B"
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.HasPrefix(line, "VmRSS:") {
			fields := strings.Fields(line)
			if len(fields) < 2 {
				break
			}
			kb, err := strconv.ParseInt(fields[1], 10, 64)
			if err != nil {
				break
			}
			return formatBytes(kb * 1024)
		}
	}
	return "0B"
}

func formatBytes(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%dB", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f%cB", float64(bytes)/float64(div), "KMGTPE"[exp])
}

// processKey creates a unique key for a user's service.
func processKey(name string, uid uint32) string {
	return fmt.Sprintf("%d:%s", uid, name)
}

// LogPath returns the resolved log file path for a service.
// Prefer the running process config; otherwise fall back to the default pqpm path.
func (m *Manager) LogPath(name string, uid uint32) string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if proc, ok := m.processes[processKey(name, uid)]; ok {
		if path, err := ResolveLogPath(name, uid, proc.Info.Config); err == nil {
			return path
		}
	}
	return defaultLogPath(name, uid)
}

// HasService reports whether the manager is tracking a service for the user.
func (m *Manager) HasService(name string, uid uint32) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	_, ok := m.processes[processKey(name, uid)]
	return ok
}

// ResolveLogPath returns the absolute log path for a service config.
// Relative log_file paths are resolved against working_dir when set.
func ResolveLogPath(name string, uid uint32, cfg types.ServiceConfig) (string, error) {
	if cfg.LogFile == "" {
		return defaultLogPath(name, uid), nil
	}

	logPath := cfg.LogFile
	if !filepath.IsAbs(logPath) {
		base := cfg.WorkingDir
		if base == "" {
			return "", fmt.Errorf("relative log_file %q requires working_dir", cfg.LogFile)
		}
		logPath = filepath.Join(base, logPath)
	}

	abs, err := filepath.Abs(logPath)
	if err != nil {
		return "", err
	}
	return abs, nil
}

func defaultLogPath(name string, uid uint32) string {
	return fmt.Sprintf("/var/log/pqpm/users/%d/%s.log", uid, name)
}

// setupLogFile creates/opens the log file for process stdout/stderr.
func setupLogFile(name string, uid uint32, cfg types.ServiceConfig) (string, *os.File, error) {
	logPath, err := ResolveLogPath(name, uid, cfg)
	if err != nil {
		return "", nil, err
	}

	// Custom log files must stay inside the user's home directory.
	if cfg.LogFile != "" {
		if err := sanitizeLogPath(logPath, uid); err != nil {
			return "", nil, err
		}
	}

	if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
		return "", nil, err
	}

	// Ensure the directory is owned by the user for custom paths so they can read logs.
	if cfg.LogFile != "" {
		_ = os.Chown(filepath.Dir(logPath), int(uid), -1)
	}

	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return "", nil, err
	}

	if err := os.Chown(logPath, int(uid), -1); err != nil {
		f.Close()
		return "", nil, err
	}

	return logPath, f, nil
}

func sanitizeLogPath(logPath string, uid uint32) error {
	u, err := user.LookupId(strconv.FormatUint(uint64(uid), 10))
	if err != nil {
		return fmt.Errorf("failed to lookup user: %w", err)
	}
	home := filepath.Clean(u.HomeDir)
	clean := filepath.Clean(logPath)
	if clean != home && !strings.HasPrefix(clean, home+string(os.PathSeparator)) {
		return fmt.Errorf("log_file %q is outside user home directory %q", logPath, home)
	}
	return nil
}

// Persist saves the list of currently managed processes to a JSON file.
func (m *Manager) Persist() error {
	var state types.DaemonState
	for _, proc := range m.processes {
		if !proc.Stopped {
			state.Services = append(state.Services, types.PersistedService{
				Name:   proc.Info.Name,
				UID:    proc.Info.UID,
				GID:    proc.Info.GID,
				Config: proc.Info.Config,
			})
		}
	}

	data, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal state: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(StateFilePath), 0755); err != nil {
		return fmt.Errorf("failed to create state directory: %w", err)
	}

	if err := os.WriteFile(StateFilePath, data, 0600); err != nil {
		return fmt.Errorf("failed to write state file: %w", err)
	}

	return nil
}

// LoadState reads the state file and restarts all previously running services.
func (m *Manager) LoadState() error {
	data, err := os.ReadFile(StateFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("failed to read state file: %w", err)
	}

	var state types.DaemonState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("failed to unmarshal state: %w", err)
	}

	logger.Log.Info("Loading persisted services", "count", len(state.Services))

	for _, svc := range state.Services {
		logger.Log.Info("Restarting persisted service", "service", svc.Name, "uid", svc.UID)
		if err := m.Start(svc.Name, svc.Config, svc.UID, svc.GID); err != nil {
			logger.Log.Error("Failed to restart persisted service",
				"service", svc.Name,
				"uid", svc.UID,
				"error", err,
			)
		}
	}

	return nil
}
