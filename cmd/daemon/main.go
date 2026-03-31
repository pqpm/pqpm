package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/pqpm/pqpm/internal/daemon"
	"github.com/pqpm/pqpm/internal/logger"
	"github.com/pqpm/pqpm/internal/process"
	"github.com/pqpm/pqpm/internal/socket"
)

func main() {
	// Ensure necessary directories exist before initializing logger
	dirs := []string{"/var/run/pqpm", "/var/log/pqpm", "/var/log/pqpm/users", "/var/lib/pqpm"}
	for _, dir := range dirs {
		if err := os.MkdirAll(dir, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Failed to create directory %s: %v\n", dir, err)
		}
	}

	// Initialize logger
	if err := logger.Init("daemon"); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}

	logger.Log.Info("Starting PQPM daemon (pqpmd)...")

	// Verify running as root (required to drop privileges)
	if os.Geteuid() != 0 {
		logger.Log.Error("pqpmd must be run as root to manage user processes")
		os.Exit(1)
	}

	// Create process manager
	mgr := process.NewManager()

	// Load and restart persisted services
	if err := mgr.LoadState(); err != nil {
		logger.Log.Warn("Failed to load persisted state", "error", err)
	}

	// Create request handler
	handler := daemon.NewHandler(mgr)

	// Start listening on Unix socket
	listener, err := socket.Listen()
	if err != nil {
		logger.Log.Error("Failed to start socket listener", "error", err)
		os.Exit(1)
	}
	defer listener.Close()

	logger.Log.Info("Daemon listening", "socket", socket.SocketPath)

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		logger.Log.Info("Received shutdown signal", "signal", sig)
		logger.Log.Info("Stopping all managed processes...")
		mgr.StopAll()
		listener.Close()
		os.Exit(0)
	}()

	// Accept connections
	for {
		conn, err := listener.Accept()
		if err != nil {
			logger.Log.Warn("Failed to accept connection", "error", err)
			continue
		}
		go handler.HandleConnection(conn)
	}
}
