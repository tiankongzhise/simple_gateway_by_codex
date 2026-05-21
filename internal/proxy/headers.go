package proxy

import (
	"net/http"
	"strings"

	"simple_gateway_by_codex/internal/models"
)

var hopByHopHeaders = map[string]bool{
	"Connection":          true,
	"Keep-Alive":          true,
	"Proxy-Authenticate":  true,
	"Proxy-Authorization": true,
	"Te":                  true,
	"Trailer":             true,
	"Transfer-Encoding":   true,
	"Upgrade":             true,
}

func copyProxyRequestHeader(dst, src http.Header) {
	for key, values := range src {
		if hopByHopHeaders[http.CanonicalHeaderKey(key)] {
			continue
		}
		dst.Del(key)
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func copyProxyResponseHeader(dst, src http.Header) {
	for key, values := range src {
		if hopByHopHeaders[http.CanonicalHeaderKey(key)] {
			continue
		}
		dst.Del(key)
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}

func applyHeaderRules(headers http.Header, rules []models.HeaderRule) {
	for _, rule := range rules {
		name := strings.TrimSpace(rule.HeaderName)
		if name == "" {
			continue
		}
		switch rule.Operation {
		case "remove":
			headers.Del(name)
		case "set":
			headers.Set(name, rule.HeaderValue)
		}
	}
}
