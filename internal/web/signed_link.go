package web

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"time"

	"simple_gateway_by_codex/internal/httpx"
	"simple_gateway_by_codex/internal/models"
	"simple_gateway_by_codex/internal/proxy"
)

const (
	defaultSignedLinkTTL = time.Hour
	maxSignedLinkTTL     = 24 * time.Hour
)

type signedLinkRequest struct {
	Method           string `json:"method"`
	Path             string `json:"path"`
	Query            string `json:"query"`
	ExpiresInSeconds int    `json:"expiresInSeconds"`
}

func (s *Server) handleCreateSignedLink(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r.Context())
	if !ok {
		writeAppError(w, errUnauthorized("未登录"))
		return
	}
	routeID, err := parseRouteID(r)
	if err != nil {
		writeAppError(w, err)
		return
	}
	var req signedLinkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAppError(w, errBadRequest("请求体必须是合法 JSON"))
		return
	}
	route, err := s.routesStore.GetRoute(r.Context(), user.ID, routeID)
	if err != nil {
		writeAppError(w, err)
		return
	}
	if route.AccessMode != models.AccessModeSignedLink {
		writeAppError(w, errBadRequest("只有 signed_link 路由可以生成签名链接"))
		return
	}
	method := strings.ToUpper(strings.TrimSpace(req.Method))
	if method == "" {
		method = http.MethodGet
	}
	path := normalizeSignedLinkPath(req.Path, route.PathPattern)
	if path == "" {
		writeAppError(w, errBadRequest("签名链接路径必须以 / 开头"))
		return
	}
	if _, err := url.ParseQuery(req.Query); err != nil {
		writeAppError(w, errBadRequest("签名链接查询参数无效"))
		return
	}
	match := proxy.MatchRoute([]models.Route{route}, method, path)
	if !match.PathMatched || !match.MethodAllowed {
		writeAppError(w, errBadRequest("签名链接方法和路径必须命中当前路由"))
		return
	}
	ttl := signedLinkTTL(req.ExpiresInSeconds)
	expiresAt := time.Now().Add(ttl)
	gatewayPath := "/gw/" + user.UserSlug + path
	signedQuery := s.linkSigner.AddSignature(method, gatewayPath, req.Query, expiresAt)
	linkURL := baseURLForRequest(s.publicBaseURL, r) + gatewayPath
	if signedQuery != "" {
		linkURL += "?" + signedQuery
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{
		"url":           linkURL,
		"method":        method,
		"expiresAtUnix": expiresAt.Unix(),
	})
}

func normalizeSignedLinkPath(path, fallback string) string {
	path = strings.TrimSpace(path)
	if path == "" {
		path = fallback
	}
	if path == "" || !strings.HasPrefix(path, "/") {
		return ""
	}
	if len(path) > 1 {
		path = strings.TrimRight(path, "/")
	}
	return path
}

func signedLinkTTL(seconds int) time.Duration {
	if seconds <= 0 {
		return defaultSignedLinkTTL
	}
	ttl := time.Duration(seconds) * time.Second
	if ttl > maxSignedLinkTTL {
		return maxSignedLinkTTL
	}
	return ttl
}

func baseURLForRequest(configured string, r *http.Request) string {
	configured = strings.TrimRight(strings.TrimSpace(configured), "/")
	if configured != "" {
		return configured
	}
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	if forwardedProto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")); forwardedProto != "" {
		scheme = forwardedProto
	}
	host := r.Host
	if forwardedHost := strings.TrimSpace(r.Header.Get("X-Forwarded-Host")); forwardedHost != "" {
		host = forwardedHost
	}
	return scheme + "://" + host
}
