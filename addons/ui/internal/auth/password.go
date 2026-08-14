package auth

import (
	"bytes"
	"fmt"
	"os/exec"
	"os/user"
	"strings"
	"time"
)

// AuthenticateUser verifies a Linux username/password via PAM-capable helpers.
// Tries `su` first (common on servers), then `pamtester` if available.
// Does not store passwords; identity for pqpm remains SO_PEERCRED after login.
func AuthenticateUser(username, password string) (*user.User, error) {
	username = strings.TrimSpace(username)
	if username == "" || password == "" {
		return nil, fmt.Errorf("username and password required")
	}
	if strings.ContainsAny(username, ":/\x00") || username == "root" {
		return nil, fmt.Errorf("invalid username")
	}

	u, err := user.Lookup(username)
	if err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}

	if err := checkPassword(username, password); err != nil {
		return nil, fmt.Errorf("invalid credentials")
	}
	return u, nil
}

func checkPassword(username, password string) error {
	// Prefer pamtester when present (explicit PAM stack).
	if path, err := exec.LookPath("pamtester"); err == nil {
		cmd := exec.Command(path, "login", username, "authenticate")
		cmd.Stdin = strings.NewReader(password + "\n")
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		if err := runWithTimeout(cmd, 5*time.Second); err == nil {
			return nil
		}
	}

	// Fallback: su validates against the system auth stack.
	suPath, err := exec.LookPath("su")
	if err != nil {
		return fmt.Errorf("no password verifier available (install pamtester or su)")
	}
	cmd := exec.Command(suPath, "-c", "true", username)
	cmd.Stdin = strings.NewReader(password + "\n")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := runWithTimeout(cmd, 5*time.Second); err != nil {
		return err
	}
	return nil
}

func runWithTimeout(cmd *exec.Cmd, d time.Duration) error {
	if err := cmd.Start(); err != nil {
		return err
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		return err
	case <-time.After(d):
		_ = cmd.Process.Kill()
		return fmt.Errorf("authentication timed out")
	}
}
