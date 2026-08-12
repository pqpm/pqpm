package cgroup

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
)

const cgroupBasePath = "/sys/fs/cgroup/pqpm"

var ensureOnce sync.Once
var ensureErr error

// EnsureHierarchy creates the pqpm cgroup root and enables memory/cpu controllers.
// Safe to call multiple times.
func EnsureHierarchy() error {
	ensureOnce.Do(func() {
		ensureErr = ensureHierarchy()
	})
	return ensureErr
}

func ensureHierarchy() error {
	// Enable controllers on the system root so children can use them.
	if err := enableControllers("/sys/fs/cgroup", "memory", "cpu"); err != nil {
		// Non-fatal if already enabled or unavailable; continue and try our subtree.
		_ = err
	}

	if err := os.MkdirAll(cgroupBasePath, 0755); err != nil {
		return fmt.Errorf("failed to create cgroup base %s: %w", cgroupBasePath, err)
	}

	if err := enableControllers(cgroupBasePath, "memory", "cpu"); err != nil {
		return fmt.Errorf("failed to enable cgroup controllers under %s: %w", cgroupBasePath, err)
	}

	return nil
}

func enableControllers(path string, controllers ...string) error {
	subtree := filepath.Join(path, "cgroup.subtree_control")
	existing, _ := os.ReadFile(subtree)
	have := string(existing)

	var toAdd []string
	for _, c := range controllers {
		if !strings.Contains(have, c) {
			toAdd = append(toAdd, "+"+c)
		}
	}
	if len(toAdd) == 0 {
		return nil
	}

	return os.WriteFile(subtree, []byte(strings.Join(toAdd, " ")), 0644)
}

func servicePath(uid uint32, serviceName string) string {
	return filepath.Join(cgroupBasePath, strconv.FormatUint(uint64(uid), 10), serviceName)
}

// ApplyLimits creates a cgroup for the given process and applies memory/CPU limits.
// This uses cgroup v2 (unified hierarchy).
func ApplyLimits(pid int, uid uint32, serviceName string, maxMemory string, cpuLimit string) error {
	if err := EnsureHierarchy(); err != nil {
		return err
	}

	cgroupPath := servicePath(uid, serviceName)

	// Ensure the per-UID parent has controllers enabled for nested service cgroups.
	uidPath := filepath.Join(cgroupBasePath, strconv.FormatUint(uint64(uid), 10))
	if err := os.MkdirAll(uidPath, 0755); err != nil {
		return fmt.Errorf("failed to create cgroup directory %s: %w", uidPath, err)
	}
	if err := enableControllers(uidPath, "memory", "cpu"); err != nil {
		return fmt.Errorf("failed to enable controllers under %s: %w", uidPath, err)
	}

	if err := os.MkdirAll(cgroupPath, 0755); err != nil {
		return fmt.Errorf("failed to create cgroup directory %s: %w", cgroupPath, err)
	}

	if maxMemory != "" {
		memBytes, err := parseMemory(maxMemory)
		if err != nil {
			return fmt.Errorf("invalid max_memory %q: %w", maxMemory, err)
		}
		memFile := filepath.Join(cgroupPath, "memory.max")
		if err := os.WriteFile(memFile, []byte(strconv.FormatInt(memBytes, 10)), 0644); err != nil {
			return fmt.Errorf("failed to set memory limit: %w", err)
		}
	}

	if cpuLimit != "" {
		cpuMax, err := parseCPULimit(cpuLimit)
		if err != nil {
			return fmt.Errorf("invalid cpu_limit %q: %w", cpuLimit, err)
		}
		cpuFile := filepath.Join(cgroupPath, "cpu.max")
		if err := os.WriteFile(cpuFile, []byte(cpuMax), 0644); err != nil {
			return fmt.Errorf("failed to set CPU limit: %w", err)
		}
	}

	procsFile := filepath.Join(cgroupPath, "cgroup.procs")
	if err := os.WriteFile(procsFile, []byte(strconv.Itoa(pid)), 0644); err != nil {
		return fmt.Errorf("failed to add PID %d to cgroup: %w", pid, err)
	}

	return nil
}

// Cleanup removes the cgroup directory for a service.
func Cleanup(uid uint32, serviceName string) error {
	return os.RemoveAll(servicePath(uid, serviceName))
}

// GetMetrics returns the current memory and CPU usage for a service.
func GetMetrics(uid uint32, serviceName string) (memory string, cpu string, err error) {
	cgroupPath := servicePath(uid, serviceName)

	memFile := filepath.Join(cgroupPath, "memory.current")
	memData, err := os.ReadFile(memFile)
	if err == nil {
		memBytes, _ := strconv.ParseInt(strings.TrimSpace(string(memData)), 10, 64)
		memory = formatMemory(memBytes)
	} else {
		memory = "0B"
	}

	cpuFile := filepath.Join(cgroupPath, "cpu.stat")
	cpuData, err := os.ReadFile(cpuFile)
	if err == nil {
		lines := strings.Split(string(cpuData), "\n")
		for _, line := range lines {
			if strings.HasPrefix(line, "usage_usec") {
				fields := strings.Fields(line)
				if len(fields) >= 2 {
					cpu = fields[1] + "us"
				}
			}
		}
	}
	if cpu == "" {
		cpu = "0us"
	}

	return memory, cpu, nil
}

// parseMemory converts a human-readable memory string (e.g. "512MB", "1GB") to bytes.
func parseMemory(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToUpper(s))

	multipliers := map[string]int64{
		"KB": 1024,
		"MB": 1024 * 1024,
		"GB": 1024 * 1024 * 1024,
		"TB": 1024 * 1024 * 1024 * 1024,
		"K":  1024,
		"M":  1024 * 1024,
		"G":  1024 * 1024 * 1024,
		"T":  1024 * 1024 * 1024 * 1024,
	}

	for suffix, mult := range multipliers {
		if strings.HasSuffix(s, suffix) {
			numStr := strings.TrimSuffix(s, suffix)
			num, err := strconv.ParseFloat(numStr, 64)
			if err != nil {
				return 0, fmt.Errorf("invalid number: %s", numStr)
			}
			return int64(num * float64(mult)), nil
		}
	}

	return strconv.ParseInt(s, 10, 64)
}

// parseCPULimit converts a percentage string (e.g. "20%") to cgroup v2 cpu.max format.
func parseCPULimit(s string) (string, error) {
	s = strings.TrimSpace(s)
	if !strings.HasSuffix(s, "%") {
		return "", fmt.Errorf("cpu_limit must be a percentage (e.g. \"20%%\")")
	}

	numStr := strings.TrimSuffix(s, "%")
	pct, err := strconv.ParseFloat(numStr, 64)
	if err != nil {
		return "", fmt.Errorf("invalid percentage: %s", numStr)
	}

	if pct <= 0 || pct > 100 {
		return "", fmt.Errorf("cpu_limit must be between 0%% and 100%%")
	}

	period := 100000
	quota := int(pct / 100.0 * float64(period))
	if quota < 1000 {
		quota = 1000
	}

	return fmt.Sprintf("%d %d", quota, period), nil
}

func formatMemory(bytes int64) string {
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
