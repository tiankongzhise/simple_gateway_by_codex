package web

import (
	"net/http"
	"strconv"
	"strings"

	"simple_gateway_by_codex/internal/models"
)

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	http.Redirect(w, r, "/routes", http.StatusFound)
}

func (s *Server) handleLoginPage(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, "login", map[string]any{"Title": "登录"})
}

func (s *Server) handleLoginForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		renderTemplate(w, "login", map[string]any{"Title": "登录", "Error": "表单无效"})
		return
	}
	user, token, err := s.auth.Login(r.Context(), r.FormValue("username"), r.FormValue("password"))
	if err != nil {
		_ = user
		renderTemplate(w, "login", map[string]any{"Title": "登录", "Error": "用户名或密码不正确"})
		return
	}
	s.setSessionCookie(w, token)
	http.Redirect(w, r, "/routes", http.StatusFound)
}

func (s *Server) handleRegisterPage(w http.ResponseWriter, r *http.Request) {
	renderTemplate(w, "register", map[string]any{"Title": "注册"})
}

func (s *Server) handleRegisterForm(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		renderTemplate(w, "register", map[string]any{"Title": "注册", "Error": "表单无效"})
		return
	}
	_, token, err := s.auth.Register(r.Context(), RegisterRequest{
		Username:          r.FormValue("username"),
		UserSlug:          r.FormValue("userSlug"),
		Password:          r.FormValue("password"),
		InviteCode:        r.FormValue("inviteCode"),
		ServiceGroupName:  r.FormValue("serviceGroupName"),
		AuthorizationCode: r.FormValue("authorizationCode"),
	})
	if err != nil {
		renderTemplate(w, "register", map[string]any{"Title": "注册", "Error": userFacingError(err)})
		return
	}
	s.setSessionCookie(w, token)
	http.Redirect(w, r, "/routes", http.StatusFound)
}

func (s *Server) handleRoutesPage(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	routes, err := s.routesStore.ListRoutes(r.Context(), user.ID)
	if err != nil {
		renderTemplate(w, "routes", map[string]any{"Title": "路由", "User": user, "Error": userFacingError(err)})
		return
	}
	renderTemplate(w, "routes", map[string]any{"Title": "路由", "User": user, "Routes": routes})
}

func (s *Server) handleNewRoutePage(w http.ResponseWriter, r *http.Request) {
	renderRouteForm(w, "新增路由", "/routes/new", models.Route{Enabled: true, MatchType: "prefix", PathPattern: "/", Methods: []string{"ALL"}, TimeoutSeconds: 30})
}

func (s *Server) handleCreateRouteForm(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	req, err := routeRequestFromForm(r)
	if err != nil {
		renderRouteFormWithError(w, "新增路由", "/routes/new", models.Route{Enabled: true, MatchType: "prefix", TimeoutSeconds: 30}, err.Error())
		return
	}
	route, err := req.toModel(user.ID, 0)
	if err == nil {
		err = s.validateRouteAuthService(r.Context(), user.ID, route)
	}
	if err == nil {
		_, err = s.routesStore.CreateRoute(r.Context(), route)
	}
	if err != nil {
		renderRouteFormWithError(w, "新增路由", "/routes/new", route, userFacingError(err))
		return
	}
	http.Redirect(w, r, "/routes", http.StatusFound)
}

func (s *Server) handleEditRoutePage(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	id, err := parseRouteID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	route, err := s.routesStore.GetRoute(r.Context(), user.ID, id)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	renderRouteForm(w, "编辑路由", "/routes/"+strconv.FormatInt(id, 10)+"/edit", route)
}

func (s *Server) handleUpdateRouteForm(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	id, err := parseRouteID(r)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	req, err := routeRequestFromForm(r)
	if err != nil {
		renderRouteFormWithError(w, "编辑路由", "/routes/"+strconv.FormatInt(id, 10)+"/edit", models.Route{ID: id}, err.Error())
		return
	}
	route, err := req.toModel(user.ID, id)
	if err == nil {
		err = s.validateRouteAuthService(r.Context(), user.ID, route)
	}
	if err == nil {
		_, err = s.routesStore.UpdateRoute(r.Context(), route)
	}
	if err != nil {
		renderRouteFormWithError(w, "编辑路由", "/routes/"+strconv.FormatInt(id, 10)+"/edit", route, userFacingError(err))
		return
	}
	http.Redirect(w, r, "/routes", http.StatusFound)
}

func (s *Server) handleDeleteRouteForm(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	id, err := parseRouteID(r)
	if err == nil {
		err = s.routesStore.DeleteRoute(r.Context(), user.ID, id)
	}
	if err != nil {
		http.Redirect(w, r, "/routes", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/routes", http.StatusFound)
}

func (s *Server) handleBindingPage(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	binding, _ := s.auth.GetServiceGroupBinding(r.Context(), user.ID)
	renderTemplate(w, "binding", map[string]any{"Title": "服务组绑定", "Binding": binding})
}

func (s *Server) handleBindingForm(w http.ResponseWriter, r *http.Request) {
	user, _ := currentUser(r.Context())
	if err := r.ParseForm(); err != nil {
		renderTemplate(w, "binding", map[string]any{"Title": "服务组绑定", "Error": "表单无效"})
		return
	}
	binding, err := s.auth.RebindServiceGroup(r.Context(), user.ID, r.FormValue("serviceGroupName"), r.FormValue("authorizationCode"))
	if err != nil {
		renderTemplate(w, "binding", map[string]any{"Title": "服务组绑定", "Error": userFacingError(err)})
		return
	}
	renderTemplate(w, "binding", map[string]any{"Title": "服务组绑定", "Binding": binding})
}

func routeRequestFromForm(r *http.Request) (routeRequest, error) {
	if err := r.ParseForm(); err != nil {
		return routeRequest{}, err
	}
	timeout, _ := strconv.Atoi(r.FormValue("timeoutSeconds"))
	retry, _ := strconv.Atoi(r.FormValue("retryCount"))
	priority, _ := strconv.Atoi(r.FormValue("priority"))
	return routeRequest{
		Name:                r.FormValue("name"),
		Description:         r.FormValue("description"),
		Enabled:             r.FormValue("enabled") == "true",
		MatchType:           r.FormValue("matchType"),
		PathPattern:         r.FormValue("pathPattern"),
		Methods:             splitCSV(r.FormValue("methods")),
		UpstreamURL:         r.FormValue("upstreamUrl"),
		StripPrefix:         r.FormValue("stripPrefix") == "true",
		TimeoutSeconds:      timeout,
		RetryCount:          retry,
		Priority:            priority,
		AuthRequired:        r.FormValue("authRequired") == "true",
		AuthServiceName:     r.FormValue("authServiceName"),
		RequestHeaderRules:  parseHeaderRuleText(r.FormValue("requestHeaders")),
		ResponseHeaderRules: parseHeaderRuleText(r.FormValue("responseHeaders")),
	}, nil
}

func renderRouteForm(w http.ResponseWriter, heading, action string, route models.Route) {
	renderRouteFormWithError(w, heading, action, route, "")
}

func renderRouteFormWithError(w http.ResponseWriter, heading, action string, route models.Route, message string) {
	renderTemplate(w, "route_form", map[string]any{
		"Title":               heading,
		"Heading":             heading,
		"Action":              action,
		"Route":               route,
		"MethodsText":         strings.Join(route.Methods, ","),
		"RequestHeadersText":  headerRulesText(route.RequestHeaders),
		"ResponseHeadersText": headerRulesText(route.ResponseHeaders),
		"Error":               message,
	})
}

func splitCSV(value string) []string {
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == '\t' })
	if len(parts) == 0 {
		return []string{"ALL"}
	}
	return parts
}

func parseHeaderRuleText(value string) []headerRuleRequest {
	lines := strings.Split(value, "\n")
	rules := make([]headerRuleRequest, 0, len(lines))
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(strings.ToLower(line), "remove ") {
			rules = append(rules, headerRuleRequest{Operation: "remove", HeaderName: strings.TrimSpace(line[len("remove "):])})
			continue
		}
		if strings.HasPrefix(strings.ToLower(line), "set ") {
			rest := strings.TrimSpace(line[len("set "):])
			name, value, _ := strings.Cut(rest, "=")
			rules = append(rules, headerRuleRequest{Operation: "set", HeaderName: strings.TrimSpace(name), HeaderValue: value})
		}
	}
	return rules
}

func headerRulesText(rules []models.HeaderRule) string {
	lines := make([]string, 0, len(rules))
	for _, rule := range rules {
		if rule.Operation == "remove" {
			lines = append(lines, "remove "+rule.HeaderName)
		} else {
			lines = append(lines, "set "+rule.HeaderName+"="+rule.HeaderValue)
		}
	}
	return strings.Join(lines, "\n")
}

func userFacingError(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
