package certmanager

import (
	"context"
	"fast-https/infra/logging"
	"log/slog"

	"github.com/rs/zerolog"
)

// newACMELogger 把 acmez 的 slog 日志接到项目控制台日志器上，
// 这样协议层的失败也能落进同一份日志，而不是另开一个输出。
// acmez 不会打印账户私钥或证书内容，只打印协议交互与错误。
func newACMELogger() *slog.Logger {
	return slog.New(zerologSlogHandler{})
}

type zerologSlogHandler struct {
	attrs []slog.Attr
}

func (h zerologSlogHandler) Enabled(context.Context, slog.Level) bool { return true }

func (h zerologSlogHandler) Handle(_ context.Context, record slog.Record) error {
	event := logging.ConsoleLog.WithLevel(slogLevel(record.Level))
	for _, attr := range h.attrs {
		event = event.Str(attr.Key, attr.Value.String())
	}
	record.Attrs(func(attr slog.Attr) bool {
		event = event.Str(attr.Key, attr.Value.String())
		return true
	})
	event.Msg("acme: " + record.Message)
	return nil
}

func (h zerologSlogHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	merged := make([]slog.Attr, 0, len(h.attrs)+len(attrs))
	merged = append(merged, h.attrs...)
	merged = append(merged, attrs...)
	return zerologSlogHandler{attrs: merged}
}

func (h zerologSlogHandler) WithGroup(string) slog.Handler { return h }

func slogLevel(level slog.Level) zerolog.Level {
	switch {
	case level <= slog.LevelDebug:
		return zerolog.DebugLevel
	case level < slog.LevelWarn:
		return zerolog.InfoLevel
	case level < slog.LevelError:
		return zerolog.WarnLevel
	default:
		return zerolog.ErrorLevel
	}
}
