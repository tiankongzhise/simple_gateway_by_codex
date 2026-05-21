package web

import (
	"testing"

	"simple_gateway_by_codex/internal/models"
)

func TestRouteRequestRequiresAuthServiceNameWhenAuthRequired(t *testing.T) {
	req := routeRequest{
		Name:           "auth route",
		Enabled:        true,
		MatchType:      "prefix",
		PathPattern:    "/api",
		Methods:        []string{"GET"},
		UpstreamURL:    "https://upstream.example",
		TimeoutSeconds: 5,
		AuthRequired:   true,
	}

	_, err := req.toModel(1, 0)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRouteRequestMapsLegacyAuthRequiredToCallerToken(t *testing.T) {
	req := routeRequest{
		Name:            "auth route",
		Enabled:         true,
		MatchType:       "prefix",
		PathPattern:     "/api",
		Methods:         []string{"GET"},
		UpstreamURL:     "https://upstream.example",
		TimeoutSeconds:  5,
		AuthRequired:    true,
		AuthServiceName: "route-service",
	}

	route, err := req.toModel(1, 0)
	if err != nil {
		t.Fatalf("toModel() error = %v", err)
	}
	if route.AccessMode != models.AccessModeCallerToken {
		t.Fatalf("AccessMode = %q", route.AccessMode)
	}
	if !route.AuthRequired {
		t.Fatal("expected AuthRequired compatibility field")
	}
}

func TestRouteRequestAcceptsSignedLinkMode(t *testing.T) {
	req := routeRequest{
		Name:            "share route",
		Enabled:         true,
		AccessMode:      models.AccessModeSignedLink,
		MatchType:       "prefix",
		PathPattern:     "/share",
		Methods:         []string{"GET"},
		UpstreamURL:     "https://upstream.example",
		TimeoutSeconds:  5,
		AuthServiceName: "share-service",
	}

	route, err := req.toModel(1, 0)
	if err != nil {
		t.Fatalf("toModel() error = %v", err)
	}
	if route.AccessMode != models.AccessModeSignedLink {
		t.Fatalf("AccessMode = %q", route.AccessMode)
	}
	if !route.AuthRequired {
		t.Fatal("expected AuthRequired compatibility field")
	}
}

func TestRouteRequestRejectsInvalidAccessMode(t *testing.T) {
	req := routeRequest{
		Name:           "bad route",
		Enabled:        true,
		AccessMode:     "service_group_fallback",
		MatchType:      "prefix",
		PathPattern:    "/api",
		Methods:        []string{"GET"},
		UpstreamURL:    "https://upstream.example",
		TimeoutSeconds: 5,
	}

	_, err := req.toModel(1, 0)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRouteRequestClearsAuthServiceNameForNonAuthRoute(t *testing.T) {
	req := routeRequest{
		Name:            "public route",
		Enabled:         true,
		MatchType:       "prefix",
		PathPattern:     "api",
		Methods:         []string{"GET", "GET"},
		UpstreamURL:     "https://upstream.example",
		TimeoutSeconds:  5,
		AuthRequired:    false,
		AuthServiceName: "should-clear",
	}

	route, err := req.toModel(1, 0)
	if err != nil {
		t.Fatalf("toModel() error = %v", err)
	}
	if route.AuthServiceName != "" {
		t.Fatalf("AuthServiceName = %q", route.AuthServiceName)
	}
	if route.AccessMode != models.AccessModePublic {
		t.Fatalf("AccessMode = %q", route.AccessMode)
	}
	if route.PathPattern != "/api" {
		t.Fatalf("PathPattern = %q", route.PathPattern)
	}
	if len(route.Methods) != 1 || route.Methods[0] != "GET" {
		t.Fatalf("Methods = %#v", route.Methods)
	}
}
