package proxy

import (
	"testing"

	"simple_gateway_by_codex/internal/models"
)

func TestParseGatewayPath(t *testing.T) {
	slug, path, ok := parseGatewayPath("/gw/alice/api/users")
	if !ok {
		t.Fatal("expected gateway path")
	}
	if slug != "alice" || path != "/api/users" {
		t.Fatalf("slug=%q path=%q", slug, path)
	}
}

func TestBuildTargetURLStripPrefix(t *testing.T) {
	route := models.Route{UpstreamURL: "https://upstream.example/base", PathPattern: "/api", StripPrefix: true}

	got, err := buildTargetURL(route, "/users", "q=1")
	if err != nil {
		t.Fatalf("buildTargetURL() error = %v", err)
	}
	if got != "https://upstream.example/base/users?q=1" {
		t.Fatalf("target = %q", got)
	}
}

func TestBuildTargetURLKeepsPrefix(t *testing.T) {
	route := models.Route{UpstreamURL: "https://upstream.example", PathPattern: "/api", StripPrefix: false}

	got, err := buildTargetURL(route, "/users", "")
	if err != nil {
		t.Fatalf("buildTargetURL() error = %v", err)
	}
	if got != "https://upstream.example/api/users" {
		t.Fatalf("target = %q", got)
	}
}
