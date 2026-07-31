// Package database provides GORM/Postgres setup and query helpers.
package database

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
	"gorm.io/plugin/opentelemetry/tracing"
)

type zapGormLogger struct {
	logger        *zap.Logger
	slowThreshold time.Duration
	level         logger.LogLevel
}

// newZapGormLogger defaults to Warn: successful queries are noisy at scale, so
// only slow queries and errors are logged by default (raise via LogMode for
// debugging). Regardless of level, Trace logs only a low-cardinality
// operation+table summary — never GORM's fully-interpolated SQL, which bakes
// literal bound values (e.g. a user's email or password hash) into the text.
func newZapGormLogger(zapLogger *zap.Logger) *zapGormLogger {
	return &zapGormLogger{
		logger:        zapLogger,
		slowThreshold: 200 * time.Millisecond,
		level:         logger.Warn,
	}
}

func (l *zapGormLogger) LogMode(level logger.LogLevel) logger.Interface {
	clone := *l
	clone.level = level
	return &clone
}

func (l *zapGormLogger) Info(ctx context.Context, msg string, data ...interface{}) {
	if l.level >= logger.Info {
		l.logger.Sugar().Infof(msg, data...)
	}
}

func (l *zapGormLogger) Warn(ctx context.Context, msg string, data ...interface{}) {
	if l.level >= logger.Warn {
		l.logger.Sugar().Warnf(msg, data...)
	}
}

func (l *zapGormLogger) Error(ctx context.Context, msg string, data ...interface{}) {
	if l.level >= logger.Error {
		l.logger.Sugar().Errorf(msg, data...)
	}
}

// sqlSelectRe extracts the target table from a SELECT's FROM clause (the
// column list between SELECT and FROM is discarded, not just skipped, so it
// can never carry a literal value). sqlWriteRe extracts the table/target name
// that immediately follows a write verb (e.g. "INSERT INTO users"). Both
// deliberately discard everything else in the statement — the WHERE/VALUES/
// SET clauses are exactly where GORM's interpolation puts literal values.
var (
	sqlSelectRe = regexp.MustCompile(`(?is)^\s*SELECT\b.*?\bFROM\s+([^\s(]+)`)
	sqlWriteRe  = regexp.MustCompile(`(?is)^\s*(INSERT\s+INTO|UPDATE|DELETE\s+FROM|CREATE\s+TABLE|ALTER\s+TABLE|DROP\s+TABLE)\s+([^\s(]+)`)
)

// sqlOperationSummary reduces a GORM-interpolated SQL string to a safe,
// low-cardinality "OPERATION table" summary for logging. It never returns any
// part of the original statement beyond the verb and target name, so it
// cannot carry bound literal values (PII, secrets) regardless of the query.
func sqlOperationSummary(sql string) string {
	if m := sqlSelectRe.FindStringSubmatch(sql); m != nil {
		return "SELECT " + strings.Trim(m[1], `"`)
	}
	if m := sqlWriteRe.FindStringSubmatch(sql); m != nil {
		return strings.ToUpper(strings.Join(strings.Fields(m[1]), " ")) + " " + strings.Trim(m[2], `"`)
	}
	return "unknown"
}

func (l *zapGormLogger) Trace(ctx context.Context, begin time.Time, fc func() (sql string, rowsAffected int64), err error) {
	if l.level <= logger.Silent {
		return
	}

	elapsed := time.Since(begin)
	sql, rows := fc()

	// Never log the fully-interpolated statement returned by fc(): GORM bakes
	// literal bound values into it, which for writes to sensitive tables (e.g.
	// users) would leak PII/secrets (email, password hash) into the log. Log
	// only a low-cardinality operation+table summary instead.
	fields := []zap.Field{
		zap.Duration("elapsed", elapsed),
		zap.Int64("rows", rows),
		zap.String("sql_summary", sqlOperationSummary(sql)),
	}

	switch {
	case err != nil && l.level >= logger.Error:
		l.logger.Error("gorm query error", append(fields, zap.Error(err))...)
	case elapsed > l.slowThreshold && l.level >= logger.Warn:
		l.logger.Warn("gorm slow query", fields...)
	case l.level >= logger.Info:
		l.logger.Debug("gorm query", fields...)
	}
}

// Config holds database connection parameters.
// Supports passwordless authentication when Password is empty (e.g., peer authentication).
type Config struct {
	Host     string
	Port     int
	User     string
	Password string // Optional: leave empty for passwordless auth (e.g., peer, trust, IAM)
	Name     string
	SSLMode  string
	Timezone string

	// Performance Settings
	PrepareStmt            bool // Enable prepared statement cache (recommended: true)
	SkipDefaultTransaction bool // Disable default transactions for write operations (use with caution)

	// Connection pool (zero values fall back to sensible defaults)
	MaxIdleConns    int
	MaxOpenConns    int
	ConnMaxLifetime time.Duration
}

func New(cfg Config, zapLogger *zap.Logger) (*gorm.DB, error) {
	// Build DSN with optional password support (for passwordless auth)
	dsn := fmt.Sprintf(
		"host=%s port=%d user=%s dbname=%s sslmode=%s TimeZone=%s",
		cfg.Host,
		cfg.Port,
		cfg.User,
		cfg.Name,
		cfg.SSLMode,
		cfg.Timezone,
	)

	// Only include password if provided (supports passwordless auth)
	if cfg.Password != "" {
		dsn = fmt.Sprintf(
			"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s TimeZone=%s",
			cfg.Host,
			cfg.Port,
			cfg.User,
			cfg.Password,
			cfg.Name,
			cfg.SSLMode,
			cfg.Timezone,
		)
	}

	// Configure GORM with performance optimizations
	gormConfig := &gorm.Config{
		Logger:                 newZapGormLogger(zapLogger),
		PrepareStmt:            cfg.PrepareStmt,            // (PERF) Cache prepared statements
		SkipDefaultTransaction: cfg.SkipDefaultTransaction, // (PERF) Skip transactions for better performance
		// Translate driver errors to GORM sentinels (e.g. unique violations to
		// gorm.ErrDuplicatedKey) so callers can handle them driver-agnostically.
		TranslateError: true,
	}

	db, err := gorm.Open(postgres.Open(dsn), gormConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}

	// Add OpenTelemetry tracing plugin
	if err := db.Use(tracing.NewPlugin()); err != nil {
		return nil, fmt.Errorf("failed to add tracing plugin: %w", err)
	}

	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get database instance: %w", err)
	}

	// Connection pool settings (configurable, with defaults)
	maxIdle := cfg.MaxIdleConns
	if maxIdle <= 0 {
		maxIdle = 10
	}
	maxOpen := cfg.MaxOpenConns
	if maxOpen <= 0 {
		maxOpen = 100
	}
	connMaxLifetime := cfg.ConnMaxLifetime
	if connMaxLifetime <= 0 {
		connMaxLifetime = time.Hour
	}
	sqlDB.SetMaxIdleConns(maxIdle)
	sqlDB.SetMaxOpenConns(maxOpen)
	sqlDB.SetConnMaxLifetime(connMaxLifetime)

	zapLogger.Info("Database connection established",
		zap.String("host", cfg.Host),
		zap.Int("port", cfg.Port),
		zap.String("database", cfg.Name),
		zap.Bool("prepare_stmt", cfg.PrepareStmt),
		zap.Bool("skip_default_transaction", cfg.SkipDefaultTransaction),
	)

	return db, nil
}

// AutoMigrate runs GORM's schema auto-migration for the given models. This
// package is domain-agnostic (no dependency on any service's entity types),
// so callers pass their own models explicitly rather than this having a
// hardcoded list.
func AutoMigrate(db *gorm.DB, models ...interface{}) error {
	return db.AutoMigrate(models...)
}

// WithContext returns a new DB with context for tracing
func WithContext(db *gorm.DB, ctx context.Context) *gorm.DB {
	return db.WithContext(ctx)
}

// Performance Helper Functions

// WithPreparedStmt creates a session with prepared statement enabled (performance boost)
// Use this for repeated queries with different parameters
//
// Example:
//
//	tx := database.WithPreparedStmt(db)
//	tx.First(&user, 1)
//	tx.Find(&users)
func WithPreparedStmt(db *gorm.DB) *gorm.DB {
	return db.Session(&gorm.Session{PrepareStmt: true})
}

// WithTransaction creates a function that runs operations within a transaction
// Only use when you need ACID guarantees (SkipDefaultTransaction bypasses this)
//
// Example:
//
//	err := database.WithTransaction(db, func(tx *gorm.DB) error {
//	    if err := tx.Create(&user).Error; err != nil {
//	        return err
//	    }
//	    if err := tx.Create(&profile).Error; err != nil {
//	        return err
//	    }
//	    return nil
//	})
func WithTransaction(db *gorm.DB, fn func(*gorm.DB) error) error {
	return db.Transaction(fn)
}

// BatchProcessor defines a function type for processing records in batches
type BatchProcessor func(tx *gorm.DB, batch int) error

// FindInBatches processes records in batches to reduce memory usage
// Useful for large datasets that don't fit in memory
//
// Example:
//
//	err := database.FindInBatches(db, &users, 1000, func(tx *gorm.DB, batch int) error {
//	    for _, user := range users {
//	        // Process each user
//	    }
//	    return nil
//	})
func FindInBatches(db *gorm.DB, dest interface{}, batchSize int, processor BatchProcessor) error {
	return db.FindInBatches(dest, batchSize, processor).Error
}

// SelectFields helper to select specific fields (avoid SELECT *)
//
// Example:
//
//	users := database.SelectFields(db, "id", "name", "email").Find(&users)
func SelectFields(db *gorm.DB, fields ...string) *gorm.DB {
	return db.Select(fields)
}

// Paginate helper for pagination
//
// Example:
//
//	users := database.Paginate(db, 1, 20).Find(&users) // page 1, 20 per page
func Paginate(db *gorm.DB, page, pageSize int) *gorm.DB {
	offset := (page - 1) * pageSize
	return db.Offset(offset).Limit(pageSize)
}
