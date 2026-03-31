package process

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/pqpm/pqpm/internal/cgroup"
	"github.com/pqpm/pqpm/internal/logger"
	"github.com/pqpm/pqpm/internal/types"
)

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
	m.mu.Lock()
	defer m.mu.Unlock()

	// Check if already running
	if proc, exists := m.processes[processKey(name, uid)]; exists && !proc.Stopped {
		return fmt.Errorf("service %q is already running (PID %d)", name, proc.Info.PID)
	}

	proc, err := m.spawnProcess(name, cfg, uid, gid)
	if err != nil {
		return err
	}

	key := processKey(name, uid)
	m.processes[key] = proc

	// Start monitoring goroutine for auto-restart
	go m.monitor(key, name, cfg, uid, gid)

	logger.Log.Infow("Process started",
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
	defer m.mu.Unlock()

	key := processKey(name, uid)
	proc, exists := m.processes[key]
	if !exists {
		return fmt.Errorf("service %q not found", name)
	}

	return m.stopProcess(key, proc)
}

// Restart stops and then starts a process.
func (m *Manager) Restart(name string, cfg types.ServiceConfig, uid, gid uint32) error {
	// Stop first (ignore error if not running)
	_ = m.Stop(name, uid)

	// Brief pause to allow cleanup
	time.Sleep(500 * time.Millisecond)

	return m.Start(name, cfg, uid, gid)
}

// Status returns info about all processes for a given user.
func (m *Manager) Status(uid uint32) []types.ProcessInfo {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var result []types.ProcessInfo
	for _, proc := range m.processes {
		if proc.Info.UID == uid {
			info := proc.Info
			if proc.Stopped || proc.Cmd.ProcessState != nil {
				info.Status = "stopped"
			} else {
				info.Status = "running"
			}
			result = append(result, info)
		}
	}
	return result
}

// StopAll gracefully stops all managed processes (used during daemon shutdown).
func (m *Manager) StopAll() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for key, proc := range m.processes {
		if err := m.stopProcess(key, proc); err != nil {
			logger.Log.Warnw("Failed to stop process during shutdown",
				"service", proc.Info.Name,
				"error", err,
			)
		}
	}
}

// spawnProcess creates and starts a new OS process with dropped privileges.
func (m *Manager) spawnProcess(name string, cfg types.ServiceConfig, uid, gid uint32) (*ManagedProcess, error) {
	parts := strings.Fields(cfg.Command)
	if len(parts) == 0 {
		return nil, fmt.Errorf("empty command for service %q", name)
	}

	cmd := exec.Command(parts[0], parts[1:]...)

	// Drop privileges to the user's UID/GID
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Credential: &syscall.Credential{
			Uid: uid,
			Gid: gid,
		},
	}

	// Set working directory if specified
	if cfg.WorkingDir != "" {
		cmd.Dir = cfg.WorkingDir
	}

	// Set up log output
	logFile, err := setupLogFile(name, uid)
	if err != nil {
		logger.Log.Warnw("Failed to set up log file, using /dev/null",
			"service", name,
			"error", err,
		)
		cmd.Stdout = nil
		cmd.Stderr = nil
	} else {
		cmd.Stdout = logFile
		cmd.Stderr = logFile
	}

	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("failed to start service %q: %w", name, err)
	}

	// Apply cgroup resource limits
	if cfg.MaxMemory != "" || cfg.CPULimit != "" {
		if err := cgroup.ApplyLimits(cmd.Process.Pid, name, cfg.MaxMemory, cfg.CPULimit); err != nil {
			logger.Log.Warnw("Failed to apply resource limits (continuing without limits)",
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
	}, nil
}

// monitor watches a process and restarts it according to the restart policy.
func (m *Manager) monitor(key, name string, cfg types.ServiceConfig, uid, gid uint32) {
	for {
		m.mu.RLock()
		proc, exists := m.processes[key]
		m.mu.RUnlock()

		if !exists || proc.Stopped {
			return
		}

		// Wait for the process to exit
		err := proc.Cmd.Wait()

		m.mu.RLock()
		stopped := proc.Stopped
		m.mu.RUnlock()

		// If we explicitly stopped, don't restart
		if stopped {
			return
		}

		// Determine restart behavior
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
			logger.Log.Infow("Process exited, not restarting",
				"service", name,
				"uid", uid,
				"error", err,
			)
			m.mu.Lock()
			proc.Info.Status = "stopped"
			m.mu.Unlock()
			return
		}

		logger.Log.Infow("Process exited, restarting...",
			"service", name,
			"uid", uid,
			"error", err,
		)

		// Brief backoff before restart
		time.Sleep(2 * time.Second)

		m.mu.Lock()
		newProc, spawnErr := m.spawnProcess(name, cfg, uid, gid)
		if spawnErr != nil {
			logger.Log.Errorw("Failed to restart process",
				"service", name,
				"error", spawnErr,
			)
			proc.Info.Status = "crashed"
			m.mu.Unlock()
			return
		}
		newProc.Info.Restarts = proc.Info.Restarts + 1
		m.processes[key] = newProc
		m.mu.Unlock()
	}
}

// stopProcess sends SIGTERM, waits, then SIGKILL if necessary.
func (m *Manager) stopProcess(key string, proc *ManagedProcess) error {
	if proc.Stopped {
		return nil
	}

	proc.Stopped = true
	close(proc.StopCh)

	if proc.Cmd.Process == nil {
		return nil
	}

	// Send SIGTERM
	if err := proc.Cmd.Process.Signal(syscall.SIGTERM); err != nil {
		// Process might already be dead
		return nil
	}

	// Wait up to 10 seconds for graceful shutdown
	done := make(chan struct{})
	go func() {
		proc.Cmd.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Clean exit
	case <-time.After(10 * time.Second):
		// Force kill
		proc.Cmd.Process.Kill()
	}

	// Cleanup cgroup
	cgroup.Cleanup(proc.Info.Name)

	logger.Log.Infow("Process stopped",
		"service", proc.Info.Name,
		"pid", proc.Info.PID,
	)

	return nil
}

// processKey creates a unique key for a user's service.
func processKey(name string, uid uint32) string {
	return fmt.Sprintf("%d:%s", uid, name)
}

// setupLogFile creates a log file for the process output.
func setupLogFile(name string, uid uint32) (*os.File, error) {
	logDir := fmt.Sprintf("/var/log/pqpm/users/%d", uid)
	if err := os.MkdirAll(logDir, 0755); err != nil {
		return nil, err
	}

	logPath := fmt.Sprintf("%s/%s.log", logDir, name)
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		return nil, err
	}

	// Chown the log file to the user
	if err := os.Chown(logPath, int(uid), -1); err != nil {
		f.Close()
		return nil, err
	}

	return f, nil
}
