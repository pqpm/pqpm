package rpc

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/pqpm/pqpm/addons/ui/internal/client"
	"github.com/pqpm/pqpm/internal/socket"
	"github.com/pqpm/pqpm/internal/types"
)

// Run executes a JSON-friendly RPC as the current process user (peercred identity).
func Run(args []string) int {
	if len(args) < 1 {
		fmt.Fprintln(os.Stderr, "usage: pqpm-ui rpc <status|log|read-config|write-config> ...")
		return 2
	}
	switch args[0] {
	case "status":
		return rpcStatus()
	case "log":
		if len(args) < 2 {
			fmt.Fprintln(os.Stderr, "usage: pqpm-ui rpc log <name> [lines]")
			return 2
		}
		n := 100
		if len(args) >= 3 {
			if v, err := strconv.Atoi(args[2]); err == nil {
				n = v
			}
		}
		return rpcLog(args[1], n)
	case "read-config":
		return rpcReadConfig()
	case "write-config":
		return rpcWriteConfig()
	default:
		fmt.Fprintf(os.Stderr, "unknown rpc command %q\n", args[0])
		return 2
	}
}

func rpcStatus() int {
	resp, err := socket.SendRequest(&types.DaemonRequest{Action: "status"})
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	if !resp.Success {
		fmt.Fprintln(os.Stderr, resp.Message)
		return 1
	}
	rows := make([]client.ServiceRow, 0, len(resp.Services))
	for _, svc := range resp.Services {
		rows = append(rows, client.ServiceRow{
			Name:        svc.Name,
			PID:         svc.PID,
			Status:      svc.Status,
			MemoryUsage: svc.MemoryUsage,
			CPUUsage:    svc.CPUUsage,
			Restarts:    svc.Restarts,
			Command:     svc.Command,
			Uptime:      svc.Uptime,
		})
	}
	if err := json.NewEncoder(os.Stdout).Encode(rows); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	return 0
}

func rpcLog(name string, n int) int {
	resp, err := socket.SendRequest(&types.DaemonRequest{Action: "log", Service: name})
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	if !resp.Success {
		fmt.Fprintln(os.Stderr, resp.Message)
		return 1
	}
	logPath := resp.LogPath
	if logPath == "" {
		logPath = strings.TrimPrefix(resp.Message, "Log file: ")
	}
	if logPath == "" {
		fmt.Fprintln(os.Stderr, "daemon did not return a log path")
		return 1
	}
	data, err := os.ReadFile(logPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	lines := strings.Split(string(data), "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if n > 0 && len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	for _, line := range lines {
		fmt.Println(line)
	}
	return 0
}

func rpcReadConfig() int {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	path := filepath.Join(home, ".pqpm.toml")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, "no such file")
			return 1
		}
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	_, _ = os.Stdout.Write(data)
	return 0
}

func rpcWriteConfig() int {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	path := filepath.Join(home, ".pqpm.toml")
	data, err := io.ReadAll(bufio.NewReader(os.Stdin))
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	return 0
}
