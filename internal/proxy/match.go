package proxy

import (
	"net/http"
	"sort"
	"strings"

	"simple_gateway_by_codex/internal/models"
)

// MatchResult is the result of matching a gateway request path.
type MatchResult struct {
	Route             models.Route
	PathMatched       bool
	MethodAllowed     bool
	RemainingPath     string
	MatchedPathLength int
}

// MatchRoute selects the best enabled route for path and method.
func MatchRoute(routes []models.Route, method, path string) MatchResult {
	path = normalizeRequestPath(path)
	candidates := make([]matchCandidate, 0, len(routes))
	pathMatched := false

	for _, route := range routes {
		if !route.Enabled {
			continue
		}
		matched, remaining := matchPath(route, path)
		if !matched {
			continue
		}
		pathMatched = true
		if !methodAllowed(route.Methods, method) {
			continue
		}
		candidates = append(candidates, matchCandidate{
			route:         route,
			remainingPath: remaining,
			pathLen:       len(route.PathPattern),
		})
	}

	if len(candidates) == 0 {
		return MatchResult{PathMatched: pathMatched, MethodAllowed: false}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].route.Priority != candidates[j].route.Priority {
			return candidates[i].route.Priority > candidates[j].route.Priority
		}
		if candidates[i].pathLen != candidates[j].pathLen {
			return candidates[i].pathLen > candidates[j].pathLen
		}
		return candidates[i].route.ID < candidates[j].route.ID
	})

	best := candidates[0]
	return MatchResult{
		Route:             best.route,
		PathMatched:       true,
		MethodAllowed:     true,
		RemainingPath:     best.remainingPath,
		MatchedPathLength: best.pathLen,
	}
}

type matchCandidate struct {
	route         models.Route
	remainingPath string
	pathLen       int
}

func matchPath(route models.Route, path string) (bool, string) {
	pattern := normalizeRequestPath(route.PathPattern)
	switch route.MatchType {
	case "exact":
		if path == pattern {
			return true, "/"
		}
		return false, ""
	default:
		if pattern == "/" {
			return true, path
		}
		if path == pattern {
			return true, "/"
		}
		if strings.HasPrefix(path, pattern+"/") {
			return true, strings.TrimPrefix(path, pattern)
		}
		return false, ""
	}
}

func methodAllowed(methods []string, method string) bool {
	method = strings.ToUpper(method)
	for _, allowed := range methods {
		allowed = strings.ToUpper(strings.TrimSpace(allowed))
		if allowed == "ALL" || allowed == method {
			return true
		}
	}
	if method == http.MethodHead {
		for _, allowed := range methods {
			if strings.ToUpper(strings.TrimSpace(allowed)) == http.MethodGet {
				return true
			}
		}
	}
	return false
}

func normalizeRequestPath(path string) string {
	if path == "" {
		return "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	if len(path) > 1 {
		path = strings.TrimRight(path, "/")
	}
	return path
}
