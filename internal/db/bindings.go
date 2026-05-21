package db

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"simple_gateway_by_codex/internal/models"
)

// UpsertServiceGroupBinding replaces the current service group binding for a user.
func (s *Store) UpsertServiceGroupBinding(ctx context.Context, binding models.ServiceGroupBinding) (models.ServiceGroupBinding, error) {
	var out models.ServiceGroupBinding
	err := s.pool.QueryRow(ctx, `
		INSERT INTO service_group_bindings (
			user_id, service_group_name, encrypted_authorization_code, salt, algorithm
		)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (user_id) DO UPDATE SET
			service_group_name = EXCLUDED.service_group_name,
			encrypted_authorization_code = EXCLUDED.encrypted_authorization_code,
			salt = EXCLUDED.salt,
			algorithm = EXCLUDED.algorithm,
			updated_at = NOW()
		RETURNING id, user_id, service_group_name, encrypted_authorization_code, salt, algorithm, created_at, updated_at
	`, binding.UserID, binding.ServiceGroupName, binding.EncryptedAuthorizationCode, binding.Salt, binding.Algorithm).Scan(
		&out.ID,
		&out.UserID,
		&out.ServiceGroupName,
		&out.EncryptedAuthorizationCode,
		&out.Salt,
		&out.Algorithm,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if err != nil {
		return models.ServiceGroupBinding{}, fmt.Errorf("upsert service group binding: %w", err)
	}
	return out, nil
}

// GetServiceGroupBinding returns the current binding for a user.
func (s *Store) GetServiceGroupBinding(ctx context.Context, userID int64) (models.ServiceGroupBinding, error) {
	var out models.ServiceGroupBinding
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, service_group_name, encrypted_authorization_code, salt, algorithm, created_at, updated_at
		FROM service_group_bindings
		WHERE user_id=$1
	`, userID).Scan(
		&out.ID,
		&out.UserID,
		&out.ServiceGroupName,
		&out.EncryptedAuthorizationCode,
		&out.Salt,
		&out.Algorithm,
		&out.CreatedAt,
		&out.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.ServiceGroupBinding{}, ErrNotFound
	}
	if err != nil {
		return models.ServiceGroupBinding{}, fmt.Errorf("get service group binding: %w", err)
	}
	return out, nil
}
