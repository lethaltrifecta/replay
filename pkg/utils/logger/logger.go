package logger

import (
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Logger wraps zap.SugaredLogger with convenience methods
type Logger struct {
	*zap.SugaredLogger
}

// New creates a new logger based on configuration
func New(logLevel string) (*Logger, error) {
	var zapConfig zap.Config

	switch logLevel {
	case "debug":
		zapConfig = zap.NewDevelopmentConfig()
	case "info", "warn", "error":
		zapConfig = zap.NewProductionConfig()
		zapConfig.Level = zap.NewAtomicLevelAt(parseLevel(logLevel))
	default:
		zapConfig = zap.NewProductionConfig()
	}

	// Customize encoding
	zapConfig.Encoding = "json"
	zapConfig.OutputPaths = []string{"stdout"}
	zapConfig.ErrorOutputPaths = []string{"stderr"}

	logger, err := zapConfig.Build()
	if err != nil {
		return nil, err
	}

	return &Logger{SugaredLogger: logger.Sugar()}, nil
}

// parseLevel converts string level to zap level
func parseLevel(level string) zapcore.Level {
	switch level {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return zapcore.InfoLevel
	}
}

// WithField adds a field to the logger
func (l *Logger) WithField(key string, value interface{}) *Logger {
	return &Logger{SugaredLogger: l.SugaredLogger.With(key, value)}
}

// WithFields adds multiple fields to the logger
func (l *Logger) WithFields(fields map[string]interface{}) *Logger {
	args := make([]interface{}, 0, len(fields)*2)
	for k, v := range fields {
		args = append(args, k, v)
	}
	return &Logger{SugaredLogger: l.SugaredLogger.With(args...)}
}

// WithError adds an error field to the logger
func (l *Logger) WithError(err error) *Logger {
	return &Logger{SugaredLogger: l.SugaredLogger.With("error", err)}
}
