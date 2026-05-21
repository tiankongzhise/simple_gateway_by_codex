package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"simple_gateway_by_codex/internal/httpx"
	"simple_gateway_by_codex/internal/models"
)

type routeStore interface {
	ListRoutes(ctx context.Context, userID int64) ([]models.Route, error)
	CreateRoute(ctx context.Context, route models.Route) (models.Route, error)
	UpdateRoute(ctx context.Context, route models.Route) (models.Route, error)
	DeleteRoute(ctx context.Context, userID, routeID int64) error
	GetRoute(ctx context.Context, userID, routeID int64) (models.Route, error)
}

type routeRequest struct {
	Name                string              `json:"name"`
	Description         string              `json:"description"`
	Enabled             bool                `json:"enabled"`
	MatchType           string              `json:"matchType"`
	PathPattern         string              `json:"pathPattern"`
	Methods             []string            `json:"methods"`
	UpstreamURL         string              `json:"upstreamUrl"`
	StripPrefix         bool                `json:"stripPrefix"`
	TimeoutSeconds      int                 `json:"timeoutSeconds"`
	RetryCount          int                 `json:"retryCount"`
	Priority            int                 `json:"priority"`
	AuthRequired        bool                `json:"authRequired"`
	AuthServiceName     string              `json:"authServiceName"`
	RequestHeaderRules  []headerRuleRequest `json:"requestHeaderRules"`
	ResponseHeaderRules []headerRuleRequest `json:"responseHeaderRules"`
}

type headerRuleRequest struct {
	Operation   string `json:"operation"`
	HeaderName  string `json:"headerName"`
	HeaderValue string `json:"headerValue"`
}

func (s *Server) handleListRoutes(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r.Context())
	if !ok {
		writeAppError(w, errUnauthorized("未登录"))
		return
	}
	routes, err := s.routesStore.ListRoutes(r.Context(), user.ID)
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"routes": routes})
}

func (s *Server) handleCreateRoute(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r.Context())
	if !ok {
		writeAppError(w, errUnauthorized("未登录"))
		return
	}
	var req routeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAppError(w, errBadRequest("请求体不是合法 JSON"))
		return
	}
	route, err := req.toModel(user.ID, 0)
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := s.validateRouteAuthService(r.Context(), user.ID, route); err != nil {
		writeAppError(w, err)
		return
	}
	created, err := s.routesStore.CreateRoute(r.Context(), route)
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"route": created})
}

func (s *Server) handleUpdateRoute(w http.ResponseWriter, r *http.Request) {
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
	var req routeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAppError(w, errBadRequest("请求体不是合法 JSON"))
		return
	}
	route, err := req.toModel(user.ID, routeID)
	if err != nil {
		writeAppError(w, err)
		return
	}
	if err := s.validateRouteAuthService(r.Context(), user.ID, route); err != nil {
		writeAppError(w, err)
		return
	}
	updated, err := s.routesStore.UpdateRoute(r.Context(), route)
	if err != nil {
		writeAppError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"route": updated})
}

func (s *Server) handleDeleteRoute(w http.ResponseWriter, r *http.Request) {
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
	if err := s.routesStore.DeleteRoute(r.Context(), user.ID, routeID); err != nil {
		writeAppError(w, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func parseRouteID(r *http.Request) (int64, error) {
	idText := r.PathValue("id")
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil || id <= 0 {
		return 0, errBadRequest("路由 ID 无效")
	}
	return id, nil
}

func (r routeRequest) toModel(userID, routeID int64) (models.Route, error) {
	route := models.Route{
		ID:              routeID,
		UserID:          userID,
		Name:            strings.TrimSpace(r.Name),
		Description:     strings.TrimSpace(r.Description),
		Enabled:         r.Enabled,
		MatchType:       strings.TrimSpace(r.MatchType),
		PathPattern:     normalizePathPattern(r.PathPattern),
		Methods:         normalizeMethods(r.Methods),
		UpstreamURL:     strings.TrimSpace(r.UpstreamURL),
		StripPrefix:     r.StripPrefix,
		TimeoutSeconds:  r.TimeoutSeconds,
		RetryCount:      r.RetryCount,
		Priority:        r.Priority,
		AuthRequired:    r.AuthRequired,
		AuthServiceName: strings.TrimSpace(r.AuthServiceName),
		RequestHeaders:  normalizeHeaderRules("request", r.RequestHeaderRules),
		ResponseHeaders: normalizeHeaderRules("response", r.ResponseHeaderRules),
	}
	if !route.AuthRequired {
		route.AuthServiceName = ""
	}
	if route.TimeoutSeconds == 0 {
		route.TimeoutSeconds = 30
	}
	if err := validateRoute(route); err != nil {
		return models.Route{}, err
	}
	return route, nil
}

func validateRoute(route models.Route) error {
	if route.Name == "" || len(route.Name) > 120 {
		return errBadRequest("路由名称不能为空且不能超过 120 个字符")
	}
	if route.MatchType != "prefix" && route.MatchType != "exact" {
		return errBadRequest("匹配方式必须是 prefix 或 exact")
	}
	if route.PathPattern == "" || !strings.HasPrefix(route.PathPattern, "/") {
		return errBadRequest("外部路径必须以 / 开头")
	}
	if len(route.Methods) == 0 {
		return errBadRequest("HTTP 方法不能为空")
	}
	parsedUpstream, err := url.Parse(route.UpstreamURL)
	if err != nil || parsedUpstream.Host == "" || (parsedUpstream.Scheme != "http" && parsedUpstream.Scheme != "https") {
		return errBadRequest("上游地址必须是合法 http 或 https URL")
	}
	if route.TimeoutSeconds <= 0 {
		return errBadRequest("超时时间必须大于 0")
	}
	if route.RetryCount < 0 {
		return errBadRequest("重试次数不能小于 0")
	}
	if route.AuthRequired && route.AuthServiceName == "" {
		return errBadRequest("鉴权路由必须填写鉴权服务名称")
	}
	for _, rule := range append(route.RequestHeaders, route.ResponseHeaders...) {
		if rule.Operation != "set" && rule.Operation != "remove" {
			return errBadRequest("头规则操作必须是 set 或 remove")
		}
		if strings.TrimSpace(rule.HeaderName) == "" {
			return errBadRequest("头规则名称不能为空")
		}
	}
	return nil
}

func normalizePathPattern(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return value
	}
	if !strings.HasPrefix(value, "/") {
		value = "/" + value
	}
	if len(value) > 1 {
		value = strings.TrimRight(value, "/")
	}
	return value
}

func normalizeMethods(methods []string) []string {
	if len(methods) == 0 {
		return []string{"ALL"}
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(methods))
	for _, method := range methods {
		method = strings.ToUpper(strings.TrimSpace(method))
		if method == "" || seen[method] {
			continue
		}
		if method == "ALL" {
			return []string{"ALL"}
		}
		seen[method] = true
		out = append(out, method)
	}
	if len(out) == 0 {
		return []string{"ALL"}
	}
	return out
}

func normalizeHeaderRules(phase string, rules []headerRuleRequest) []models.HeaderRule {
	out := make([]models.HeaderRule, 0, len(rules))
	for _, rule := range rules {
		out = append(out, models.HeaderRule{
			Phase:       phase,
			Operation:   strings.ToLower(strings.TrimSpace(rule.Operation)),
			HeaderName:  strings.TrimSpace(rule.HeaderName),
			HeaderValue: rule.HeaderValue,
		})
	}
	return out
}

func (s *Server) validateRouteAuthService(ctx context.Context, userID int64, route models.Route) error {
	if !route.AuthRequired {
		return nil
	}
	if s.authVerifier == nil || s.authCodeCipher == nil {
		return errInternal(errors.New("route auth verifier is not configured"))
	}
	binding, err := s.auth.GetServiceGroupBinding(ctx, userID)
	if err != nil {
		return errUnauthorized("请先绑定鉴权服务组")
	}
	authorizationCode, err := s.authCodeCipher.DecryptBinding(
		binding.EncryptedAuthorizationCode,
		binding.Salt,
		binding.Algorithm,
	)
	if err != nil {
		return errInternal(err)
	}
	return s.authVerifier.ValidateManagedService(ctx, binding.ServiceGroupName, authorizationCode, route.AuthServiceName)
}
