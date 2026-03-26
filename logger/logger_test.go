package logger

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestInitLogger_InvalidLogLevel(t *testing.T) {
	err := InitLogger("invalid", "", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid log level")
}

func TestInitLogger_SuppressConsoleWithNopCore(t *testing.T) {
	err := InitLogger("info", "", true)
	require.NoError(t, err)

	core := zap.L().Core()
	assert.False(t, core.Enabled(zapcore.InfoLevel), "NopCore should not enable any level")
}

func TestInitLogger_SuppressConsoleWithStdout(t *testing.T) {
	err := InitLogger("info", "stdout", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed when console output is suppressed")
}

func TestInitLogger_SuppressConsoleWithStderr(t *testing.T) {
	err := InitLogger("info", "stderr", true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not allowed when console output is suppressed")
}

func TestInitLogger_ValidLogLevels(t *testing.T) {
	for _, level := range []string{"debug", "info", "warn", "error"} {
		t.Run(level, func(t *testing.T) {
			err := InitLogger(level, "", true)
			require.NoError(t, err)
		})
	}
}

func TestInitLogger_ConsoleEnabled(t *testing.T) {
	err := InitLogger("info", "", false)
	require.NoError(t, err)

	core := zap.L().Core()
	assert.True(t, core.Enabled(zapcore.InfoLevel), "console core should enable info level")
}

func TestInitLogger_DebugConsoleEnabled(t *testing.T) {
	err := InitLogger("debug", "", false)
	require.NoError(t, err)

	core := zap.L().Core()
	assert.True(t, core.Enabled(zapcore.DebugLevel), "console core should enable debug level")
}

func TestInitLogger_ErrorLevelFiltering(t *testing.T) {
	err := InitLogger("error", "", false)
	require.NoError(t, err)

	core := zap.L().Core()
	assert.False(t, core.Enabled(zapcore.WarnLevel), "error level should not enable warn")
	assert.False(t, core.Enabled(zapcore.InfoLevel), "error level should not enable info")
	assert.True(t, core.Enabled(zapcore.ErrorLevel), "error level should enable error")
}
