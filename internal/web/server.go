package web

import (
	"net/http"

	"simple_gateway_by_codex/internal/httpx"
)

// Server contains HTTP handlers for public, API, UI, and gateway routes.
type Server struct {
	publicBaseURL string
	cookieSecure  bool
	auth          authService
	mux           *http.ServeMux
}

// NewServer constructs a web server with currently implemented endpoints.
func NewServer(publicBaseURL string) *Server {
	s := &Server{
		publicBaseURL: publicBaseURL,
		mux:           http.NewServeMux(),
	}
	s.routes()
	return s
}

// NewServerWithServices constructs a server with application services.
func NewServerWithServices(publicBaseURL string, cookieSecure bool, auth authService) *Server {
	s := &Server{
		publicBaseURL: publicBaseURL,
		cookieSecure:  cookieSecure,
		auth:          auth,
		mux:           http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/public/usage", s.handleUsage)
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	if s.auth != nil {
		s.mux.HandleFunc("POST /api/register", s.handleRegister)
		s.mux.HandleFunc("POST /api/login", s.handleLogin)
		s.mux.HandleFunc("POST /api/logout", s.requireAuth(s.handleLogout))
		s.mux.HandleFunc("GET /api/me", s.requireAuth(s.handleMe))
		s.mux.HandleFunc("PUT /api/service-group-binding", s.requireAuth(s.handleRebindServiceGroup))
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, BuildUsageDocument(s.publicBaseURL))
}
