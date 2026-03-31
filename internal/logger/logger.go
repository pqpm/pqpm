package logger

import (
	"io"
	"log/slog"
	"os"
)

// SLog is the global structured logger.
var Log *slog.Logger

// Init initializes the global slog logger.
// mode can be "daemon" for production-style JSON logging or "cli" for text logging.
func Init(mode string) error {
	var handler slog.Handler

	if mode == "daemon" {
		// For daemon, log to both file and stdout/stderr
		logFile, err := os.OpenFile("/var/log/pqpm/pqpmd.log", os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return err
		}

		// MultiWriter to log to both file and stdout
		mw := io.MultiWriter(os.Stdout, logFile)
		handler = slog.NewJSONHandler(mw, &slog.HandlerOptions{
			Level: slog.LevelInfo,
		})
	} else {
		// For CLI, use text handler on stdout
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
			Level: slog.LevelDebug,
		})
	}

	Log = slog.New(handler)
	slog.SetDefault(Log)
	return nil
}

// Sync is a no-op for slog (kept for compatibility with previous zap-based code)
func Sync() {
}
