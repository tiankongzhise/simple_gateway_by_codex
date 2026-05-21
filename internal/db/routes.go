package db

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"simple_gateway_by_codex/internal/models"
)

// CreateRoute inserts a route and its header rules for one user.
func (s *Store) CreateRoute(ctx context.Context, route models.Route) (models.Route, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return models.Route{}, fmt.Errorf("begin create route: %w", err)
	}
	defer tx.Rollback(ctx)

	created, err := insertRoute(ctx, tx, route)
	if err != nil {
		return models.Route{}, err
	}
	if err := replaceHeaderRules(ctx, tx, created.ID, route.RequestHeaders, route.ResponseHeaders); err != nil {
		return models.Route{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return models.Route{}, fmt.Errorf("commit create route: %w", err)
	}
	return s.GetRoute(ctx, route.UserID, created.ID)
}

// UpdateRoute replaces a user-owned route and its header rules.
func (s *Store) UpdateRoute(ctx context.Context, route models.Route) (models.Route, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return models.Route{}, fmt.Errorf("begin update route: %w", err)
	}
	defer tx.Rollback(ctx)

	var updated models.Route
	err = tx.QueryRow(ctx, `
		UPDATE routes SET
			name=$3,
			description=$4,
			enabled=$5,
			access_mode=$6,
			match_type=$7,
			path_pattern=$8,
			methods=$9,
			upstream_url=$10,
			strip_prefix=$11,
			timeout_seconds=$12,
			retry_count=$13,
			priority=$14,
			auth_required=$15,
			auth_service_name=$16,
			updated_at=NOW()
		WHERE user_id=$1 AND id=$2
		RETURNING id, user_id, name, description, enabled, access_mode, match_type, path_pattern, methods,
			upstream_url, strip_prefix, timeout_seconds, retry_count, priority, auth_required,
			auth_service_name, created_at, updated_at
	`, route.UserID, route.ID, route.Name, route.Description, route.Enabled, route.AccessMode, route.MatchType, route.PathPattern,
		route.Methods, route.UpstreamURL, route.StripPrefix, route.TimeoutSeconds, route.RetryCount,
		route.Priority, route.AuthRequired, route.AuthServiceName).Scan(
		&updated.ID,
		&updated.UserID,
		&updated.Name,
		&updated.Description,
		&updated.Enabled,
		&updated.AccessMode,
		&updated.MatchType,
		&updated.PathPattern,
		&updated.Methods,
		&updated.UpstreamURL,
		&updated.StripPrefix,
		&updated.TimeoutSeconds,
		&updated.RetryCount,
		&updated.Priority,
		&updated.AuthRequired,
		&updated.AuthServiceName,
		&updated.CreatedAt,
		&updated.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return models.Route{}, ErrNotFound
	}
	if err != nil {
		return models.Route{}, fmt.Errorf("update route: %w", err)
	}

	if _, err := tx.Exec(ctx, `DELETE FROM route_header_rules WHERE route_id=$1`, route.ID); err != nil {
		return models.Route{}, fmt.Errorf("delete route header rules: %w", err)
	}
	if err := replaceHeaderRules(ctx, tx, route.ID, route.RequestHeaders, route.ResponseHeaders); err != nil {
		return models.Route{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return models.Route{}, fmt.Errorf("commit update route: %w", err)
	}
	return s.GetRoute(ctx, route.UserID, route.ID)
}

// DeleteRoute deletes a route owned by userID.
func (s *Store) DeleteRoute(ctx context.Context, userID, routeID int64) error {
	tag, err := s.pool.Exec(ctx, `DELETE FROM routes WHERE user_id=$1 AND id=$2`, userID, routeID)
	if err != nil {
		return fmt.Errorf("delete route: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// GetRoute returns one user-owned route with header rules.
func (s *Store) GetRoute(ctx context.Context, userID, routeID int64) (models.Route, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, name, description, enabled, access_mode, match_type, path_pattern, methods,
			upstream_url, strip_prefix, timeout_seconds, retry_count, priority, auth_required,
			auth_service_name, created_at, updated_at
		FROM routes
		WHERE user_id=$1 AND id=$2
	`, userID, routeID)
	if err != nil {
		return models.Route{}, fmt.Errorf("get route: %w", err)
	}
	routes, err := pgx.CollectRows(rows, pgx.RowToStructByName[routeRow])
	if err != nil {
		return models.Route{}, fmt.Errorf("collect route: %w", err)
	}
	if len(routes) == 0 {
		return models.Route{}, ErrNotFound
	}
	route := routes[0].toModel()
	if err := s.loadHeaderRules(ctx, []*models.Route{&route}); err != nil {
		return models.Route{}, err
	}
	return route, nil
}

// ListRoutes returns all routes for a user ordered for management display.
func (s *Store) ListRoutes(ctx context.Context, userID int64) ([]models.Route, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, name, description, enabled, access_mode, match_type, path_pattern, methods,
			upstream_url, strip_prefix, timeout_seconds, retry_count, priority, auth_required,
			auth_service_name, created_at, updated_at
		FROM routes
		WHERE user_id=$1
		ORDER BY priority DESC, path_pattern DESC, id ASC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list routes: %w", err)
	}
	collected, err := pgx.CollectRows(rows, pgx.RowToStructByName[routeRow])
	if err != nil {
		return nil, fmt.Errorf("collect routes: %w", err)
	}
	routes := make([]models.Route, 0, len(collected))
	for _, row := range collected {
		routes = append(routes, row.toModel())
	}
	ptrs := make([]*models.Route, 0, len(routes))
	for i := range routes {
		ptrs = append(ptrs, &routes[i])
	}
	if err := s.loadHeaderRules(ctx, ptrs); err != nil {
		return nil, err
	}
	return routes, nil
}

// ListEnabledRoutes returns enabled routes for runtime gateway matching.
func (s *Store) ListEnabledRoutes(ctx context.Context, userID int64) ([]models.Route, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, user_id, name, description, enabled, access_mode, match_type, path_pattern, methods,
			upstream_url, strip_prefix, timeout_seconds, retry_count, priority, auth_required,
			auth_service_name, created_at, updated_at
		FROM routes
		WHERE user_id=$1 AND enabled=TRUE
		ORDER BY priority DESC, path_pattern DESC, id ASC
	`, userID)
	if err != nil {
		return nil, fmt.Errorf("list enabled routes: %w", err)
	}
	collected, err := pgx.CollectRows(rows, pgx.RowToStructByName[routeRow])
	if err != nil {
		return nil, fmt.Errorf("collect enabled routes: %w", err)
	}
	routes := make([]models.Route, 0, len(collected))
	for _, row := range collected {
		routes = append(routes, row.toModel())
	}
	ptrs := make([]*models.Route, 0, len(routes))
	for i := range routes {
		ptrs = append(ptrs, &routes[i])
	}
	if err := s.loadHeaderRules(ctx, ptrs); err != nil {
		return nil, err
	}
	return routes, nil
}

type routeRow struct {
	ID              int64     `db:"id"`
	UserID          int64     `db:"user_id"`
	Name            string    `db:"name"`
	Description     string    `db:"description"`
	Enabled         bool      `db:"enabled"`
	AccessMode      string    `db:"access_mode"`
	MatchType       string    `db:"match_type"`
	PathPattern     string    `db:"path_pattern"`
	Methods         []string  `db:"methods"`
	UpstreamURL     string    `db:"upstream_url"`
	StripPrefix     bool      `db:"strip_prefix"`
	TimeoutSeconds  int       `db:"timeout_seconds"`
	RetryCount      int       `db:"retry_count"`
	Priority        int       `db:"priority"`
	AuthRequired    bool      `db:"auth_required"`
	AuthServiceName string    `db:"auth_service_name"`
	CreatedAt       time.Time `db:"created_at"`
	UpdatedAt       time.Time `db:"updated_at"`
}

func (r routeRow) toModel() models.Route {
	return models.Route{
		ID:              r.ID,
		UserID:          r.UserID,
		Name:            r.Name,
		Description:     r.Description,
		Enabled:         r.Enabled,
		AccessMode:      r.AccessMode,
		MatchType:       r.MatchType,
		PathPattern:     r.PathPattern,
		Methods:         r.Methods,
		UpstreamURL:     r.UpstreamURL,
		StripPrefix:     r.StripPrefix,
		TimeoutSeconds:  r.TimeoutSeconds,
		RetryCount:      r.RetryCount,
		Priority:        r.Priority,
		AuthRequired:    r.AuthRequired,
		AuthServiceName: r.AuthServiceName,
		CreatedAt:       r.CreatedAt,
		UpdatedAt:       r.UpdatedAt,
	}
}

type queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

func insertRoute(ctx context.Context, q queryer, route models.Route) (models.Route, error) {
	var created models.Route
	err := q.QueryRow(ctx, `
		INSERT INTO routes (
			user_id, name, description, enabled, access_mode, match_type, path_pattern, methods, upstream_url,
			strip_prefix, timeout_seconds, retry_count, priority, auth_required, auth_service_name
		)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15)
		RETURNING id, user_id, name, description, enabled, access_mode, match_type, path_pattern, methods,
			upstream_url, strip_prefix, timeout_seconds, retry_count, priority, auth_required,
			auth_service_name, created_at, updated_at
	`, route.UserID, route.Name, route.Description, route.Enabled, route.AccessMode, route.MatchType, route.PathPattern,
		route.Methods, route.UpstreamURL, route.StripPrefix, route.TimeoutSeconds, route.RetryCount,
		route.Priority, route.AuthRequired, route.AuthServiceName).Scan(
		&created.ID,
		&created.UserID,
		&created.Name,
		&created.Description,
		&created.Enabled,
		&created.AccessMode,
		&created.MatchType,
		&created.PathPattern,
		&created.Methods,
		&created.UpstreamURL,
		&created.StripPrefix,
		&created.TimeoutSeconds,
		&created.RetryCount,
		&created.Priority,
		&created.AuthRequired,
		&created.AuthServiceName,
		&created.CreatedAt,
		&created.UpdatedAt,
	)
	if err != nil {
		return models.Route{}, fmt.Errorf("insert route: %w", err)
	}
	return created, nil
}

func replaceHeaderRules(ctx context.Context, q queryer, routeID int64, requestRules, responseRules []models.HeaderRule) error {
	for _, rule := range requestRules {
		rule.RouteID = routeID
		rule.Phase = "request"
		if err := insertHeaderRule(ctx, q, rule); err != nil {
			return err
		}
	}
	for _, rule := range responseRules {
		rule.RouteID = routeID
		rule.Phase = "response"
		if err := insertHeaderRule(ctx, q, rule); err != nil {
			return err
		}
	}
	return nil
}

func insertHeaderRule(ctx context.Context, q queryer, rule models.HeaderRule) error {
	if _, err := q.Exec(ctx, `
		INSERT INTO route_header_rules (route_id, phase, operation, header_name, header_value)
		VALUES ($1, $2, $3, $4, $5)
	`, rule.RouteID, rule.Phase, rule.Operation, rule.HeaderName, rule.HeaderValue); err != nil {
		return fmt.Errorf("insert header rule: %w", err)
	}
	return nil
}

func (s *Store) loadHeaderRules(ctx context.Context, routes []*models.Route) error {
	if len(routes) == 0 {
		return nil
	}
	routeByID := make(map[int64]*models.Route, len(routes))
	ids := make([]int64, 0, len(routes))
	for _, route := range routes {
		routeByID[route.ID] = route
		ids = append(ids, route.ID)
	}

	rows, err := s.pool.Query(ctx, `
		SELECT id, route_id, phase, operation, header_name, header_value, created_at
		FROM route_header_rules
		WHERE route_id = ANY($1)
		ORDER BY id ASC
	`, ids)
	if err != nil {
		return fmt.Errorf("load header rules: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var rule models.HeaderRule
		if err := rows.Scan(&rule.ID, &rule.RouteID, &rule.Phase, &rule.Operation, &rule.HeaderName, &rule.HeaderValue, &rule.CreatedAt); err != nil {
			return fmt.Errorf("scan header rule: %w", err)
		}
		route := routeByID[rule.RouteID]
		if route == nil {
			continue
		}
		if rule.Phase == "request" {
			route.RequestHeaders = append(route.RequestHeaders, rule)
		} else {
			route.ResponseHeaders = append(route.ResponseHeaders, rule)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterate header rules: %w", err)
	}
	return nil
}
