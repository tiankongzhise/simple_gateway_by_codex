package proxy

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
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

func TestForwardingLogsSuccessAndGeneratedRequestID(t *testing.T) {
	store := &fakeStore{
		user: models.User{ID: 1, UserSlug: "alice"},
		routes: []models.Route{{
			ID:             7,
			Name:           "users",
			Enabled:        true,
			MatchType:      "prefix",
			PathPattern:    "/api",
			Methods:        []string{"GET"},
			UpstreamURL:    "http://upstream.local/base",
			StripPrefix:    true,
			TimeoutSeconds: 5,
			Priority:       10,
		}},
	}
	var upstreamRequestID string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamRequestID = r.Header.Get(requestIDHeader)
		w.WriteHeader(http.StatusCreated)
	}))
	defer upstream.Close()
	store.routes[0].UpstreamURL = upstream.URL + "/base"

	logger, logs := testLogger()
	handler := NewHandler(store).WithLogger(logger)
	req := httptest.NewRequest(http.MethodGet, "/gw/alice/api/users?api_key=query-secret", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d", rr.Code)
	}
	requestID := rr.Header().Get(requestIDHeader)
	if requestID == "" {
		t.Fatal("missing response request id")
	}
	if upstreamRequestID != requestID {
		t.Fatalf("upstream request id = %q, want %q", upstreamRequestID, requestID)
	}
	output := logs.String()
	for _, event := range []string{
		"proxy.request.start",
		"proxy.route.lookup",
		"proxy.route.matched",
		"proxy.upstream.request",
		"proxy.upstream.response",
		"proxy.request.finish",
	} {
		if !strings.Contains(output, event) {
			t.Fatalf("missing log event %q in:\n%s", event, output)
		}
	}
	if !strings.Contains(output, requestID) {
		t.Fatalf("logs do not include request id %q:\n%s", requestID, output)
	}
	if strings.Contains(output, "query-secret") {
		t.Fatalf("logs leaked query secret:\n%s", output)
	}
}

func TestForwardingReusesIncomingRequestID(t *testing.T) {
	store := &fakeStore{
		user: models.User{ID: 1, UserSlug: "alice"},
		routes: []models.Route{{
			ID:             8,
			Enabled:        true,
			MatchType:      "prefix",
			PathPattern:    "/api",
			Methods:        []string{"GET"},
			UpstreamURL:    "http://upstream.local",
			TimeoutSeconds: 5,
		}},
	}
	var upstreamRequestID string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamRequestID = r.Header.Get(requestIDHeader)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	store.routes[0].UpstreamURL = upstream.URL

	logger, logs := testLogger()
	handler := NewHandler(store).WithLogger(logger)
	req := httptest.NewRequest(http.MethodGet, "/gw/alice/api", nil)
	req.Header.Set(requestIDHeader, "incoming-request-id")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rr.Code)
	}
	if rr.Header().Get(requestIDHeader) != "incoming-request-id" {
		t.Fatalf("response request id = %q", rr.Header().Get(requestIDHeader))
	}
	if upstreamRequestID != "incoming-request-id" {
		t.Fatalf("upstream request id = %q", upstreamRequestID)
	}
	if !strings.Contains(logs.String(), "incoming-request-id") {
		t.Fatalf("logs do not include incoming request id:\n%s", logs.String())
	}
}

func TestForwardingKeepsRequestIDAfterHeaderRules(t *testing.T) {
	store := &fakeStore{
		user: models.User{ID: 1, UserSlug: "alice"},
		routes: []models.Route{{
			ID:             10,
			Enabled:        true,
			MatchType:      "prefix",
			PathPattern:    "/api",
			Methods:        []string{"GET"},
			UpstreamURL:    "http://upstream.local",
			TimeoutSeconds: 5,
			RequestHeaders: []models.HeaderRule{{
				Operation:  "remove",
				HeaderName: requestIDHeader,
			}},
			ResponseHeaders: []models.HeaderRule{{
				Operation:  "remove",
				HeaderName: requestIDHeader,
			}},
		}},
	}
	var upstreamRequestID string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamRequestID = r.Header.Get(requestIDHeader)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer upstream.Close()
	store.routes[0].UpstreamURL = upstream.URL

	handler := NewHandler(store)
	req := httptest.NewRequest(http.MethodGet, "/gw/alice/api", nil)
	req.Header.Set(requestIDHeader, "rule-resistant-id")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d", rr.Code)
	}
	if upstreamRequestID != "rule-resistant-id" {
		t.Fatalf("upstream request id = %q", upstreamRequestID)
	}
	if rr.Header().Get(requestIDHeader) != "rule-resistant-id" {
		t.Fatalf("response request id = %q", rr.Header().Get(requestIDHeader))
	}
}

func TestForwardingLogsDoNotExposeSensitiveValues(t *testing.T) {
	store := &fakeStore{
		user: models.User{ID: 1, UserSlug: "alice"},
		routes: []models.Route{{
			ID:              9,
			Enabled:         true,
			MatchType:       "prefix",
			PathPattern:     "/api",
			Methods:         []string{"POST"},
			UpstreamURL:     "http://upstream.local",
			TimeoutSeconds:  5,
			AuthRequired:    true,
			AuthServiceName: "route-service",
		}},
	}
	auth := &fakeAuthClient{verifyOK: true}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()
	store.routes[0].UpstreamURL = upstream.URL

	logger, logs := testLogger()
	handler := NewHandler(store).WithAuth(auth, fakeCipher{}).WithLogger(logger)
	req := httptest.NewRequest(http.MethodPost, "/gw/alice/api?api_key=query-secret", strings.NewReader("body-secret"))
	req.Header.Set("Access-Token", "caller-secret-token")
	req.Header.Set("Cookie", "gateway_session=session-secret")
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	output := logs.String()
	for _, secret := range []string{"caller-secret-token", "session-secret", "body-secret", "query-secret"} {
		if strings.Contains(output, secret) {
			t.Fatalf("logs leaked %q:\n%s", secret, output)
		}
	}
}

func TestForwardingLogsFailures(t *testing.T) {
	t.Run("route not found", func(t *testing.T) {
		store := &fakeStore{
			user: models.User{ID: 1, UserSlug: "alice"},
			routes: []models.Route{{
				ID:          1,
				Enabled:     true,
				MatchType:   "prefix",
				PathPattern: "/api",
				Methods:     []string{"GET"},
				UpstreamURL: "http://upstream.local",
			}},
		}
		logger, logs := testLogger()
		handler := NewHandler(store).WithLogger(logger)
		req := httptest.NewRequest(http.MethodGet, "/gw/alice/other", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusNotFound {
			t.Fatalf("status = %d", rr.Code)
		}
		assertLogContains(t, logs.String(), "proxy.request.failed", `"stage":"route_match"`, `"status":404`)
	})

	t.Run("method not allowed", func(t *testing.T) {
		store := &fakeStore{
			user: models.User{ID: 1, UserSlug: "alice"},
			routes: []models.Route{{
				ID:          1,
				Enabled:     true,
				MatchType:   "prefix",
				PathPattern: "/api",
				Methods:     []string{"POST"},
				UpstreamURL: "http://upstream.local",
			}},
		}
		logger, logs := testLogger()
		handler := NewHandler(store).WithLogger(logger)
		req := httptest.NewRequest(http.MethodGet, "/gw/alice/api", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusMethodNotAllowed {
			t.Fatalf("status = %d", rr.Code)
		}
		assertLogContains(t, logs.String(), "proxy.request.failed", `"stage":"method_match"`, `"status":405`)
	})

	t.Run("auth failed", func(t *testing.T) {
		store := &fakeStore{
			user: models.User{ID: 1, UserSlug: "alice"},
			routes: []models.Route{{
				ID:              1,
				Enabled:         true,
				MatchType:       "prefix",
				PathPattern:     "/api",
				Methods:         []string{"GET"},
				UpstreamURL:     "http://upstream.local",
				AuthRequired:    true,
				AuthServiceName: "route-service",
			}},
		}
		logger, logs := testLogger()
		handler := NewHandler(store).WithAuth(&fakeAuthClient{verifyOK: false}, fakeCipher{}).WithLogger(logger)
		req := httptest.NewRequest(http.MethodGet, "/gw/alice/api", nil)
		req.Header.Set("Access-Token", "caller-secret-token")
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusForbidden {
			t.Fatalf("status = %d", rr.Code)
		}
		output := logs.String()
		assertLogContains(t, output, "proxy.auth.failed", "proxy.request.failed", `"stage":"auth"`, `"status":403`)
		if strings.Contains(output, "caller-secret-token") {
			t.Fatalf("logs leaked caller token:\n%s", output)
		}
	})

	t.Run("upstream error", func(t *testing.T) {
		store := &fakeStore{
			user: models.User{ID: 1, UserSlug: "alice"},
			routes: []models.Route{{
				ID:             1,
				Enabled:        true,
				MatchType:      "prefix",
				PathPattern:    "/api",
				Methods:        []string{"GET"},
				UpstreamURL:    "http://upstream.local",
				TimeoutSeconds: 5,
			}},
		}
		logger, logs := testLogger()
		handler := NewHandler(store).WithLogger(logger)
		handler.httpClient = &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return nil, errors.New("upstream-secret-token")
		})}
		req := httptest.NewRequest(http.MethodGet, "/gw/alice/api", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)

		if rr.Code != http.StatusBadGateway {
			t.Fatalf("status = %d", rr.Code)
		}
		output := logs.String()
		assertLogContains(t, output, "proxy.upstream.request_failed", "proxy.request.failed", `"stage":"upstream_request"`, `"status":502`)
		if strings.Contains(output, "upstream-secret-token") {
			t.Fatalf("logs leaked upstream error details:\n%s", output)
		}
	})
}

type fakeStore struct {
	user       models.User
	userErr    error
	routes     []models.Route
	routesErr  error
	binding    models.ServiceGroupBinding
	bindingErr error
}

func (s *fakeStore) GetUserBySlug(ctx context.Context, slug string) (models.User, error) {
	if s.userErr != nil {
		return models.User{}, s.userErr
	}
	return s.user, nil
}

func (s *fakeStore) ListEnabledRoutes(ctx context.Context, userID int64) ([]models.Route, error) {
	if s.routesErr != nil {
		return nil, s.routesErr
	}
	return s.routes, nil
}

func (s *fakeStore) GetServiceGroupBinding(ctx context.Context, userID int64) (models.ServiceGroupBinding, error) {
	if s.bindingErr != nil {
		return models.ServiceGroupBinding{}, s.bindingErr
	}
	return s.binding, nil
}

type fakeAuthClient struct {
	verifyOK       bool
	verifyErr      error
	groupToken     string
	groupTokenErr  error
	lastTokenGroup string
	lastVerify     VerifyHeaders
}

func (c *fakeAuthClient) LatestServiceGroupToken(ctx context.Context, serviceGroupName, authorizationCode string) (GroupToken, error) {
	c.lastTokenGroup = serviceGroupName
	if c.groupTokenErr != nil {
		return GroupToken{}, c.groupTokenErr
	}
	return GroupToken{AccessToken: c.groupToken}, nil
}

func (c *fakeAuthClient) Verify(ctx context.Context, headers VerifyHeaders) (VerifyResult, error) {
	c.lastVerify = headers
	if c.verifyErr != nil {
		return VerifyResult{}, c.verifyErr
	}
	return VerifyResult{OK: c.verifyOK}, nil
}

type fakeCipher struct {
	code string
	err  error
}

func (c fakeCipher) DecryptBinding(ciphertext, salt, algorithm string) (string, error) {
	if c.err != nil {
		return "", c.err
	}
	return c.code, nil
}

func testLogger() (*slog.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return slog.New(slog.NewJSONHandler(&buf, nil)), &buf
}

func assertLogContains(t *testing.T, output string, parts ...string) {
	t.Helper()
	for _, part := range parts {
		if !strings.Contains(output, part) {
			t.Fatalf("logs do not contain %q:\n%s", part, output)
		}
	}
}

type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
