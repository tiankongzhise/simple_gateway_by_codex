package web

import (
	"net/http"

	"simple_gateway_by_codex/internal/httpx"
)

// Server contains HTTP handlers for public, API, UI, and gateway routes.
type Server struct {
	publicBaseURL string
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

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/public/usage", s.handleUsage)
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, BuildUsageDocument(s.publicBaseURL))
}
