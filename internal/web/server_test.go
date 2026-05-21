package web

import (
	"context"
	"testing"

	"simple_gateway_by_codex/internal/models"
)

func TestNewServerWithDependenciesRegistersRoutesWithoutConflict(t *testing.T) {
	server := NewServerWithDependencies(
		"http://localhost:8080",
		false,
		stubAuthService{},
		stubRouteStore{},
		nil,
		nil,
		nil,
	)
	if server == nil {
		t.Fatal("expected server")
	}
}

type stubAuthService struct{}

func (stubAuthService) Register(context.Context, RegisterRequest) (models.User, string, error) {
	return models.User{}, "", nil
}

func (stubAuthService) Login(context.Context, string, string) (models.User, string, error) {
	return models.User{}, "", nil
}

func (stubAuthService) GetSessionUser(context.Context, string) (models.User, error) {
	return models.User{}, nil
}

func (stubAuthService) DeleteSession(context.Context, string) error {
	return nil
}

func (stubAuthService) RebindServiceGroup(context.Context, int64, string, string) (models.ServiceGroupBinding, error) {
	return models.ServiceGroupBinding{}, nil
}

func (stubAuthService) GetServiceGroupBinding(context.Context, int64) (models.ServiceGroupBinding, error) {
	return models.ServiceGroupBinding{}, nil
}

type stubRouteStore struct{}

func (stubRouteStore) GetUserBySlug(context.Context, string) (models.User, error) {
	return models.User{}, nil
}

func (stubRouteStore) GetServiceGroupBinding(context.Context, int64) (models.ServiceGroupBinding, error) {
	return models.ServiceGroupBinding{}, nil
}

func (stubRouteStore) ListRoutes(context.Context, int64) ([]models.Route, error) {
	return nil, nil
}

func (stubRouteStore) ListEnabledRoutes(context.Context, int64) ([]models.Route, error) {
	return nil, nil
}

func (stubRouteStore) CreateRoute(context.Context, models.Route) (models.Route, error) {
	return models.Route{}, nil
}

func (stubRouteStore) UpdateRoute(context.Context, models.Route) (models.Route, error) {
	return models.Route{}, nil
}

func (stubRouteStore) DeleteRoute(context.Context, int64, int64) error {
	return nil
}

func (stubRouteStore) GetRoute(context.Context, int64, int64) (models.Route, error) {
	return models.Route{}, nil
}
