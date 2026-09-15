package log

import (
	"os"
	"path/filepath"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"

	"antelope/internal/modules/setting"
)

var (
	global = zap.NewNop()
	sugar  = zap.NewNop().Sugar()
	once   sync.Once
)

// Init configures the global logger from settings. Must be called once at
// startup, before any other code calls L() or S(). Calling it again after
// the first call is a no-op.
func Init(cfg setting.ZapConfig) error {
	var initErr error
	once.Do(func() {
		logger, err := buildZapLogger(cfg)
		if err != nil {
			initErr = err
			return
		}
		global = logger
		sugar = logger.Sugar()
		zap.ReplaceGlobals(logger)
	})
	return initErr
}

// L returns the global structured logger. Safe to call from any goroutine.
// Before Init is called, returns a no-op logger.
func L() *zap.Logger {
	return global
}

// S returns the global sugared logger for printf-style logging.
// Prefer L() and structured fields for production code paths.
func S() *zap.SugaredLogger {
	return sugar
}

// Sync flushes any buffered log entries. Call via defer before process exit.
func Sync() error {
	return global.Sync()
}

// Close no error control as it's the last step before process exit
func Close() {
	_ = global.Sync()
}

func buildZapLogger(cfg setting.ZapConfig) (*zap.Logger, error) {
	level, err := zapcore.ParseLevel(cfg.Level)
	if err != nil {
		level = zapcore.DebugLevel
	}

	// Create the zap encoder for the log files
	encoder := cfg.Encoder()

	var cores []zapcore.Core

	// Collect all levels from the configured minimum up to Fatal, one file each.
	levels := cfg.Levels()
	for _, lvl := range levels {
		// lvl := lvl  // loop variable capture fix; not needed in Go 1.22+ (this project uses Go 1.24)
		filename := filepath.Join(cfg.Director, lvl.String()+".log")
		fileWriter := zapcore.AddSync(&lumberjack.Logger{
			Filename:   filename,
			MaxBackups: cfg.MaxBackUps,
			MaxAge:     cfg.RetentionDay,
			Compress:   true,
		})
		enabler := zap.LevelEnablerFunc(func(l zapcore.Level) bool { return l == lvl })
		cores = append(cores, zapcore.NewCore(encoder, fileWriter, enabler))
	}

	if cfg.LogInConsole {
		stderrWriter := zapcore.AddSync(os.Stderr)
		cores = append(cores, zapcore.NewCore(encoder, stderrWriter, level))
	}

	if len(cores) == 0 {
		cores = append(cores, zapcore.NewCore(encoder, zapcore.AddSync(os.Stderr), level))
	}

	opts := []zap.Option{zap.AddStacktrace(zapcore.ErrorLevel)}
	if cfg.ShowLine {
		opts = append(opts, zap.AddCaller())
	}

	return zap.New(zapcore.NewTee(cores...), opts...), nil
}
