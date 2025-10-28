package utils

import (
	"os"
	"time"

	"github.com/cppla/aibbs/config"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	lumberjack "gopkg.in/natefinch/lumberjack.v2"
)

var (
	// Logger is the global structured logger
	Logger *zap.Logger
	// Sugar is a sugared logger for convenience
	Sugar *zap.SugaredLogger
)

// InitLogger initializes a zap logger with console + rolling file outputs based on configuration.
func InitLogger(cfg config.AppConfig) error {
	// Ensure logs directory exists if Path includes one
	if cfg.LogPath != "" {
		if dir := dirOf(cfg.LogPath); dir != "" {
			_ = os.MkdirAll(dir, 0o755)
		}
	}

	// Level
	level := parseLevel(cfg.LogLevel)

	// Encoder config
	encCfg := zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     timeEncoder,
		EncodeDuration: zapcore.SecondsDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	consoleEncoder := zapcore.NewJSONEncoder(encCfg)

	// File sink via lumberjack
	var cores []zapcore.Core
	consoleWS := zapcore.AddSync(os.Stdout)
	cores = append(cores, zapcore.NewCore(consoleEncoder, consoleWS, levelToConsoleEnabler(level)))

	if cfg.LogPath != "" {
		lj := &lumberjack.Logger{
			Filename:   cfg.LogPath,
			MaxSize:    nz(cfg.LogMaxSizeMB, 100), // megabytes
			MaxBackups: nz(cfg.LogMaxBackups, 3),
			MaxAge:     nz(cfg.LogMaxAgeDays, 7), // days
			Compress:   cfg.LogCompress,
		}
		fileWS := zapcore.AddSync(lj)
		fileEncoder := zapcore.NewJSONEncoder(encCfg)
		cores = append(cores, zapcore.NewCore(fileEncoder, fileWS, levelToFileEnabler(level)))
	}

	core := zapcore.NewTee(cores...)

	opts := []zap.Option{zap.AddCaller()}
	if cfg.LogLevel == "debug" {
		opts = append(opts, zap.Development())
	}
	Logger = zap.New(core, opts...)
	Sugar = Logger.Sugar()
	return nil
}

func timeEncoder(t time.Time, enc zapcore.PrimitiveArrayEncoder) {
	enc.AppendString(t.Format("2006-01-02 15:04:05.000"))
}

func parseLevel(s string) zapcore.Level {
	switch s {
	case "debug":
		return zapcore.DebugLevel
	case "info", "":
		return zapcore.InfoLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	case "dpanic":
		return zapcore.DPanicLevel
	case "panic":
		return zapcore.PanicLevel
	case "fatal":
		return zapcore.FatalLevel
	default:
		return zapcore.InfoLevel
	}
}

// Console prints everything at level and above
func levelToConsoleEnabler(level zapcore.Level) zapcore.LevelEnabler {
	return zap.LevelEnablerFunc(func(l zapcore.Level) bool { return l >= level })
}

// File prints info and above by default (can be adjusted by level)
func levelToFileEnabler(level zapcore.Level) zapcore.LevelEnabler {
	// Follow the same level for simplicity
	return zap.LevelEnablerFunc(func(l zapcore.Level) bool { return l >= level })
}

func nz(v, def int) int {
	if v == 0 {
		return def
	}
	return v
}

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' {
			if i == 0 {
				return "/"
			}
			return path[:i]
		}
	}
	return ""
}
