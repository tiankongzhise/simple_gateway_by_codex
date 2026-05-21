package web

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"simple_gateway_by_codex/internal/linksign"
	"simple_gateway_by_codex/internal/models"
)

func TestHandleCreateSignedLink(t *testing.T) {
	server := NewServerWithServices("https://gateway.example.com", false, stubAuthService{})
	server.routesStore = signedLinkRouteStore{route: models.Route{
		ID:          11,
		UserID:      1,
		Enabled:     true,
		AccessMode:  models.AccessModeSignedLink,
		MatchType:   "prefix",
		PathPattern: "/share",
		Methods:     []string{"GET"},
		UpstreamURL: "https://upstream.example",
	}}
	server.linkSigner = linksign.New("secret")

	req := httptest.NewRequest(http.MethodPost, "/api/routes/11/signed-link", strings.NewReader(`{
		"method":"GET",
		"path":"/share/report",
		"query":"file=2026",
		"expiresInSeconds":60
	}`))
	req.SetPathValue("id", "11")
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, models.User{ID: 1, UserSlug: "alice"}))
	rr := httptest.NewRecorder()

	server.handleCreateSignedLink(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
	var body struct {
		URL           string `json:"url"`
		Method        string `json:"method"`
		ExpiresAtUnix int64  `json:"expiresAtUnix"`
	}
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Method != http.MethodGet {
		t.Fatalf("method = %q", body.Method)
	}
	if !strings.HasPrefix(body.URL, "https://gateway.example.com/gw/alice/share/report?") {
		t.Fatalf("url = %q", body.URL)
	}
	if !strings.Contains(body.URL, "file=2026") || !strings.Contains(body.URL, linksign.ExpiresParam+"=") || !strings.Contains(body.URL, linksign.SignatureParam+"=") {
		t.Fatalf("url missing query params: %q", body.URL)
	}
	if body.ExpiresAtUnix == 0 {
		t.Fatal("missing expiresAtUnix")
	}
}

func TestHandleCreateSignedLinkRejectsNonSignedLinkRoute(t *testing.T) {
	server := NewServerWithServices("https://gateway.example.com", false, stubAuthService{})
	server.routesStore = signedLinkRouteStore{route: models.Route{
		ID:          12,
		UserID:      1,
		AccessMode:  models.AccessModeCallerToken,
		MatchType:   "prefix",
		PathPattern: "/api",
		Methods:     []string{"GET"},
		UpstreamURL: "https://upstream.example",
	}}
	server.linkSigner = linksign.New("secret")

	req := httptest.NewRequest(http.MethodPost, "/api/routes/12/signed-link", strings.NewReader(`{"method":"GET","path":"/api"}`))
	req.SetPathValue("id", "12")
	req = req.WithContext(context.WithValue(req.Context(), userContextKey, models.User{ID: 1, UserSlug: "alice"}))
	rr := httptest.NewRecorder()

	server.handleCreateSignedLink(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d body = %s", rr.Code, rr.Body.String())
	}
}
