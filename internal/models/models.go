package models

import "time"

// User is an account that owns isolated route configuration.
type User struct {
	ID           int64     `json:"id"`
	Username     string    `json:"username"`
	UserSlug     string    `json:"userSlug"`
	PasswordHash string    `json:"-"`
	CreatedAt    time.Time `json:"createdAt"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// ServiceGroupBinding stores the current auth-service group binding for a user.
type ServiceGroupBinding struct {
	ID                         int64     `json:"id"`
	UserID                     int64     `json:"userId"`
	ServiceGroupName           string    `json:"serviceGroupName"`
	EncryptedAuthorizationCode string    `json:"-"`
	Salt                       string    `json:"-"`
	Algorithm                  string    `json:"algorithm"`
	CreatedAt                  time.Time `json:"createdAt"`
	UpdatedAt                  time.Time `json:"updatedAt"`
}

// Session is a server-side login session.
type Session struct {
	ID        int64
	UserID    int64
	TokenHash string
	ExpiresAt time.Time
	CreatedAt time.Time
}

// Route describes one user-owned gateway forwarding rule.
type Route struct {
	ID              int64        `json:"id"`
	UserID          int64        `json:"userId"`
	Name            string       `json:"name"`
	Description     string       `json:"description"`
	Enabled         bool         `json:"enabled"`
	MatchType       string       `json:"matchType"`
	PathPattern     string       `json:"pathPattern"`
	Methods         []string     `json:"methods"`
	UpstreamURL     string       `json:"upstreamUrl"`
	StripPrefix     bool         `json:"stripPrefix"`
	TimeoutSeconds  int          `json:"timeoutSeconds"`
	RetryCount      int          `json:"retryCount"`
	Priority        int          `json:"priority"`
	AuthRequired    bool         `json:"authRequired"`
	AuthServiceName string       `json:"authServiceName"`
	RequestHeaders  []HeaderRule `json:"requestHeaderRules,omitempty"`
	ResponseHeaders []HeaderRule `json:"responseHeaderRules,omitempty"`
	CreatedAt       time.Time    `json:"createdAt"`
	UpdatedAt       time.Time    `json:"updatedAt"`
}

// HeaderRule mutates request or response headers during proxying.
type HeaderRule struct {
	ID          int64     `json:"id"`
	RouteID     int64     `json:"routeId"`
	Phase       string    `json:"phase"`
	Operation   string    `json:"operation"`
	HeaderName  string    `json:"headerName"`
	HeaderValue string    `json:"headerValue"`
	CreatedAt   time.Time `json:"createdAt"`
}
