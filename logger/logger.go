package logger

import (
	"fmt"
	"os"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// InitLogger initializes the global logger.
// suppressConsole should be true for stdio transport to avoid MCP protocol interference.
func InitLogger(logLevel string, logPath string, suppressConsole bool) error {
	level, err := zapcore.ParseLevel(logLevel)
	if err != nil {
		return fmt.Errorf("invalid log level %q: %w", logLevel, err)
	}

	if suppressConsole && (logPath == "stdout" || logPath == "stderr") {
		return fmt.Errorf("log path %q is not allowed when console output is suppressed (stdio mode)", logPath)
	}

	var cores []zapcore.Core

	if !suppressConsole {
		cores = append(cores, buildConsoleCores(level)...)
	}

	if logPath != "" && logPath != "stdout" && logPath != "stderr" {
		fileCore, err := buildFileCore(level, logPath)
		if err != nil {
			return fmt.Errorf("failed to open log file %q: %w", logPath, err)
		}
		cores = append(cores, fileCore)
	}

	var core zapcore.Core
	if len(cores) == 0 {
		core = zapcore.NewNopCore()
	} else {
		core = zapcore.NewTee(cores...)
	}

	logger := zap.New(core,
		zap.AddCaller(),
		zap.AddStacktrace(zapcore.ErrorLevel),
	)
	zap.ReplaceGlobals(logger)

	zap.S().Infow("Logger initialized",
		"log_level", logLevel,
		"log_path", logPath,
		"suppress_console", suppressConsole,
	)

	return nil
}

func buildConsoleCores(level zapcore.Level) []zapcore.Core {
	var encoder zapcore.Encoder
	if level <= zapcore.DebugLevel {
		cfg := zap.NewDevelopmentEncoderConfig()
		cfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
		encoder = zapcore.NewConsoleEncoder(cfg)
	} else {
		encoder = zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig())
	}

	infoAndBelow := zap.LevelEnablerFunc(func(l zapcore.Level) bool {
		return l >= level && l < zapcore.WarnLevel
	})
	warnAndAbove := zap.LevelEnablerFunc(func(l zapcore.Level) bool {
		return l >= level && l >= zapcore.WarnLevel
	})

	return []zapcore.Core{
		zapcore.NewCore(encoder, zapcore.Lock(zapcore.AddSync(os.Stdout)), infoAndBelow),
		zapcore.NewCore(encoder.Clone(), zapcore.Lock(zapcore.AddSync(os.Stderr)), warnAndAbove),
	}
}

func buildFileCore(level zapcore.Level, logPath string) (zapcore.Core, error) {
	file, err := os.OpenFile(logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	prodEncoderConfig := zap.NewProductionEncoderConfig()
	jsonEncoder := zapcore.NewJSONEncoder(prodEncoderConfig)

	return zapcore.NewCore(jsonEncoder, zapcore.Lock(zapcore.AddSync(file)), level), nil
}

// Sync flushes any buffered log entries
func Sync() error {
	if err := zap.S().Sync(); err != nil {
		return err
	}
	if err := zap.L().Sync(); err != nil {
		return err
	}

	return nil
}
