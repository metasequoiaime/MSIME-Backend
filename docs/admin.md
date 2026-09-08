# 管理后台

`admin-web/` 与官网 MSIME-Web 使用相同核心技术栈：React 19、TypeScript、Vite 8、Sass、TanStack Router / Query、Zod、pnpm 10.15 和 Biome。Router 负责页面路由，Query 负责请求缓存与变更刷新，Zod 校验 API 响应。Vite 生成 `dist/`，`embed.go` 将产物嵌入 Go 二进制；Docker 在 Node 构建阶段重新构建前端，再编译进 Go 镜像。与现有 HTTP 服务共用端口，不需要单独启动 Node、前端容器或静态文件服务器。

## 启用与部署

1. 配置现有 PostgreSQL 用户体系（`auth.enabled: true`），设置 `MSIME_DATABASE_URL` 与 `MSIME_AUTH_PEPPER`。
2. 在 Google Cloud 项目中创建 Web OAuth 客户端，授权重定向 URI 设为 `https://admin.msime.app/api/auth/google/callback`；只使用 `openid email` 登录范围。将 Client Secret 保存到部署环境的 `MSIME_ADMIN_GOOGLE_SECRET`，将管理员邮箱白名单保存到 `MSIME_ADMIN_GOOGLE_EMAILS`（逗号分隔）。
3. 在原有配置中加入：

   ```json
   "admin": {
     "enabled": true,
     "host": "admin.msime.app",
     "token_env": "MSIME_ADMIN_TOKEN",
     "google": {
       "client_id": "YOUR_WEB_CLIENT_ID.apps.googleusercontent.com",
       "secret_env": "MSIME_ADMIN_GOOGLE_SECRET",
       "redirect_uri": "https://admin.msime.app/api/auth/google/callback",
       "allowed_emails_env": "MSIME_ADMIN_GOOGLE_EMAILS"
     }
   }
   ```

4. Google 登录模式不需要设置 `MSIME_ADMIN_TOKEN`。如果保留它，界面会额外提供管理员密钥登录作为兼容入口；不配置 Google 时仍需要至少 32 字节的独立随机管理员密钥。不要将任何密钥放进前端源码、安装包或版本库。
5. 使用新二进制先执行迁移：`./msime-server -config /config/config.json -migrate-users`。镜像中可在正常入口后追加 `-migrate-users`。迁移是幂等的；后台启用但缺少表时服务会拒绝启动。
6. 正常启动镜像；容器中的 `listen` 应为 `0.0.0.0:8080`。配置 `admin.msime.app` 的 DNS 指向入口，并在入口终止 HTTPS，将该域名的请求转发到相同的 Go 端口，保留原始 Host。Go 不信任 `X-Forwarded-Host`。

示例 Nginx HTTPS 虚拟主机（证书路径、后端地址按部署调整）：

```nginx
server {
    listen 443 ssl;
    server_name admin.msime.app;
    ssl_certificate /etc/nginx/certs/admin.msime.app/fullchain.pem;
    ssl_certificate_key /etc/nginx/certs/admin.msime.app/privkey.pem;
    location / {
        proxy_set_header Host $host;
        proxy_pass http://msime-backend:8080;
    }
}
```

浏览器访问 `https://admin.msime.app`，点击“使用 Google 账号登录”。Google 验证完成后，后端校验 ID Token 的签名、issuer、audience、有效期、nonce，以及 `email_verified` 和邮箱白名单。普通 Google 用户不会因此成为管理员，也不会自动创建普通输入法用户账户。支持配置 `allowed_emails` 数组；配置 `allowed_emails_env` 时以环境变量为准。

授权码流程使用 PKCE S256 和一次性 state；state 与浏览器 HttpOnly Cookie 绑定，并在数据库中保留 10 分钟。管理员会话在 PostgreSQL 中仅存令牌哈希，有效期固定为 8 小时；浏览器使用 Secure、HttpOnly、SameSite=Lax、无 Domain 的 `__Host-` Cookie，页面刷新可恢复登录。每个请求重新校验当前邮箱白名单，删除白名单账号并重启所有副本后，其旧会话也不再有效。退出会删除服务端会话；Cookie 管理写操作还要求 Origin 与配置的回调来源完全相同。过期登录流程和会话按现有每小时清理任务回收。

审计的 `actor` 记录 Google subject 与邮箱，旧数据和管理员密钥操作记为 `legacy-token`。Google Client Secret 和授权令牌不会返回给前端。此处遵循 [Google OpenID Connect 服务端流程](https://developers.google.com/identity/openid-connect/openid-connect)。

所有后台 `/api/*` 均校验管理员权限（仅登录元数据、开始登录、回调与退出接口有各自认证流程），普通用户/设备令牌无权访问。后台域名不承载 `/v1/*` 客户端 API。默认 `admin.enabled: false`，不会改变已有域名路由。

登录端点：

| 端点 | 用途 |
| --- | --- |
| `GET /api/auth/session` | 返回登录状态、管理员邮箱和启用的登录方式 |
| `GET /api/auth/google/start` | 创建 state/nonce/PKCE，跳转到 Google |
| `GET /api/auth/google/callback` | 校验回调并创建管理员会话 |
| `POST /api/auth/logout` | 删除服务端会话并清除 Cookie |

OAuth 客户端只允许配置固定回调。生产反向代理必须保持 Host，不应改写回调路径。Google Cloud 如果仍处于测试发布状态，需要将管理员加入该项目的测试用户；只有基础登录范围通常不涉及敏感 API 访问。真实 Google 回调验证需要新服务已在上述 HTTPS 域名上线。

本地私密配置可放在 `config.admin.local.json` 和 `.env.admin.local`（均被 Git 忽略）。使用前将现有部署的数据库、验证码密钥和客户端认证配置合并进去；Go 不自动加载 `.env`，可由部署工具注入或在本机先 `set -a; . ./.env.admin.local; set +a`。不要把本地凭据文件提交到仓库。

## 功能与统计口径

- 数据总览：累计注册账户、近 30 天注册数、持有有效会话的用户、安装包下载上报、崩溃及待处理数、社区皮肤、共享词库、回复模板和资源收藏。有效会话用户不是 DAU。
- 30 天趋势：UTC 自然日，每天的注册、下载上报和崩溃上报；图表展示下载，展开明细可看全部指标。
- 用户：搜索、分页、撤销全部会话（包括刷新令牌）；不展示邮箱/手机号、私人词典或剪贴板。
- 社区皮肤、词库与模板：查看列表与统计，删除公开内容。删除是永久操作，浏览器会要求确认，并连带删除对应下载/收藏/评分，所以这些社区统计反映当前留存记录。内置文件皮肤和引擎词库仍通过现有构建/配置管理。
- 崩溃：版本、平台、错误及堆栈详情，标记已处理或重新打开。
- 审计：管理变更与审计写入在同一数据库事务中，失败不部分生效。
- 列表：每页 50 条、搜索、前后分页；最大 10000 页。

安装包下载目前以新上报事件为数据来源，不会自动从 CDN、GitHub Release 或应用商店回填历史数据。皮肤下载沿用原有 `(skin_id,user_id)` 去重，资源收藏沿用 `(resource_id,user_id)`；不能与安装包下载混为一个总量。

## 客户端数据接入

向现有 API 域名发送 `POST /v1/telemetry/events`，携带现有设备或用户 Bearer 令牌：

```json
{
  "id": "8ff0faf8-5c26-4a15-bff5-e11c92bac154",
  "kind": "download",
  "platform": "windows",
  "version": "1.0.0"
}
```

崩溃示例：

```json
{
  "id": "f0c84d7e-7ca2-48cd-bcb1-941e07ba9dc5",
  "kind": "crash",
  "platform": "windows",
  "version": "1.0.0",
  "message": "Unhandled exception in keyboard initialization",
  "stack": "Keyboard::Initialize\nApplication::Start"
}
```

- 每个事件生成一个全局唯一 ID（16–128 字符），重试必须复用；同一 ID 只记录第一次，重复也返回 `202 {"accepted":true}`。推荐 UUID，不含用户身份。
- `platform` 为 1–32 字符，`version` 为 1–64 字符。崩溃 `message` 必填，最多 1000 字符，`stack` 最多 16000 字符；总请求体最多 32 KiB。下载事件不接受错误与堆栈字段的非空值。
- 使用服务端接收时间，离线上报算在接收日。客户端须在用户允许采集后发送，先清理输入文本、密码、令牌及个人信息；后台不额外存 IP 或用户身份。
- 接口沿用现有身份认证、速率限制与并发限制。数据保存在 PostgreSQL，多副本共享；当前不自动清理事件，需按实际规模设置归档/保留策略。
- 必须执行新迁移；采集接口不要求开启后台域名，但要求启用用户数据库。完整接口见生成的 OpenAPI。

## 本地开发与验证

```sh
pnpm --dir admin-web install --frozen-lockfile
pnpm --dir admin-web lint
pnpm --dir admin-web build
go test ./...
go build -o /tmp/msime-server ./cmd/msime-server
```

修改 `admin-web/src/` 后执行 `pnpm --dir admin-web build`，再重新编译 Go。生成的 `dist/` 随源码提交，保证直接 `go build` 也可用；CI 会重新构建并核对产物。运行镜像不需要 Node.js。

开发时将 Go 的 `admin.host` 配为 `admin.localhost`、`listen` 配为 `127.0.0.1:18089`，运行 `pnpm --dir admin-web dev`；Vite 将 `/api` 请求代理到该 Go 服务并设置开发 Host/Origin。也可直接访问 `http://admin.localhost:18089` 验证实际嵌入产物。生产必须通过 HTTPS 入口访问。

PostgreSQL 集成测试需设置 `MSIME_TEST_DATABASE_URL`，数据库名称必须含 `msime_auth_test`，仅可使用一次性测试库。测试会清空测试表。
