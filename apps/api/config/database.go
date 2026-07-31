package config

import (
	"time"

	"veemon-common/infra/database"
	"veemon/entity"

	"go.uber.org/zap"
	"gorm.io/gorm"
)

func NewDatabase(cfg *Config, log *zap.Logger) (*gorm.DB, error) {
	db, err := database.New(database.Config{
		Host:     cfg.DBHost,
		Port:     cfg.DBPort,
		User:     cfg.DBUser,
		Password: cfg.DBPassword,
		Name:     cfg.DBName,
		SSLMode:  cfg.DBSSLMode,
		Timezone: cfg.DBTimezone,

		// Performance optimizations
		PrepareStmt:            cfg.DBPrepareStmt,
		SkipDefaultTransaction: cfg.DBSkipDefaultTransaction,

		// Connection pool
		MaxIdleConns:    cfg.DBMaxIdleConns,
		MaxOpenConns:    cfg.DBMaxOpenConns,
		ConnMaxLifetime: time.Duration(cfg.DBConnMaxLifetime) * time.Minute,
	}, log)
	if err != nil {
		return nil, err
	}

	// golang-migrate SQL migrations are the source of truth. AutoMigrate is
	// opt-in (DB_AUTO_MIGRATE) for local development convenience only, so
	// production schema changes always go through reviewed migrations.
	if cfg.DBAutoMigrate {
		log.Warn("DB_AUTO_MIGRATE is enabled; GORM AutoMigrate is running. " +
			"Use golang-migrate (`make migrate`) as the source of truth in production.")
		if err := database.AutoMigrate(db, &entity.User{}); err != nil {
			return nil, err
		}
	}

	return db, nil
}
