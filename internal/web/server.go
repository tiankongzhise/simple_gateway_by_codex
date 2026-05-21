package web

import (
	"net/http"

	"simple_gateway_by_codex/internal/authclient"
	"simple_gateway_by_codex/internal/httpx"
	"simple_gateway_by_codex/internal/proxy"
)

// Server contains HTTP handlers for public, API, UI, and gateway routes.
type Server struct {
	publicBaseURL  string
	cookieSecure   bool
	auth           authService
	routesStore    routeStore
	authVerifier   bindingVerifier
	authCodeCipher authCodeCipher
	gateway        http.Handler
	mux            *http.ServeMux
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

// NewServerWithDependencies constructs a server with all implemented services.
func NewServerWithDependencies(publicBaseURL string, cookieSecure bool, auth authService, routes routeStore, verifier bindingVerifier, cipher authCodeCipher, authClient *authclient.Client) *Server {
	gateway := proxy.NewHandler(routes)
	if authClient != nil && cipher != nil {
		gateway.WithAuth(newProxyAuthAdapter(authClient), cipher)
	}
	s := &Server{
		publicBaseURL:  publicBaseURL,
		cookieSecure:   cookieSecure,
		auth:           auth,
		routesStore:    routes,
		authVerifier:   verifier,
		authCodeCipher: cipher,
		gateway:        gateway,
		mux:            http.NewServeMux(),
	}
	s.routes()
	return s
}

func (s *Server) routes() {
	s.mux.HandleFunc("GET /api/public/usage", s.handleUsage)
	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		httpx.WriteJSON(w, http.StatusOK, map[string]bool{"ok": true})
	})
	if s.gateway != nil {
		s.mux.Handle("/gw/", s.gateway)
	}
	if s.auth != nil {
		s.mux.HandleFunc("GET /{$}", s.handleIndex)
		s.mux.HandleFunc("GET /login", s.handleLoginPage)
		s.mux.HandleFunc("POST /login", s.handleLoginForm)
		s.mux.HandleFunc("GET /register", s.handleRegisterPage)
		s.mux.HandleFunc("POST /register", s.handleRegisterForm)
		s.mux.HandleFunc("POST /api/register", s.handleRegister)
		s.mux.HandleFunc("POST /api/login", s.handleLogin)
		s.mux.HandleFunc("POST /api/logout", s.requireAuth(s.handleLogout))
		s.mux.HandleFunc("GET /api/me", s.requireAuth(s.handleMe))
		s.mux.HandleFunc("PUT /api/service-group-binding", s.requireAuth(s.handleRebindServiceGroup))
	}
	if s.auth != nil && s.routesStore != nil {
		s.mux.HandleFunc("GET /routes", s.requireAuth(s.handleRoutesPage))
		s.mux.HandleFunc("GET /routes/new", s.requireAuth(s.handleNewRoutePage))
		s.mux.HandleFunc("POST /routes/new", s.requireAuth(s.handleCreateRouteForm))
		s.mux.HandleFunc("GET /routes/{id}/edit", s.requireAuth(s.handleEditRoutePage))
		s.mux.HandleFunc("POST /routes/{id}/edit", s.requireAuth(s.handleUpdateRouteForm))
		s.mux.HandleFunc("POST /routes/{id}/delete", s.requireAuth(s.handleDeleteRouteForm))
		s.mux.HandleFunc("GET /binding", s.requireAuth(s.handleBindingPage))
		s.mux.HandleFunc("POST /binding", s.requireAuth(s.handleBindingForm))
		s.mux.HandleFunc("GET /api/routes", s.requireAuth(s.handleListRoutes))
		s.mux.HandleFunc("POST /api/routes", s.requireAuth(s.handleCreateRoute))
		s.mux.HandleFunc("PUT /api/routes/{id}", s.requireAuth(s.handleUpdateRoute))
		s.mux.HandleFunc("DELETE /api/routes/{id}", s.requireAuth(s.handleDeleteRoute))
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mux.ServeHTTP(w, r)
}

func (s *Server) handleUsage(w http.ResponseWriter, r *http.Request) {
	httpx.WriteJSON(w, http.StatusOK, BuildUsageDocument(s.publicBaseURL))
}
