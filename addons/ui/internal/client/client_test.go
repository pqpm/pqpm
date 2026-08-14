package client

import (
	"strings"
	"testing"

	"github.com/pqpm/pqpm/internal/types"
)

func TestValidateServiceRejectsOperators(t *testing.T) {
	err := validateService("bad", types.ServiceConfig{Command: "echo hi; rm -rf /"})
	if err == nil || !strings.Contains(err.Error(), "dangerous") {
		t.Fatalf("expected dangerous operator error, got %v", err)
	}
}

func TestValidateServiceOK(t *testing.T) {
	err := validateService("ok", types.ServiceConfig{
		Command: "/usr/bin/python3 /home/u/app.py",
		Restart: "always",
	})
	if err != nil {
		t.Fatal(err)
	}
}
