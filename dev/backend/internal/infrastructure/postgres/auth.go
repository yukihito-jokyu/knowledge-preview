package postgres

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/domain"
	"github.com/yukihito-jokyu/knowledge-preview/dev/backend/internal/usecase"
)

type AuthRepository struct{ pool *pgxpool.Pool }

func NewAuthRepository(pool *pgxpool.Pool) *AuthRepository { return &AuthRepository{pool: pool} }

func (r *AuthRepository) SaveOAuthState(
	ctx context.Context,
	hash, verifierHash string,
	state usecase.OAuthState,
) error {
	if _, err := r.pool.Exec(ctx, `DELETE FROM auth_oauth_states WHERE expires_at<=clock_timestamp()`); err != nil {
		return dbError(err)
	}

	_, err := r.pool.Exec(
		ctx,
		`INSERT INTO auth_oauth_states(state_hash,verifier_hash,return_to,expires_at) VALUES($1,$2,$3,$4)`,
		hash,
		verifierHash,
		state.ReturnTo,
		state.Expires,
	)

	return dbError(err)
}

func (r *AuthRepository) ConsumeOAuthState(ctx context.Context, hash, verifierHash string) (usecase.OAuthState, error) {
	var state usecase.OAuthState

	err := r.pool.QueryRow(ctx, `UPDATE auth_oauth_states SET used_at=clock_timestamp() WHERE state_hash=$1 AND verifier_hash=$2 AND used_at IS NULL AND expires_at>clock_timestamp() RETURNING return_to,expires_at`, hash, verifierHash).
		Scan(&state.ReturnTo, &state.Expires)
	if errors.Is(err, pgx.ErrNoRows) {
		return state, domain.ErrBadRequest
	}

	return state, dbError(err)
}

func (r *AuthRepository) UpsertUser(ctx context.Context, user usecase.OAuthUser) error {
	_, err := r.pool.Exec(
		ctx,
		`INSERT INTO auth_users(github_id,display_name,email,last_login_at) VALUES($1,$2,$3,clock_timestamp()) ON CONFLICT(github_id) DO UPDATE SET display_name=EXCLUDED.display_name,email=EXCLUDED.email,last_login_at=clock_timestamp()`,
		user.ID,
		user.DisplayName,
		user.Email,
	)

	return dbError(err)
}

func (r *AuthRepository) User(ctx context.Context, id string) (usecase.OAuthUser, error) {
	var user usecase.OAuthUser

	err := r.pool.QueryRow(ctx, `SELECT github_id,display_name,email FROM auth_users WHERE github_id=$1`, id).
		Scan(&user.ID, &user.DisplayName, &user.Email)

	return user, dbError(err)
}

func (r *AuthRepository) CreateRefreshSession(
	ctx context.Context,
	session usecase.RefreshSession,
	hash string,
	expires time.Time,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return dbError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(
		ctx,
		`INSERT INTO auth_refresh_families(family_id,session_id,github_id) VALUES($1,$2,$3)`,
		session.FamilyID,
		session.SessionID,
		session.UserID,
	); err != nil {
		return dbError(err)
	}

	if _, err := tx.Exec(
		ctx,
		`INSERT INTO auth_refresh_sessions(token_hash,session_id,family_id,github_id,expires_at) VALUES($1,$2,$3,$4,$5)`,
		hash,
		session.SessionID,
		session.FamilyID,
		session.UserID,
		expires,
	); err != nil {
		return dbError(err)
	}

	return dbError(tx.Commit(ctx))
}

func (r *AuthRepository) RotateRefreshToken(
	ctx context.Context,
	hash, nextHash string,
	expires time.Time,
) (usecase.RefreshSession, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return usecase.RefreshSession{}, dbError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var (
		session usecase.RefreshSession
		revoked *time.Time
	)

	var familyRevoked *time.Time

	err = tx.QueryRow(ctx, `SELECT family_id::text FROM auth_refresh_sessions WHERE token_hash=$1`, hash).
		Scan(&session.FamilyID)
	if errors.Is(err, pgx.ErrNoRows) {
		return session, domain.ErrUnauthenticated
	}

	if err != nil {
		return session, dbError(err)
	}

	if err := tx.QueryRow(ctx, `SELECT revoked_at FROM auth_refresh_families WHERE family_id=$1 FOR UPDATE`, session.FamilyID).
		Scan(&familyRevoked); err != nil {
		return session, dbError(err)
	}

	err = tx.QueryRow(ctx, `SELECT session_id::text,github_id,expires_at,revoked_at FROM auth_refresh_sessions WHERE token_hash=$1 FOR UPDATE`, hash).
		Scan(&session.SessionID, &session.UserID, &session.Expires, &revoked)
	if errors.Is(err, pgx.ErrNoRows) {
		return session, domain.ErrUnauthenticated
	}

	if err != nil {
		return session, dbError(err)
	}

	if familyRevoked != nil {
		return session, domain.ErrUnauthenticated
	}

	if revoked != nil {
		if _, err := tx.Exec(
			ctx,
			`UPDATE auth_refresh_families SET revoked_at=COALESCE(revoked_at,clock_timestamp()) WHERE family_id=$1`,
			session.FamilyID,
		); err != nil {
			return session, dbError(err)
		}

		if _, err := tx.Exec(
			ctx,
			`UPDATE auth_refresh_sessions SET revoked_at=COALESCE(revoked_at,clock_timestamp()) WHERE family_id=$1`,
			session.FamilyID,
		); err != nil {
			return session, dbError(err)
		}

		if err := tx.Commit(ctx); err != nil {
			return session, dbError(err)
		}

		return session, domain.ErrRefreshReuse
	}

	if !session.Expires.After(time.Now()) {
		return session, domain.ErrUnauthenticated
	}

	if _, err := tx.Exec(
		ctx,
		`UPDATE auth_refresh_sessions SET revoked_at=clock_timestamp(),replaced_by_hash=$2 WHERE token_hash=$1`,
		hash,
		nextHash,
	); err != nil {
		return session, dbError(err)
	}

	if _, err := tx.Exec(
		ctx,
		`INSERT INTO auth_refresh_sessions(token_hash,session_id,family_id,github_id,expires_at) VALUES($1,$2,$3,$4,$5)`,
		nextHash,
		session.SessionID,
		session.FamilyID,
		session.UserID,
		expires,
	); err != nil {
		return session, dbError(err)
	}

	return session, dbError(tx.Commit(ctx))
}

func (r *AuthRepository) RevokeSession(ctx context.Context, sessionID string) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return dbError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	rows, err := tx.Query(
		ctx,
		`SELECT family_id::text FROM auth_refresh_families WHERE session_id=$1 FOR UPDATE`,
		sessionID,
	)
	if err != nil {
		return dbError(err)
	}

	var families []string

	for rows.Next() {
		var familyID string
		if err := rows.Scan(&familyID); err != nil {
			rows.Close()
			return dbError(err)
		}

		families = append(families, familyID)
	}

	if err := rows.Err(); err != nil {
		rows.Close()
		return dbError(err)
	}

	rows.Close()

	for _, familyID := range families {
		if _, err := tx.Exec(
			ctx,
			`UPDATE auth_refresh_families SET revoked_at=COALESCE(revoked_at,clock_timestamp()) WHERE family_id=$1`,
			familyID,
		); err != nil {
			return dbError(err)
		}

		if _, err := tx.Exec(
			ctx,
			`UPDATE auth_refresh_sessions SET revoked_at=COALESCE(revoked_at,clock_timestamp()) WHERE family_id=$1`,
			familyID,
		); err != nil {
			return dbError(err)
		}
	}

	return dbError(tx.Commit(ctx))
}

func (r *AuthRepository) RevokeRefreshToken(ctx context.Context, hash string) (string, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return "", dbError(err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var familyID, userID string
	if err := tx.QueryRow(ctx, `SELECT family_id::text,github_id FROM auth_refresh_sessions WHERE token_hash=$1`, hash).
		Scan(&familyID, &userID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return "", domain.ErrNotFound
		}

		return "", dbError(err)
	}

	if err := tx.QueryRow(ctx, `SELECT family_id FROM auth_refresh_families WHERE family_id=$1 FOR UPDATE`, familyID).
		Scan(new(string)); err != nil {
		return "", dbError(err)
	}

	if _, err := tx.Exec(
		ctx,
		`UPDATE auth_refresh_families SET revoked_at=COALESCE(revoked_at,clock_timestamp()) WHERE family_id=$1`,
		familyID,
	); err != nil {
		return "", dbError(err)
	}

	if _, err := tx.Exec(
		ctx,
		`UPDATE auth_refresh_sessions SET revoked_at=COALESCE(revoked_at,clock_timestamp()) WHERE family_id=$1`,
		familyID,
	); err != nil {
		return "", dbError(err)
	}

	if err := tx.Commit(ctx); err != nil {
		return "", dbError(err)
	}

	return userID, nil
}

func (r *AuthRepository) Active(ctx context.Context, principal domain.Principal) (bool, error) {
	var exists bool

	err := r.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM auth_refresh_sessions s JOIN auth_refresh_families f ON f.family_id=s.family_id WHERE s.session_id=$1 AND s.github_id=$2 AND s.revoked_at IS NULL AND f.revoked_at IS NULL AND s.expires_at>clock_timestamp())`, principal.SessionID, principal.OwnerID).
		Scan(&exists)

	return exists, dbError(err)
}

func (r *AuthRepository) Audit(ctx context.Context, event, userID string) error {
	if _, err := r.pool.Exec(
		ctx,
		`DELETE FROM auth_audit WHERE created_at<clock_timestamp()-interval '90 days'`,
	); err != nil {
		return dbError(err)
	}

	_, err := r.pool.Exec(ctx, `INSERT INTO auth_audit(event,github_id) VALUES($1,NULLIF($2,''))`, event, userID)

	return dbError(err)
}
