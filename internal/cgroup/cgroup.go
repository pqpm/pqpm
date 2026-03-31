package cgroup

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

const cgroupBasePath = "/sys/fs/cgroup/pqpm"

// ApplyLimits creates a cgroup for the given process and applies memory/CPU limits.
// This uses cgroup v2 (unified hierarchy).
func ApplyLimits(pid int, serviceName string, maxMemory string, cpuLimit string) error {
	cgroupPath := filepath.Join(cgroupBasePath, serviceName)

	// Create the cgroup directory
	if err := os.MkdirAll(cgroupPath, 0755); err != nil {
		return fmt.Errorf("failed to create cgroup directory %s: %w", cgroupPath, err)
	}

	// Apply memory limit
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

	// Apply CPU limit (convert percentage to cgroup v2 cpu.max format)
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

	// Add the process to this cgroup
	procsFile := filepath.Join(cgroupPath, "cgroup.procs")
	if err := os.WriteFile(procsFile, []byte(strconv.Itoa(pid)), 0644); err != nil {
		return fmt.Errorf("failed to add PID %d to cgroup: %w", pid, err)
	}

	return nil
}

// Cleanup removes the cgroup directory for a service.
func Cleanup(serviceName string) error {
	cgroupPath := filepath.Join(cgroupBasePath, serviceName)
	return os.RemoveAll(cgroupPath)
}

// GetMetrics returns the current memory and CPU usage for a service.
func GetMetrics(serviceName string) (memory string, cpu string, err error) {
	cgroupPath := filepath.Join(cgroupBasePath, serviceName)

	// Memory usage
	memFile := filepath.Join(cgroupPath, "memory.current")
	memData, err := os.ReadFile(memFile)
	if err == nil {
		memBytes, _ := strconv.ParseInt(strings.TrimSpace(string(memData)), 10, 64)
		memory = formatMemory(memBytes)
	} else {
		memory = "0B"
	}

	// CPU usage (this is a simplified version, as true CPU % requires delta over time)
	// For now, we'll just return total usage time or a placeholder
	cpuFile := filepath.Join(cgroupPath, "cpu.stat")
	cpuData, err := os.ReadFile(cpuFile)
	if err == nil {
		// Example format:
		// usage_usec 12345
		// user_usec 10000
		// system_usec 2345
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

	// Assume raw bytes
	return strconv.ParseInt(s, 10, 64)
}

// parseCPULimit converts a percentage string (e.g. "20%") to cgroup v2 cpu.max format.
// cpu.max format is "$MAX $PERIOD" where period is typically 100000 (100ms).
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

	period := 100000 // 100ms in microseconds
	quota := int(pct / 100.0 * float64(period))
	if quota < 1000 {
		quota = 1000 // minimum 1ms
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
