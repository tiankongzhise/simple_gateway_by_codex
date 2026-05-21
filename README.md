# simple_gateway_by_codex

一个使用 Go + PostgreSQL 开发的多用户 HTTP 网关服务。用户可以注册、绑定鉴权服务组、配置自己的路由规则，并通过 `/gw/{userSlug}/...` 对外提供代理入口。不同用户之间的路由配置按用户隔离，互相不可见，也不会在运行时跨用户匹配。

## 核心能力

- 邀请码注册，邀请码来自 `.env` 的 `INVITE_CODE`。
- 注册时必须绑定鉴权服务组名称和永久授权码。
- 绑定信息通过 `https://auth-service.baichengedu.com` 校验。
- 永久授权码使用 RSA-OAEP-SHA256 加随机 salt 加密存储。
- 管理后台支持登录、注册、服务组换绑、路由新增/编辑/删除。
- 路由支持前缀/精确匹配、方法限制、上游地址、去前缀、超时、重试、优先级、启停、请求/响应头规则。
- 路由可选择鉴权或不鉴权。
- 鉴权路由创建时会验证绑定服务组是否有权限管理目标鉴权服务。
- 公共接口 `GET /api/public/usage` 会返回本服务对外接口说明。

## 环境要求

- Go 1.26 或更新版本。
- PostgreSQL 13 或更新版本。

## PostgreSQL 准备

示例：

```sql
CREATE DATABASE simple_gateway;
CREATE USER simple_gateway WITH PASSWORD 'change-me';
GRANT ALL PRIVILEGES ON DATABASE simple_gateway TO simple_gateway;
```

服务启动时会自动执行内置 migration，创建 `users`、`sessions`、`service_group_bindings`、`routes`、`route_header_rules`、`schema_migrations` 等表。

## 生成 RSA 私钥

可以使用 OpenSSL 生成 RSA 私钥：

```bash
openssl genrsa -out auth_code_private.pem 2048
```

`.env` 中可以直接放 PEM 内容，也可以把换行写成 `\n`。

## 配置 `.env`

在项目根目录创建 `.env`：

```env
SERVER_ADDR=:8080
DATABASE_URL=postgres://simple_gateway:change-me@localhost:5432/simple_gateway?sslmode=disable
INVITE_CODE=replace-with-your-invite-code
SESSION_SECRET=replace-with-a-long-random-secret
AUTH_SERVICE_BASE_URL=https://auth-service.baichengedu.com
AUTH_CODE_RSA_PRIVATE_KEY="-----BEGIN RSA PRIVATE KEY-----\n...\n-----END RSA PRIVATE KEY-----"
COOKIE_SECURE=false
PUBLIC_BASE_URL=http://localhost:8080
DEFAULT_PROXY_TIMEOUT_SECONDS=30
MAX_PROXY_RETRIES=3
```

必填项：

- `DATABASE_URL`
- `INVITE_CODE`
- `SESSION_SECRET`
- `AUTH_CODE_RSA_PRIVATE_KEY`

## 启动服务

```bash
go run ./cmd/gateway
```

默认监听 `http://localhost:8080`。

健康检查：

```bash
curl http://localhost:8080/healthz
```

公共使用说明：

```bash
curl http://localhost:8080/api/public/usage
```

## 使用后台

打开：

```text
http://localhost:8080/register
```

注册时填写：

- 用户名。
- 用户 slug，例如 `alice`。公网入口会是 `/gw/alice/...`。
- 密码。
- 邀请码。
- 鉴权服务组名称。
- 服务组永久授权码。

注册成功后进入路由后台：

```text
http://localhost:8080/routes
```

服务组换绑页面：

```text
http://localhost:8080/binding
```

## 创建路由

路由字段包括：

- 名称和描述。
- 匹配方式：`prefix` 或 `exact`。
- 外部路径，例如 `/api`。
- HTTP 方法，例如 `ALL` 或 `GET,POST`。
- 上游基础地址，例如 `https://example.com`。
- 是否去除匹配前缀。
- 超时秒数、重试次数、优先级。
- 是否需要鉴权。
- 鉴权服务名称。
- 请求头/响应头规则。

头规则格式：

```text
set X-Gateway=simple
remove Server
```

如果选择“需要鉴权”，必须填写鉴权服务名称。保存时网关会：

1. 使用当前用户绑定的服务组永久授权码调用 `/api/service-groups/token/latest` 获取服务组 token。
2. 调用鉴权服务 `/api/auth/verify`。
3. 使用 `Service-Name: {serviceGroupName}`、`Target-Service-Name: {authServiceName}`、`Access-Token: {groupToken}` 校验该服务是否已注册且归当前服务组管理。
4. 校验失败则拒绝保存路由。

## 调用网关

公网入口：

```text
/gw/{userSlug}/{path}
```

例如用户 slug 为 `alice`，路由外部路径为 `/api`：

```bash
curl http://localhost:8080/gw/alice/api/users
```

非鉴权路由会直接转发。

鉴权路由转发前会调用鉴权服务：

- 如果请求携带 `Access-Token`，网关优先使用该 token。
- 如果请求携带 `Service-Name`，网关会透传该服务名。
- 如果请求未携带 `Service-Name`，网关使用路由配置中的鉴权服务名称兜底。
- 如果请求没有 `Access-Token`，网关使用当前用户绑定的服务组 token 校验目标服务。

示例：

```bash
curl \
  -H "Access-Token: caller-access-token" \
  -H "Service-Name: target-service" \
  -H "Origin: https://caller.example.com" \
  http://localhost:8080/gw/alice/api/users
```

未携带 `Access-Token` 时：

```bash
curl http://localhost:8080/gw/alice/api/users
```

此时网关会用绑定服务组的 token 兜底校验，目标服务名优先取请求头 `Service-Name`，否则取路由配置的鉴权服务名称。

## 管理 API

主要 API 可通过 `GET /api/public/usage` 查看完整说明。常用接口：

- `POST /api/register`
- `POST /api/login`
- `POST /api/logout`
- `GET /api/me`
- `PUT /api/service-group-binding`
- `GET /api/routes`
- `POST /api/routes`
- `PUT /api/routes/{id}`
- `DELETE /api/routes/{id}`

管理 API 登录后通过 `gateway_session` HttpOnly Cookie 鉴权。

## 常见错误

- `400`：请求字段无效，例如鉴权路由缺少鉴权服务名称。
- `401`：未登录、会话过期、授权码或 token 无效。
- `403`：邀请码错误，或绑定服务组无权限管理目标服务。
- `404`：用户、路由、服务组或目标服务不存在。
- `405`：路由路径匹配但 HTTP 方法不允许。
- `409`：用户名或用户 slug 重复。
- `429`：鉴权服务限流。
- `502`：鉴权服务或上游服务异常。
- `504`：上游请求超时。

## 测试

建议使用工作区内缓存，避免 Windows 用户目录权限问题：

```powershell
New-Item -ItemType Directory -Force -Path .gocache | Out-Null
New-Item -ItemType Directory -Force -Path .gomodcache | Out-Null
$env:GOCACHE=(Resolve-Path .gocache).Path
$env:GOMODCACHE=(Resolve-Path .gomodcache).Path
$env:GOMAXPROCS='1'
go test -p 1 ./...
```

`GOMAXPROCS=1` 是为了规避部分 Windows 环境下 Go 编译器并发编译崩溃。
