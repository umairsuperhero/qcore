package config

import (
	"path/filepath"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func testdataPath(name string) string {
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "testdata", name)
}

func TestLoad_ValidConfig(t *testing.T) {
	cfg, err := Load(testdataPath("config_valid.yaml"))
	require.NoError(t, err)

	assert.Equal(t, "test-hss", cfg.HSS.Name)
	assert.Equal(t, "127.0.0.1", cfg.HSS.BindAddress)
	assert.Equal(t, 9999, cfg.HSS.APIPort)
	assert.Equal(t, "db.example.com", cfg.Database.Host)
	assert.Equal(t, 5433, cfg.Database.Port)
	assert.Equal(t, "testdb", cfg.Database.Name)
	assert.Equal(t, "testuser", cfg.Database.User)
	assert.Equal(t, "testpass", cfg.Database.Password)
	assert.Equal(t, "require", cfg.Database.SSLMode)
	assert.Equal(t, "debug", cfg.Logging.Level)
	assert.Equal(t, "json", cfg.Logging.Format)
	assert.False(t, cfg.Metrics.Enabled)
	assert.Equal(t, 9191, cfg.Metrics.Port)
}

func TestLoad_MinimalConfig(t *testing.T) {
	cfg, err := Load(testdataPath("config_minimal.yaml"))
	require.NoError(t, err)

	assert.Equal(t, "minimal-hss", cfg.HSS.Name)
	// Defaults should be populated
	assert.Equal(t, "0.0.0.0", cfg.HSS.BindAddress)
	assert.Equal(t, 8080, cfg.HSS.APIPort)
	assert.Equal(t, "localhost", cfg.Database.Host)
	assert.Equal(t, 5432, cfg.Database.Port)
	assert.Equal(t, "info", cfg.Logging.Level)
}

func TestLoad_NoFile(t *testing.T) {
	cfg, err := Load("")
	require.NoError(t, err)

	// All defaults
	assert.Equal(t, "qcore-hss", cfg.HSS.Name)
	assert.Equal(t, 8080, cfg.HSS.APIPort)
}

func TestLoad_MissingFile(t *testing.T) {
	_, err := Load("/nonexistent/path/config.yaml")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reading config file")
}

func TestLoad_EnvOverride(t *testing.T) {
	t.Setenv("QCORE_HSS_API_PORT", "4444")
	t.Setenv("QCORE_DATABASE_HOST", "env-host")

	cfg, err := Load("")
	require.NoError(t, err)

	assert.Equal(t, 4444, cfg.HSS.APIPort)
	assert.Equal(t, "env-host", cfg.Database.Host)
}

func TestDatabaseConfig_DSN(t *testing.T) {
	cfg := DatabaseConfig{
		Host:     "localhost",
		Port:     5432,
		Name:     "testdb",
		User:     "testuser",
		Password: "testpass",
		SSLMode:  "disable",
	}
	dsn := cfg.DSN()
	assert.Contains(t, dsn, "host=localhost")
	assert.Contains(t, dsn, "port=5432")
	assert.Contains(t, dsn, "dbname=testdb")
	assert.Contains(t, dsn, "user=testuser")
	assert.Contains(t, dsn, "password=testpass")
	assert.Contains(t, dsn, "sslmode=disable")
}
