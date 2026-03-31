package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var Log *zap.SugaredLogger

// Init initializes the global logger.
// For the daemon, use production config; for CLI, use development config.
func Init(mode string) error {
	var cfg zap.Config

	switch mode {
	case "daemon":
		cfg = zap.NewProductionConfig()
		cfg.OutputPaths = []string{"/var/log/pqpm/pqpmd.log", "stdout"}
		cfg.ErrorOutputPaths = []string{"/var/log/pqpm/pqpmd.log", "stderr"}
	default:
		cfg = zap.NewDevelopmentConfig()
		cfg.EncoderConfig.EncodeLevel = zapcore.CapitalColorLevelEncoder
	}

	logger, err := cfg.Build()
	if err != nil {
		return err
	}

	Log = logger.Sugar()
	return nil
}

// Sync flushes any buffered log entries.
func Sync() {
	if Log != nil {
		_ = Log.Sync()
	}
}
