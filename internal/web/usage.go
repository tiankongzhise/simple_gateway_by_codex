package web

import "net/http"

// UsageDocument describes the public API surface of this gateway.
type UsageDocument struct {
	ServiceName string         `json:"serviceName"`
	Version     string         `json:"version"`
	Description string         `json:"description"`
	Conventions map[string]any `json:"conventions"`
	Endpoints   []EndpointDoc  `json:"endpoints"`
	Gateway     GatewayDoc     `json:"gateway"`
	ErrorCodes  []ErrorCodeDoc `json:"errorCodes"`
}

type EndpointDoc struct {
	Method       string     `json:"method"`
	Path         string     `json:"path"`
	AuthRequired bool       `json:"authRequired"`
	Description  string     `json:"description"`
	Headers      []FieldDoc `json:"headers,omitempty"`
	RequestBody  []FieldDoc `json:"requestBody,omitempty"`
	ResponseBody []FieldDoc `json:"responseBody,omitempty"`
}

type FieldDoc struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Required    bool   `json:"required"`
	Description string `json:"description"`
}

type GatewayDoc struct {
	Entry       string     `json:"entry"`
	Description string     `json:"description"`
	Headers     []FieldDoc `json:"headers"`
	Behaviors   []string   `json:"behaviors"`
	PrivateData string     `json:"privateData"`
}

type ErrorCodeDoc struct {
	Status      int    `json:"status"`
	Description string `json:"description"`
}

// BuildUsageDocument returns a static, privacy-safe usage document.
func BuildUsageDocument(publicBaseURL string) UsageDocument {
	base := publicBaseURL
	if base == "" {
		base = "http://localhost:8080"
	}

	return UsageDocument{
		ServiceName: "simple_gateway_by_codex",
		Version:     "1.0",
		Description: "Multi-tenant HTTP gateway with isolated route management, optional auth-service verification, and reverse proxy forwarding.",
		Conventions: map[string]any{
			"contentType": "JSON APIs use application/json. HTML management pages use standard form posts.",
			"baseURL":     base,
			"session":     "Management APIs use the gateway_session HttpOnly cookie after login.",
			"gatewayPath": "/gw/{userSlug}/{path}",
			"headerNaming": []string{
				"Access-Token",
				"Service-Name",
				"Origin",
				"Referer",
				"model",
			},
		},
		Endpoints: []EndpointDoc{
			{
				Method:       http.MethodGet,
				Path:         "/api/public/usage",
				AuthRequired: false,
				Description:  "Return this public usage document. It never exposes private user route configuration or authorization codes.",
				ResponseBody: []FieldDoc{
					{Name: "serviceName", Type: "string", Required: true, Description: "Gateway service name."},
					{Name: "version", Type: "string", Required: true, Description: "Gateway public API version."},
					{Name: "endpoints", Type: "array<object>", Required: true, Description: "Public and management API descriptions."},
					{Name: "gateway", Type: "object", Required: true, Description: "Gateway forwarding entry and header rules."},
					{Name: "errorCodes", Type: "array<object>", Required: true, Description: "Common HTTP error codes."},
				},
			},
			{
				Method:       http.MethodPost,
				Path:         "/api/register",
				AuthRequired: false,
				Description:  "Register a user with invite code and bind an auth-service service group.",
				Headers:      []FieldDoc{{Name: "Content-Type", Type: "string", Required: true, Description: "application/json"}},
				RequestBody: []FieldDoc{
					{Name: "username", Type: "string", Required: true, Description: "Unique login username."},
					{Name: "userSlug", Type: "string", Required: true, Description: "Unique public gateway slug used in /gw/{userSlug}."},
					{Name: "password", Type: "string", Required: true, Description: "Login password."},
					{Name: "inviteCode", Type: "string", Required: true, Description: "Shared invite code configured in .env."},
					{Name: "serviceGroupName", Type: "string", Required: true, Description: "Auth-service service group name."},
					{Name: "authorizationCode", Type: "string", Required: true, Description: "Permanent authorization code of the service group."},
				},
				ResponseBody: []FieldDoc{{Name: "user", Type: "object", Required: true, Description: "Created user profile without secrets."}},
			},
			{
				Method:       http.MethodPost,
				Path:         "/api/login",
				AuthRequired: false,
				Description:  "Login with username and password and receive a session cookie.",
				RequestBody: []FieldDoc{
					{Name: "username", Type: "string", Required: true, Description: "Login username."},
					{Name: "password", Type: "string", Required: true, Description: "Login password."},
				},
				ResponseBody: []FieldDoc{{Name: "user", Type: "object", Required: true, Description: "Current user profile."}},
			},
			{
				Method:       http.MethodPost,
				Path:         "/api/logout",
				AuthRequired: true,
				Description:  "Delete the current session.",
				ResponseBody: []FieldDoc{{Name: "ok", Type: "boolean", Required: true, Description: "Whether logout succeeded."}},
			},
			{
				Method:       http.MethodGet,
				Path:         "/api/me",
				AuthRequired: true,
				Description:  "Return the current authenticated user profile and service group binding summary.",
				ResponseBody: []FieldDoc{{Name: "user", Type: "object", Required: true, Description: "Current user and binding summary without secrets."}},
			},
			{
				Method:       http.MethodPut,
				Path:         "/api/service-group-binding",
				AuthRequired: true,
				Description:  "Replace the current user's auth-service group binding after validating the permanent authorization code.",
				RequestBody: []FieldDoc{
					{Name: "serviceGroupName", Type: "string", Required: true, Description: "New service group name."},
					{Name: "authorizationCode", Type: "string", Required: true, Description: "Permanent authorization code for the service group."},
				},
				ResponseBody: []FieldDoc{{Name: "binding", Type: "object", Required: true, Description: "Updated binding summary without secrets."}},
			},
			{
				Method:       http.MethodGet,
				Path:         "/api/routes",
				AuthRequired: true,
				Description:  "List only the current user's route rules.",
				ResponseBody: []FieldDoc{{Name: "routes", Type: "array<object>", Required: true, Description: "Route rules owned by the current user."}},
			},
			{
				Method:       http.MethodPost,
				Path:         "/api/routes",
				AuthRequired: true,
				Description:  "Create a route. If authRequired is true, authServiceName is required and must be managed by the bound service group.",
				RequestBody:  routeFields(),
				ResponseBody: []FieldDoc{{Name: "route", Type: "object", Required: true, Description: "Created route."}},
			},
			{
				Method:       http.MethodPut,
				Path:         "/api/routes/{id}",
				AuthRequired: true,
				Description:  "Update a route owned by the current user. Auth routes revalidate service-group permission.",
				RequestBody:  routeFields(),
				ResponseBody: []FieldDoc{{Name: "route", Type: "object", Required: true, Description: "Updated route."}},
			},
			{
				Method:       http.MethodDelete,
				Path:         "/api/routes/{id}",
				AuthRequired: true,
				Description:  "Delete a route owned by the current user.",
				ResponseBody: []FieldDoc{{Name: "ok", Type: "boolean", Required: true, Description: "Whether deletion succeeded."}},
			},
		},
		Gateway: GatewayDoc{
			Entry:       "/gw/{userSlug}/{path}",
			Description: "Forward requests through the matched enabled route owned by userSlug.",
			Headers: []FieldDoc{
				{Name: "Access-Token", Type: "string", Required: false, Description: "For auth routes, caller token used first when present."},
				{Name: "Service-Name", Type: "string", Required: false, Description: "For auth routes, forwarded to auth-service when present; otherwise route authServiceName is used."},
				{Name: "Origin", Type: "string", Required: false, Description: "Forwarded to auth-service for normal service token verification."},
				{Name: "Referer", Type: "string", Required: false, Description: "Fallback source header for auth-service verification."},
				{Name: "model", Type: "string", Required: false, Description: "Forwarded to auth-service; dev bypass behavior is controlled by auth-service."},
			},
			Behaviors: []string{
				"Non-auth routes forward directly to upstream.",
				"Auth routes verify before forwarding and reject failed requests with 401 or 403.",
				"If Access-Token is absent, the gateway uses the user's bound service-group token to verify the target service.",
				"Route matching is isolated by userSlug and never searches another user's routes.",
			},
			PrivateData: "The public usage document does not include user route configuration, authorization codes, or tokens.",
		},
		ErrorCodes: []ErrorCodeDoc{
			{Status: 400, Description: "Invalid request fields or JSON body."},
			{Status: 401, Description: "Missing or invalid session or access token."},
			{Status: 403, Description: "Invite code mismatch, forbidden user data access, or auth-service permission denied."},
			{Status: 404, Description: "User, route, service group, service, or upstream resource not found."},
			{Status: 405, Description: "Gateway route path exists but HTTP method is not allowed."},
			{Status: 409, Description: "Duplicate username, user slug, or conflicting data."},
			{Status: 429, Description: "Auth-service rate limit exceeded."},
			{Status: 502, Description: "Upstream or auth-service returned an invalid gateway response."},
			{Status: 504, Description: "Upstream request timed out."},
			{Status: 500, Description: "Unexpected gateway server error."},
		},
	}
}

func routeFields() []FieldDoc {
	return []FieldDoc{
		{Name: "name", Type: "string", Required: true, Description: "Route display name."},
		{Name: "description", Type: "string", Required: false, Description: "Route description."},
		{Name: "enabled", Type: "boolean", Required: true, Description: "Whether the route can match gateway requests."},
		{Name: "matchType", Type: "string", Required: true, Description: "prefix or exact."},
		{Name: "pathPattern", Type: "string", Required: true, Description: "External path below /gw/{userSlug}."},
		{Name: "methods", Type: "array<string>", Required: true, Description: "ALL or selected HTTP methods."},
		{Name: "upstreamUrl", Type: "string", Required: true, Description: "Upstream base URL."},
		{Name: "stripPrefix", Type: "boolean", Required: true, Description: "Remove matched pathPattern before forwarding."},
		{Name: "timeoutSeconds", Type: "number", Required: true, Description: "Proxy timeout in seconds."},
		{Name: "retryCount", Type: "number", Required: true, Description: "Network/5xx retry count."},
		{Name: "priority", Type: "number", Required: true, Description: "Higher priority wins before longest path."},
		{Name: "authRequired", Type: "boolean", Required: true, Description: "Whether auth-service verification is required before forwarding."},
		{Name: "authServiceName", Type: "string", Required: false, Description: "Required when authRequired is true; used as Service-Name fallback."},
		{Name: "requestHeaderRules", Type: "array<object>", Required: false, Description: "Set/remove rules applied before upstream request."},
		{Name: "responseHeaderRules", Type: "array<object>", Required: false, Description: "Set/remove rules applied before responding to caller."},
	}
}
