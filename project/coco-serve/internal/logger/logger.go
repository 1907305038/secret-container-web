package logger

import (
	"os"
	"path/filepath"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// 5 个分类 Logger
var (
	Access *zap.Logger // HTTP 请求日志
	Pod    *zap.Logger // Pod 生命周期
	Memory *zap.Logger // 内存验证
	Audit  *zap.Logger // 安全审计
	System *zap.Logger // 系统和错误
)

// Init 初始化所有日志实例，dir 为日志目录路径
func Init(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	encoderConfig := zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.TimeEncoderOfLayout("15:04:05"),
		EncodeDuration: zapcore.StringDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	// 控制台输出：彩色、简单格式
	consoleCfg := encoderConfig
	consoleCfg.EncodeLevel = zapcore.CapitalColorLevelEncoder
	consoleCore := zapcore.NewCore(
		zapcore.NewConsoleEncoder(consoleCfg),
		zapcore.AddSync(os.Stdout),
		zapcore.InfoLevel,
	)

	// 每个分类 Logger：同时输出到控制台和对应文件
	Access = newLogger(consoleCore, filepath.Join(dir, "access.log"))
	Pod = newLogger(consoleCore, filepath.Join(dir, "pod.log"))
	Memory = newLogger(consoleCore, filepath.Join(dir, "memory.log"))
	Audit = newLogger(consoleCore, filepath.Join(dir, "audit.log"))
	System = newLogger(consoleCore, filepath.Join(dir, "system.log"))

	System.Info("logger initialized", zap.String("dir", dir))
	return nil
}

// newLogger 创建同时输出到控制台和文件的 Logger
func newLogger(consoleCore zapcore.Core, filePath string) *zap.Logger {
	file, err := os.OpenFile(filePath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		// 文件创建失败则只输出到控制台
		return zap.New(consoleCore, zap.AddCaller())
	}

	fileEncoder := zapcore.NewConsoleEncoder(zapcore.EncoderConfig{
		TimeKey:        "time",
		LevelKey:       "level",
		MessageKey:     "msg",
		EncodeLevel:    zapcore.CapitalLevelEncoder,
		EncodeTime:     zapcore.TimeEncoderOfLayout("2006-01-02 15:04:05"),
		EncodeDuration: zapcore.StringDurationEncoder,
	})

	fileCore := zapcore.NewCore(fileEncoder, zapcore.AddSync(file), zapcore.InfoLevel)
	teeCore := zapcore.NewTee(consoleCore, fileCore)
	return zap.New(teeCore, zap.AddCaller())
}

// Sync 刷新所有 Logger 缓冲区
func Sync() {
	for _, l := range []*zap.Logger{Access, Pod, Memory, Audit, System} {
		_ = l.Sync()
	}
}
