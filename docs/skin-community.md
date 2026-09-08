# 用户皮肤社区

社区接口位于 `/v1/community/skins`，与既有 `/v1/skins` 桌面 CSS 目录分开。当前设计格式对应 Apple 自定义键盘 v1，描述颜色、键帽、纹理、渐变及可选 JPEG 壁纸，不接受代码、任意资源 URL 或 CSS。其他平台需实现此设计格式后才能使用。

## 接口

- `GET /v1/community/skins?q=&offset=0`：公开目录，每页 20 条，返回 `skins` 和 `has_more`。目录不含壁纸字节。
- `GET /v1/community/skins/{id}`：公开详情，含完整 design；登录时额外返回自己的评分与是否为作者。
- `POST /v1/community/skins`：需要用户会话，提交 `{id,name,description,design}`。id 为客户端生成的 UUID，用于网络失败后的安全重试。每个账号最多 50 款。
- `POST /v1/community/skins/{id}/download`：需要用户会话，返回 `{design}`，每个账号只计一次下载。
- `PUT /v1/community/skins/{id}/rating`：需要用户会话，提交 `{stars:1..5}`。必须已下载，作者不可自评；重复提交更新同一条评分。
- `DELETE /v1/community/skins/{id}`：仅作者可下架；不删除其他设备已下载的本地副本。

摘要字段：id、name、description、author、design、downloads、rating_count、rating_average、owned、my_rating。人数代表累计去重下载账号数，不代表实时活跃使用人数。发布之后不允许原地替换设计以继承旧版评分；修改设计需发布新作品。

标题最多 32 个 Unicode 字符，说明最多 280 个；设计颜色为 24-bit RGB、数值范围与 iOS 编辑器一致。JSON 请求最大 710,000 字节，壁纸最多 512,000 字节且长宽均不超过 1024。服务器解码后重编码 JPEG，移除原图元数据。未知字段或错误图片会被拒绝。

## 账号与 K8s

复用现有 PostgreSQL 用户体系。配置 `auth.apple.client_ids: ["app.msime.ios"]`；Apple Developer 中为相同 App ID 启用 Sign in with Apple，并重新生成包含该 entitlement 的签名配置。客户端不包含设备共享令牌或 Apple 私钥。Apple nonce 来自后端挑战，ID Token 校验继续使用现有 issuer/audience/signature/nonce 校验。

上线前使用迁移账号执行新版本的 `-migrate-users`，或者由数据库管理员在事务中执行 `internal/account/community_schema.sql`，并给运行角色授予三张新表的 SELECT/INSERT/UPDATE/DELETE。这些表放在现有 PostgreSQL 中，无需 K8s 本地目录或 PVC。新版本启动检查表已迁移；先迁移再滚动更新，旧二进制可兼容新增表。数据库需按现有方案备份。

下载及评分的唯一键保证多副本并发去重；发布锁定账号行保证配额。浏览和写入继续使用数据库限流。没有给下载用户数设置产品上限；实际吞吐需按部署容量测试，不能把配额或副本数解释为可承载人数。

Apple 客户端入口：皮肤 → 皮肤社区。用户显式确认公开素材后发布，浏览不会修改当前皮肤。账号注销级联删除作品、评分和下载记录。登录令牌保存到本机 Keychain，刷新串行执行。
