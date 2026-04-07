package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"testing"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNew_JSONFormat(t *testing.T) {
	l := New("info", "json")
	require.NotNil(t, l)

	ll := l.(*logrusLogger)
	_, ok := ll.entry.Logger.Formatter.(*logrus.JSONFormatter)
	assert.True(t, ok, "expected JSON formatter")
}

func TestNew_ConsoleFormat(t *testing.T) {
	l := New("debug", "console")
	require.NotNil(t, l)

	ll := l.(*logrusLogger)
	_, ok := ll.entry.Logger.Formatter.(*logrus.TextFormatter)
	assert.True(t, ok, "expected Text formatter")
}

func TestNew_InvalidLevel(t *testing.T) {
	l := New("invalid", "json")
	require.NotNil(t, l, "should default to info level on invalid input")
}

func TestLogger_JSONOutput(t *testing.T) {
	var buf bytes.Buffer
	l := New("debug", "json")
	ll := l.(*logrusLogger)
	ll.entry.Logger.SetOutput(&buf)

	l.Info("test message")

	var entry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)
	assert.Equal(t, "test message", entry["msg"])
	assert.Equal(t, "info", entry["level"])
}

func TestLogger_WithField(t *testing.T) {
	var buf bytes.Buffer
	l := New("info", "json")
	ll := l.(*logrusLogger)
	ll.entry.Logger.SetOutput(&buf)

	l.WithField("key", "value").Info("with field")

	var entry map[string]interface{}
	err := json.Unmarshal(buf.Bytes(), &entry)
	require.NoError(t, err)
	assert.Equal(t, "value", entry["key"])
}

func TestLogger_LevelFiltering(t *testing.T) {
	var buf bytes.Buffer
	l := New("warn", "json")
	ll := l.(*logrusLogger)
	ll.entry.Logger.SetOutput(&buf)

	l.Info("should not appear")
	assert.Empty(t, buf.String(), "info should be filtered at warn level")

	l.Warn("should appear")
	assert.NotEmpty(t, buf.String(), "warn should pass at warn level")
}

func TestContext_RoundTrip(t *testing.T) {
	l := New("info", "json")
	ctx := NewContext(context.Background(), l)
	recovered := FromContext(ctx)
	assert.Equal(t, l, recovered)
}

func TestFromContext_Default(t *testing.T) {
	l := FromContext(context.Background())
	assert.NotNil(t, l, "should return default logger when none in context")
}
