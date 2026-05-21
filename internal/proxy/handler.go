package proxy

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"simple_gateway_by_codex/internal/httpx"
	"simple_gateway_by_codex/internal/models"
)

type Store interface {
	GetUserBySlug(ctx context.Context, slug string) (models.User, error)
	ListEnabledRoutes(ctx context.Context, userID int64) ([]models.Route, error)
}

// Handler serves /gw/{userSlug}/... requests.
type Handler struct {
	store      Store
	httpClient *http.Client
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

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	slug, remaining, ok := parseGatewayPath(r.URL.Path)
	if !ok {
		httpx.Error(w, http.StatusNotFound, "not_found", "网关路径不存在")
		return
	}
	user, err := h.store.GetUserBySlug(r.Context(), slug)
	if err != nil {
		httpx.Error(w, http.StatusNotFound, "not_found", "用户不存在")
		return
	}
	routes, err := h.store.ListEnabledRoutes(r.Context(), user.ID)
	if err != nil {
		httpx.Error(w, http.StatusInternalServerError, "internal_error", "读取路由失败")
		return
	}
	match := MatchRoute(routes, r.Method, remaining)
	if !match.PathMatched {
		httpx.Error(w, http.StatusNotFound, "not_found", "未找到匹配路由")
		return
	}
	if !match.MethodAllowed {
		httpx.Error(w, http.StatusMethodNotAllowed, "method_not_allowed", "路由不允许当前 HTTP 方法")
		return
	}
	if match.Route.AuthRequired {
		httpx.Error(w, http.StatusUnauthorized, "unauthorized", "鉴权代理尚未启用")
		return
	}
	h.forward(w, r, match)
}

func (h *Handler) forward(w http.ResponseWriter, r *http.Request, match MatchResult) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		httpx.Error(w, http.StatusBadRequest, "bad_request", "读取请求体失败")
		return
	}

	target, err := buildTargetURL(match.Route, match.RemainingPath, r.URL.RawQuery)
	if err != nil {
		httpx.Error(w, http.StatusBadGateway, "bad_gateway", "上游地址无效")
		return
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
		req, err := http.NewRequestWithContext(ctx, r.Method, target, bytes.NewReader(body))
		if err != nil {
			httpx.Error(w, http.StatusBadGateway, "bad_gateway", "构造上游请求失败")
			return
		}
		copyHeader(req.Header, r.Header)
		req.Host = req.URL.Host
		applyHeaderRules(req.Header, match.Route.RequestHeaders)

		resp, err := h.httpClient.Do(req)
		if err != nil {
			lastErr = err
			if ctx.Err() != nil {
				httpx.Error(w, http.StatusGatewayTimeout, "gateway_timeout", "上游请求超时")
				return
			}
			continue
		}
		defer resp.Body.Close()
		if shouldRetry(resp.StatusCode) && attempt < attempts-1 {
			io.Copy(io.Discard, resp.Body)
			continue
		}
		copyHeader(w.Header(), resp.Header)
		applyHeaderRules(w.Header(), match.Route.ResponseHeaders)
		w.WriteHeader(resp.StatusCode)
		_, _ = io.Copy(w, resp.Body)
		return
	}
	if lastErr != nil {
		httpx.Error(w, http.StatusBadGateway, "bad_gateway", "上游请求失败")
		return
	}
	httpx.Error(w, http.StatusBadGateway, "bad_gateway", "上游请求失败")
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

func copyHeader(dst, src http.Header) {
	for key, values := range src {
		dst.Del(key)
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func applyHeaderRules(headers http.Header, rules []models.HeaderRule) {
	for _, rule := range rules {
		switch rule.Operation {
		case "remove":
			headers.Del(rule.HeaderName)
		case "set":
			headers.Set(rule.HeaderName, rule.HeaderValue)
		}
	}
}

func shouldRetry(status int) bool {
	return status == http.StatusBadGateway || status == http.StatusServiceUnavailable || status == http.StatusGatewayTimeout
}
