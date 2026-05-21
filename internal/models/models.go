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
