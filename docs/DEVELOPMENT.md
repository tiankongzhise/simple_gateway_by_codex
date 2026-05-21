# 简单网关服务开发文档

## 1. 架构

服务使用 Go 开发，单进程同时提供：

- 管理后台 HTML 页面。
- 管理 API。
- 公共使用说明 API。
- 多租户反向代理入口。

主要模块：

- `cmd/gateway`：程序入口。
- `internal/config`：环境变量和 `.env` 加载。
- `internal/db`：PostgreSQL 连接、迁移和数据访问。
- `internal/crypto`：RSA-OAEP-SHA256 授权码加密。
- `internal/authclient`：外部鉴权服务客户端。
- `internal/httpx`：HTTP 辅助函数。
- `internal/web`：管理后台、API、公共 usage。
- `internal/proxy`：路由匹配、鉴权和反向代理。

## 2. 配置

必填环境变量：

- `DATABASE_HOST`：PostgreSQL 主机。
- `DATABASE_PORT`：PostgreSQL 端口。
- `DATABASE_NAME`：PostgreSQL 数据库名。
- `DATABASE_USER`：PostgreSQL 用户名。
- `DATABASE_PASSWORD`：PostgreSQL 密码。
- `INVITE_CODE`：共享注册邀请码。
- `SESSION_SECRET`：会话签名密钥。
- `AUTH_CODE_RSA_PRIVATE_KEY_FILE`：PEM RSA 私钥文件路径，相对路径按 `.env` 所在目录解析。
- `AUTH_CODE_RSA_PRIVATE_KEY`：PEM 格式 RSA 私钥内容，支持 `\n` 转义。

可选环境变量：

- `SERVER_ADDR`：监听地址，默认 `:8080`。
- `DATABASE_SSLMODE`：PostgreSQL SSL 模式，默认 `disable`。
- `AUTH_SERVICE_BASE_URL`：鉴权服务地址，默认 `https://auth-service.baichengedu.com`。
- `COOKIE_SECURE`：是否仅通过 HTTPS 发送 Cookie，默认 `false`。
- `PUBLIC_BASE_URL`：对外访问基准地址，用于 usage 文档。
- `DEFAULT_PROXY_TIMEOUT_SECONDS`：默认代理超时，默认 30。
- `MAX_PROXY_RETRIES`：最大允许重试次数，默认 3。

## 3. 数据库

使用内置 SQL migration 创建表。

### 3.1 users

- `id`
- `username`
- `user_slug`
- `password_hash`
- `created_at`
- `updated_at`

`username` 和 `user_slug` 均唯一。

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
- `match_type`
- `path_pattern`
- `methods`
- `upstream_url`
- `strip_prefix`
- `timeout_seconds`
- `retry_count`
- `priority`
- `auth_required`
- `auth_service_name`
- `created_at`
- `updated_at`

`methods` 以字符串数组保存，包含 `ALL` 时表示允许所有方法。

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

密码使用 Argon2id 或 bcrypt 哈希。考虑 Go 标准生态依赖稳定性，v1 使用 `golang.org/x/crypto/bcrypt`。

授权码加密流程：

1. 解析 `.env` 中 RSA 私钥。
2. 从私钥推导公钥。
3. 每次保存授权码时生成 32 字节随机 salt。
4. 结构化明文为 `base64(salt) + ":" + authorizationCode`。
5. 使用 RSA-OAEP-SHA256 和公钥加密。
6. 数据库存储 base64 密文、base64 salt 和算法名 `RSA-OAEP-SHA256+salt-v1`。

解密仅在需要调用鉴权服务时进行。

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

成功返回 access token。失败时注册、换绑或鉴权路由保存均应中止。

### 5.2 校验 token

普通服务 token 校验：

- `Service-Name`：服务名。
- `Access-Token`：请求方 token。
- 可透传 `Origin`、`Referer`、`model`。

服务组校验目标服务：

- `Service-Name`：服务组名称。
- `Target-Service-Name`：目标服务名称。
- `Access-Token`：服务组 access token。

服务组校验用于：

- 创建或更新鉴权路由时确认目标服务已注册且当前服务组有管理权限。
- 运行时请求未携带 `Access-Token` 时的兜底鉴权。

## 6. 路由匹配

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

## 7. 运行时鉴权

路由未开启鉴权时直接转发。

路由开启鉴权时：

- 请求携带 `Access-Token`：
  - token 使用请求头值。
  - `Service-Name` 优先使用请求头；缺省使用路由 `auth_service_name`。
  - 调用 `/api/auth/verify`。
- 请求未携带 `Access-Token`：
  - 解密当前用户绑定的永久授权码。
  - 调用 `/api/service-groups/token/latest` 获取服务组 token。
  - `Target-Service-Name` 优先使用请求头 `Service-Name`，缺省使用路由 `auth_service_name`。
  - 调用 `/api/auth/verify` 做服务组目标服务校验。

失败时不访问上游。

## 8. 代理转发

代理行为：

- 保留原查询参数。
- 保留请求体。
- 使用 `upstream_url` 拼接改写后的路径。
- `strip_prefix=true` 时去除命中的 `path_pattern`。
- 应用请求头规则后访问上游。
- 使用路由超时。
- 对网络错误和 502/503/504 做有限重试。
- 返回上游状态码、响应头和响应体。
- 应用响应头规则。

## 9. 公共 usage 接口

`GET /api/public/usage` 返回固定 JSON 文档。该接口不读取用户私有配置。

返回结构包含：

- `serviceName`
- `version`
- `description`
- `conventions`
- `endpoints`
- `gateway`
- `errorCodes`

## 10. 测试策略

单元测试：

- 配置校验。
- RSA 加密解密。
- 密码哈希。
- usage 文档结构。
- 路由匹配。
- 路径改写和头规则。

HTTP 测试：

- 使用 `httptest` 模拟鉴权服务。
- 使用 `httptest` 模拟上游服务。
- 覆盖注册、换绑、路由保存、鉴权转发、非鉴权转发。

数据库测试：

- 优先使用真实 PostgreSQL，并通过 `DATABASE_HOST`、`DATABASE_PORT`、`DATABASE_NAME`、`DATABASE_USER`、`DATABASE_PASSWORD` 配置连接。
- 未配置测试数据库时跳过集成测试。

## 11. 提交规范

每完成一个功能点提交一次 commit。commit message 使用中文或带中文说明的 Conventional Commits，例如：

```text
feat: 增加公共使用说明接口
```

提交前至少运行：

```bash
go test ./...
```
