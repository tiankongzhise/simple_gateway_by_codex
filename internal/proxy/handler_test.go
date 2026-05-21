package proxy

import (
	"context"
	"net/http"
	"net/http/httptest"
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

func TestAuthRouteUsesCallerTokenAndServiceNameFallback(t *testing.T) {
	store := &fakeStore{
		user: models.User{ID: 1, UserSlug: "alice"},
		routes: []models.Route{{
			ID:              1,
			Enabled:         true,
			MatchType:       "prefix",
			PathPattern:     "/api",
			Methods:         []string{"GET"},
			UpstreamURL:     "http://upstream.local",
			TimeoutSeconds:  5,
			AuthRequired:    true,
			AuthServiceName: "route-service",
		}},
	}
	auth := &fakeAuthClient{verifyOK: true}
	handler := NewHandler(store).WithAuth(auth, fakeCipher{})
	handler.httpClient = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})).Client()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	store.routes[0].UpstreamURL = upstream.URL

	req := httptest.NewRequest(http.MethodGet, "/gw/alice/api", nil)
	req.Header.Set("Access-Token", "caller-token")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rr.Code)
	}
	if auth.lastVerify.ServiceName != "route-service" {
		t.Fatalf("ServiceName = %q", auth.lastVerify.ServiceName)
	}
	if auth.lastVerify.AccessToken != "caller-token" {
		t.Fatalf("AccessToken = %q", auth.lastVerify.AccessToken)
	}
}

func TestAuthRouteFallsBackToGroupToken(t *testing.T) {
	store := &fakeStore{
		user:    models.User{ID: 1, UserSlug: "alice"},
		binding: models.ServiceGroupBinding{ServiceGroupName: "group", EncryptedAuthorizationCode: "cipher", Salt: "salt", Algorithm: "alg"},
		routes: []models.Route{{
			ID:              1,
			Enabled:         true,
			MatchType:       "prefix",
			PathPattern:     "/api",
			Methods:         []string{"GET"},
			UpstreamURL:     "http://upstream.local",
			TimeoutSeconds:  5,
			AuthRequired:    true,
			AuthServiceName: "route-service",
		}},
	}
	auth := &fakeAuthClient{verifyOK: true, groupToken: "group-token"}
	handler := NewHandler(store).WithAuth(auth, fakeCipher{code: "permanent"})
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	store.routes[0].UpstreamURL = upstream.URL

	req := httptest.NewRequest(http.MethodGet, "/gw/alice/api", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rr.Code)
	}
	if auth.lastTokenGroup != "group" {
		t.Fatalf("lastTokenGroup = %q", auth.lastTokenGroup)
	}
	if auth.lastVerify.ServiceName != "group" || auth.lastVerify.TargetServiceName != "route-service" {
		t.Fatalf("lastVerify = %#v", auth.lastVerify)
	}
}

type fakeStore struct {
	user    models.User
	routes  []models.Route
	binding models.ServiceGroupBinding
}

func (s *fakeStore) GetUserBySlug(ctx context.Context, slug string) (models.User, error) {
	return s.user, nil
}

func (s *fakeStore) ListEnabledRoutes(ctx context.Context, userID int64) ([]models.Route, error) {
	return s.routes, nil
}

func (s *fakeStore) GetServiceGroupBinding(ctx context.Context, userID int64) (models.ServiceGroupBinding, error) {
	return s.binding, nil
}

type fakeAuthClient struct {
	verifyOK       bool
	groupToken     string
	lastTokenGroup string
	lastVerify     VerifyHeaders
}

func (c *fakeAuthClient) LatestServiceGroupToken(ctx context.Context, serviceGroupName, authorizationCode string) (GroupToken, error) {
	c.lastTokenGroup = serviceGroupName
	return GroupToken{AccessToken: c.groupToken}, nil
}

func (c *fakeAuthClient) Verify(ctx context.Context, headers VerifyHeaders) (VerifyResult, error) {
	c.lastVerify = headers
	return VerifyResult{OK: c.verifyOK}, nil
}

type fakeCipher struct {
	code string
}

func (c fakeCipher) DecryptBinding(ciphertext, salt, algorithm string) (string, error) {
	return c.code, nil
}
