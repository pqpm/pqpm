// Package client runs pqpm operations as a specific Linux user.
// Prefer subprocess identity so SO_PEERCRED on the daemon socket matches the target UID.
package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/pqpm/pqpm/internal/types"
)

const defaultTimeout = 30 * time.Second

// ServiceRow is one row from pqpm status (JSON via rpc subcommand).
type ServiceRow struct {
	Name        string `json:"name"`
	PID         int    `json:"pid"`
	Status      string `json:"status"`
	MemoryUsage string `json:"memory_usage"`
	CPUUsage    string `json:"cpu_usage"`
	Restarts    int    `json:"restarts"`
	Command     string `json:"command"`
	Uptime      string `json:"uptime"`
}

// Result is a generic command result.
type Result struct {
	OK      bool   `json:"ok"`
	Message string `json:"message"`
}

// Client executes pqpm as a given user.
type Client struct {
	// PQPM is the path to the pqpm CLI (default: "pqpm" on PATH).
	PQPM string
	// Self is the path to pqpm-ui (for the rpc subcommand). Empty = os.Executable().
	Self string
	// Username is the Linux account to act as. Empty = current process user.
	Username string
}

func (c *Client) pqpmBin() string {
	if c.PQPM != "" {
		return c.PQPM
	}
	return "pqpm"
}

func (c *Client) selfBin() (string, error) {
	if c.Self != "" {
		return c.Self, nil
	}
	return os.Executable()
}

func (c *Client) lookup() (*user.User, error) {
	if c.Username == "" {
		return user.Current()
	}
	return user.Lookup(c.Username)
}

func (c *Client) credential() (*syscall.Credential, *user.User, error) {
	u, err := c.lookup()
	if err != nil {
		return nil, nil, err
	}
	uid, err := strconv.ParseUint(u.Uid, 10, 32)
	if err != nil {
		return nil, nil, err
	}
	gid, err := strconv.ParseUint(u.Gid, 10, 32)
	if err != nil {
		return nil, nil, err
	}
	cur, err := user.Current()
	if err != nil {
		return nil, nil, err
	}
	// Only set credentials when targeting another user (requires root/CAP_SETUID).
	if cur.Uid == u.Uid {
		return nil, u, nil
	}
	return &syscall.Credential{Uid: uint32(uid), Gid: uint32(gid)}, u, nil
}

func (c *Client) run(ctx context.Context, name string, args ...string) (stdout, stderr string, err error) {
	cred, _, err := c.credential()
	if err != nil {
		return "", "", err
	}
	cmd := exec.CommandContext(ctx, name, args...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	if cred != nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: cred}
		if u, lookupErr := c.lookup(); lookupErr == nil {
			cmd.Dir = u.HomeDir
			cmd.Env = append(os.Environ(),
				"HOME="+u.HomeDir,
				"USER="+u.Username,
				"LOGNAME="+u.Username,
			)
		}
	}
	err = cmd.Run()
	return outBuf.String(), errBuf.String(), err
}

// Status returns managed services for the user.
func (c *Client) Status(ctx context.Context) ([]ServiceRow, error) {
	self, err := c.selfBin()
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()
	}
	out, errOut, err := c.run(ctx, self, "rpc", "status")
	if err != nil {
		return nil, fmt.Errorf("status failed: %w (%s)", err, strings.TrimSpace(errOut+out))
	}
	var rows []ServiceRow
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		return nil, fmt.Errorf("decode status: %w", err)
	}
	return rows, nil
}

// Ping checks daemon reachability as the user.
func (c *Client) Ping(ctx context.Context) error {
	_, errOut, err := c.run(ctx, c.pqpmBin(), "ping")
	if err != nil {
		return fmt.Errorf("ping failed: %w (%s)", err, strings.TrimSpace(errOut))
	}
	return nil
}

// Start starts a named service.
func (c *Client) Start(ctx context.Context, name string) (string, error) {
	return c.action(ctx, "start", name)
}

// Stop stops a named service.
func (c *Client) Stop(ctx context.Context, name string) (string, error) {
	return c.action(ctx, "stop", name)
}

// Restart restarts a named service.
func (c *Client) Restart(ctx context.Context, name string) (string, error) {
	return c.action(ctx, "restart", name)
}

// Reload reloads config and restarts a service (or all if name is "*").
func (c *Client) Reload(ctx context.Context, name string) (string, error) {
	if name == "*" || name == "--all" {
		out, errOut, err := c.run(ctx, c.pqpmBin(), "reload", "--all")
		msg := strings.TrimSpace(out)
		if err != nil {
			return msg, fmt.Errorf("%s", strings.TrimSpace(errOut+out))
		}
		return msg, nil
	}
	return c.action(ctx, "reload", name)
}

func (c *Client) action(ctx context.Context, action, name string) (string, error) {
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()
	}
	out, errOut, err := c.run(ctx, c.pqpmBin(), action, name)
	msg := strings.TrimSpace(out)
	if err != nil {
		detail := strings.TrimSpace(errOut + out)
		if detail == "" {
			detail = err.Error()
		}
		return msg, fmt.Errorf("%s", detail)
	}
	return msg, nil
}

// LogLines returns the last n lines of a service log (via rpc for path + read as user).
func (c *Client) LogLines(ctx context.Context, name string, n int) (string, error) {
	self, err := c.selfBin()
	if err != nil {
		return "", err
	}
	if n <= 0 {
		n = 100
	}
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()
	}
	out, errOut, err := c.run(ctx, self, "rpc", "log", name, strconv.Itoa(n))
	if err != nil {
		return "", fmt.Errorf("log failed: %w (%s)", err, strings.TrimSpace(errOut+out))
	}
	return out, nil
}

// ReadConfig returns the raw ~/.pqpm.toml contents for the user.
func (c *Client) ReadConfig(ctx context.Context) (string, string, error) {
	u, err := c.lookup()
	if err != nil {
		return "", "", err
	}
	path := filepath.Join(u.HomeDir, ".pqpm.toml")
	self, err := c.selfBin()
	if err != nil {
		return "", path, err
	}
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()
	}
	out, errOut, err := c.run(ctx, self, "rpc", "read-config")
	if err != nil {
		// Missing file is OK — return empty.
		if strings.Contains(errOut+out, "no such file") || strings.Contains(errOut+out, "not found") {
			return "", path, nil
		}
		return "", path, fmt.Errorf("read config: %w (%s)", err, strings.TrimSpace(errOut+out))
	}
	return out, path, nil
}

// WriteConfig validates and writes ~/.pqpm.toml as the user.
func (c *Client) WriteConfig(ctx context.Context, content string) error {
	var cfg types.UserConfig
	if err := toml.Unmarshal([]byte(content), &cfg); err != nil {
		return fmt.Errorf("invalid TOML: %w", err)
	}
	if cfg.Service == nil {
		cfg.Service = map[string]types.ServiceConfig{}
	}
	for name, svc := range cfg.Service {
		if err := validateService(name, svc); err != nil {
			return err
		}
	}
	self, err := c.selfBin()
	if err != nil {
		return err
	}
	if ctx == nil {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(context.Background(), defaultTimeout)
		defer cancel()
	}
	cred, u, err := c.credential()
	if err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, self, "rpc", "write-config")
	cmd.Stdin = strings.NewReader(content)
	var errBuf bytes.Buffer
	cmd.Stderr = &errBuf
	if cred != nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{Credential: cred}
		cmd.Dir = u.HomeDir
		cmd.Env = append(os.Environ(), "HOME="+u.HomeDir, "USER="+u.Username, "LOGNAME="+u.Username)
	}
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("write config: %w (%s)", err, strings.TrimSpace(errBuf.String()))
	}
	return nil
}

func validateService(name string, svc types.ServiceConfig) error {
	if strings.TrimSpace(svc.Command) == "" {
		return fmt.Errorf("service %q: command is required", name)
	}
	dangerous := []string{";", "&&", "||", "|", ">", "<", "`", "$("}
	for _, op := range dangerous {
		if strings.Contains(svc.Command, op) {
			return fmt.Errorf("service %q: command contains dangerous shell operator %q", name, op)
		}
	}
	validRestart := map[string]bool{"always": true, "on-failure": true, "never": true, "": true}
	if !validRestart[svc.Restart] {
		return fmt.Errorf("service %q: invalid restart policy %q", name, svc.Restart)
	}
	if svc.WorkingDir != "" && !filepath.IsAbs(svc.WorkingDir) {
		return fmt.Errorf("service %q: working_dir must be absolute", name)
	}
	return nil
}
