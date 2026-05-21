package web

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"simple_gateway_by_codex/internal/db"
	"simple_gateway_by_codex/internal/httpx"
	"simple_gateway_by_codex/internal/models"
	"simple_gateway_by_codex/internal/security"
)

const (
	sessionCookieName = "gateway_session"
	sessionTTL        = 7 * 24 * time.Hour
)

type contextKey string

const userContextKey contextKey = "user"

var slugPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{2,62}$`)

type authService interface {
	Register(ctx context.Context, req RegisterRequest) (models.User, string, error)
	Login(ctx context.Context, username, password string) (models.User, string, error)
	GetSessionUser(ctx context.Context, token string) (models.User, error)
	DeleteSession(ctx context.Context, token string) error
}

type basicAuthService struct {
	store        *db.Store
	inviteCode   string
	cookieSecure bool
}

type RegisterRequest struct {
	Username          string `json:"username"`
	UserSlug          string `json:"userSlug"`
	Password          string `json:"password"`
	InviteCode        string `json:"inviteCode"`
	ServiceGroupName  string `json:"serviceGroupName"`
	AuthorizationCode string `json:"authorizationCode"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func newBasicAuthService(store *db.Store, inviteCode string, cookieSecure bool) *basicAuthService {
	return &basicAuthService{store: store, inviteCode: inviteCode, cookieSecure: cookieSecure}
}

// NewBasicAuthForApp creates the default auth service used by the application.
func NewBasicAuthForApp(store *db.Store, inviteCode string, cookieSecure bool) authService {
	return newBasicAuthService(store, inviteCode, cookieSecure)
}

func (s *basicAuthService) Register(ctx context.Context, req RegisterRequest) (models.User, string, error) {
	req.Username = strings.TrimSpace(req.Username)
	req.UserSlug = strings.TrimSpace(req.UserSlug)
	if req.InviteCode != s.inviteCode {
		return models.User{}, "", errForbidden("邀请码不正确")
	}
	if req.Username == "" || len(req.Username) > 80 {
		return models.User{}, "", errBadRequest("用户名不能为空且不能超过 80 个字符")
	}
	if !slugPattern.MatchString(req.UserSlug) {
		return models.User{}, "", errBadRequest("用户 slug 只能包含字母、数字、下划线和中划线，长度 3-63")
	}
	if len(req.Password) < 8 {
		return models.User{}, "", errBadRequest("密码长度至少 8 位")
	}

	hash, err := security.HashPassword(req.Password)
	if err != nil {
		return models.User{}, "", errInternal(err)
	}
	user, err := s.store.CreateUser(ctx, req.Username, req.UserSlug, hash)
	if err != nil {
		return models.User{}, "", errConflict("用户名或用户 slug 已存在")
	}
	token, err := s.createSession(ctx, user.ID)
	if err != nil {
		return models.User{}, "", err
	}
	return user, token, nil
}

func (s *basicAuthService) Login(ctx context.Context, username, password string) (models.User, string, error) {
	user, err := s.store.GetUserByUsername(ctx, strings.TrimSpace(username))
	if err != nil {
		return models.User{}, "", errUnauthorized("用户名或密码不正确")
	}
	if !security.CheckPassword(user.PasswordHash, password) {
		return models.User{}, "", errUnauthorized("用户名或密码不正确")
	}
	token, err := s.createSession(ctx, user.ID)
	if err != nil {
		return models.User{}, "", err
	}
	return user, token, nil
}

func (s *basicAuthService) GetSessionUser(ctx context.Context, token string) (models.User, error) {
	if token == "" {
		return models.User{}, errUnauthorized("未登录")
	}
	return s.store.GetSessionUser(ctx, security.HashToken(token), time.Now())
}

func (s *basicAuthService) DeleteSession(ctx context.Context, token string) error {
	if token == "" {
		return nil
	}
	return s.store.DeleteSession(ctx, security.HashToken(token))
}

func (s *basicAuthService) createSession(ctx context.Context, userID int64) (string, error) {
	token, err := security.NewToken(32)
	if err != nil {
		return "", errInternal(err)
	}
	if err := s.store.CreateSession(ctx, userID, security.HashToken(token), time.Now().Add(sessionTTL)); err != nil {
		return "", errInternal(err)
	}
	return token, nil
}

func (s *Server) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req RegisterRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAppError(w, errBadRequest("请求体不是合法 JSON"))
		return
	}
	user, token, err := s.auth.Register(r.Context(), req)
	if err != nil {
		writeAppError(w, err)
		return
	}
	s.setSessionCookie(w, token)
	httpx.WriteJSON(w, http.StatusCreated, map[string]any{"user": publicUser(user)})
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeAppError(w, errBadRequest("请求体不是合法 JSON"))
		return
	}
	user, token, err := s.auth.Login(r.Context(), req.Username, req.Password)
	if err != nil {
		writeAppError(w, err)
		return
	}
	s.setSessionCookie(w, token)
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"user": publicUser(user)})
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	token := sessionToken(r)
	if err := s.auth.DeleteSession(r.Context(), token); err != nil {
		writeAppError(w, err)
		return
	}
	s.clearSessionCookie(w)
	httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) {
	user, ok := currentUser(r.Context())
	if !ok {
		writeAppError(w, errUnauthorized("未登录"))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"user": publicUser(user)})
}

func (s *Server) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := sessionToken(r)
		user, err := s.auth.GetSessionUser(r.Context(), token)
		if err != nil {
			writeAppError(w, errUnauthorized("未登录或会话已过期"))
			return
		}
		ctx := context.WithValue(r.Context(), userContextKey, user)
		next(w, r.WithContext(ctx))
	}
}

func (s *Server) setSessionCookie(w http.ResponseWriter, token string) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cookieSecure,
		Expires:  time.Now().Add(sessionTTL),
		MaxAge:   int(sessionTTL.Seconds()),
	})
}

func (s *Server) clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   s.cookieSecure,
		Expires:  time.Unix(0, 0),
		MaxAge:   -1,
	})
}

func sessionToken(r *http.Request) string {
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

func currentUser(ctx context.Context) (models.User, bool) {
	user, ok := ctx.Value(userContextKey).(models.User)
	return user, ok
}

func publicUser(user models.User) map[string]any {
	return map[string]any{
		"id":        user.ID,
		"username":  user.Username,
		"userSlug":  user.UserSlug,
		"createdAt": user.CreatedAt,
		"updatedAt": user.UpdatedAt,
	}
}

type appError struct {
	status  int
	code    string
	message string
	cause   error
}

func (e appError) Error() string {
	if e.cause != nil {
		return e.message + ": " + e.cause.Error()
	}
	return e.message
}

func errBadRequest(message string) error {
	return appError{status: http.StatusBadRequest, code: "bad_request", message: message}
}
func errUnauthorized(message string) error {
	return appError{status: http.StatusUnauthorized, code: "unauthorized", message: message}
}
func errForbidden(message string) error {
	return appError{status: http.StatusForbidden, code: "forbidden", message: message}
}
func errConflict(message string) error {
	return appError{status: http.StatusConflict, code: "conflict", message: message}
}
func errInternal(cause error) error {
	return appError{status: http.StatusInternalServerError, code: "internal_error", message: "服务内部错误", cause: cause}
}

func writeAppError(w http.ResponseWriter, err error) {
	var appErr appError
	if errors.As(err, &appErr) {
		httpx.Error(w, appErr.status, appErr.code, appErr.message)
		return
	}
	if errors.Is(err, db.ErrNotFound) {
		httpx.Error(w, http.StatusNotFound, "not_found", "资源不存在")
		return
	}
	httpx.Error(w, http.StatusInternalServerError, "internal_error", "服务内部错误")
}
