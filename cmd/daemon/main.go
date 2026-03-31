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
	// Initialize logger
	if err := logger.Init("daemon"); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to initialize logger: %v\n", err)
		os.Exit(1)
	}
	defer logger.Sync()

	logger.Log.Info("Starting PQPM daemon (pqpmd)...")

	// Verify running as root (required to drop privileges)
	if os.Geteuid() != 0 {
		logger.Log.Fatal("pqpmd must be run as root to manage user processes")
	}

	// Create process manager
	mgr := process.NewManager()

	// Create request handler
	handler := daemon.NewHandler(mgr)

	// Start listening on Unix socket
	listener, err := socket.Listen()
	if err != nil {
		logger.Log.Fatalw("Failed to start socket listener", "error", err)
	}
	defer listener.Close()

	logger.Log.Infow("Daemon listening", "socket", socket.SocketPath)

	// Handle graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		sig := <-sigCh
		logger.Log.Infow("Received shutdown signal", "signal", sig)
		logger.Log.Info("Stopping all managed processes...")
		mgr.StopAll()
		listener.Close()
		logger.Sync()
		os.Exit(0)
	}()

	// Accept connections
	for {
		conn, err := listener.Accept()
		if err != nil {
			logger.Log.Warnw("Failed to accept connection", "error", err)
			continue
		}
		go handler.HandleConnection(conn)
	}
}
