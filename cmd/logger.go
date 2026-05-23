package cmd

import (
	"os"

	"github.com/mattn/go-isatty"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func newZapEncoderConfig(debug bool) zapcore.EncoderConfig {
	result := zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		NameKey:        "channel",
		MessageKey:     "message",
		StacktraceKey:  "stacktrace",
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	if debug {
		result.EncodeCaller = zapcore.FullCallerEncoder
	}

	return result
}

func getZapEncoding() string {
	if isatty.IsTerminal(os.Stdout.Fd()) || isatty.IsCygwinTerminal(os.Stdout.Fd()) {
		return "console"
	}

	return "json"
}

func getZapCfg(debug bool, level int) zap.Config {
	return zap.Config{
		Level:       zap.NewAtomicLevelAt(zapcore.Level(level)),
		Development: debug,
		Sampling: &zap.SamplingConfig{
			Initial:    100,
			Thereafter: 100,
		},
		Encoding:         getZapEncoding(),
		EncoderConfig:    newZapEncoderConfig(debug),
		OutputPaths:      []string{"stderr"},
		ErrorOutputPaths: []string{"stderr"},
	}
}

func createLogger(debug bool, level int) *zap.Logger {
	logger, err := getZapCfg(debug, level).Build()
	if err != nil {
		panic(err)
	}

	return logger
}

func createLoggerAndOverrideStdLog(debug bool, level int) *zap.Logger {
	logger := createLogger(debug, level)
	zap.RedirectStdLog(logger)

	return logger
}
