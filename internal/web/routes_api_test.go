package web

import "testing"

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
	if route.PathPattern != "/api" {
		t.Fatalf("PathPattern = %q", route.PathPattern)
	}
	if len(route.Methods) != 1 || route.Methods[0] != "GET" {
		t.Fatalf("Methods = %#v", route.Methods)
	}
}
