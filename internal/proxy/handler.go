package proxy

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"simple_gateway_by_codex/internal/httpx"
	"simple_gateway_by_codex/internal/linksign"
	"simple_gateway_by_codex/internal/models"
)

const requestIDHeader = "X-Request-ID"

type Store interface {
	GetUserBySlug(ctx context.Context, slug string) (models.User, error)
	ListEnabledRoutes(ctx context.Context, userID int64) ([]models.Route, error)
	GetServiceGroupBinding(ctx context.Context, userID int64) (models.ServiceGroupBinding, error)
}

type AuthClient interface {
	LatestServiceGroupToken(ctx context.Context, serviceGroupName, authorizationCode string) (GroupToken, error)
	Verify(ctx context.Context, headers VerifyHeaders) (VerifyResult, error)
}

type GroupToken struct {
	AccessToken string
}

type VerifyHeaders struct {
	ServiceName       string
	TargetServiceName string
	AccessToken       string
	Origin            string
	Referer           string
	Model             string
}

type VerifyResult struct {
	OK bool
}

type AuthCodeCipher interface {
	DecryptBinding(ciphertext, salt, algorithm string) (string, error)
}

type traceContext struct {
	RequestID string
	Start     time.Time
	Logger    *slog.Logger
}

type forwardResult struct {
	OK             bool
	Status         int
	UpstreamStatus int
	Attempts       int
}

// Handler serves /gw/{userSlug}/... requests.
type Handler struct {
	store      Store
	httpClient *http.Client
	authClient AuthClient
	cipher     AuthCodeCipher
	linkSigner *linksign.Signer
	logger     *slog.Logger
}

// NewHandler constructs a gateway proxy handler.
func NewHandler(store Store) *Handler {
	return &Handler{
		store: store,
		httpClient: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: (&net.Dialer{
					Timeout:   10 * time.Second,
					KeepAlive: 30 * time.Second,
				}).DialContext,
				ForceAttemptHTTP2:     true,
				MaxIdleConns:          100,
				IdleConnTimeout:       90 * time.Second,
				TLSHandshakeTimeout:   10 * time.Second,
				ExpectContinueTimeout: 1 * time.Second,
			},
		},
	}
}

// WithAuth enables runtime auth-service verification for auth routes.
func (h *Handler) WithAuth(authClient AuthClient, cipher AuthCodeCipher) *Handler {
	h.authClient = authClient
	h.cipher = cipher
	return h
}

func (h *Handler) WithLinkSigner(signer linksign.Signer) *Handler {
	h.linkSigner = &signer
	return h
}

// WithLogger sets the structured logger used for gateway forwarding traces.
func (h *Handler) WithLogger(logger *slog.Logger) *Handler {
	if logger != nil {
		h.logger = logger
	}
	return h
}

func (h *Handler) activeLogger() *slog.Logger {
	if h.logger != nil {
		return h.logger
	}
	return slog.Default()
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := ensureRequestID(r)
	w.Header().Set(requestIDHeader, requestID)
	trace := traceContext{
		RequestID: requestID,
		Start:     time.Now(),
		Logger:    h.activeLogger().With("request_id", requestID),
	}
	trace.Logger.Info("proxy.request.start",
		"method", r.Method,
		"path", r.URL.Path,
		"has_query", r.URL.RawQuery != "",
		"remote_addr", r.RemoteAddr,
		"user_agent", r.UserAgent(),
	)

	slug, remaining, ok := parseGatewayPath(r.URL.Path)
	if !ok {
		httpx.Error(w, http.StatusNotFound, "not_found", "网关路径不存在")
		logRequestFailed(trace, "parse_gateway_path", http.StatusNotFound, "not_found", nil)
		return
	}
	user, err := h.store.GetUserBySlug(r.Context(), slug)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "用户不存在")
		logRequestFailed(trace, "lookup_user", http.StatusNotFound, "not_found", err)
		return
	}
	routes, err := h.store.ListEnabledRoutes(r.Context(), user.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "读取路由失败")
		logRequestFailed(trace, "list_routes", http.StatusInternalServerError, "internal_error", err)
		return
	}
	match := MatchRoute(routes, r.Method, remaining)
	trace.Logger.Info("proxy.route.lookup",
		"slug", slug,
		"user_id", user.ID,
		"enabled_route_count", len(routes),
		"path_matched", match.PathMatched,
		"method_allowed", match.MethodAllowed,
	)
	if !match.PathMatched {
		httpx.Error(w, http.StatusNotFound, "not_found", "未找到匹配路由")
		logRequestFailed(trace, "route_match", http.StatusNotFound, "not_found", nil)
		return
	}
	if !match.MethodAllowed {
		httpx.Error(w, http.StatusMethodNotAllowed, "method_not_allowed", "路由不允许当前 HTTP 方法")
		logRequestFailed(trace, "method_match", http.StatusMethodNotAllowed, "method_not_allowed", nil)
		return
	}
	trace.Logger.Info("proxy.route.matched",
		"route_id", match.Route.ID,
		"route_name", match.Route.Name,
		"match_type", match.Route.MatchType,
		"path_pattern", match.Route.PathPattern,
		"remaining_path", match.RemainingPath,
		"priority", match.Route.Priority,
		"access_mode", match.Route.AccessMode,
		"auth_required", match.Route.AuthRequired,
	)
	if routeRequiresCallerToken(match.Route) {
		if err := h.verifyRouteAuth(r, match.Route, trace); err != nil {
			status, code := writeProxyAuthError(w, err)
			logRequestFailed(trace, "auth", status, code, err)
			return
		}
	}
	if routeRequiresSignedLink(match.Route) {
		if err := h.verifySignedLink(r, match.Route, trace); err != nil {
			status, code := writeProxyAuthError(w, err)
			logRequestFailed(trace, "signed_link", status, code, err)
			return
		}
	}
	result := h.forward(w, r, match, trace)
	if !result.OK {
		return
	}
	trace.Logger.Info("proxy.request.finish",
		"status", result.Status,
		"route_id", match.Route.ID,
		"upstream_status", result.UpstreamStatus,
		"attempts", result.Attempts,
		"duration_ms", durationMS(trace.Start),
	)
}

func (h *Handler) verifyRouteAuth(r *http.Request, route models.Route, trace traceContext) error {
	if h.authClient == nil {
		return proxyAuthError{status: http.StatusInternalServerError, code: "auth_not_configured", message: "网关鉴权未配置"}
	}
	callerToken := strings.TrimSpace(r.Header.Get("Access-Token"))
	serviceName := strings.TrimSpace(r.Header.Get("Service-Name"))
	if serviceName == "" {
		serviceName = route.AuthServiceName
	}
	if callerToken == "" {
		trace.Logger.Warn("proxy.auth.failed",
			"auth_mode", "caller_token",
			"service_name", serviceName,
			"route_id", route.ID,
			"error", "missing_access_token",
		)
		return proxyAuthError{status: http.StatusUnauthorized, code: "missing_access_token", message: "缺少 Access-Token"}
	}
	trace.Logger.Info("proxy.auth.start",
		"auth_mode", "caller_token",
		"service_name", serviceName,
		"route_id", route.ID,
	)
	result, err := h.authClient.Verify(r.Context(), VerifyHeaders{
		ServiceName: serviceName,
		AccessToken: callerToken,
		Origin:      r.Header.Get("Origin"),
		Referer:     r.Header.Get("Referer"),
		Model:       r.Header.Get("model"),
	})
	if err != nil {
		trace.Logger.Warn("proxy.auth.failed",
			"auth_mode", "caller_token",
			"service_name", serviceName,
			"route_id", route.ID,
			"error", safeError(err),
		)
		return mapAuthError(err)
	}
	if !result.OK {
		trace.Logger.Warn("proxy.auth.failed",
			"auth_mode", "caller_token",
			"service_name", serviceName,
			"route_id", route.ID,
			"result_ok", false,
		)
		return proxyAuthError{status: http.StatusForbidden, code: "forbidden", message: "鉴权失败"}
	}
	trace.Logger.Info("proxy.auth.success",
		"auth_mode", "caller_token",
		"service_name", serviceName,
		"route_id", route.ID,
	)
	return nil
}

func routeRequiresCallerToken(route models.Route) bool {
	if route.AccessMode == "" {
		return route.AuthRequired
	}
	return route.AccessMode == models.AccessModeCallerToken
}

func routeRequiresSignedLink(route models.Route) bool {
	return route.AccessMode == models.AccessModeSignedLink
}

func (h *Handler) verifySignedLink(r *http.Request, route models.Route, trace traceContext) error {
	if h.linkSigner == nil {
		return proxyAuthError{status: http.StatusInternalServerError, code: "signed_link_not_configured", message: "签名链接未配置"}
	}
	trace.Logger.Info("proxy.signed_link.start",
		"route_id", route.ID,
	)
	businessQuery, err := h.linkSigner.Verify(r.Method, r.URL.Path, r.URL.RawQuery, time.Now())
	if err != nil {
		trace.Logger.Warn("proxy.signed_link.failed",
			"route_id", route.ID,
			"error", safeError(err),
		)
		return mapSignedLinkError(err)
	}
	r.URL.RawQuery = businessQuery
	trace.Logger.Info("proxy.signed_link.success",
		"route_id", route.ID,
		"has_business_query", businessQuery != "",
	)
	return nil
}

type proxyAuthError struct {
	status  int
	code    string
	message string
}

func (e proxyAuthError) Error() string {
	return e.message
}

func writeProxyAuthError(w http.ResponseWriter, err error) (int, string) {
	var authErr proxyAuthError
	if errors.As(err, &authErr) {
		httpx.Error(w, authErr.status, authErr.code, authErr.message)
		return authErr.status, authErr.code
	}
	httpx.Error(w, http.StatusUnauthorized, "unauthorized", "鉴权失败")
	return http.StatusUnauthorized, "unauthorized"
}

func mapAuthError(err error) error {
	type statusCoder interface {
		error
		Status() int
	}
	var withStatus statusCoder
	if errors.As(err, &withStatus) {
		switch withStatus.Status() {
		case http.StatusUnauthorized:
			return proxyAuthError{status: http.StatusUnauthorized, code: "unauthorized", message: "鉴权 token 无效"}
		case http.StatusForbidden:
			return proxyAuthError{status: http.StatusForbidden, code: "forbidden", message: "鉴权服务拒绝访问"}
		case http.StatusTooManyRequests:
			return proxyAuthError{status: http.StatusTooManyRequests, code: "rate_limited", message: "鉴权服务限流"}
		default:
			return proxyAuthError{status: http.StatusBadGateway, code: "auth_service_error", message: "鉴权服务异常"}
		}
	}
	return proxyAuthError{status: http.StatusBadGateway, code: "auth_service_error", message: "鉴权服务异常"}
}

func mapSignedLinkError(err error) error {
	switch {
	case errors.Is(err, linksign.ErrMissingSignature):
		return proxyAuthError{status: http.StatusUnauthorized, code: "missing_signature", message: "缺少签名链接参数"}
	case errors.Is(err, linksign.ErrInvalidExpires):
		return proxyAuthError{status: http.StatusUnauthorized, code: "invalid_signature_expires", message: "签名链接过期时间无效"}
	case errors.Is(err, linksign.ErrExpired):
		return proxyAuthError{status: http.StatusUnauthorized, code: "signature_expired", message: "签名链接已过期"}
	default:
		return proxyAuthError{status: http.StatusUnauthorized, code: "invalid_signature", message: "签名链接无效"}
	}
}

func (h *Handler) forward(w http.ResponseWriter, r *http.Request, match MatchResult, trace traceContext) forwardResult {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "bad_request", "读取请求体失败")
		logRequestFailed(trace, "read_request_body", http.StatusBadRequest, "bad_request", err)
		return forwardResult{Status: http.StatusBadRequest}
	}

	target, err := buildTargetURL(match.Route, match.RemainingPath, r.URL.RawQuery)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "bad_gateway", "上游地址无效")
		logRequestFailed(trace, "build_target_url", http.StatusBadGateway, "bad_gateway", err)
		return forwardResult{Status: http.StatusBadGateway}
	}

	timeout := time.Duration(match.Route.TimeoutSeconds) * time.Second
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	attempts := match.Route.RetryCount + 1
	if attempts < 1 {
		attempts = 1
	}
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		attemptNumber := attempt + 1
		trace.Logger.Info("proxy.upstream.request",
			"route_id", match.Route.ID,
			"target_url", logURL(target),
			"target_has_query", hasURLQuery(target),
			"timeout_seconds", int(timeout.Seconds()),
			"attempt", attemptNumber,
			"max_attempts", attempts,
			"request_header_rule_count", len(match.Route.RequestHeaders),
		)
		req, err := http.NewRequestWithContext(ctx, r.Method, target, bytes.NewReader(body))
		if err != nil {
			httpx.Error(w, http.StatusBadGateway, "bad_gateway", "构造上游请求失败")
			logRequestFailed(trace, "build_upstream_request", http.StatusBadGateway, "bad_gateway", err)
			return forwardResult{Status: http.StatusBadGateway, Attempts: attemptNumber}
		}
		copyProxyRequestHeader(req.Header, r.Header)
		req.Host = req.URL.Host
		applyHeaderRules(req.Header, match.Route.RequestHeaders)
		req.Header.Set(requestIDHeader, trace.RequestID)

		resp, err := h.httpClient.Do(req)
		if err != nil {
			lastErr = err
			if ctx.Err() != nil {
				httpx.Error(w, http.StatusGatewayTimeout, "gateway_timeout", "上游请求超时")
				logRequestFailed(trace, "upstream_timeout", http.StatusGatewayTimeout, "gateway_timeout", err)
				return forwardResult{Status: http.StatusGatewayTimeout, Attempts: attemptNumber}
			}
			trace.Logger.Warn("proxy.upstream.request_failed",
				"route_id", match.Route.ID,
				"target_url", logURL(target),
				"target_has_query", hasURLQuery(target),
				"attempt", attemptNumber,
				"max_attempts", attempts,
				"error", safeError(err),
			)
			continue
		}
		defer resp.Body.Close()
		retry := shouldRetry(resp.StatusCode) && attempt < attempts-1
		trace.Logger.Info("proxy.upstream.response",
			"route_id", match.Route.ID,
			"status", resp.StatusCode,
			"attempt", attemptNumber,
			"retry", retry,
			"response_header_rule_count", len(match.Route.ResponseHeaders),
		)
		if retry {
			io.Copy(io.Discard, resp.Body)
			continue
		}
		copyProxyResponseHeader(w.Header(), resp.Header)
		applyHeaderRules(w.Header(), match.Route.ResponseHeaders)
		w.Header().Set(requestIDHeader, trace.RequestID)
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
		return forwardResult{OK: true, Status: resp.StatusCode, UpstreamStatus: resp.StatusCode, Attempts: attemptNumber}
	}
	if lastErr != nil {
		httpx.Error(w, http.StatusBadGateway, "bad_gateway", "上游请求失败")
		logRequestFailed(trace, "upstream_request", http.StatusBadGateway, "bad_gateway", lastErr)
		return forwardResult{Status: http.StatusBadGateway, Attempts: attempts}
	}
	httpx.Error(w, http.StatusBadGateway, "bad_gateway", "上游请求失败")
	logRequestFailed(trace, "upstream_request", http.StatusBadGateway, "bad_gateway", nil)
	return forwardResult{Status: http.StatusBadGateway, Attempts: attempts}
}

func parseGatewayPath(path string) (string, string, bool) {
	path = strings.TrimPrefix(path, "/")
	parts := strings.SplitN(path, "/", 3)
	if len(parts) < 2 || parts[0] != "gw" || parts[1] == "" {
		return "", "", false
	}
	remaining := "/"
	if len(parts) == 3 && parts[2] != "" {
		remaining = "/" + parts[2]
	}
	return parts[1], remaining, true
}

func buildTargetURL(route models.Route, remainingPath, rawQuery string) (string, error) {
	base, err := url.Parse(route.UpstreamURL)
	if err != nil {
		return "", err
	}
	targetPath := remainingPath
	if !route.StripPrefix {
		targetPath = route.PathPattern
		if remainingPath != "/" {
			targetPath = strings.TrimRight(route.PathPattern, "/") + remainingPath
		}
	}
	base.Path = joinURLPath(base.Path, targetPath)
	base.RawQuery = rawQuery
	return base.String(), nil
}

func joinURLPath(basePath, routePath string) string {
	if basePath == "" || basePath == "/" {
		return normalizeRequestPath(routePath)
	}
	return strings.TrimRight(basePath, "/") + normalizeRequestPath(routePath)
}

func shouldRetry(status int) bool {
	return status == http.StatusBadGateway || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}

func ensureRequestID(r *http.Request) string {
	requestID := strings.TrimSpace(r.Header.Get(requestIDHeader))
	if requestID == "" {
		requestID = newRequestID()
	}
	r.Header.Set(requestIDHeader, requestID)
	return requestID
}

func newRequestID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err == nil {
		return hex.EncodeToString(b[:])
	}
	return strconv.FormatInt(time.Now().UnixNano(), 36)
}

func logRequestFailed(trace traceContext, stage string, status int, code string, err error) {
	args := []any{
		"stage", stage,
		"status", status,
		"code", code,
		"duration_ms", durationMS(trace.Start),
	}
	if err != nil {
		args = append(args, "error", safeError(err))
	}
	trace.Logger.Warn("proxy.request.failed", args...)
}

func durationMS(start time.Time) int64 {
	return time.Since(start).Milliseconds()
}

func safeError(err error) string {
	if err == nil {
		return ""
	}
	var authErr proxyAuthError
	if errors.As(err, &authErr) {
		return authErr.code
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		if netErr.Timeout() {
			return "network_timeout"
		}
		return "network_error"
	}
	type statusCoder interface {
		error
		Status() int
	}
	var withStatus statusCoder
	if errors.As(err, &withStatus) {
		return "status_" + strconv.Itoa(withStatus.Status())
	}
	return "error"
}

func logURL(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func hasURLQuery(rawURL string) bool {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return parsed.RawQuery != ""
}
