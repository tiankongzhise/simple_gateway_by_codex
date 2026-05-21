package web

import (
	"html/template"
	"net/http"
)

var pageTemplates = template.Must(template.New("pages").Parse(`
{{define "header"}}
<!doctype html>
<html lang="zh-CN">
<head>
	<meta charset="utf-8">
	<meta name="viewport" content="width=device-width, initial-scale=1">
	<title>{{.Title}} - Simple Gateway</title>
	<style>
		body{font-family:system-ui,-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;margin:0;background:#f7f7f5;color:#1e293b}
		header{background:#0f766e;color:#fff;padding:14px 24px;display:flex;justify-content:space-between;align-items:center}
		header a{color:#fff;text-decoration:none;margin-left:14px}
		main{max-width:1120px;margin:24px auto;padding:0 18px}
		.panel{background:#fff;border:1px solid #d9e2df;border-radius:8px;padding:20px;margin-bottom:18px}
		label{display:block;font-weight:650;margin:12px 0 6px}
		input,select,textarea{box-sizing:border-box;width:100%;padding:10px;border:1px solid #b8c4c0;border-radius:6px;font:inherit}
		textarea{min-height:80px}
		button,.button{display:inline-flex;align-items:center;gap:6px;background:#0f766e;color:#fff;border:0;border-radius:6px;padding:10px 14px;text-decoration:none;font:inherit;cursor:pointer}
		.button.secondary,button.secondary{background:#475569}
		.button.danger,button.danger{background:#b91c1c}
		.actions{display:flex;gap:10px;align-items:center;flex-wrap:wrap;margin-top:16px}
		.error{background:#fee2e2;color:#991b1b;border:1px solid #fecaca;padding:10px;border-radius:6px}
		.grid{display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:12px}
		table{width:100%;border-collapse:collapse;background:#fff}
		th,td{text-align:left;border-bottom:1px solid #e2e8f0;padding:10px;vertical-align:top}
		th{font-size:13px;color:#475569;background:#f1f5f9}
		code{background:#e2e8f0;border-radius:4px;padding:2px 5px}
		.muted{color:#64748b;font-size:14px}
	</style>
</head>
<body>
<header>
	<strong>Simple Gateway</strong>
	<nav>
		<a href="/routes">路由</a>
		<a href="/binding">服务组</a>
		<a href="/api/public/usage">Usage</a>
	</nav>
</header>
<main>
{{end}}

{{define "footer"}}
</main>
</body>
</html>
{{end}}

{{define "login"}}
{{template "header" .}}
<section class="panel">
	<h1>登录</h1>
	{{if .Error}}<div class="error">{{.Error}}</div>{{end}}
	<form method="post" action="/login">
		<label>用户名</label><input name="username" required>
		<label>密码</label><input name="password" type="password" required>
		<div class="actions"><button type="submit">登录</button><a class="button secondary" href="/register">注册</a></div>
	</form>
</section>
{{template "footer" .}}
{{end}}

{{define "register"}}
{{template "header" .}}
<section class="panel">
	<h1>注册</h1>
	{{if .Error}}<div class="error">{{.Error}}</div>{{end}}
	<form method="post" action="/register">
		<div class="grid">
			<div><label>用户名</label><input name="username" required maxlength="80"></div>
			<div><label>用户 slug</label><input name="userSlug" required pattern="[A-Za-z0-9][A-Za-z0-9_-]{2,62}"></div>
		</div>
		<label>密码</label><input name="password" type="password" required minlength="8">
		<label>邀请码</label><input name="inviteCode" required>
		<div class="grid">
			<div><label>鉴权服务组名称</label><input name="serviceGroupName" required></div>
			<div><label>服务组永久授权码</label><input name="authorizationCode" required></div>
		</div>
		<div class="actions"><button type="submit">注册并绑定</button><a class="button secondary" href="/login">返回登录</a></div>
	</form>
</section>
{{template "footer" .}}
{{end}}

{{define "routes"}}
{{template "header" .}}
<section class="panel">
	<div style="display:flex;justify-content:space-between;gap:12px;align-items:center">
		<div><h1>路由规则</h1><p class="muted">公网入口为 <code>/gw/{{.User.UserSlug}}/...</code></p></div>
		<a class="button" href="/routes/new">新增路由</a>
	</div>
	{{if .Error}}<div class="error">{{.Error}}</div>{{end}}
	<table>
		<thead><tr><th>名称</th><th>匹配</th><th>上游</th><th>鉴权</th><th>状态</th><th>操作</th></tr></thead>
		<tbody>
		{{range .Routes}}
		<tr>
			<td><strong>{{.Name}}</strong><div class="muted">{{.Description}}</div></td>
			<td>{{.MatchType}} <code>{{.PathPattern}}</code><div class="muted">{{range .Methods}}{{.}} {{end}}</div></td>
			<td><code>{{.UpstreamURL}}</code></td>
			<td>{{.AccessMode}}{{if .AuthServiceName}}<div class="muted">{{.AuthServiceName}}</div>{{end}}</td>
			<td>{{if .Enabled}}启用{{else}}停用{{end}}</td>
			<td class="actions"><a class="button secondary" href="/routes/{{.ID}}/edit">编辑</a><form method="post" action="/routes/{{.ID}}/delete"><button class="danger" type="submit">删除</button></form></td>
		</tr>
		{{else}}
		<tr><td colspan="6" class="muted">暂无路由</td></tr>
		{{end}}
		</tbody>
	</table>
</section>
{{template "footer" .}}
{{end}}

{{define "route_form"}}
{{template "header" .}}
<section class="panel">
	<h1>{{.Heading}}</h1>
	{{if .Error}}<div class="error">{{.Error}}</div>{{end}}
	<form method="post" action="{{.Action}}">
		<label>名称</label><input name="name" value="{{.Route.Name}}" required>
		<label>描述</label><textarea name="description">{{.Route.Description}}</textarea>
		<div class="grid">
			<div><label>匹配方式</label><select name="matchType"><option value="prefix" {{if eq .Route.MatchType "prefix"}}selected{{end}}>前缀</option><option value="exact" {{if eq .Route.MatchType "exact"}}selected{{end}}>精确</option></select></div>
			<div><label>外部路径</label><input name="pathPattern" value="{{.Route.PathPattern}}" required></div>
		</div>
		<label>HTTP 方法</label><input name="methods" value="{{.MethodsText}}" placeholder="ALL 或 GET,POST">
		<label>上游基础地址</label><input name="upstreamUrl" value="{{.Route.UpstreamURL}}" required>
		<div class="grid">
			<div><label>超时秒数</label><input name="timeoutSeconds" type="number" min="1" value="{{.Route.TimeoutSeconds}}"></div>
			<div><label>重试次数</label><input name="retryCount" type="number" min="0" value="{{.Route.RetryCount}}"></div>
			<div><label>优先级</label><input name="priority" type="number" value="{{.Route.Priority}}"></div>
		</div>
		<label><input style="width:auto" type="checkbox" name="enabled" value="true" {{if .Route.Enabled}}checked{{end}}> 启用</label>
		<label><input style="width:auto" type="checkbox" name="stripPrefix" value="true" {{if .Route.StripPrefix}}checked{{end}}> 转发时去除匹配前缀</label>
		<label>访问模式</label>
		<select name="accessMode">
			<option value="public" {{if eq .Route.AccessMode "public"}}selected{{end}}>公开访问</option>
			<option value="caller_token" {{if eq .Route.AccessMode "caller_token"}}selected{{end}}>调用方 Token</option>
			<option value="signed_link" {{if eq .Route.AccessMode "signed_link"}}selected{{end}}>短期签名链接</option>
		</select>
		<label>鉴权服务名称</label><input name="authServiceName" value="{{.Route.AuthServiceName}}">
		<label>请求头规则</label><textarea name="requestHeaders" placeholder="set X-Name=value&#10;remove X-Secret">{{.RequestHeadersText}}</textarea>
		<label>响应头规则</label><textarea name="responseHeaders" placeholder="set X-Gateway=simple&#10;remove Server">{{.ResponseHeadersText}}</textarea>
		<div class="actions"><button type="submit">保存</button><a class="button secondary" href="/routes">返回</a></div>
	</form>
</section>
{{template "footer" .}}
{{end}}

{{define "binding"}}
{{template "header" .}}
<section class="panel">
	<h1>服务组绑定</h1>
	{{if .Error}}<div class="error">{{.Error}}</div>{{end}}
	{{if .Binding}}<p>当前服务组：<strong>{{.Binding.ServiceGroupName}}</strong></p>{{end}}
	<form method="post" action="/binding">
		<label>服务组名称</label><input name="serviceGroupName" required>
		<label>永久授权码</label><input name="authorizationCode" required>
		<div class="actions"><button type="submit">换绑</button><a class="button secondary" href="/routes">返回路由</a></div>
	</form>
</section>
{{template "footer" .}}
{{end}}
`))

func renderTemplate(w http.ResponseWriter, name string, data any) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := pageTemplates.ExecuteTemplate(w, name, data); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
	}
}
