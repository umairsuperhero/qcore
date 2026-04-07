package logger

import (
	"context"
	"io"
	"os"

	"github.com/sirupsen/logrus"
)

type contextKey struct{}

type Logger interface {
	Debug(args ...interface{})
	Info(args ...interface{})
	Warn(args ...interface{})
	Error(args ...interface{})
	Fatal(args ...interface{})
	Debugf(format string, args ...interface{})
	Infof(format string, args ...interface{})
	Warnf(format string, args ...interface{})
	Errorf(format string, args ...interface{})
	Fatalf(format string, args ...interface{})
	WithField(key string, value interface{}) Logger
	WithFields(fields map[string]interface{}) Logger
	WithError(err error) Logger
	Writer() io.Writer
}

type logrusLogger struct {
	entry *logrus.Entry
}

func New(level, format string) Logger {
	log := logrus.New()
	log.SetOutput(os.Stdout)

	lvl, err := logrus.ParseLevel(level)
	if err != nil {
		lvl = logrus.InfoLevel
	}
	log.SetLevel(lvl)

	switch format {
	case "json":
		log.SetFormatter(&logrus.JSONFormatter{
			TimestampFormat: "2006-01-02T15:04:05.000Z07:00",
		})
	default:
		log.SetFormatter(&logrus.TextFormatter{
			FullTimestamp:   true,
			TimestampFormat: "15:04:05",
			ForceColors:     true,
		})
	}

	return &logrusLogger{entry: logrus.NewEntry(log)}
}

func NewContext(ctx context.Context, l Logger) context.Context {
	return context.WithValue(ctx, contextKey{}, l)
}

func FromContext(ctx context.Context) Logger {
	if l, ok := ctx.Value(contextKey{}).(Logger); ok {
		return l
	}
	return New("info", "console")
}

func (l *logrusLogger) Debug(args ...interface{})                 { l.entry.Debug(args...) }
func (l *logrusLogger) Info(args ...interface{})                  { l.entry.Info(args...) }
func (l *logrusLogger) Warn(args ...interface{})                  { l.entry.Warn(args...) }
func (l *logrusLogger) Error(args ...interface{})                 { l.entry.Error(args...) }
func (l *logrusLogger) Fatal(args ...interface{})                 { l.entry.Fatal(args...) }
func (l *logrusLogger) Debugf(format string, args ...interface{}) { l.entry.Debugf(format, args...) }
func (l *logrusLogger) Infof(format string, args ...interface{})  { l.entry.Infof(format, args...) }
func (l *logrusLogger) Warnf(format string, args ...interface{})  { l.entry.Warnf(format, args...) }
func (l *logrusLogger) Errorf(format string, args ...interface{}) { l.entry.Errorf(format, args...) }
func (l *logrusLogger) Fatalf(format string, args ...interface{}) { l.entry.Fatalf(format, args...) }

func (l *logrusLogger) WithField(key string, value interface{}) Logger {
	return &logrusLogger{entry: l.entry.WithField(key, value)}
}

func (l *logrusLogger) WithFields(fields map[string]interface{}) Logger {
	return &logrusLogger{entry: l.entry.WithFields(fields)}
}

func (l *logrusLogger) WithError(err error) Logger {
	return &logrusLogger{entry: l.entry.WithError(err)}
}

func (l *logrusLogger) Writer() io.Writer {
	return l.entry.Writer()
}
