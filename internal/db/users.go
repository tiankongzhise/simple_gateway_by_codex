package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"simple_gateway_by_codex/internal/models"
)

// CreateUser inserts a new user.
func (s *Store) CreateUser(ctx context.Context, username, userSlug, passwordHash string) (models.User, error) {
	var user models.User
	err := s.pool.QueryRow(ctx, `
		INSERT INTO users (username, user_slug, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id, username, user_slug, password_hash, created_at, updated_at
	`, username, userSlug, passwordHash).Scan(
		&user.ID,
		&user.Username,
		&user.UserSlug,
		&user.PasswordHash,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		return models.User{}, fmt.Errorf("create user: %w", err)
	}
	return user, nil
}

// CreateUserWithBinding creates a user and the initial service group binding atomically.
func (s *Store) CreateUserWithBinding(ctx context.Context, username, userSlug, passwordHash string, binding models.ServiceGroupBinding) (models.User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return models.User{}, fmt.Errorf("begin create user with binding: %w", err)
	}
	defer tx.Rollback(ctx)

	var user models.User
	err = tx.QueryRow(ctx, `
		INSERT INTO users (username, user_slug, password_hash)
		VALUES ($1, $2, $3)
		RETURNING id, username, user_slug, password_hash, created_at, updated_at
	`, username, userSlug, passwordHash).Scan(
		&user.ID,
		&user.Username,
		&user.UserSlug,
		&user.PasswordHash,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		return models.User{}, fmt.Errorf("create user: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO service_group_bindings (
			user_id, service_group_name, encrypted_authorization_code, salt, algorithm
		)
		VALUES ($1, $2, $3, $4, $5)
	`, user.ID, binding.ServiceGroupName, binding.EncryptedAuthorizationCode, binding.Salt, binding.Algorithm); err != nil {
		return models.User{}, fmt.Errorf("create initial service group binding: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return models.User{}, fmt.Errorf("commit create user with binding: %w", err)
	}
	return user, nil
}

// GetUserByUsername returns a user by login username.
func (s *Store) GetUserByUsername(ctx context.Context, username string) (models.User, error) {
	return s.scanUser(ctx, `SELECT id, username, user_slug, password_hash, created_at, updated_at FROM users WHERE username=$1`, username)
}

// GetUserByID returns a user by id.
func (s *Store) GetUserByID(ctx context.Context, id int64) (models.User, error) {
	return s.scanUser(ctx, `SELECT id, username, user_slug, password_hash, created_at, updated_at FROM users WHERE id=$1`, id)
}

// GetUserBySlug returns a user by public slug.
func (s *Store) GetUserBySlug(ctx context.Context, slug string) (models.User, error) {
	return s.scanUser(ctx, `SELECT id, username, user_slug, password_hash, created_at, updated_at FROM users WHERE user_slug=$1`, slug)
}

func (s *Store) scanUser(ctx context.Context, query string, args ...any) (models.User, error) {
	var user models.User
	err := s.pool.QueryRow(ctx, query, args...).Scan(
		&user.ID,
		&user.Username,
		&user.UserSlug,
		&user.PasswordHash,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.User{}, ErrNotFound
	}
	if err != nil {
		return models.User{}, fmt.Errorf("scan user: %w", err)
	}
	return user, nil
}

// CreateSession stores a login session token hash.
func (s *Store) CreateSession(ctx context.Context, userID int64, tokenHash string, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO sessions (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
	`, userID, tokenHash, expiresAt)
	if err != nil {
		return fmt.Errorf("create session: %w", err)
	}
	return nil
}

// GetSessionUser finds the user associated with a non-expired session token hash.
func (s *Store) GetSessionUser(ctx context.Context, tokenHash string, now time.Time) (models.User, error) {
	var user models.User
	err := s.pool.QueryRow(ctx, `
		SELECT u.id, u.username, u.user_slug, u.password_hash, u.created_at, u.updated_at
		FROM sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.token_hash=$1 AND s.expires_at > $2
	`, tokenHash, now).Scan(
		&user.ID,
		&user.Username,
		&user.UserSlug,
		&user.PasswordHash,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.User{}, ErrNotFound
	}
	if err != nil {
		return models.User{}, fmt.Errorf("get session user: %w", err)
	}
	return user, nil
}

// DeleteSession removes a session token hash.
func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	if _, err := s.pool.Exec(ctx, `DELETE FROM sessions WHERE token_hash=$1`, tokenHash); err != nil {
		return fmt.Errorf("delete session: %w", err)
	}
	return nil
}
