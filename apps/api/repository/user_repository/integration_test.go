//go:build integration

// Integration tests that require a real PostgreSQL (run with:
//
//	go test -tags integration ./repository/user_repository/...
//
// with DB_* env vars pointing at a database that has the migrations applied).
package user_repository_test

import (
	"context"
	"os"
	"strconv"
	"testing"

	"veemon-common/infra/database"
	"veemon/entity"
	"veemon/repository/user_repository"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	port, _ := strconv.Atoi(envOr("DB_PORT", "5432"))
	db, err := database.New(database.Config{
		Host:     envOr("DB_HOST", "localhost"),
		Port:     port,
		User:     envOr("DB_USER", "postgres"),
		Password: envOr("DB_PASSWORD", "postgres"),
		Name:     envOr("DB_NAME", "veemon_db"),
		SSLMode:  envOr("DB_SSL_MODE", "disable"),
		Timezone: envOr("DB_TIMEZONE", "UTC"),
	}, zap.NewNop())
	require.NoError(t, err)
	return db
}

// The regression that motivated this test: Roles ([]string over text[]) could
// not be scanned by GORM/pgx, so every user read failed. This proves the field
// round-trips through a real Postgres.
func TestIntegration_UserRolesRoundTrip(t *testing.T) {
	repo := user_repository.New(testDB(t))
	ctx := context.Background()

	email := "int-" + uuid.NewString() + "@example.com"
	u := &entity.User{
		Email:    email,
		Password: "hash",
		Name:     "Integration User",
		Roles:    pq.StringArray{"admin", "user"},
		Status:   entity.UserStatusActive,
	}
	require.NoError(t, repo.Create(ctx, u))
	t.Cleanup(func() { _ = repo.Delete(ctx, u.ID) })

	got, err := repo.FindByID(ctx, u.ID)
	require.NoError(t, err)
	require.Equal(t, []string{"admin", "user"}, []string(got.Roles))

	// FindByEmail is the Login read path.
	byEmail, err := repo.FindByEmail(ctx, email)
	require.NoError(t, err)
	require.Equal(t, u.ID, byEmail.ID)

	// FindAll is the ListUsers read path.
	list, total, err := repo.FindAll(ctx, user_repository.ListParams{Page: 1, Size: 50, Search: "Integration User"})
	require.NoError(t, err)
	require.GreaterOrEqual(t, total, int64(1))
	require.NotEmpty(t, list)
}

// Proves the partial unique index: a soft-deleted user's email can be reused.
func TestIntegration_SoftDeletedEmailReusable(t *testing.T) {
	repo := user_repository.New(testDB(t))
	ctx := context.Background()

	email := "reuse-" + uuid.NewString() + "@example.com"
	first := &entity.User{Email: email, Password: "h", Name: "First", Status: entity.UserStatusActive}
	require.NoError(t, repo.Create(ctx, first))
	require.NoError(t, repo.Delete(ctx, first.ID)) // soft delete

	second := &entity.User{Email: email, Password: "h", Name: "Second", Status: entity.UserStatusActive}
	require.NoError(t, repo.Create(ctx, second), "re-registering a soft-deleted email should succeed")
	t.Cleanup(func() { _ = repo.Delete(ctx, second.ID) })
}
