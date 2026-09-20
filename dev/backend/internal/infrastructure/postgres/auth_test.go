package postgres_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/domain"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/infrastructure/postgres"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/usecase"
)

func TestAuthIntegration(t *testing.T) {
	database := os.Getenv("KNOWLEDGE_TEST_DATABASE_URL")
	if database == "" {
		t.Skip("set KNOWLEDGE_TEST_DATABASE_URL to a disposable local PostgreSQL")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, database)
	require.NoError(t, err)

	defer pool.Close()

	schema := "auth_test_" + time.Now().Format("20060102150405000000")
	_, err = pool.Exec(ctx, "CREATE SCHEMA "+schema)
	require.NoError(t, err)

	defer func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cleanupCancel()

		_, err := pool.Exec(cleanupCtx, "DROP SCHEMA "+schema+" CASCADE")
		require.NoError(t, err)
	}()

	cfg, err := pgxpool.ParseConfig(database)
	require.NoError(t, err)

	cfg.ConnConfig.RuntimeParams["search_path"] = schema
	testPool, err := pgxpool.NewWithConfig(ctx, cfg)
	require.NoError(t, err)

	defer testPool.Close()

	for _, name := range []string{"000001_knowledge.up.sql", "000002_auth.up.sql"} {
		migration, err := os.ReadFile(filepath.Join("../../../migrations", name))
		require.NoError(t, err)
		_, err = testPool.Exec(ctx, string(migration))
		require.NoError(t, err)
	}

	_, err = testPool.Exec(ctx, `INSERT INTO auth_users(github_id,display_name) VALUES('42','user')`)
	require.NoError(t, err)

	repo := postgres.NewAuthRepository(testPool)
	session := usecase.RefreshSession{
		SessionID: "11111111-1111-4111-8111-111111111111",
		FamilyID:  "22222222-2222-4222-8222-222222222222",
		UserID:    "42",
	}
	require.NoError(t, repo.CreateRefreshSession(ctx, session, tokenHash("first"), time.Now().Add(time.Hour)))
	_, err = repo.RotateRefreshToken(ctx, tokenHash("first"), tokenHash("second"), time.Now().Add(time.Hour))
	require.NoError(t, err)

	familyLock := lockFamily(t, testPool, session.FamilyID)
	rotationResults := make(chan error, 2)
	rotationDone := false

	defer func() {
		rollbackTx(familyLock)

		cancel()

		if !rotationDone {
			awaitResults(rotationResults, 2)
		}
	}()

	go func() {
		_, err := repo.RotateRefreshToken(ctx, tokenHash("second"), tokenHash("third"), time.Now().Add(time.Hour))
		rotationResults <- err
	}()
	go func() {
		_, err := repo.RotateRefreshToken(ctx, tokenHash("first"), tokenHash("reuse"), time.Now().Add(time.Hour))
		rotationResults <- err
	}()

	waitForFamilyLockWaiters(t, testPool, 2)
	require.NoError(t, familyLock.Commit(ctx))

	var sawReuse, sawRotationOutcome bool

	for range 2 {
		err := <-rotationResults
		switch {
		case errors.Is(err, domain.ErrRefreshReuse):
			sawReuse = true
		case err == nil, errors.Is(err, domain.ErrUnauthenticated):
			sawRotationOutcome = true
		default:
			t.Fatalf("unexpected rotation error: %v", err)
		}
	}

	rotationDone = true

	require.True(t, sawReuse)
	require.True(t, sawRotationOutcome)

	active, err := repo.Active(ctx, domain.Principal{OwnerID: "42", SessionID: session.SessionID})
	require.NoError(t, err)
	require.False(t, active)
	assertRefreshTokensRejected(t, repo, ctx, "first", "second", "third")

	second := usecase.RefreshSession{
		SessionID: "33333333-3333-4333-8333-333333333333",
		FamilyID:  "44444444-4444-4444-8444-444444444444",
		UserID:    "42",
	}
	require.NoError(t, repo.CreateRefreshSession(ctx, second, tokenHash("logout"), time.Now().Add(time.Hour)))
	_, err = repo.RotateRefreshToken(ctx, tokenHash("logout"), tokenHash("logout-next"), time.Now().Add(time.Hour))
	require.NoError(t, err)

	secondFamilyLock := lockFamily(t, testPool, second.FamilyID)
	logoutResult := make(chan error, 1)
	secondRotationResult := make(chan error, 1)
	logoutDone := false

	defer func() {
		rollbackTx(secondFamilyLock)

		cancel()

		if !logoutDone {
			awaitResults(logoutResult, 1)
			awaitResults(secondRotationResult, 1)
		}
	}()

	go func() {
		_, err := repo.RotateRefreshToken(
			ctx,
			tokenHash("logout-next"),
			tokenHash("logout-third"),
			time.Now().Add(time.Hour),
		)
		secondRotationResult <- err
	}()
	go func() {
		_, err := repo.RevokeRefreshToken(ctx, tokenHash("logout"))
		logoutResult <- err
	}()

	waitForFamilyLockWaiters(t, testPool, 2)
	require.NoError(t, secondFamilyLock.Commit(ctx))
	require.NoError(t, <-logoutResult)

	if err := <-secondRotationResult; err != nil {
		require.ErrorIs(t, err, domain.ErrUnauthenticated)
	}

	logoutDone = true

	active, err = repo.Active(ctx, domain.Principal{OwnerID: "42", SessionID: second.SessionID})
	require.NoError(t, err)
	require.False(t, active)
	assertRefreshTokensRejected(t, repo, ctx, "logout", "logout-next", "logout-third")

	down, err := os.ReadFile("../../../migrations/000002_auth.down.sql")
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, string(down))
	require.NoError(t, err)
	up, err := os.ReadFile("../../../migrations/000002_auth.up.sql")
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, string(up))
	require.NoError(t, err)
	_, err = testPool.Exec(ctx, string(down))
	require.NoError(t, err)
}

func lockFamily(t *testing.T, pool *pgxpool.Pool, familyID string) pgx.Tx {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin family lock transaction: %v", err)
	}

	var lockedFamily string

	err = tx.QueryRow(
		ctx,
		`SELECT family_id::text FROM auth_refresh_families WHERE family_id=$1 FOR UPDATE`,
		familyID,
	).Scan(&lockedFamily)
	if err != nil {
		rollbackTx(tx)

		t.Fatalf("lock family row: %v", err)
	}

	if lockedFamily != familyID {
		rollbackTx(tx)

		t.Fatalf("locked family %q, want %q", lockedFamily, familyID)
	}

	return tx
}

func rollbackTx(tx pgx.Tx) {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	_ = tx.Rollback(ctx)
}

func waitForFamilyLockWaiters(t *testing.T, pool *pgxpool.Pool, expected int) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()

	for {
		var waiting int

		err := pool.QueryRow(ctx, `
			SELECT count(*)::int
			FROM pg_stat_activity
			WHERE pid <> pg_backend_pid()
			  AND wait_event_type = 'Lock'
			  AND query LIKE '%FROM auth_refresh_families%'
		`).Scan(&waiting)
		require.NoError(t, err)

		if waiting >= expected {
			return
		}

		select {
		case <-ctx.Done():
			require.FailNowf(t, "family lock waiters timed out", "expected %d", expected)
		case <-ticker.C:
		}
	}
}

func awaitResults(results <-chan error, expected int) {
	deadline := time.NewTimer(time.Second)
	defer deadline.Stop()

	for range expected {
		select {
		case <-results:
		case <-deadline.C:
			return
		}
	}
}

func assertRefreshTokensRejected(
	t *testing.T,
	repo *postgres.AuthRepository,
	ctx context.Context,
	tokens ...string,
) {
	t.Helper()

	for _, token := range tokens {
		_, err := repo.RotateRefreshToken(
			ctx,
			tokenHash(token),
			tokenHash(token+"-after-revocation"),
			time.Now().Add(time.Hour),
		)
		require.ErrorIs(t, err, domain.ErrUnauthenticated, token)
	}
}

func tokenHash(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}
