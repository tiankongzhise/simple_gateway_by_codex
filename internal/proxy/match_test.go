package proxy

import (
	"net/http"
	"testing"

	"simple_gateway_by_codex/internal/models"
)

func TestMatchRouteChoosesPriorityThenLongestPath(t *testing.T) {
	routes := []models.Route{
		{ID: 1, Enabled: true, MatchType: "prefix", PathPattern: "/api", Methods: []string{"ALL"}, Priority: 1},
		{ID: 2, Enabled: true, MatchType: "prefix", PathPattern: "/api/users", Methods: []string{"GET"}, Priority: 1},
		{ID: 3, Enabled: true, MatchType: "prefix", PathPattern: "/api/users", Methods: []string{"GET"}, Priority: 3},
	}

	got := MatchRoute(routes, http.MethodGet, "/api/users/42")
	if !got.MethodAllowed {
		t.Fatal("expected method allowed")
	}
	if got.Route.ID != 3 {
		t.Fatalf("Route.ID = %d", got.Route.ID)
	}
	if got.RemainingPath != "/42" {
		t.Fatalf("RemainingPath = %q", got.RemainingPath)
	}
}

func TestMatchRouteReportsMethodNotAllowed(t *testing.T) {
	routes := []models.Route{{ID: 1, Enabled: true, MatchType: "prefix", PathPattern: "/api", Methods: []string{"POST"}}}

	got := MatchRoute(routes, http.MethodGet, "/api")
	if !got.PathMatched {
		t.Fatal("expected path match")
	}
	if got.MethodAllowed {
		t.Fatal("expected method to be rejected")
	}
}

func TestExactRouteRequiresExactPath(t *testing.T) {
	routes := []models.Route{{ID: 1, Enabled: true, MatchType: "exact", PathPattern: "/health", Methods: []string{"ALL"}}}

	if !MatchRoute(routes, http.MethodGet, "/health").MethodAllowed {
		t.Fatal("expected exact route to match")
	}
	if MatchRoute(routes, http.MethodGet, "/health/live").PathMatched {
		t.Fatal("did not expect child path to match exact route")
	}
}
