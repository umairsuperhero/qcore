package database

import (
	"fmt"
	"time"

	"github.com/qcore-project/qcore/pkg/config"
	"github.com/qcore-project/qcore/pkg/logger"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

func Connect(cfg *config.DatabaseConfig, log logger.Logger) (*gorm.DB, error) {
	dsn := cfg.DSN()

	gormCfg := &gorm.Config{
		Logger: gormlogger.New(
			newGormWriter(log),
			gormlogger.Config{
				SlowThreshold: 200 * time.Millisecond,
				LogLevel:      gormlogger.Warn,
			},
		),
	}

	var db *gorm.DB
	var err error

	for attempt := 1; attempt <= 5; attempt++ {
		db, err = gorm.Open(postgres.Open(dsn), gormCfg)
		if err == nil {
			break
		}
		wait := time.Duration(1<<uint(attempt-1)) * time.Second
		log.Warnf("Database connection attempt %d/5 failed, retrying in %v: %v", attempt, wait, err)
		time.Sleep(wait)
	}

	if err != nil {
		return nil, fmt.Errorf("connecting to database after 5 attempts: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("getting underlying sql.DB: %w", err)
	}

	sqlDB.SetMaxOpenConns(cfg.MaxOpenConns)
	sqlDB.SetMaxIdleConns(cfg.MaxIdleConns)
	sqlDB.SetConnMaxLifetime(time.Duration(cfg.ConnMaxLifetime) * time.Second)

	log.Info("Connected to database")
	return db, nil
}

func AutoMigrate(db *gorm.DB, models ...interface{}) error {
	if err := db.AutoMigrate(models...); err != nil {
		return fmt.Errorf("running auto migration: %w", err)
	}
	return nil
}

func HealthCheck(db *gorm.DB) error {
	sqlDB, err := db.DB()
	if err != nil {
		return fmt.Errorf("getting sql.DB for health check: %w", err)
	}
	return sqlDB.Ping()
}

type gormWriter struct {
	log logger.Logger
}

func newGormWriter(log logger.Logger) *gormWriter {
	return &gormWriter{log: log}
}

func (w *gormWriter) Printf(format string, args ...interface{}) {
	w.log.Warnf(format, args...)
}
