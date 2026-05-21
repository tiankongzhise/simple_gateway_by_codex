# 简单网关服务开发文档

## 1. 架构

服务使用 Go 开发，单进程同时提供管理后台 HTML 页面、管理 API、公共 usage API 和多租户反向代理入口。

主要模块：

- `cmd/gateway`：程序入口。
- `internal/config`：环境变量和 `.env` 加载。
- `internal/db`：PostgreSQL 连接、迁移和数据访问。
- `internal/cryptoutil`：RSA-OAEP-SHA256 授权码加密。
- `internal/authclient`：外部鉴权服务客户端。
- `internal/httpx`：HTTP 辅助函数。
- `internal/web`：管理后台、API、公共 usage。
- `internal/proxy`：路由匹配、访问模式校验和反向代理。

## 2. 配置

必填环境变量：

- `DATABASE_HOST`、`DATABASE_PORT`、`DATABASE_NAME`、`DATABASE_USER`、`DATABASE_PASSWORD`。
- `INVITE_CODE`。
- `SESSION_SECRET`。
- `AUTH_CODE_RSA_PRIVATE_KEY_FILE` 或 `AUTH_CODE_RSA_PRIVATE_KEY`。

可选环境变量：

- `SERVER_ADDR`：监听地址，默认 `:8080`。
- `DATABASE_SSLMODE`：PostgreSQL SSL 模式，默认 `disable`。
- `AUTH_SERVICE_BASE_URL`：鉴权服务地址，默认 `https://auth-service.baichengedu.com`。
- `COOKIE_SECURE`：是否仅通过 HTTPS 发送 Cookie，默认 `false`。
- `PUBLIC_BASE_URL`：对外访问基准地址，用于 usage 文档和签名链接生成。
- `DEFAULT_PROXY_TIMEOUT_SECONDS`：默认代理超时，默认 30。
- `MAX_PROXY_RETRIES`：最大允许重试次数，默认 3。
- `LOG_LEVEL`：JSON 日志等级，支持 `debug`、`info`、`warn`、`error`，默认 `info`。

签名链接使用 `SESSION_SECRET` 派生 HMAC-SHA256 签名密钥，不新增必填环境变量。

## 3. 数据库

使用内置 SQL migration 创建和升级表。新增迁移应独立文件命名为递增前缀，例如 `002_route_access_mode.sql`。

### 3.1 users

- `id`
- `username`
- `user_slug`
- `password_hash`
- `created_at`
- `updated_at`

### 3.2 sessions

- `id`
- `user_id`
- `token_hash`
- `expires_at`
- `created_at`

Cookie 中保存原始随机 token，数据库只保存 token hash。

### 3.3 service_group_bindings

- `id`
- `user_id`
- `service_group_name`
- `encrypted_authorization_code`
- `salt`
- `algorithm`
- `created_at`
- `updated_at`

一个用户最多一个当前绑定。

### 3.4 routes

- `id`
- `user_id`
- `name`
- `description`
- `enabled`
- `access_mode`：`public`、`caller_token` 或 `signed_link`。
- `match_type`
- `path_pattern`
- `methods`
- `upstream_url`
- `strip_prefix`
- `timeout_seconds`
- `retry_count`
- `priority`
- `auth_required`：兼容旧 API 和旧文档的派生字段。
- `auth_service_name`
- `created_at`
- `updated_at`

约束：

- `access_mode` 只能是 `public`、`caller_token`、`signed_link`。
- `access_mode != 'public'` 时 `auth_service_name` 必须非空。
- `access_mode = 'public'` 时应用层保存前应清空 `auth_service_name`。

历史迁移：

- 新增 `access_mode TEXT NOT NULL DEFAULT 'public'`。
- 将已有 `auth_required=true` 的路由更新为 `caller_token`。
- 保留 `auth_required` 字段以兼容旧代码路径和旧客户端，应用层读写时保持与 `access_mode != 'public'` 一致。

### 3.5 route_header_rules

- `id`
- `route_id`
- `phase`：`request` 或 `response`。
- `operation`：`set` 或 `remove`。
- `header_name`
- `header_value`
- `created_at`

### 3.6 schema_migrations

- `version`
- `applied_at`

## 4. 安全与加密

密码使用 `golang.org/x/crypto/bcrypt`。

授权码加密流程：

1. 解析 `.env` 中 RSA 私钥。
2. 从私钥推导公钥。
3. 每次保存授权码时生成 32 字节随机 salt。
4. 结构化明文为 `base64(salt) + ":" + authorizationCode`。
5. 使用 RSA-OAEP-SHA256 和公钥加密。
6. 数据库存储 base64 密文、base64 salt 和算法名 `RSA-OAEP-SHA256+salt-v1`。

解密仅在注册、换绑和保存受保护路由时调用鉴权服务所需的场景发生。运行时 `caller_token` 与 `signed_link` 不解密服务组授权码。

## 5. 鉴权服务客户端

鉴权服务基准地址来自 `AUTH_SERVICE_BASE_URL`。

### 5.1 获取服务组 token

请求：

```http
POST /api/service-groups/token/latest
Content-Type: application/json
```

请求体：

```json
{
  "serviceGroupName": "core-group",
  "authorizationCode": "32-character-code"
}
```

成功返回 access token。失败时注册、换绑或受保护路由保存均应中止。

### 5.2 校验 token

普通服务 token 校验：

- `Service-Name`：服务名。
- `Access-Token`：请求方 token。
- 可透传 `Origin`、`Referer`、`model`。

服务组校验目标服务：

- `Service-Name`：服务组名称。
- `Target-Service-Name`：目标服务名称。
- `Access-Token`：服务组 access token。

服务组校验只用于创建或更新 `caller_token`、`signed_link` 路由时确认目标服务已注册且当前服务组有管理权限，不用于运行时兜底放行。

## 6. 路由 API 与表单

`routeRequest` 新增 `accessMode` 字段，并继续接受旧字段 `authRequired`。

解析规则：

- `accessMode` 为空时，按旧字段兼容：`authRequired=true` 转为 `caller_token`，否则转为 `public`。
- `accessMode=public` 时清空 `authServiceName`，并设置模型中的 `AuthRequired=false`。
- `accessMode=caller_token` 或 `signed_link` 时要求 `authServiceName` 非空，并设置 `AuthRequired=true`。
- 非法 `accessMode` 返回 400。

管理后台路由表单使用 `<select name="accessMode">` 选择访问模式。旧的“是否需要鉴权”复选框移除或停止提交。

签名链接生成接口：

```http
POST /api/routes/{id}/signed-link
Content-Type: application/json
```

请求结构：

- `method`：HTTP 方法，默认 `GET`。
- `path`：相对网关路径，必须以 `/` 开头，默认使用路由 `path_pattern`。
- `query`：业务查询字符串，不包含 `gw_expires` 和 `gw_signature`。
- `expiresInSeconds`：有效期秒数，必须大于 0，默认 3600，最大 86400。

处理规则：

- 当前登录用户只能为自己的路由生成签名链接。
- 路由必须是 `signed_link` 模式。
- `method` 和 `path` 必须能命中该路由。
- 返回绝对 URL；如果 `PUBLIC_BASE_URL` 为空，则根据当前请求的 scheme/host 生成。

## 7. 签名链接算法

签名查询参数：

- `gw_expires`：Unix 秒级时间戳。
- `gw_signature`：base64url，无 padding。

签名输入使用换行拼接：

```text
METHOD
PATH
CANONICAL_QUERY
EXPIRES
```

规则：

- `METHOD` 使用大写 HTTP 方法。
- `PATH` 使用完整网关入口路径，例如 `/gw/alice/report/2026`。
- `CANONICAL_QUERY` 是移除 `gw_expires`、`gw_signature` 后的查询参数，使用 `url.Values.Encode()` 生成稳定顺序。
- `EXPIRES` 是 Unix 秒级字符串。
- 签名密钥为 `HMAC-SHA256(SESSION_SECRET, "simple-gateway:signed-link:v1")` 的结果。
- 校验使用 `hmac.Equal`，避免时序泄漏。

代理转发前必须从原始请求 query 中删除 `gw_expires` 和 `gw_signature`，上游只接收业务查询参数。

## 8. 路由匹配

入口格式：

```text
/gw/{userSlug}/{remainingPath}
```

匹配步骤：

1. 根据 `userSlug` 查询用户。
2. 查询该用户启用路由。
3. 过滤 HTTP 方法。
4. 精确匹配要求路径完全等于 `path_pattern`。
5. 前缀匹配要求路径等于前缀或以 `prefix/` 开头。
6. 排序：优先级高者优先；优先级相同则路径更长者优先；再按创建时间早者优先。

如果存在路径匹配但方法不匹配，返回 405。

## 9. 运行时访问模式

`public`：

- 不调用鉴权服务。
- 直接进入代理转发。

`caller_token`：

- 请求缺少 `Access-Token` 时返回 401。
- 请求携带 `Access-Token` 时调用 `/api/auth/verify`。
- `Service-Name` 优先使用请求头，缺省使用路由 `auth_service_name`。
- 透传 `Origin`、`Referer`、`model`。
- 鉴权失败时不访问上游。

`signed_link`：

- 请求必须携带合法且未过期的 `gw_expires` 和 `gw_signature`。
- 校验通过后删除签名查询参数并转发。
- 不调用外部鉴权服务。

运行时不再支持无 `Access-Token` 时使用绑定服务组 token 兜底访问上游。

## 10. 代理转发

代理行为：

- 保留业务查询参数。
- 保留请求体。
- 使用 `upstream_url` 拼接改写后的路径。
- `strip_prefix=true` 时去除命中的 `path_pattern`。
- 应用请求头规则后访问上游。
- 使用路由超时。
- 对网络错误和 502/503/504 做有限重试。
- 返回上游状态码、响应头和响应体。
- 应用响应头规则。
- 每个转发请求生成或复用 `X-Request-ID`，并写入客户端响应头、上游请求头和日志。

运行时日志：

- 使用 `log/slog` JSON 输出到 stdout。
- 事件包括 `proxy.request.start`、`proxy.route.lookup`、`proxy.route.matched`、`proxy.auth.*`、`proxy.signed_link.*`、`proxy.upstream.*`、`proxy.request.finish`、`proxy.request.failed`。
- 只记录安全元数据，例如 `request_id`、`route_id`、`target_url`、`status`、`attempt`、`duration_ms`。
- 不记录请求/响应 body，不输出 `Access-Token`、Cookie、授权码、签名值；日志里的 `target_url` 会移除 query。

## 11. 公共 usage 接口

`GET /api/public/usage` 返回固定 JSON 文档。该接口不读取用户私有配置。

返回结构包含：

- `serviceName`
- `version`
- `description`
- `conventions`
- `endpoints`
- `gateway`
- `errorCodes`

usage 文档应说明 `accessMode`、`caller_token` 请求头和 `signed_link` 查询参数。

## 12. 测试策略

单元测试：

- 配置校验。
- RSA 加密解密。
- 密码哈希。
- usage 文档结构。
- 路由匹配。
- 路径改写和头规则。
- 签名链接生成与校验，包括过期、路径篡改、方法篡改、查询参数篡改。

HTTP 测试：

- 使用 `httptest` 模拟鉴权服务。
- 使用 `httptest` 模拟上游服务。
- 覆盖注册、换绑、路由保存、`public` 转发、`caller_token` 转发、缺 token 拒绝、`signed_link` 成功转发、签名参数不转发到上游。

数据库测试：

- 优先使用真实 PostgreSQL，并通过 `DATABASE_HOST`、`DATABASE_PORT`、`DATABASE_NAME`、`DATABASE_USER`、`DATABASE_PASSWORD` 配置连接。
- 未配置测试数据库时跳过集成测试。

## 13. 提交规范

每完成一个功能点提交一次 commit。commit message 使用中文或带中文说明的 Conventional Commits，例如：

```text
feat: 增加路由访问模式
```

提交前至少运行：

```bash
go test ./...
```
